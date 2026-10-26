package reflect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
)

var now = time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)

func writeFeatureDoc(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "charts", "feature.md")
	doc := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When regulus runs\n- Then the exit code is 0\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeReceipts(t *testing.T, root, task string, n int) {
	t.Helper()
	dir := filepath.Join(root, ".regulus", "evidence", receipts.Slug(task))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, "gate-"+strconv.Itoa(i)+".json")
		if err := os.WriteFile(path, []byte(`{"verdict":"pass"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunValidatesSpecContract(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.md")
	if err := os.WriteFile(path, []byte("# F\n\nno acceptance here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Run(root, Input{
		FeatureDoc: path, Domain: "go", Task: "feature/T1",
		Cue: "c", Lesson: "l", Now: now,
	}, "abc")
	if err == nil || !strings.Contains(err.Error(), "spec contract") {
		t.Fatalf("want spec-contract refusal, got %v", err)
	}
}

func TestRunRefusesWithoutEvidence(t *testing.T) {
	root := t.TempDir()
	doc := writeFeatureDoc(t, root)
	_, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T1",
		Cue: "c", Lesson: "l", Now: now,
	}, "abc")
	if err == nil || !strings.Contains(err.Error(), "no evidence") {
		t.Fatalf("want no-evidence refusal, got %v", err)
	}
}

func TestRunRefusesEmptyCue(t *testing.T) {
	root := t.TempDir()
	doc := writeFeatureDoc(t, root)
	writeReceipts(t, root, "feature/T1", 2)
	_, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T1",
		Cue: "  ", Lesson: "l", Now: now,
	}, "abc")
	if err == nil || !strings.Contains(err.Error(), "cue") {
		t.Fatalf("want cue refusal, got %v", err)
	}
}

func TestRunAdmitsEvidenceBackedLesson(t *testing.T) {
	root := t.TempDir()
	doc := writeFeatureDoc(t, root)
	writeReceipts(t, root, "feature/T1", 2)
	l, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T1",
		Cue: "when the gauntlet passes", Lesson: "admit the lesson",
		Now: now,
	}, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(l.ID, "les_") {
		t.Fatalf("id = %q", l.ID)
	}
	lessons, err := library.List(root, "go")
	if err != nil || len(lessons) != 1 {
		t.Fatalf("library List = %+v, %v", lessons, err)
	}
	if len(lessons[0].Evidence) != 2 {
		t.Fatalf("lesson must cite its receipts, got %+v", lessons[0])
	}
	entries, err := budget.Show(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("budget = %+v, %v", entries, err)
	}
	if entries[0].Outcome != "candidate-admitted" || entries[0].Task != "feature/T1" || entries[0].Domain != "go" {
		t.Fatalf("entry = %+v", entries[0])
	}
	index, err := os.ReadFile(library.IndexFile(root, "go"))
	if err != nil || !strings.Contains(string(index), l.ID) {
		t.Fatalf("INDEX.md must list the admitted lesson: %v\n%s", err, string(index))
	}
}
