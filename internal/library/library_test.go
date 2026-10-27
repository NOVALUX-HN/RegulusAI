package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)

func validLesson() Lesson {
	return Lesson{
		ID:         "les_fixed",
		Domain:     "go",
		Task:       "feature/T1",
		Cue:        "when the gauntlet passes",
		Lesson:     "admit the lesson with its receipts",
		Evidence:   []string{".regulus/evidence/feature__T1/typecheck-1.json"},
		Confidence: ConfidenceLow,
		GitHead:    "abc123",
		CreatedAt:  t0,
	}
}

func TestAdmitRefusesMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Lesson)
		want   string
	}{
		{"empty domain", func(l *Lesson) { l.Domain = "" }, "domain"},
		{"empty cue", func(l *Lesson) { l.Cue = " " }, "cue"},
		{"empty lesson", func(l *Lesson) { l.Lesson = "" }, "lesson"},
		{"no evidence", func(l *Lesson) { l.Evidence = nil }, "no evidence"},
		{"bad confidence", func(l *Lesson) { l.Confidence = "ultra" }, "confidence"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := validLesson()
			tc.mutate(&l)
			err := Admit(t.TempDir(), l, "regulus-gate-verifier")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q refusal, got %v", tc.want, err)
			}
		})
	}
}

func TestAdmitRequiresGuardian(t *testing.T) {
	l := validLesson()
	err := Admit(t.TempDir(), l, "  ")
	if err == nil || !strings.Contains(err.Error(), "guardian") {
		t.Fatalf("want guardian refusal, got %v", err)
	}
}

func TestNewID(t *testing.T) {
	id := NewID(t0)
	if !strings.HasPrefix(id, "les_") {
		t.Fatalf("id = %q, want les_ prefix", id)
	}
	if NewID(t0) != id {
		t.Fatal("NewID must be deterministic for one timestamp")
	}
}

