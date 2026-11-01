package reflect

import (
	"path/filepath"
	"strings"
	"testing"
)

// COVER (gap-sweep/G3): Run carried CRAP 26.65 at 76.6% coverage.
// Three zero-behavior tests for never-exercised guards: unreadable doc,
// empty lesson, zero Now defaulting.
func TestRunRefusesUnreadableDocCover(t *testing.T) {
	root := t.TempDir()
	_, err := Run(root, Input{
		FeatureDoc: filepath.Join(root, "charts", "missing.md"),
		Domain:     "go", Task: "feature/T1",
		Cue: "c", Lesson: "l", Now: now,
	}, "abc")
	if err == nil || !strings.Contains(err.Error(), "cannot read feature document") {
		t.Fatalf("want unreadable-doc refusal, got %v", err)
	}
}

func TestRunRefusesEmptyLessonCover(t *testing.T) {
	root := t.TempDir()
	doc := writeFeatureDoc(t, root)
	writeReceipts(t, root, "feature/T1", 1)
	_, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T1",
		Cue: "c", Lesson: "   ", Now: now,
	}, "abc")
	if err == nil || !strings.Contains(err.Error(), "lesson must not be empty") {
		t.Fatalf("want lesson refusal, got %v", err)
	}
}

func TestRunDefaultsZeroNowCover(t *testing.T) {
	root := t.TempDir()
	doc := writeFeatureDoc(t, root)
	writeReceipts(t, root, "feature/T1", 1)
	l, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T1",
		Cue: "c", Lesson: "l",
	}, "abc")
	if err != nil {
		t.Fatalf("zero Now must default, got %v", err)
	}
	if l.ID == "" || !strings.HasPrefix(l.ID, "les_") {
		t.Fatalf("id = %q, want les_ prefix", l.ID)
	}
	if l.CreatedAt.IsZero() {
		t.Fatal("zero Now must default to a real timestamp")
	}
}
