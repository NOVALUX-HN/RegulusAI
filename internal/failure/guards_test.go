// Guard, store and lesson tests (F13.5/NOV-108): replan
// confirmed-set protection, rollback scope confinement,
// reassignment verification, the append-only decision log and the
// F12.6 hypothesis-only feeding contract.
package failure

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/recovery"
)

func TestCheckReplanPreservesConfirmed(t *testing.T) {
	if err := CheckReplan([]string{"t1"}, []string{"t1", "t2"}, nil); err != nil {
		t.Fatalf("intact replan refused: %v", err)
	}
	if err := CheckReplan([]string{"t1"}, []string{"t2"}, nil); err == nil {
		t.Fatal("replan dropping confirmed t1 must refuse")
	}
	if err := CheckReplan([]string{"t1"}, []string{"t1"}, []string{"t1"}); err == nil {
		t.Fatal("replan modifying confirmed t1 must refuse")
	}
}

func TestCheckRollbackScopeConfinesUnit(t *testing.T) {
	if err := CheckRollbackScope([]string{"msn_a/t1", "run7/t2"}, []string{"msn_a", "run7"}); err != nil {
		t.Fatalf("owned rollback refused: %v", err)
	}
	if err := CheckRollbackScope([]string{"other/t9"}, []string{"msn_a"}); err == nil {
		t.Fatal("rollback of external work must refuse")
	}
	if err := CheckRollbackScope([]string{"msn_a/t1"}, nil); err == nil {
		t.Fatal("rollback with no provenance must refuse")
	}
}

func verifyRun(t *testing.T) *recovery.RunRecord {
	t.Helper()
	now := time.Now().UTC()
	run, err := recovery.NewRun("run1", now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = run.RecordCheckpoint(recovery.CheckpointInput{
		TaskID: "t1", AgentID: "a1", State: recovery.StateRunning,
		Scopes: []string{"read", "write"}, RequiredCaps: []string{"go"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestVerifyReassignAttestsTarget(t *testing.T) {
	now := time.Now().UTC()
	detail, err := VerifyReassign(VerifyReassignInput{
		Rec: verifyRun(t), TaskID: "t1", FromAgent: "a1",
		ToAgent: "b1", ToCaps: []string{"go"}, VerifiedCaps: []string{"go"},
		ToScopes: []string{"read"}, ToHost: "h2",
		WorktreeState: "clean", Reason: "worker lost",
	}, now, DefaultPolicy())
	if err != nil {
		t.Fatalf("verified bid refused: %v", err)
	}
	if !strings.Contains(detail, "a1->b1") {
		t.Fatalf("detail must cite the move, got %q", detail)
	}
}

func TestVerifyReassignRefusals(t *testing.T) {
	now := time.Now().UTC()
	base := VerifyReassignInput{
		Rec: verifyRun(t), TaskID: "t1", FromAgent: "a1",
		ToAgent: "b1", ToCaps: []string{"go"}, VerifiedCaps: []string{"go"},
		ToScopes: []string{"read"}, WorktreeState: "clean",
	}
	cases := map[string]func(*VerifyReassignInput){
		"dirty worktree":   func(in *VerifyReassignInput) { in.WorktreeState = "dirty" },
		"unknown worktree": func(in *VerifyReassignInput) { in.WorktreeState = "unknown" },
		"missing cap":      func(in *VerifyReassignInput) { in.ToCaps = []string{"rust"} },
		"unverified cap":   func(in *VerifyReassignInput) { in.VerifiedCaps = nil },
		"widened scopes":   func(in *VerifyReassignInput) { in.ToScopes = []string{"read", "admin"} },
	}
	for name, mutate := range cases {
		in := base
		mutate(&in)
		if _, err := VerifyReassign(in, now, DefaultPolicy()); err == nil {
			t.Fatalf("%s: reassignment must refuse", name)
		}
	}
}

func TestDecisionLogRoundTripAndFailClosed(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	rec, err := Decide(Signal{MissionID: "msn_a", ErrorText: "boom"}, ReassignBid{}, DefaultPolicy(), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, appended, err := AppendDecision(dir, rec); err != nil || !appended {
		t.Fatalf("append: appended=%v err=%v", appended, err)
	}
	recs, err := LoadDecisions(dir, "msn_a")
	if err != nil || len(recs) != 1 || recs[0].ID != rec.ID {
		t.Fatalf("load: %+v %v", recs, err)
	}
	if got, err := FindDecision(recs, "1"); err != nil || got.ID != rec.ID {
		t.Fatalf("index lookup: %v %v", got, err)
	}
	if got, err := FindDecision(recs, rec.ID[:8]); err != nil || got.ID != rec.ID {
		t.Fatalf("prefix lookup: %v %v", got, err)
	}
	if _, err := FindDecision(recs, "9"); err == nil {
		t.Fatal("out-of-range index must refuse")
	}
	if _, err := LoadDecisions(dir, "msn_missing"); err != nil {
		t.Fatalf("missing log must read as zero decisions: %v", err)
	}
}

func TestLessonIsHypothesisOnly(t *testing.T) {
	rec, err := Decide(Signal{MissionID: "msn_a", ErrorText: "gate red", GateVerdict: "red"}, ReassignBid{}, DefaultPolicy(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	lesson := BuildLesson(rec, []string{"rcpt1"}, time.Now().UTC())
	if lesson.Type != almanac.KindHypothesis {
		t.Fatalf("lesson kind = %q, want hypothesis (never verified-solution)", lesson.Type)
	}
	if lesson.Type == almanac.KindVerified {
		t.Fatal("coordinator must never author verified-solution")
	}
	if lesson.Confidence != "low" && lesson.Confidence != "medium" {
		t.Fatalf("confidence = %q, want low|medium", lesson.Confidence)
	}
	if lesson.Author == "" || lesson.Scope == "" || lesson.Provenance == nil || lesson.Provenance.Task != "msn_a" {
		t.Fatalf("lesson must carry author, scope and mission provenance: %+v", lesson)
	}
}

func TestExplainQuotesSealedRecord(t *testing.T) {
	rec, err := Decide(Signal{MissionID: "msn_a", ErrorText: "permission denied by policy", MissionState: "executing"}, ReassignBid{}, DefaultPolicy(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	out := Explain(rec)
	for _, want := range []string{rec.ID, "fail_closed", "discarded alternatives", rec.Result} {
		if !strings.Contains(out, want) {
			t.Fatalf("explain must quote %q, got:\n%s", want, out)
		}
	}
}
