package reflect

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
)

var now = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

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

func propose(t *testing.T, root, task, cue string, at time.Time) library.Lesson {
	t.Helper()
	doc := writeFeatureDoc(t, root)
	l, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: task,
		Cue: cue, Lesson: "admit the lesson with its receipts", Now: at,
	}, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	return l
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

func TestRunProposesCandidate(t *testing.T) {
	root := t.TempDir()
	writeReceipts(t, root, "feature/T1", 2)
	l := propose(t, root, "feature/T1", "when the gauntlet passes", now)
	if !strings.HasPrefix(l.ID, "les_") {
		t.Fatalf("id = %q", l.ID)
	}
	if _, err := os.Stat(filepath.Join(library.CandidateDir(root, "go"), l.ID, "LESSON.md")); err != nil {
		t.Fatalf("the candidate must exist: %v", err)
	}
	lessons, err := library.List(root, "go")
	if err != nil || len(lessons) != 0 {
		t.Fatalf("nothing is admitted before the guardian, got %+v (%v)", lessons, err)
	}
	cands, err := library.Candidates(root, "go")
	if err != nil || len(cands) != 1 || cands[0].ID != l.ID {
		t.Fatalf("candidates = %+v (%v)", cands, err)
	}
	entries, err := budget.Show(root)
	if err != nil || len(entries) != 1 || entries[0].Outcome != budget.OutcomeCandidateProposed {
		t.Fatalf("ledger = %+v (%v)", entries, err)
	}
	obs, err := almanac.All(root)
	if err != nil || len(obs) != 0 {
		t.Fatalf("the Almanac observation happens at admission, got %d (%v)", len(obs), err)
	}
}

func TestAdmitCompletesTheFlow(t *testing.T) {
	root := t.TempDir()
	writeReceipts(t, root, "feature/T1", 2)
	l := propose(t, root, "feature/T1", "when the gauntlet passes", now)
	adm, err := Admit(root, "go", l.ID, "regulus-gate-verifier", "abc123", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if adm.Status != "admitted" || adm.Guardian != "regulus-gate-verifier" {
		t.Fatalf("admitted lesson = %+v", adm)
	}
	lessons, err := library.List(root, "go")
	if err != nil || len(lessons) != 1 || lessons[0].ID != l.ID || lessons[0].Status != "admitted" || lessons[0].Guardian != "regulus-gate-verifier" {
		t.Fatalf("library = %+v (%v)", lessons, err)
	}
	index, err := os.ReadFile(library.IndexFile(root, "go"))
	if err != nil || !strings.Contains(string(index), l.ID) {
		t.Fatalf("INDEX must list the admitted lesson: %v\n%s", err, string(index))
	}
	if _, err := os.Stat(filepath.Join(library.CandidateDir(root, "go"), l.ID, "LESSON.md")); !os.IsNotExist(err) {
		t.Fatal("the candidate must be removed after admission")
	}
	obs, err := almanac.All(root)
	if err != nil || len(obs) != 1 || obs[0].Type != almanac.TypeLesson || obs[0].Lesson == nil || obs[0].Lesson.Cue != "when the gauntlet passes" {
		t.Fatalf("almanac lesson observation = %+v (%v)", obs, err)
	}
	if obs[0].Provenance == nil || obs[0].Provenance.Task != "feature/T1" || len(obs[0].Provenance.Receipts) != 2 {
		t.Fatalf("almanac provenance = %+v", obs[0].Provenance)
	}
	entries, err := budget.Show(root)
	if err != nil || len(entries) != 2 || entries[1].Outcome != budget.OutcomeAdmitted || !strings.Contains(entries[1].Detail, "regulus-gate-verifier") {
		t.Fatalf("ledger = %+v (%v)", entries, err)
	}
}

func TestAdmitRequiresGuardian(t *testing.T) {
	root := t.TempDir()
	writeReceipts(t, root, "feature/T1", 2)
	l := propose(t, root, "feature/T1", "cue", now)
	if _, err := Admit(root, "go", l.ID, "  ", "abc", now); err == nil || !strings.Contains(err.Error(), "guardian") {
		t.Fatalf("want guardian refusal, got %v", err)
	}
}

func TestAdmitUnknownCandidateRefused(t *testing.T) {
	if _, err := Admit(t.TempDir(), "go", "les_nope", "g", "abc", now); err == nil || !strings.Contains(err.Error(), "no candidate") {
		t.Fatalf("want no-candidate refusal, got %v", err)
	}
}

func TestRunRefusesWhenCapReached(t *testing.T) {
	root := t.TempDir()
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"true"}],"evolution":{"max_per_day":1}}`
	if err := os.WriteFile(filepath.Join(root, "regulus.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReceipts(t, root, "feature/T1", 2)
	writeReceipts(t, root, "feature/T2", 2)
	propose(t, root, "feature/T1", "first", now)
	doc := writeFeatureDoc(t, root)
	_, err := Run(root, Input{
		FeatureDoc: doc, Domain: "go", Task: "feature/T2",
		Cue: "second", Lesson: "over the cap", Now: now.Add(time.Hour),
	}, "abc123")
	if err == nil || !strings.Contains(err.Error(), "evolution cap reached") {
		t.Fatalf("want cap refusal, got %v", err)
	}
	entries, _ := budget.Show(root)
	if len(entries) != 2 || entries[1].Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("the refusal must be recorded, got %+v", entries)
	}
	cands, _ := library.Candidates(root, "go")
	if len(cands) != 1 {
		t.Fatalf("nothing may be proposed over the cap, got %+v", cands)
	}
}
