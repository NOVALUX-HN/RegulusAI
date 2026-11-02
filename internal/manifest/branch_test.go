// V3 (charts/dashboard-vision.md): the run manifest pins the checkout
// branch best-effort — real branch inside git, Unknown ("—") outside it
// — and the branch survives the write/read round trip.
package manifest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBranchOfOutsideGit pins the fallback: no checkout, no branch.
func TestBranchOfOutsideGit(t *testing.T) {
	root := t.TempDir()
	if got := BranchOf(root); got != Unknown {
		t.Fatalf("BranchOf outside git = %q, want %q", got, Unknown)
	}
	if got := HeadOf(root); got != Unknown {
		t.Fatalf("HeadOf outside git = %q, want %q", got, Unknown)
	}
	if got := BranchOf(""); got != Unknown {
		t.Fatalf("BranchOf empty root = %q, want %q", got, Unknown)
	}
}

// TestBuildPinsBranchFallback covers the writer path: manifests built
// outside git still carry the field, with the Unknown fallback.
func TestBuildPinsBranchFallback(t *testing.T) {
	root := t.TempDir()
	m, err := Build(context.Background(), &fakeRunner{results: gitOK("h", "", "")}, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if m.Branch != Unknown {
		t.Fatalf("manifest branch outside git = %q, want %q", m.Branch, Unknown)
	}
}

// TestManifestBranchRoundTrip proves the branch persists in
// manifest-run_*.json files.
func TestManifestBranchRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Schema: SchemaVersion, RunID: "run_branchroundtrip1", Task: "demo/T1", Branch: "feature-x"}
	path, err := Write(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "manifest-run_branchroundtrip1.json") {
		t.Fatalf("unexpected manifest path %q", path)
	}
	back, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Branch != "feature-x" {
		t.Fatalf("branch round trip = %q, want %q", back.Branch, "feature-x")
	}
}

// gitFixture inits a throwaway checkout on branch name with one commit
// and returns its root. It skips when git is unavailable.
func gitFixture(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "v3-test@example.com")
	run("config", "user.name", "v3-test")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "seed")
	run("checkout", "-b", name)
	return root
}

// TestBranchOfRealRepo resolves a real branch and short HEAD from a
// throwaway checkout — the same resolution the writer and /v1/git share.
func TestBranchOfRealRepo(t *testing.T) {
	root := gitFixture(t, "feature-v3-probe")
	if got := BranchOf(root); got != "feature-v3-probe" {
		t.Fatalf("BranchOf = %q, want %q", got, "feature-v3-probe")
	}
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := HeadOf(root); got != strings.TrimSpace(string(out)) {
		t.Fatalf("HeadOf = %q, want %q", got, strings.TrimSpace(string(out)))
	}
}

// TestBuildPinsRealBranch proves manifests created inside a checkout
// carry its branch forward.
func TestBuildPinsRealBranch(t *testing.T) {
	root := gitFixture(t, "feature-v3-build")
	m, err := Build(context.Background(), &fakeRunner{results: gitOK("h", "", "")}, baseParams(root))
	if err != nil {
		t.Fatal(err)
	}
	if m.Branch != "feature-v3-build" {
		t.Fatalf("manifest branch = %q, want %q", m.Branch, "feature-v3-build")
	}
}
