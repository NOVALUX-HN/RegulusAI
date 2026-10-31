// Package manifest builds the reproducible candidate manifest (Phase 1):
// the exact evaluated content a Git HEAD alone cannot identify.
package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeResult struct {
	stdout, stderr string
	code           int
	err            error
}

type fakeRunner struct {
	results map[string]fakeResult
	calls   []string
}

func (f *fakeRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	res, ok := f.results[command]
	if !ok {
		return "", "", 1, errors.New("unexpected command: " + command)
	}
	return res.stdout, res.stderr, res.code, res.err
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseParams(root string) Params {
	return Params{
		Root:             root,
		Task:             "demo/T1",
		RunID:            "run_test0123456789",
		RegulusVersion:   "test",
		Contract:         "regulus.contract/v1",
		ConfigBytes:      []byte(`{"gates":[{"name":"a","layer":1,"command":"echo ok"}]}`),
		MaxTimeoutSec:    600,
		AllowDestructive: false,
		Isolation:        "host-contained",
	}
}

func gitOK(head, porcelain, diff string) map[string]fakeResult {
	return map[string]fakeResult{
		"git rev-parse --verify HEAD":                                {stdout: head + "\n"},
		"git status --porcelain=v1 -z --untracked-files=normal -- .": {stdout: porcelain},
		"git diff HEAD -- .":                                         {stdout: diff},
	}
}

func TestBuildFullProvenance(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "package a\n")
	writeFile(t, root, "new.txt", "untracked content\n")
	porcelain := " M a.go\x00?? new.txt\x00"
	r := &fakeRunner{results: gitOK("abc123", porcelain, "diff --git a/a.go b/a.go\n")}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if m.Schema != SchemaVersion {
		t.Fatalf("want schema %d, got %d", SchemaVersion, m.Schema)
	}
	if !m.Git.Available || m.Git.Head != "abc123" {
		t.Fatalf("want git available at abc123, got %v/%v", m.Git.Available, m.Git.Head)
	}
	if m.Git.DiffHash == "" {
		t.Fatal("diff hash must be set")
	}
	if len(m.Git.Modified) != 1 || m.Git.Modified[0].Path != "a.go" {
		t.Fatalf("want [a.go] modified, got %+v", m.Git.Modified)
	}
	if m.Git.Modified[0].SHA256 == "" {
		t.Fatal("small modified file must be hashed")
	}
	if len(m.Git.Untracked) != 1 || m.Git.Untracked[0] != "new.txt" {
		t.Fatalf("want [new.txt] untracked, got %v", m.Git.Untracked)
	}
	if m.Config.Fingerprint == "" {
		t.Fatal("config fingerprint must be set")
	}
	if m.Tools.Go == "" || m.Tools.OS == "" || m.Tools.Arch == "" {
		t.Fatalf("tool versions must be set, got %+v", m.Tools)
	}
	if m.Env.Count == 0 || m.Env.NamesSHA256 == "" {
		t.Fatal("env fingerprint must be set")
	}
	if m.Task != "demo/T1" || m.RunID != "run_test0123456789" {
		t.Fatalf("identity must be kept, got %v/%v", m.Task, m.RunID)
	}
}

func TestBuildHashConsistency(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "package a\n")
	res := gitOK("abc123", " M a.go\x00", "DIFF")
	m1, err := Build(context.Background(), &fakeRunner{results: res}, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Build(context.Background(), &fakeRunner{results: res}, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if m1.Git.DiffHash != m2.Git.DiffHash || m1.Config.Fingerprint != m2.Config.Fingerprint {
		t.Fatal("same tree must hash identically")
	}
}

func TestBuildArtifactChangeDetected(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", "package a\n")
	r1 := &fakeRunner{results: gitOK("abc123", " M a.go\x00", "DIFF-ONE")}
	m1, err := Build(context.Background(), r1, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "a.go", "package a // changed\n")
	r2 := &fakeRunner{results: gitOK("abc123", " M a.go\x00", "DIFF-TWO")}
	m2, err := Build(context.Background(), r2, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if m1.Git.DiffHash == m2.Git.DiffHash {
		t.Fatal("changed diff must change the hash")
	}
	if m1.Git.Modified[0].SHA256 == m2.Git.Modified[0].SHA256 {
		t.Fatal("changed file must change its fingerprint")
	}
}

func TestBuildNoGitDegrades(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{results: map[string]fakeResult{
		"git rev-parse --verify HEAD": {err: errors.New("not a repo")},
	}}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatalf("missing git must degrade, not fail: %v", err)
	}
	if m.Git.Available || m.Git.Head != "" || m.Git.DiffHash != "" {
		t.Fatalf("git state must be empty, got %+v", m.Git)
	}
	if len(m.Git.Modified) != 0 || len(m.Git.Untracked) != 0 {
		t.Fatal("inventories must be empty without git")
	}
}

func TestBuildIgnoresNoiseAndCaps(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	for i := 0; i < MaxFiles+50; i++ {
		b.WriteString(" M f" + strings.Repeat("x", 4) + "\x00")
	}
	b.WriteString("!! ignored.out\x00")
	r := &fakeRunner{results: gitOK("h", b.String(), "")}
	p := baseParams(root)
	m, err := Build(context.Background(), r, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Git.Modified) != MaxFiles || !m.Git.ModifiedTruncated {
		t.Fatalf("modified must cap at %d with truncated flag, got %d/%v", MaxFiles, len(m.Git.Modified), m.Git.ModifiedTruncated)
	}
	for _, e := range m.Git.Modified {
		if e.Path == "ignored.out" {
			t.Fatal("ignored entries must never enter the inventory")
		}
	}
}

func TestBuildBigFileSizeOnly(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, MaxFileHashBytes+10)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(root, "big.bin"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{results: gitOK("h", " M big.bin\x00", "d")}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Git.Modified) != 1 {
		t.Fatalf("want one entry, got %+v", m.Git.Modified)
	}
	e := m.Git.Modified[0]
	if e.SHA256 != "" || e.Size != int64(len(big)) {
		t.Fatalf("oversize file must record size only, got %+v", e)
	}
}

