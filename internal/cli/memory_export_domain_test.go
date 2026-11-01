package cli

// M3 portable domain bundles: `memory export --domain` scopes the bundle
// to one domain (committable via `--out <tracked-dir>`), and `memory
// import` merges it without duplicates — re-importing is a no-op and the
// stores stay byte-identical.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

func seedDomainLog(t *testing.T) (go1, go2, ts1 string) {
	t.Helper()
	saves := []struct {
		typ, title, domain, content string
	}{
		{"decision", "go one", "go", "postgres pooling wins"},
		{"discovery", "go two", "go", "sqlite simplifies operations"},
		{"decision", "ts one", "ts", "strict compiler flags ship"},
	}
	for _, s := range saves {
		c, _, errb := newCLI(nil)
		args := []string{"memory", "save", "--type", s.typ, "--title", s.title, "--domain", s.domain, s.content}
		if code := c.Run(args); code != 0 {
			t.Fatalf("save %q: exit = %d (%s)", s.title, code, errb.String())
		}
	}
	all, err := almanac.All(".")
	if err != nil || len(all) != 3 {
		t.Fatalf("seeded log = %+v (%v)", all, err)
	}
	ids := map[string]string{}
	for _, o := range all {
		ids[o.Title] = o.ID
	}
	if err := almanac.Pin(".", ids["ts one"], true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := almanac.Pin(".", ids["go two"], true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := almanac.Relate(".", ids["go one"], almanac.RelationRelated, ids["go two"], time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return ids["go one"], ids["go two"], ids["ts one"]
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestMemoryExportDomainFiltersAndMerges(t *testing.T) {
	t.Chdir(t.TempDir())
	go1, go2, ts1 := seedDomainLog(t)
	// The default-out domain bundle keeps the generated-artifact message.
	cD, outD, _ := newCLI(nil)
	if code := cD.Run([]string{"memory", "export", "--domain", "go"}); code != 0 {
		t.Fatalf("export --domain: exit = %d", code)
	}
	if !strings.Contains(outD.String(), `domain "go" bundle in .regulus-export/`) {
		t.Fatalf("domain export must name its scope, got:\n%s", outD.String())
	}
	// The committable bundle lives outside .regulus-export (a tracked path
	// in real use — Engram's .engram/ pattern).
	cE, outE, _ := newCLI(nil)
	if code := cE.Run([]string{"memory", "export", "--domain", "go", "--out", "bundle-go"}); code != 0 {
		t.Fatalf("export --domain --out: exit = %d", code)
	}
	if !strings.Contains(outE.String(), "committable bundle at bundle-go") {
		t.Fatalf("relocated bundle must read committable, got:\n%s", outE.String())
	}
	for _, want := range []string{
		filepath.Join("bundle-go", "almanac.jsonl"),
		filepath.Join("bundle-go", "INDEX.md"),
		filepath.Join("bundle-go", "pins.jsonl"),
		filepath.Join("bundle-go", "relations.jsonl"),
		filepath.Join("bundle-go", "status.jsonl"),
	} {
		if !strings.Contains(outE.String(), want) {
			t.Fatalf("export must list %s, got:\n%s", want, outE.String())
		}
	}
	// Scope: only the go observations travel; the ts pin stays behind.
	lines := readLines(t, filepath.Join("bundle-go", "almanac.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("domain bundle = %d line(s), want 2", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, go1) || !strings.Contains(joined, go2) || strings.Contains(joined, ts1) {
		t.Fatalf("bundle must carry the go ids only, got:\n%s", joined)
	}
	pins := strings.Join(readLines(t, filepath.Join("bundle-go", "pins.jsonl")), "\n")
	if !strings.Contains(pins, go2) || strings.Contains(pins, ts1) {
		t.Fatalf("bundle pins must follow the domain, got:\n%s", pins)
	}
	rels := strings.Join(readLines(t, filepath.Join("bundle-go", "relations.jsonl")), "\n")
	if !strings.Contains(rels, go1) || !strings.Contains(rels, go2) {
		t.Fatalf("bundle must carry the in-domain relation, got:\n%s", rels)
	}
	idx, err := os.ReadFile(filepath.Join("bundle-go", "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`domain "go"`, "go one", "go two"} {
		if !strings.Contains(string(idx), want) {
			t.Fatalf("INDEX must scope to the domain (%q), got:\n%s", want, idx)
		}
	}
	if strings.Contains(string(idx), "ts one") {
		t.Fatalf("INDEX must not leak the other domain, got:\n%s", idx)
	}
	// Cloud copy: only the bundle survives, then it merges without dupes.
	if err := os.RemoveAll(".regulus"); err != nil {
		t.Fatal(err)
	}
	cI, outI, _ := newCLI(nil)
	if code := cI.Run([]string{"memory", "import", "bundle-go"}); code != 0 {
		t.Fatalf("import: exit = %d", code)
	}
	if !strings.Contains(outI.String(), "imported 2, skipped 0") {
		t.Fatalf("import output = %q", outI.String())
	}
	got, err := almanac.All(".")
	if err != nil || len(got) != 2 {
		t.Fatalf("merged log = %+v (%v)", got, err)
	}
	if pins, _ := almanac.PinnedIDs("."); !pins[go2] || len(pins) != 1 {
		t.Fatalf("the in-domain pin must restore alone, got %v", pins)
	}
	if st, _ := almanac.EffectiveStatus(".", go1); st != almanac.StatusProposed {
		t.Fatalf("imported knowledge stays PROPOSED, got %q", st)
	}
	// Re-importing the same bundle is a no-op — nothing duplicates.
	snapshot := map[string]string{}
	for _, p := range []string{almanac.File("."), filepath.Join(".regulus", "almanac", "pins.jsonl"), filepath.Join(".regulus", "almanac", "relations.jsonl"), almanac.StatusFile(".")} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		snapshot[p] = string(raw)
	}
	cR, outR, _ := newCLI(nil)
	if code := cR.Run([]string{"memory", "import", "bundle-go"}); code != 0 {
		t.Fatalf("re-import: exit = %d", code)
	}
	if !strings.Contains(outR.String(), "imported 0, skipped 2") {
		t.Fatalf("re-import output = %q", outR.String())
	}
	for p, before := range snapshot {
		after, err := os.ReadFile(p)
		if err != nil || string(after) != before {
			t.Fatalf("re-import must leave %s byte-identical", p)
		}
	}
}

func TestMemoryExportOutRelocatesFullBundle(t *testing.T) {
	t.Chdir(t.TempDir())
	seedDomainLog(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "export", "--out", "bundle-full"}); code != 0 {
		t.Fatalf("export --out: exit = %d", code)
	}
	if !strings.Contains(out.String(), "committable bundle at bundle-full") {
		t.Fatalf("relocated bundle must read committable, got:\n%s", out.String())
	}
	if n := len(readLines(t, filepath.Join("bundle-full", "almanac.jsonl"))); n != 3 {
		t.Fatalf("unscoped --out exports everything, got %d line(s)", n)
	}
}

func TestMemoryExportDomainEmptyScope(t *testing.T) {
	t.Chdir(t.TempDir())
	seedDomainLog(t)
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"memory", "export", "--domain", "nope", "--out", "bundle-empty"}); code != 0 {
		t.Fatalf("empty domain export: exit = %d", code)
	}
	if n := len(readLines(t, filepath.Join("bundle-empty", "almanac.jsonl"))); n != 0 {
		t.Fatalf("unknown domain exports no observations, got %d line(s)", n)
	}
	if err := os.RemoveAll(".regulus"); err != nil {
		t.Fatal(err)
	}
	cI, outI, _ := newCLI(nil)
	if code := cI.Run([]string{"memory", "import", "bundle-empty"}); code != 0 {
		t.Fatalf("empty import: exit = %d", code)
	}
	if !strings.Contains(outI.String(), "imported 0, skipped 0") {
		t.Fatalf("empty import output = %q", outI.String())
	}
}

func TestMemoryExportUsage(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"memory", "export", "--domain"}); code != 2 {
		t.Fatalf("missing flag value: exit = %d, want 2", code)
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"memory", "export", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "usage: regulus memory export") {
		t.Fatalf("stderr must print export usage, got %q — %q", errb2.String(), errb.String())
	}
}