func TestAdmitWritesLessonAndIndex(t *testing.T) {
	root := t.TempDir()
	l := validLesson()
	l.ID = NewID(t0)
	if err := Admit(root, l, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(Dir(root, "go"), "lessons", l.ID, "LESSON.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{l.ID, l.Cue, l.Lesson, l.Evidence[0], l.Confidence, l.GitHead, l.Domain, l.Task, "admitted", "regulus-gate-verifier"} {
		if !strings.Contains(content, want) {
			t.Fatalf("LESSON.md must contain %q, got:\n%s", want, content)
		}
	}
	index, err := os.ReadFile(IndexFile(root, "go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), l.ID) || !strings.Contains(string(index), l.Cue) {
		t.Fatalf("INDEX.md must list the lesson, got:\n%s", string(index))
	}
}

func TestListParsesBackAdmittedLessons(t *testing.T) {
	root := t.TempDir()
	l1 := validLesson()
	l1.ID = NewID(t0)
	if err := Admit(root, l1, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	l2 := validLesson()
	l2.ID = NewID(t0.Add(time.Second))
	l2.Cue = "second lesson cue"
	if err := Admit(root, l2, "regulus-reviewer"); err != nil {
		t.Fatal(err)
	}
	got, err := List(root, "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 lessons, got %+v", got)
	}
	if got[0].ID != l1.ID || got[1].ID != l2.ID {
		t.Fatalf("lessons must be oldest first, got %+v", got)
	}
	if got[0].Cue != l1.Cue || got[1].Cue != l2.Cue {
		t.Fatalf("cues must round-trip, got %+v", got)
	}
	if len(got[0].Evidence) != 1 || got[0].Evidence[0] != l1.Evidence[0] {
		t.Fatalf("evidence must round-trip, got %+v", got[0])
	}
	if got[0].Confidence != ConfidenceLow {
		t.Fatalf("confidence must round-trip, got %+v", got[0])
	}
	if got[0].Status != "admitted" || got[1].Guardian != "regulus-reviewer" {
		t.Fatalf("status and guardian must round-trip, got %+v", got)
	}
}

func TestRebuildIndexRegeneratesFromDisk(t *testing.T) {
	root := t.TempDir()
	l1 := validLesson()
	l1.ID = NewID(t0)
	if err := Admit(root, l1, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	l2 := validLesson()
	l2.ID = NewID(t0.Add(time.Second))
	if err := Admit(root, l2, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	orig, err := os.ReadFile(IndexFile(root, "go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(IndexFile(root, "go")); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(root, "go"); err != nil {
		t.Fatal(err)
	}
	regenerated, err := os.ReadFile(IndexFile(root, "go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(regenerated) {
		t.Fatalf("RebuildIndex must be deterministic:\norig:\n%s\nregenerated:\n%s", orig, regenerated)
	}
}

func TestDomainsListsAdmittedDomains(t *testing.T) {
	root := t.TempDir()
	l1 := validLesson()
	l1.ID = NewID(t0)
	if err := Admit(root, l1, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	l2 := validLesson()
	l2.ID = NewID(t0.Add(time.Second))
	l2.Domain = "typescript"
	if err := Admit(root, l2, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	domains, err := Domains(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 2 || domains[0] != "go" || domains[1] != "typescript" {
		t.Fatalf("domains = %v", domains)
	}
}

func TestProposeCreatesCandidateWithoutIndex(t *testing.T) {
	root := t.TempDir()
	l := validLesson()
	l.ID = NewID(t0)
	if err := Propose(root, l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(CandidateDir(root, "go"), l.ID, "LESSON.md")); err != nil {
		t.Fatalf("the candidate must exist: %v", err)
	}
	if _, err := os.Stat(IndexFile(root, "go")); !os.IsNotExist(err) {
		t.Fatal("a proposal must not build the INDEX")
	}
	lessons, err := List(root, "go")
	if err != nil || len(lessons) != 0 {
		t.Fatalf("candidates are not lessons, got %+v (%v)", lessons, err)
	}
	cands, err := Candidates(root, "go")
	if err != nil || len(cands) != 1 || cands[0].Status != "candidate" {
		t.Fatalf("candidates = %+v (%v)", cands, err)
	}
}

func TestRemoveCandidate(t *testing.T) {
	root := t.TempDir()
	l := validLesson()
	l.ID = NewID(t0)
	if err := Propose(root, l); err != nil {
		t.Fatal(err)
	}
	if err := RemoveCandidate(root, "go", l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(CandidateDir(root, "go"), l.ID, "LESSON.md")); !os.IsNotExist(err) {
		t.Fatal("RemoveCandidate must delete the candidate")
	}
}

func TestDimensionRoundTripThroughAdmitAndCandidates(t *testing.T) {
	root := t.TempDir()
	l := validLesson()
	l.ID = NewID(t0)
	l.Dimension = DimensionINV
	if err := Admit(root, l, "regulus-gate-verifier"); err != nil {
		t.Fatal(err)
	}
	lessons, err := List(root, "go")
	if err != nil || len(lessons) != 1 || lessons[0].Dimension != DimensionINV {
		t.Fatalf("admitted round-trip = %+v (%v)", lessons, err)
	}
	data, err := os.ReadFile(IndexFile(root, "go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dimension: inv") {
		t.Fatalf("INDEX must carry the dimension, got:\n%s", data)
	}
	c := validLesson()
	c.ID = NewID(t0.Add(time.Hour))
	c.Dimension = DimensionDOM
	if err := Propose(root, c); err != nil {
		t.Fatal(err)
	}
	cands, err := Candidates(root, "go")
	if err != nil || len(cands) != 1 || cands[0].Dimension != DimensionDOM {
		t.Fatalf("candidate round-trip = %+v (%v)", cands, err)
	}
}

func TestDimensionValidation(t *testing.T) {
	l := validLesson()
	l.Dimension = "sideways"
	if err := Admit(t.TempDir(), l, "regulus-gate-verifier"); err == nil || !strings.Contains(err.Error(), "dimension") {
		t.Fatalf("want dimension refusal, got %v", err)
	}
	legacy := validLesson()
	legacy.Dimension = ""
	if err := Admit(t.TempDir(), legacy, "regulus-gate-verifier"); err != nil {
		t.Fatalf("unset dimension (legacy) must stay valid, got %v", err)
	}
}

func TestLegacyLessonWithoutDimensionLineParses(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(Dir(root, "go"), "lessons", "les_legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "# les_legacy\n\n- **domain**: go\n- **task**: feature/T1\n- **cue**: c\n- **lesson**: l\n- **evidence**: rec_1.json\n- **confidence**: low\n- **git_head**: abc\n- **created_at**: 2026-10-26T08:00:00Z\n"
	if err := os.WriteFile(filepath.Join(dir, "LESSON.md"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	lessons, err := List(root, "go")
	if err != nil || len(lessons) != 1 {
		t.Fatalf("legacy list = %+v (%v)", lessons, err)
	}
	if lessons[0].Dimension != "" {
		t.Fatalf("legacy lesson dimension = %q, want unset", lessons[0].Dimension)
	}
}
