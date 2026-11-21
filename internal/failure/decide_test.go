// Decision-matrix tests (F13.5/NOV-108): every class selects
// its documented action inside budgets and degrades safely when
// budgets or guards refuse. Pure Decide calls — no store, no
// mutation — plus the repeated-recovery idempotency proof
// against the F12.5 ledger (reused, never reimplemented).
package failure

import (
	"testing"
	"time"

	"regulus-ai/internal/recovery"
)

func decide(t *testing.T, sig Signal, bid ReassignBid, p Policy) DecisionRecord {
	t.Helper()
	if sig.MissionID == "" {
		sig.MissionID = "msn_test"
	}
	rec, err := Decide(sig, bid, p, time.Now().UTC())
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return rec
}

func TestDecideTransientRetriesWithinBudget(t *testing.T) {
	rec := decide(t, Signal{ErrorText: "connection reset by peer", MissionState: "executing"}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionRetry {
		t.Fatalf("decision = %q, want retry", rec.Decision)
	}
	if rec.BackoffMS <= 0 {
		t.Fatal("retry must quote its deterministic backoff")
	}
	if rec.SuggestedMissionTo != "recovering" {
		t.Fatalf("suggested edge = %q, want recovering", rec.SuggestedMissionTo)
	}
	if len(rec.Discarded) == 0 || rec.Result == "" {
		t.Fatal("the record must name discarded alternatives and the proposed result")
	}
}

func TestDecideTransientBudgetExhaustionEscalates(t *testing.T) {
	p := DefaultPolicy()
	rec := decide(t, Signal{ErrorText: "tls handshake timeout", RetriesUsed: p.Retry.MaxRetries, MissionState: "executing"}, ReassignBid{}, p)
	if rec.Decision != DecisionPauseForApproval {
		t.Fatalf("exhausted transient decision = %q, want pause_for_approval", rec.Decision)
	}
}

func TestDecideWorkerMissingReassignsWhenVerified(t *testing.T) {
	bid := ReassignBid{Present: true, Verified: true, ToAgent: "agent-b", Detail: "verified caps/scopes/worktree"}
	rec := decide(t, Signal{ErrorText: "worker lost heartbeat", MissionState: "executing"}, bid, DefaultPolicy())
	if rec.Decision != DecisionReassign {
		t.Fatalf("verified offline decision = %q, want reassign", rec.Decision)
	}
}

func TestDecideWorkerMissingIsolatesWhenUnverified(t *testing.T) {
	bid := ReassignBid{Present: true, ToAgent: "agent-x", Detail: "target lacks required capability gpu"}
	rec := decide(t, Signal{ErrorText: "agent missing", MissionState: "executing"}, bid, DefaultPolicy())
	if rec.Decision != DecisionIsolate {
		t.Fatalf("unverified offline decision = %q, want isolate", rec.Decision)
	}
}

func TestDecideHostOfflineReassigns(t *testing.T) {
	bid := ReassignBid{Present: true, Verified: true, ToAgent: "agent-c", ToHost: "host-b"}
	rec := decide(t, Signal{ErrorText: "host down, node offline", MissionState: "executing"}, bid, DefaultPolicy())
	if rec.Decision != DecisionReassign {
		t.Fatalf("host_offline decision = %q, want reassign", rec.Decision)
	}
}

func TestDecideMergeConflictIsolates(t *testing.T) {
	rec := decide(t, Signal{ErrorText: "merge conflict in plan files", MissionState: "integrating"}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionIsolate || rec.SuggestedMissionTo != "blocked" {
		t.Fatalf("conflict decision = %q edge = %q, want isolate/blocked", rec.Decision, rec.SuggestedMissionTo)
	}
}

func TestDecideGateRedReplansPreservingConfirmed(t *testing.T) {
	rec := decide(t, Signal{
		GateVerdict: "red", MissionState: "validating",
		Confirmed: []string{"t1", "t2"}, PlanTasks: []string{"t1", "t2", "t3"},
	}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionReplan {
		t.Fatalf("gate red decision = %q, want replan", rec.Decision)
	}
}

func TestDecideGateRedRefusingGuardPauses(t *testing.T) {
	rec := decide(t, Signal{
		GateVerdict: "red", MissionState: "validating",
		Confirmed: []string{"t1"}, PlanTasks: []string{"t2"},
	}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionPauseForApproval {
		t.Fatalf("guard-violating replan decision = %q, want pause_for_approval", rec.Decision)
	}
}

func TestDecideBudgetExhaustedPauses(t *testing.T) {
	rec := decide(t, Signal{ErrorText: "cost limit hit", BudgetExhausted: true, MissionState: "executing"}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionPauseForApproval || RetryableClass(rec.Class) {
		t.Fatalf("budget decision = %q (retryable=%v), want pause_for_approval", rec.Decision, RetryableClass(rec.Class))
	}
}

func TestDecidePermissionDeniedFailsClosed(t *testing.T) {
	rec := decide(t, Signal{ErrorText: "forbidden: policy denied scope", MissionState: "executing"}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionFailClosed || rec.SuggestedMissionTo != "blocked" {
		t.Fatalf("denial decision = %q edge = %q, want fail_closed/blocked", rec.Decision, rec.SuggestedMissionTo)
	}
}

func TestDecideCorruptAndUnknownFailClosed(t *testing.T) {
	for _, text := range []string{"run data sha mismatch", "something odd happened"} {
		rec := decide(t, Signal{ErrorText: text, MissionState: "executing"}, ReassignBid{}, DefaultPolicy())
		if rec.Decision != DecisionFailClosed {
			t.Fatalf("%q decision = %q, want fail_closed", text, rec.Decision)
		}
	}
}

func TestDecidePermanentRollsBackOwnedUnit(t *testing.T) {
	rec := decide(t, Signal{
		ErrorText: "refused: permanent plan invalid", MissionState: "executing",
		CompensationsPending: 2, CompensationKeys: []string{"msn_test/t1", "msn_test/t2"},
		OwnedPrefixes: []string{"msn_test"},
	}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionRollback {
		t.Fatalf("permanent+compensations decision = %q, want rollback", rec.Decision)
	}
}

func TestDecidePermanentWithoutCompensationsStops(t *testing.T) {
	rec := decide(t, Signal{ErrorText: "refused: permanent plan invalid", MissionState: "executing"}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionFailClosed || rec.SuggestedMissionTo != "failed" {
		t.Fatalf("permanent decision = %q edge = %q, want fail_closed/failed", rec.Decision, rec.SuggestedMissionTo)
	}
}

func TestDecideRollbackScopeViolationStops(t *testing.T) {
	rec := decide(t, Signal{
		ErrorText: "refused: permanent plan invalid", MissionState: "executing",
		CompensationsPending: 1, CompensationKeys: []string{"other-mission/t9"},
		OwnedPrefixes: []string{"msn_test"},
	}, ReassignBid{}, DefaultPolicy())
	if rec.Decision != DecisionFailClosed {
		t.Fatalf("external-scope decision = %q, want fail_closed (never revert work outside the mission)", rec.Decision)
	}
}

func TestRepeatedRecoveryReplaysWithoutDuplicating(t *testing.T) {
	// Crash between effect and acknowledgement: the F12.5 ledger
	// (reused) executes fn once and replays the stored output.
	dir := t.TempDir()
	now := time.Now().UTC()
	run, err := recovery.NewRun("run1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.RecordCheckpoint(recovery.CheckpointInput{
		TaskID: "t1", AgentID: "a1", State: recovery.StateRunning,
	}, now); err != nil {
		t.Fatal(err)
	}
	calls := 0
	apply := func() (string, error) { calls++; return "done", nil }
	if _, err := run.ApplyEffect("t1", "a1", "fx", "", "", now, apply); err != nil {
		t.Fatal(err)
	}
	if _, err := run.ApplyEffect("t1", "a1", "fx", "", "", now, apply); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("effect executed %d times, want exactly once", calls)
	}
	// Same idem key twice: the decision log replays, never appends.
	sig := Signal{MissionID: "msn_test", ErrorText: "connection reset", IdemKey: "k1"}
	first := decide(t, sig, ReassignBid{}, DefaultPolicy())
	first.IdemKey = "k1"
	if _, appended, err := AppendDecision(dir, first); err != nil || !appended {
		t.Fatalf("first append: appended=%v err=%v", appended, err)
	}
	second := decide(t, sig, ReassignBid{}, DefaultPolicy())
	second.IdemKey = "k1"
	stored, appended, err := AppendDecision(dir, second)
	if err != nil || appended {
		t.Fatalf("replay: appended=%v err=%v (must replay without appending)", appended, err)
	}
	if stored.ID != first.ID {
		t.Fatalf("replay id = %q, want stored %q", stored.ID, first.ID)
	}
}