func TestBuildEnvNeverLeaksValues(t *testing.T) {
	t.Setenv("MANIFEST_PHASE1_SECRET", "super-secret-value-12345")
	root := t.TempDir()
	r := &fakeRunner{results: gitOK("h", "", "")}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), "super-secret-value-12345") {
		t.Fatal("manifest must never contain environment values")
	}
	if !strings.Contains(string(raw), "MANIFEST_PHASE1_SECRET") {
		t.Log("note: secret name itself is not stored either — only the names hash")
	}
}

func TestNewRunIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := NewRunID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, "run_") || len(id) != 20 {
			t.Fatalf("bad run id shape %q", id)
		}
		if seen[id] {
			t.Fatal("duplicate run id")
		}
		seen[id] = true
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{results: gitOK("h", "", "")}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	Attach(&m, []ArtifactRef{{ReceiptID: "rec_1", File: "a-1.json", SHA256: "ab", Gate: "a", Status: "PASS", Reason: "ok"}})
	dir := t.TempDir()
	path, err := Write(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "manifest-run_test0123456789.json") {
		t.Fatalf("unexpected manifest path %q", path)
	}
	back, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Artifacts) != 1 || back.Artifacts[0].ReceiptID != "rec_1" {
		t.Fatalf("artifacts must round-trip, got %+v", back.Artifacts)
	}
	if _, err := ReadFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing manifest must fail to read")
	}
}

func TestFileNameMatchesWrite(t *testing.T) {
	if got := FileName("run_abc123"); got != "manifest-run_abc123.json" {
		t.Fatalf("FileName = %q", got)
	}
	if got := FileName("a/b\\c"); strings.ContainsAny(got, "/\\\\") {
		t.Fatalf("FileName must sanitize, got %q", got)
	}
}

func TestParseReproducibility(t *testing.T) {
	if ParseReproducibility("") != "unknown" {
		t.Fatal("empty must normalize to unknown")
	}
	for _, v := range []string{"exact", "statistical", "unknown"} {
		if ParseReproducibility(v) != v {
			t.Fatalf("%q must pass through", v)
		}
	}
	if ParseReproducibility("sometimes") != "" {
		t.Fatal("invalid value must be rejected")
	}
}

func TestBuildCountsReproducibility(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{results: gitOK("h", "", "")}
	p := baseParams(root)
	p.Gates = []GateSpec{
		{Name: "a", Layer: 1, Command: "echo ok", Reproducibility: "exact"},
		{Name: "b", Layer: 2, Command: "date", Reproducibility: "statistical"},
		{Name: "c", Layer: 3, Command: "go test ./..."},
	}
	m, err := Build(context.Background(), r, p)
	if err != nil {
		t.Fatal(err)
	}
	if m.ReproExact != 1 || m.ReproStatistical != 1 || m.ReproUnknown != 1 {
		t.Fatalf("bad repro counts: %v/%v/%v", m.ReproExact, m.ReproStatistical, m.ReproUnknown)
	}
	p.Gates = append(p.Gates, GateSpec{Name: "d", Layer: 4, Command: "x", Reproducibility: "sometimes"})
	if _, err := Build(context.Background(), &fakeRunner{results: gitOK("h", "", "")}, p); err == nil {
		t.Fatal("invalid reproducibility must fail the build")
	}
}

func TestBuildRejectsBadParams(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{results: gitOK("h", "", "")}
	for _, p := range []Params{
		func() Params { p := baseParams(root); p.Task = ""; return p }(),
		func() Params { p := baseParams(root); p.RunID = ""; return p }(),
		func() Params { p := baseParams(root); p.Root = ""; return p }(),
	} {
		if _, err := Build(context.Background(), r, p); err == nil {
			t.Fatalf("bad params must be rejected: %v/%v", p.Task, p.RunID)
		}
	}
	if _, err := Build(context.Background(), nil, baseParams(root)); err == nil {
		t.Fatal("nil runner must be rejected")
	}
}

func TestManifestJSONShape(t *testing.T) {
	root := t.TempDir()
	r := &fakeRunner{results: gitOK("abc", " M a.go\x00?? new.txt\x00", "D")}
	m, err := Build(context.Background(), r, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema", "run_id", "task", "created_at", "git", "config", "tools", "env", "policy", "artifacts"} {
		if _, ok := decoded[k]; !ok {
			t.Fatalf("manifest JSON must carry %q", k)
		}
	}
	_ = time.Now
}
