package evidence

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// S6/C7: verifyDirs was cov11 (only the task!="" branch ran).
// Covers valid/invalid dirs, corrupt skip, missing root and bad pattern.
func TestVerifyDirsTaskDirect(t *testing.T) {
	root := t.TempDir()
	dirs, err := verifyDirs(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != TaskDir(root, "feat/T1") {
		t.Fatalf("task dir = %v, want [%s]", dirs, TaskDir(root, "feat/T1"))
	}
}

func TestVerifyDirsEmptyRoot(t *testing.T) {
	root := t.TempDir()
	dirs, err := verifyDirs(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Fatalf("empty root must list no dirs, got %v", dirs)
	}
}

func TestVerifyDirsListsAndSkipsCorrupt(t *testing.T) {
	root := t.TempDir()
	dir1 := TaskDir(root, "feat/T1")
	dir2 := TaskDir(root, "feat/T2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeReceipt(t, dir1, "a", t0, nil)
	writeReceipt(t, dir1, "b", t0.Add(time.Second), nil)
	writeReceipt(t, dir2, "a", t0, nil)
	// Quarantined copies live under corrupt/ and must never surface as
	// their own evidence dir.
	qdir := filepath.Join(dir1, "corrupt")
	if err := os.MkdirAll(qdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qdir, "old-1.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stray non-JSON file must not create a dir either.
	if err := os.WriteFile(filepath.Join(dir2, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := verifyDirs(root, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{dir1, dir2}
	sort.Strings(want)
	sort.Strings(dirs)
	if len(dirs) != 2 || dirs[0] != want[0] || dirs[1] != want[1] {
		t.Fatalf("dirs = %v, want %v", dirs, want)
	}
	for _, d := range dirs {
		if filepath.Base(d) == "corrupt" {
			t.Fatalf("corrupt/ must be skipped, got %v", dirs)
		}
	}

	// The all-tasks Verify path runs through verifyDirs("") too.
	rep, err := Verify(root, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 3 {
		t.Fatalf("Checked = %d, want 3 (2+1 receipts, corrupt skipped)", rep.Checked)
	}
}

func TestVerifyDirsBadPattern(t *testing.T) {
	// "[" makes the glob pattern malformed: Glob reports ErrBadPattern.
	if _, err := verifyDirs("[", ""); err == nil {
		t.Fatal("malformed glob pattern must error")
	}
}

func TestVerifyAllTasksMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	rep, err := Verify(root, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 0 || len(rep.Failed) != 0 {
		t.Fatalf("missing root must verify clean, got %+v", rep)
	}
}
