package almanac

import (
	"strings"
	"testing"
	"time"
)

func scopedFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	mk := func(o *Observation, at time.Time) {
		t.Helper()
		if _, err := Save(root, o, at); err != nil {
			t.Fatal(err)
		}
	}
	mk(&Observation{Type: TypeSessionNote, Title: "standup", Content: "shipped x", Provenance: &Provenance{Task: "feat/T1"}}, now)
	mk(&Observation{Type: TypeSessionNote, Title: "plan", Content: "ship y", Provenance: &Provenance{Task: "feat/T2"}}, now.Add(time.Minute))
	mk(&Observation{Type: TypeDiscovery, Title: "found", Content: "x is slow", Provenance: &Provenance{Paths: []string{"internal/x.go"}}}, now.Add(2*time.Minute))
	mk(&Observation{Type: TypeLesson, Title: "l", Content: "c", Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "high"}}, now.Add(3*time.Minute))
	return root
}

func TestContextScopedFilters(t *testing.T) {
	root := scopedFixture(t)
	now := time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)
	full, err := ContextScoped(root, now, 0, ContextScope{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full, "scoped:") {
		// unscoped output carries no scope line (byte-identical to legacy)
	} else {
		t.Fatalf("unscoped output must not name a scope, got:\n%s", full)
	}
	task, err := ContextScoped(root, now, 0, ContextScope{Task: "feat/T1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(task, "scoped: task=feat/T1") {
		t.Fatalf("scoped output must name the scope, got:\n%s", task)
	}
	if strings.Contains(task, "ship y") || strings.Contains(task, "x is slow") {
		t.Fatalf("task filter must exclude other tasks, got:\n%s", task)
	}
	if !strings.Contains(task, "shipped x") {
		t.Fatalf("task filter must keep the match, got:\n%s", task)
	}
	path, err := ContextScoped(root, now, 0, ContextScope{Path: "internal/x.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "x is slow") || strings.Contains(path, "shipped x") {
		t.Fatalf("path filter must isolate, got:\n%s", path)
	}
	sys, err := ContextScoped(root, now, 0, ContextScope{System: SystemEvolution})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sys, "Lessons admitted") || strings.Contains(sys, "shipped x") {
		t.Fatalf("system filter must isolate evolution, got:\n%s", sys)
	}
}

func TestContextScopedBudget(t *testing.T) {
	root := scopedFixture(t)
	now := time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "found2", Content: "y is slow"}, now); err != nil {
		t.Fatal(err)
	}
	capped, err := ContextScoped(root, now, 0, ContextScope{Budget: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(capped, "showing latest 1 of 2") {
		t.Fatalf("budget must cap the notes list with a pointer, got:\n%s", capped)
	}
	if strings.Contains(capped, "shipped x") {
		t.Fatalf("budget must cap the notes list, got:\n%s", capped)
	}
	if !strings.Contains(capped, "ship y") {
		t.Fatalf("budget keeps the latest, got:\n%s", capped)
	}
	if !strings.Contains(capped, "narrow with --task/--path/--system") {
		t.Fatalf("budget must cap the decisions list with a pointer, got:\n%s", capped)
	}
	if strings.Contains(capped, "x is slow") {
		t.Fatalf("budget must cap the decisions list, got:\n%s", capped)
	}
}

func TestContextUnscopedUnchanged(t *testing.T) {
	root := scopedFixture(t)
	now := time.Date(2026, 6, 1, 13, 0, 0, 0, time.UTC)
	a, err := ContextWithLimit(root, now, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ContextScoped(root, now, 0, ContextScope{})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("unscoped scoped-context must equal the legacy block:\n--- legacy ---\n%s\n--- scoped ---\n%s", a, b)
	}
}
