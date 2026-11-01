package evidence

// S2 jail: TaskDirSafe/Plan/Verify/List refuse traversal and absolute
// task ids without leaving .regulus/evidence/.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/receipts"
)

const jailMsg = "evidence: task escapes evidence dir"

var jailValid = []struct {
	task string
	slug string
}{
	{"feat/T1", "feat__T1"},
	{"gate-hardening/G2", "gate-hardening__G2"},
	{"feat__T1", "feat__T1"},
	{"plain", "plain"},
	{"a/b", "a__b"},
}

var jailEvil = []string{
	"..",
	"../evil",
	"..__evil",
	"a/../b",
	"a/..",
	"/abs",
	`C:\x`,
	"C:/x",
	"C:x",
	"~/x",
	"~",
	"a\x00b",
	`..\evil`,
	`a\b`,
	`\\srv\share`,
}

func TestTaskDirSafeAcceptsNormalTasks(t *testing.T) {
	root := t.TempDir()
	for _, v := range jailValid {
		got, err := TaskDirSafe(root, v.task)
		if err != nil {
			t.Fatalf("TaskDirSafe(%q) must pass, got %v", v.task, err)
		}
		want := filepath.Join(Dir(root), v.slug)
		if got != want {
			t.Fatalf("TaskDirSafe(%q) = %q, want %q", v.task, got, want)
		}
		if got != TaskDir(root, v.task) {
			t.Fatalf("Safe must resolve like TaskDir for %q: %q vs %q", v.task, got, TaskDir(root, v.task))
		}
	}
}

func TestTaskDirSafeRejectsEvilSlugs(t *testing.T) {
	root := t.TempDir()
	for _, task := range jailEvil {
		if _, err := TaskDirSafe(root, task); err == nil || !strings.Contains(err.Error(), jailMsg) {
			t.Fatalf("TaskDirSafe(%q) must refuse with %q, got %v", task, jailMsg, err)
		}
	}
}

// The raw join really would escape for "..": the vector is genuine and
// the jail (not Slug) is what stops it.
func TestTraversalVectorReallyEscapesRawJoin(t *testing.T) {
	root := t.TempDir()
	absDir, err := filepath.Abs(Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	absRaw, err := filepath.Abs(TaskDir(root, ".."))
	if err != nil {
		t.Fatal(err)
	}
	if absRaw == absDir || strings.HasPrefix(absRaw, absDir+string(filepath.Separator)) {
		t.Fatalf("raw TaskDir(..) must escape the evidence root, got %q inside %q", absRaw, absDir)
	}
	if _, err := TaskDirSafe(root, ".."); err == nil {
		t.Fatal("TaskDirSafe(..) must refuse")
	}
}

// Slug is a naming contract, not a guard: it still maps, the jail refuses.
func TestSlugContractUnchanged(t *testing.T) {
	if got := receipts.Slug("../evil"); got != "..__evil" {
		t.Fatalf("Slug(../evil) = %q, want ..__evil", got)
	}
	if got := receipts.Slug("a/b"); got != "a__b" {
		t.Fatalf("Slug(a/b) = %q, want a__b", got)
	}
}

func TestPlanRejectsEvilTasks(t *testing.T) {
	root := t.TempDir()
	for _, task := range jailEvil {
		if _, _, _, err := Plan(root, task, 0); err == nil || !strings.Contains(err.Error(), jailMsg) {
			t.Fatalf("Plan(%q) must refuse with %q, got %v", task, jailMsg, err)
		}
	}
}

func TestVerifyRejectsEvilTasks(t *testing.T) {
	root := t.TempDir()
	for _, task := range jailEvil {
		if _, err := Verify(root, task, false); err == nil || !strings.Contains(err.Error(), jailMsg) {
			t.Fatalf("Verify(%q) must refuse with %q, got %v", task, jailMsg, err)
		}
	}
}

func TestListRejectsEvilTasks(t *testing.T) {
	root := t.TempDir()
	for _, task := range jailEvil {
		if _, err := List(root, task); err == nil || !strings.Contains(err.Error(), jailMsg) {
			t.Fatalf("List(%q) must refuse with %q, got %v", task, jailMsg, err)
		}
	}
}

// Refused calls must not touch anything outside the evidence root.
func TestJailLeavesNoTraceOutsideEvidence(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sibling dir an escape would land in.
	if err := os.MkdirAll(filepath.Join(root, ".regulus"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, task := range jailEvil {
		_, _, _, _ = Plan(root, task, 0)
		_, _ = Verify(root, task, true)
		_, _ = List(root, task)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "untouched" {
		t.Fatalf("sentinel must survive refused calls: %q, %v", data, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != ".regulus" && !strings.HasPrefix(e.Name(), ".") && e.Name() != "sentinel.txt" {
			t.Fatalf("refused call created %q outside the evidence root", e.Name())
		}
	}
}

// The jail lets legitimate work through: Plan and Verify on a real task.
func TestJailLetsLegitWorkThrough(t *testing.T) {
	root := t.TempDir()
	dir, err := TaskDirSafe(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "lint", t0, nil)
	toDelete, _, _, err := Plan(root, "feat/T1", 0)
	if err != nil {
		t.Fatalf("Plan on a valid task must pass the jail: %v", err)
	}
	if len(toDelete) != 1 {
		t.Fatalf("want 1 receipt to delete, got %v", toDelete)
	}
	rep, err := Verify(root, "feat/T1", false)
	if err != nil {
		t.Fatalf("Verify on a valid task must pass the jail: %v", err)
	}
	if rep.Checked != 1 {
		t.Fatalf("Checked = %d, want 1", rep.Checked)
	}
	entries, err := List(root, "feat/T1")
	if err != nil {
		t.Fatalf("List on a valid task must pass the jail: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 listed receipt, got %d", len(entries))
	}
}
