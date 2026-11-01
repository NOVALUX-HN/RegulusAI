package almanac

// M3 portable domain bundles: ExportDomain scopes the Export bundle
// shape to one domain ("" = everything) — portable and committable,
// merged back by Import without duplicates. All roots are tmp dirs —
// never the real .regulus/.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// seedDomainScope builds a two-domain log: two go observations (one
// pinned, related pair, one verified) plus a ts observation (pinned,
// verified, cross-related) that must stay behind in a go bundle.
func seedDomainScope(t *testing.T, root string) (go1, go2, ts1 Observation) {
	t.Helper()
	var err error
	go1, err = Save(root, &Observation{Type: TypeDecision, Title: "go one", Content: "postgres pooling wins", Domain: "go"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	go2, err = Save(root, &Observation{Type: TypeDiscovery, Title: "go two", Content: "sqlite simplifies operations", Domain: "go"}, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	ts1, err = Save(root, &Observation{Type: TypeDecision, Title: "ts one", Content: "strict compiler flags ship", Domain: "ts"}, t0.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, ts1.ID, true, t0.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Pin(root, go2.ID, true, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, go1.ID, RelationRelated, go2.ID, t0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, go1.ID, RelationRelated, ts1.ID, t0.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RecordStatus(root, go1.ID, StatusVerified, "ana", "checked", t0.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := RecordStatus(root, ts1.ID, StatusVerified, "ana", "checked", t0.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return go1, go2, ts1
}

func bundleLines(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestExportDomainScopesBundle(t *testing.T) {
	root := t.TempDir()
	go1, go2, ts1 := seedDomainScope(t, root)
	out := filepath.Join(t.TempDir(), "bundle-go")
	paths, err := ExportDomain(root, "go", out)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 5 {
		t.Fatalf("want the jsonl + INDEX + pins + relations + status, got %v", paths)
	}
	obs := bundleLines(t, out, "almanac.jsonl")
	if strings.Count(strings.TrimSpace(obs), "\n")+1 != 2 {
		t.Fatalf("domain bundle = %q, want 2 lines", obs)
	}
	for _, want := range []string{go1.ID, go2.ID} {
		if !strings.Contains(obs, want) {
			t.Fatalf("bundle must carry %s, got:\n%s", want, obs)
		}
	}
	if strings.Contains(obs, ts1.ID) {
		t.Fatalf("bundle must not carry the other domain, got:\n%s", obs)
	}
	pins := bundleLines(t, out, "pins.jsonl")
	if !strings.Contains(pins, go2.ID) || strings.Contains(pins, ts1.ID) {
		t.Fatalf("bundle pins must follow the domain, got:\n%s", pins)
	}
	rels := bundleLines(t, out, "relations.jsonl")
	if !strings.Contains(rels, go1.ID) || !strings.Contains(rels, go2.ID) {
		t.Fatalf("bundle must carry the in-domain relation, got:\n%s", rels)
	}
	if strings.Contains(rels, ts1.ID) {
		t.Fatalf("bundle must drop the cross-domain relation, got:\n%s", rels)
	}
	status := bundleLines(t, out, "status.jsonl")
	if !strings.Contains(status, go1.ID) || strings.Contains(status, ts1.ID) {
		t.Fatalf("bundle status must follow the domain, got:\n%s", status)
	}
	idx := bundleLines(t, out, "INDEX.md")
	for _, want := range []string{`domain "go"`, "go one", "go two"} {
		if !strings.Contains(idx, want) {
			t.Fatalf("INDEX must scope to the domain (%q), got:\n%s", want, idx)
		}
	}
	if strings.Contains(idx, "ts one") {
		t.Fatalf("INDEX must not leak the other domain, got:\n%s", idx)
	}
	// The bundle merges into a clean tree without duplicates.
	dst := t.TempDir()
	imported, skipped, err := Import(dst, out)
	if err != nil {
		t.Fatal(err)
	}
	if imported != 2 || skipped != 0 {
		t.Fatalf("clean-tree bundle import = %d/%d, want 2/0", imported, skipped)
	}
	if pins, _ := PinnedIDs(dst); !pins[go2.ID] || len(pins) != 1 {
		t.Fatalf("the in-domain pin must restore alone, got %v", pins)
	}
	if st, _ := EffectiveStatus(dst, go1.ID); st != StatusVerified {
		t.Fatalf("the governed observation keeps its trail, got %q", st)
	}
	if st, _ := EffectiveStatus(dst, go2.ID); st != StatusProposed {
		t.Fatalf("uncovered imports enter PROPOSED, got %q", st)
	}
	before := map[string]string{}
	for _, p := range []string{File(dst), pinsFile(dst), relationsFile(dst), StatusFile(dst)} {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		before[p] = string(raw)
	}
	imported, skipped, err = Import(dst, out)
	if err != nil || imported != 0 || skipped != 2 {
		t.Fatalf("re-import = %d/%d (%v), want 0/2", imported, skipped, err)
	}
	for p, want := range before {
		if raw, _ := os.ReadFile(p); string(raw) != want {
			t.Fatalf("re-import must leave %s byte-identical", p)
		}
	}
}

func TestExportDomainEmptyMatchesFullExport(t *testing.T) {
	root := t.TempDir()
	seedDomainScope(t, root)
	if _, err := Export(root); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle-all")
	if _, err := ExportDomain(root, "", out); err != nil {
		t.Fatal(err)
	}
	// An unscoped domain bundle is a pure relocation of the full bundle:
	// every file byte-identical, so the scope can only ever restrict.
	for _, name := range []string{"almanac.jsonl", "INDEX.md", "pins.jsonl", "relations.jsonl", "status.jsonl"} {
		full, err := os.ReadFile(filepath.Join(root, ".regulus-export", name))
		if err != nil {
			t.Fatal(err)
		}
		scoped, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(full, scoped) {
			t.Fatalf("%s differs from the full bundle:\nfull:\n%s\nscoped:\n%s", name, full, scoped)
		}
	}
}

func TestExportDomainUnknownDomainExportsEmpty(t *testing.T) {
	root := t.TempDir()
	seedDomainScope(t, root)
	out := filepath.Join(t.TempDir(), "bundle-nope")
	paths, err := ExportDomain(root, "nope", out)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 5 {
		t.Fatalf("empty scope keeps the bundle shape, got %v", paths)
	}
	if raw := strings.TrimSpace(bundleLines(t, out, "almanac.jsonl")); raw != "" {
		t.Fatalf("unknown domain exports no observations, got %q", raw)
	}
	if idx := bundleLines(t, out, "INDEX.md"); !strings.Contains(idx, "0 observation(s)") {
		t.Fatalf("INDEX must report the empty scope, got:\n%s", idx)
	}
	dst := t.TempDir()
	if imported, skipped, err := Import(dst, out); err != nil || imported != 0 || skipped != 0 {
		t.Fatalf("empty import = %d/%d (%v), want 0/0", imported, skipped, err)
	}
}

func TestExportDomainMissingLogExportsEmpty(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "bundle-fresh")
	if _, err := ExportDomain(root, "go", out); err != nil {
		t.Fatalf("a missing log exports empty, never errors: %v", err)
	}
	for _, name := range []string{"almanac.jsonl", "pins.jsonl", "relations.jsonl", "status.jsonl"} {
		if raw := strings.TrimSpace(bundleLines(t, out, name)); raw != "" {
			t.Fatalf("%s must be empty, got %q", name, raw)
		}
	}
}

func TestExportDomainPreservesUnparseableLines(t *testing.T) {
	root := t.TempDir()
	seedDomainScope(t, root)
	// Only the stores the reader tolerates line-by-line (the log skips
	// malformed lines with a warning; the status bundle is filtered raw):
	// pins/relations readers are strict by design, and a corrupt line
	// there already aborts the full Export today — same contract here.
	for _, p := range []string{File(root), StatusFile(root)} {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("not json\n"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle-raw")
	if _, err := ExportDomain(root, "go", out); err != nil {
		t.Fatal(err)
	}
	// Nothing attributable is dropped for being unreadable — Export
	// copies the log verbatim, and the domain copy keeps that promise.
	for _, name := range []string{"almanac.jsonl", "status.jsonl"} {
		if raw := bundleLines(t, out, name); !strings.Contains(raw, "not json") {
			t.Fatalf("%s must preserve the unparseable line verbatim, got:\n%s", name, raw)
		}
	}
}

func TestLineIDAndKeepPredicates(t *testing.T) {
	if got := lineID(`{"id":"obs_1"}`); got != "obs_1" {
		t.Fatalf("lineID = %q", got)
	}
	if got := lineID("not json"); got != "" {
		t.Fatalf("malformed lineID = %q", got)
	}
	if got := lineID(""); got != "" {
		t.Fatalf("blank lineID = %q", got)
	}
	keep := map[string]bool{"a": true}
	rel := keepRelationLine(keep)
	if !rel(`{"from":"a","relation":"related","to":"a","at":"2026-10-26T09:00:00Z"}`) {
		t.Fatal("in-scope relation must stay")
	}
	if rel(`{"from":"a","relation":"related","to":"ghost","at":"2026-10-26T09:00:00Z"}`) {
		t.Fatal("cross-scope relation must go")
	}
	if !rel("not json") {
		t.Fatal("unparseable relation lines are preserved verbatim")
	}
	st := keepStatusLine(keep)
	if !st(`{"id":"a","to":"VERIFIED","by":"ana","at":"2026-10-26T09:00:00Z"}`) {
		t.Fatal("in-scope status must stay")
	}
	if st(`{"id":"ghost","to":"VERIFIED","by":"ana","at":"2026-10-26T09:00:00Z"}`) {
		t.Fatal("out-of-scope status must go")
	}
	if !st("not json") {
		t.Fatal("unparseable status lines are preserved verbatim")
	}
	if got := string(selectLines([]byte("a\n\nb\n"), func(string) bool { return true })); got != "a\nb\n" {
		t.Fatalf("selectLines must skip empties, got %q", got)
	}
}
