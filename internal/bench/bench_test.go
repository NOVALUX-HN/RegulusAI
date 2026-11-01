package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuiteBand(t *testing.T) {
	tasks := Suite()
	if len(tasks) < 5 || len(tasks) > 10 {
		t.Fatalf("suite has %d tasks, want 5-10", len(tasks))
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.ID == "" || task.Title == "" || task.Kind == "" || task.Prompt == "" {
			t.Fatalf("task %+v misses visible fields", task)
		}
		if task.Check == nil {
			t.Fatalf("task %s has no hidden check", task.ID)
		}
		if seen[task.ID] {
			t.Fatalf("duplicate task id %s", task.ID)
		}
		seen[task.ID] = true
	}
}

func TestSuitePromptsHideAnswers(t *testing.T) {
	for _, task := range Suite() {
		lower := strings.ToLower(task.Prompt)
		for _, leak := range []string{"want 2", "2.5", "traversal", "sha"} {
			if strings.Contains(lower, leak) {
				t.Fatalf("task %s prompt leaks its check %q", task.ID, leak)
			}
		}
	}
}

func TestRunAllPassOffline(t *testing.T) {
	rep := Run("tree", "abc123")
	if rep.Suite != SuiteVersion {
		t.Fatalf("suite = %q, want %q", rep.Suite, SuiteVersion)
	}
	if rep.Passed != len(rep.Tasks) || rep.Failed != 0 {
		t.Fatalf("want all pass, got %d/%d", rep.Passed, len(rep.Tasks))
	}
	if rep.Surface != "tree" || rep.GitHead != "abc123" {
		t.Fatalf("report identity lost: %+v", rep)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	rep := Run("tree", "abc")
	path := filepath.Join(t.TempDir(), "sub", "rep.json")
	if err := Save(path, rep); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Passed != rep.Passed || len(back.Tasks) != len(rep.Tasks) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back.Passed, rep.Passed)
	}
	for i, task := range back.Tasks {
		if task.ID != rep.Tasks[i].ID || task.Pass != rep.Tasks[i].Pass {
			t.Fatalf("task %d changed in round-trip", i)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing report must error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bad); err == nil {
		t.Fatal("malformed report must error")
	}
}

func TestCompareAdopt(t *testing.T) {
	base := Run("tree", "a")
	cand := Run("candidate", "b")
	cmp := Compare(base, cand)
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("identical green runs: verdict = %s (%s)", cmp.Verdict, cmp.Reason)
	}
	if len(cmp.Rows) != len(base.Tasks) {
		t.Fatalf("rows = %d, want %d", len(cmp.Rows), len(base.Tasks))
	}
}

func TestCompareRejectOnRegression(t *testing.T) {
	base := Run("tree", "a")
	cand := Run("candidate", "b")
	cand.Tasks[0].Pass = false
	cand.Passed--
	cmp := Compare(base, cand)
	if cmp.Verdict != "REJECT" {
		t.Fatalf("regressed run: verdict = %s, want REJECT", cmp.Verdict)
	}
	if cmp.Regressions != 1 {
		t.Fatalf("regressions = %d, want 1", cmp.Regressions)
	}
}

func TestCompareCountsDroppedOnly(t *testing.T) {
	base := Run("tree", "a")
	cand := base
	cand.Passed = base.Passed - 1
	cmp := Compare(base, cand)
	if cmp.Verdict != "REJECT" {
		t.Fatalf("dropped count: verdict = %s, want REJECT", cmp.Verdict)
	}
}

func TestCompareImprovementKept(t *testing.T) {
	base := Run("tree", "a")
	base.Tasks[0].Pass = false
	base.Passed--
	cand := Run("candidate", "b")
	cmp := Compare(base, cand)
	if cmp.Verdict != "ADOPT" {
		t.Fatalf("fixed task: verdict = %s, want ADOPT", cmp.Verdict)
	}
	if cmp.Improvements != 1 {
		t.Fatalf("improvements = %d, want 1", cmp.Improvements)
	}
}

func TestEachCheckPasses(t *testing.T) {
	for _, task := range Suite() {
		detail, err := task.Check()
		if err != nil {
			t.Fatalf("task %s check failed: %v", task.ID, err)
		}
		if strings.TrimSpace(detail) == "" {
			t.Fatalf("task %s check returned empty detail", task.ID)
		}
	}
}
