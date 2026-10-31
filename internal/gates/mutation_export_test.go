package gates

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/executil"
	"regulus-ai/internal/receipts"
)

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	executil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v: %s", args, dir, err, out)
	}
	return string(out)
}

// initProbeRepo creates a git repo with one committed file. Callers may add
// untracked bloat afterwards (e.g. node_modules/) to prove the export drops
// it. Skips when git is absent.
func initProbeRepo(t *testing.T, committed map[string]string) string {
	t.Helper()
	if !gitAvailable() {
		t.Skip("git absent")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "probe@test.local")
	runGit(t, dir, "config", "user.name", "probe")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	for name, content := range committed {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "probe")
	return dir
}

func TestIsMutationGateDetection(t *testing.T) {
	cases := []struct {
		gate Gate
		want bool
	}{
		{Gate{Name: "mutation", Command: "gremlins unleash ./internal/receipts --threshold-efficacy 70 --workers 1 --timeout-coefficient 10"}, true},
		{Gate{Name: "Mutation", Command: "echo hello"}, true},
		{Gate{Name: "other", Command: "gremlins unleash ./x"}, true},
		{Gate{Name: "other", Command: "gremlins --help"}, false},
		{Gate{Name: "other", Command: "go test ./..."}, false},
		{Gate{Name: "", Command: ""}, false},
	}
	for i, tc := range cases {
		if got := isMutationGate(tc.gate); got != tc.want {
			t.Fatalf("case %d (%+v): got %v want %v", i, tc.gate, got, tc.want)
		}
	}
}

