package guardians

// F2 (spec S2): guardian decision log — validation refusals,
// JSONL round-trip under .regulus/guardians/decisions.jsonl,
// absence-reads-empty, and the consolidation hook. RED test: fails
// before decisions.go exists.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/receipts"
)

func TestRecordDecisionValidation(t *testing.T) {
	root := t.TempDir()
	if _, err := RecordDecision(root, "task-a", "", DecisionAgree, "note"); err == nil {
		t.Error("empty guardian must be refused")
	}
	if _, err := RecordDecision(root, "task-a", "  ", DecisionAgree, "note"); err == nil {
		t.Error("blank guardian must be refused")
	}
	if _, err := RecordDecision(root, "task-a", "regulus-reviewer", "", "note"); err == nil {
		t.Error("empty verdict must be refused")
	}
	for _, bad := range []string{"pass", "FAIL", "maybe", "agreed", "agree/disagree"} {
		if _, err := RecordDecision(root, "task-a", "regulus-reviewer", bad, "note"); err == nil {
			t.Errorf("verdict %q must be refused (want agree|disagree)", bad)
		}
	}
	// Refused writes leave no store behind.
	if _, err := os.Stat(DecisionsFile(root)); !os.IsNotExist(err) {
		t.Errorf("refused decisions must not create the log (stat err = %v)", err)
	}
	if got, err := ListDecisions(root, "task-a"); err != nil || len(got) != 0 {
		t.Errorf("refused decisions must read back empty: got=%v err=%v", got, err)
	}
}

func TestRecordDecisionRoundTrip(t *testing.T) {
	root := t.TempDir()
	d1, err := RecordDecision(root, "feat/T1", "regulus-reviewer", DecisionAgree, "matches receipts")
	if err != nil {
		t.Fatalf("RecordDecision agree: %v", err)
	}
	if d1.Task != "feat__T1" || d1.Guardian != "regulus-reviewer" || d1.Verdict != DecisionAgree {
		t.Fatalf("recorded decision fields wrong: %+v", d1)
	}
	if _, err := RecordDecision(root, "feat/T1", "regulus-evidence-auditor", DecisionDisagree, "missing receipt"); err != nil {
		t.Fatalf("RecordDecision disagree: %v", err)
	}
	// The store is JSONL: two lines under .regulus/guardians.
	raw, err := os.ReadFile(filepath.Join(root, ".regulus", "guardians", "decisions.jsonl"))
	if err != nil {
		t.Fatalf("read decisions log: %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(string(raw)), "\n"); len(lines) != 2 {
		t.Fatalf("want 2 JSONL lines, got %d: %q", len(lines), raw)
	}
	got, err := ListDecisions(root, "feat/T1")
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 decisions, got %+v", got)
	}
	if got[0].Guardian != "regulus-reviewer" || got[0].Verdict != DecisionAgree {
		t.Errorf("oldest first: %+v", got[0])
	}
	if got[1].Guardian != "regulus-evidence-auditor" || got[1].Verdict != DecisionDisagree {
		t.Errorf("second: %+v", got[1])
	}
	// Task scope is sanitized: other tasks read empty.
	if scoped, err := ListDecisions(root, "other"); err != nil || len(scoped) != 0 {
		t.Errorf("other task must read empty: got=%v err=%v", scoped, err)
	}
	// Empty task lists every task's decisions.
	if all, err := ListDecisions(root, ""); err != nil || len(all) != 2 {
		t.Errorf("empty task must list all: got=%v err=%v", all, err)
	}
}

func TestListDecisionsAbsentIsEmpty(t *testing.T) {
	got, err := ListDecisions(t.TempDir(), "task-a")
	if err != nil {
		t.Fatalf("absent log must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("absent log must read empty, got %+v", got)
	}
}

func TestConsolidateRecordsDecisions(t *testing.T) {
	root := t.TempDir()
	id := writeVerdictReceipt(t, root, "feat/T1", "build", "FAIL", receipts.VerdictFail, 1)
	if _, err := Evaluate(root, "feat/T1", "regulus-evidence-auditor", "FAIL", "gate failed as specified", []string{id}); err != nil {
		t.Fatal(err)
	}
	if _, err := Evaluate(root, "feat/T1", "regulus-reviewer", "PASS", "looks fine to me", []string{id}); err != nil {
		t.Fatal(err)
	}
	con, err := Consolidate(root, "feat/T1")
	if err != nil {
		t.Fatal(err)
	}
	// Verdict logic is unchanged by the logging hook.
	if con.Verdict != "FAIL" || con.Agree != 1 || con.Dispute != 1 {
		t.Fatalf("hook must not change verdict logic, got %+v", con)
	}
	got, err := ListDecisions(root, "feat/T1")
	if err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("consolidation must record one decision per evaluation, got %+v", got)
	}
	byGuardian := map[string]string{}
	for _, d := range got {
		byGuardian[d.Guardian] = d.Verdict
	}
	if byGuardian["regulus-evidence-auditor"] != DecisionAgree {
		t.Errorf("agreeing evaluation must record agree, got %v", byGuardian)
	}
	if byGuardian["regulus-reviewer"] != DecisionDisagree {
		t.Errorf("disputing evaluation must record disagree, got %v", byGuardian)
	}
}
