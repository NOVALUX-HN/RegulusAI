package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/receipts"
)

var t0 = time.Unix(1700000000, 0).UTC()

func writeTwoTasks(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(TaskDir(root, "feat/T1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(TaskDir(root, "feat/T1"), Receipt(t0, "typecheck", receipts.VerdictPass)); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(TaskDir(root, "feat/T1"), Receipt(t0.Add(time.Second), "lint", receipts.VerdictFail)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(TaskDir(root, "other/T2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Write(TaskDir(root, "other/T2"), Receipt(t0.Add(2*time.Second), "unit-tests", receipts.VerdictPass)); err != nil {
		t.Fatal(err)
	}
}

func Receipt(at time.Time, gate, verdict string) receipts.Receipt {
	return receipts.Receipt{
		Task: "t", Gate: gate, Layer: 1, Command: "cmd",
		ExitCode: 0, Verdict: verdict, StartedAt: at, DurationMS: 3,
	}
}

func TestDirAndTaskDir(t *testing.T) {
	root := "proj"
	if got, want := Dir(root), filepath.Join("proj", ".regulus", "evidence"); got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
	if got, want := TaskDir(root, "feat/T1"), filepath.Join("proj", ".regulus", "evidence", "feat__T1"); got != want {
		t.Fatalf("TaskDir = %q, want %q", got, want)
	}
}

func TestListAllTasks(t *testing.T) {
	root := t.TempDir()
	writeTwoTasks(t, root)
	entries, err := List(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 receipts across tasks, got %+v", entries)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Path > entries[i].Path {
			t.Fatalf("entries must be path-ordered: %q then %q", entries[i-1].Path, entries[i].Path)
		}
	}
}

func TestListOneTask(t *testing.T) {
	root := t.TempDir()
	writeTwoTasks(t, root)
	entries, err := List(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 receipts for feat/T1, got %+v", entries)
	}
	for _, e := range entries {
		if !strings.Contains(e.Path, "feat__T1") {
			t.Fatalf("receipt of another task leaked in: %q", e.Path)
		}
	}
}

func TestListEmptyWhenNoEvidence(t *testing.T) {
	entries, err := List(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("want an empty audit, got %+v", entries)
	}
}

func TestListRejectsMalformedReceipt(t *testing.T) {
	root := t.TempDir()
	dir := TaskDir(root, "bad/T1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "garbage.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := List(root, "")
	if err == nil || !strings.Contains(err.Error(), "malformed receipt") {
		t.Fatalf("want malformed-receipt refusal, got %v", err)
	}
}