func TestExportCleanTreeExcludesUntracked(t *testing.T) {
	repo := initProbeRepo(t, map[string]string{"tracked.txt": "committed\n"})
	bloat := filepath.Join(repo, "node_modules", "evil.js")
	if err := os.MkdirAll(filepath.Dir(bloat), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bloat, []byte("untracked bloat"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp, cleanup, err := exportCleanTree(context.Background(), repo)
	if err != nil {
		t.Fatalf("export must succeed in a git checkout: %v", err)
	}
	defer cleanup()
	data, err := os.ReadFile(filepath.Join(tmp, "tracked.txt"))
	if err != nil || strings.TrimSpace(string(data)) != "committed" {
		t.Fatalf("export must contain committed files: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("export must NOT contain untracked node_modules: %v", err)
	}
}

func TestExportCleanTreeFallbackNonGit(t *testing.T) {
	dir := t.TempDir()
	if _, cleanup, err := exportCleanTree(context.Background(), dir); err == nil {
		cleanup()
		t.Fatal("export outside a git checkout must fail so the caller falls back")
	}
}

func readReceipt(t *testing.T, path string) receipts.Receipt {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rec receipts.Receipt
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestMutationReceiptCleanExportNote(t *testing.T) {
	repo := initProbeRepo(t, map[string]string{"tracked.txt": "hi\n"})
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "evil.js"), []byte("bloat"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Gates: []Gate{{Name: "mutation", Layer: 1, Command: "echo hello"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "probe/T1", "abc", ShRunner{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("res = %+v", res)
	}
	rec := readReceipt(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, MutationNoteCleanExport) {
		t.Fatalf("receipt stderr must carry the clean-export mode, got %q", rec.Stderr)
	}
	if strings.Contains(rec.Stderr, MutationNoteInSitu) {
		t.Fatalf("clean export must not carry the in-situ warning, got %q", rec.Stderr)
	}
}

func TestMutationReceiptInSituWarning(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Gates: []Gate{{Name: "mutation", Layer: 1, Command: "echo hello"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "probe/T1", "", ShRunner{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("fallback in-situ must still run the verdict: %+v", res)
	}
	rec := readReceipt(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, MutationNoteInSitu) {
		t.Fatalf("receipt stderr must carry the in-situ warning, got %q", rec.Stderr)
	}
}

// TestMutationE2EGremlinsInExport runs the real gremlins binary (when on
// PATH) through the hermetic path with a trivial fast subcommand. It skips
// when gremlins or git is absent: then only the export+fallback units above
// run.
func TestMutationE2EGremlinsInExport(t *testing.T) {
	if _, err := exec.LookPath("gremlins"); err != nil {
		t.Skip("gremlins absent")
	}
	repo := initProbeRepo(t, map[string]string{"tracked.txt": "hi\n"})
	cfg := Config{Gates: []Gate{{Name: "mutation", Layer: 1, Command: "gremlins --help"}}}
	res, err := Run(context.Background(), cfg, t.TempDir(), "probe/T1", "abc", ShRunner{Dir: repo})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("gremlins --help in export must pass: %+v", res)
	}
	rec := readReceipt(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, MutationNoteCleanExport) {
		t.Fatalf("e2e receipt must carry the clean-export mode, got %q", rec.Stderr)
	}
}

// TestMutationRunTaskCleanExportNote guards the production path (gate run
// goes through RunTask, which pins the runner to the repo root): the
// receipt must still carry the clean-export mode there.
func TestMutationRunTaskCleanExportNote(t *testing.T) {
	repo := initProbeRepo(t, map[string]string{"tracked.txt": "hi\n"})
	if err := os.MkdirAll(filepath.Join(repo, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "node_modules", "evil.js"), []byte("bloat"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Gates: []Gate{{Name: "mutation", Layer: 1, Command: "echo hello"}}}
	res, _, err := RunTask(context.Background(), cfg, []byte(`{}`), repo, "probe/T9", "v", "c", "", ShRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed || res.Passed != 1 {
		t.Fatalf("res = %+v", res)
	}
	rec := readReceipt(t, res.Receipts[0])
	if !strings.Contains(rec.Stderr, MutationNoteCleanExport) {
		t.Fatalf("RunTask receipt must carry the clean-export mode, got %q", rec.Stderr)
	}
}

// buildTarEntry is one in-memory tar member for the untar unit tests.
type buildTarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
}

// buildTar archives entries in-memory (no git, no disk fixtures).
func buildTar(t *testing.T, entries []buildTarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, e := range entries {
		flag := e.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: flag, Linkname: e.linkname}
		if flag == tar.TypeDir {
			hdr.Size = 0
		}
		if err := w.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if flag == tar.TypeReg || flag == tar.TypeRegA {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUntarRegularAndNestedDirs(t *testing.T) {
	data := buildTar(t, []buildTarEntry{
		{name: "a.txt", body: "alpha"},
		{name: "sub/", typeflag: tar.TypeDir},
		{name: "sub/b.txt", body: "beta"},
	})
	dest := t.TempDir()
	if err := untar(data, dest); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a.txt": "alpha", "sub/b.txt": "beta"} {
		got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
}

func TestUntarSkipsEmptyDotAndUnsupported(t *testing.T) {
	data := buildTar(t, []buildTarEntry{
		{name: "", body: "x"},
		{name: ".", body: ""},
		{name: "dev", typeflag: tar.TypeChar},
		{name: "ok.txt", body: "fine"},
	})
	dest := t.TempDir()
	if err := untar(data, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "ok.txt"))
	if err != nil || strings.TrimSpace(string(got)) != "fine" {
		t.Fatalf("ok.txt = %q, %v", got, err)
	}
}

func TestUntarRejectsEscape(t *testing.T) {
	data := buildTar(t, []buildTarEntry{{name: "../evil.txt", body: "x"}})
	if err := untar(data, t.TempDir()); err == nil {
		t.Fatal("entries escaping dest must fail so the caller falls back")
	}
}

func TestUntarRejectsCorrupt(t *testing.T) {
	if err := untar([]byte("not a tar archive at all"), t.TempDir()); err == nil {
		t.Fatal("corrupt archives must fail")
	}
}

func TestUntarSymlinkBestEffort(t *testing.T) {
	data := buildTar(t, []buildTarEntry{
		{name: "target.txt", body: "t"},
		{name: "link.txt", typeflag: tar.TypeSymlink, linkname: "target.txt"},
	})
	dest := t.TempDir()
	// Symlinks need privilege on some platforms: either outcome is fine as
	// long as the export itself never fails for it.
	if err := untar(data, dest); err != nil {
		t.Fatal(err)
	}
}

func TestWithinDir(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "root")
	cases := []struct {
		target string
		want   bool
	}{
		{filepath.Join(dest, "a.txt"), true},
		{filepath.Join(dest, "sub", "b.txt"), true},
		// target == dest is unreachable (empty/dot names skip in
		// extractTarEntry) and harmless; Join with a real name always
		// lands below dest.
		{dest, true},
		{filepath.Join(dest, "..", "evil"), false},
	}
	for _, tc := range cases {
		if got := withinDir(dest, tc.target); got != tc.want {
			t.Fatalf("withinDir(%q) = %v, want %v", tc.target, got, tc.want)
		}
	}
}
