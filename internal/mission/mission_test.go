// Behavior tests for F13.1/NOV-104: the mission state machine,
// its invariants, validation and the exactly-once effect ledger.
package mission

import (
	"strings"
	"testing"
	"time"
)

func testNow() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func testInput(id string) NewInput {
	return NewInput{
		ID: id, Objective: "ship the mission engine",
		Description: "durable missions", Repo: "regulus-ai",
		Constraints: []string{"no network"}, Criteria: []string{"green CI"},
		Risk: RiskMedium, Creator: "tester",
	}
}

func mustMission(t *testing.T, id string) *Mission {
	t.Helper()
	m, err := NewMission(testInput(id), testNow())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustTransition(t *testing.T, m *Mission, to string) {
	t.Helper()
	if _, err := m.Transition(to, "tester", "test step", "", testNow()); err != nil {
		t.Fatalf("Transition to %q: %v", to, err)
	}
}

func TestCreateAndValidate(t *testing.T) {
	m := mustMission(t, "m1")
	if m.State != StateCreated {
		t.Fatalf("new mission state = %q, want created", m.State)
	}
	if m.Risk != RiskMedium {
		t.Fatalf("default risk = %q, want medium", m.Risk)
	}
	if len(m.History) != 1 {
		t.Fatalf("new mission history = %d events, want 1", len(m.History))
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestCreateValidationRefusals(t *testing.T) {
	bad := []NewInput{
		{ID: "m-bad", Objective: ""},
		{ID: "../escape", Objective: "x"},
		{ID: "m-bad", Objective: "x", Risk: "extreme"},
		{ID: "m-bad", Objective: "x", Budget: Budget{TimeMS: -1}},
		{ID: "m-bad", Objective: "line1\nline2"},
		{ID: "m-bad", Objective: "x", Constraints: []string{""}},
		{ID: "m-bad", Objective: "x", Criteria: []string{"ok\nevil"}},
	}
	for i, in := range bad {
		if _, err := NewMission(in, testNow()); err == nil {
			t.Fatalf("case %d: NewMission must refuse %v", i, in)
		}
	}
}

func TestForwardPathTransitions(t *testing.T) {
	m := mustMission(t, "m-forward")
	for _, to := range []string{
		StateAnalyzing, StatePlanning, StateAwaitingApproval,
		StateExecuting, StateIntegrating, StateValidating, StateCompleted,
	} {
		mustTransition(t, m, to)
	}
	if m.State != StateCompleted {
		t.Fatalf("end state = %q, want completed", m.State)
	}
	if len(m.History) != 8 {
		t.Fatalf("history = %d events, want 8 (create + 7 moves)", len(m.History))
	}
}

func TestIllegalTransitionsRefused(t *testing.T) {
	m := mustMission(t, "m-illegal")
	if _, err := m.Transition(StateExecuting, "t", "skip", "", testNow()); err == nil {
		t.Fatal("created -> executing must refuse")
	} else if !strings.Contains(err.Error(), "illegal transition") {
		t.Fatalf("must name the illegal edge, got %q", err)
	}
	mustTransition(t, m, StateAnalyzing)
	if _, err := m.Transition(StateCompleted, "t", "skip", "", testNow()); err == nil {
		t.Fatal("analyzing -> completed must refuse")
	}
	mustTransition(t, m, StatePlanning)
	if _, err := m.Transition(StateCreated, "t", "back", "", testNow()); err == nil {
		t.Fatal("planning -> created must refuse")
	}
	if m.State != StatePlanning {
		t.Fatalf("refused transition must not move the mission, state = %q", m.State)
	}
	if _, err := m.Transition("teleport", "t", "x", "", testNow()); err == nil {
		t.Fatal("unknown target state must refuse")
	} else if !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("must name the unknown state, got %q", err)
	}
}

func TestTerminalStatesNeverReopenViaTransition(t *testing.T) {
	for _, terminal := range []string{StateCompleted, StatePartiallyCompleted, StateFailed, StateCancelled} {
		m := mustMission(t, "m-term")
		mustTransition(t, m, StateAnalyzing)
		m.State = terminal
		for _, to := range []string{StateExecuting, StateRecovering, StateBlocked, StateCreated} {
			if _, err := m.Transition(to, "t", "reopen", "", testNow()); err == nil {
				t.Fatalf("%s -> %s must refuse (terminal reopens only via Reopen)", terminal, to)
			} else if !strings.Contains(err.Error(), "terminal") {
				t.Fatalf("%s -> %s must name terminality, got %q", terminal, to, err)
			}
		}
	}
}

func TestPauseResumeRoundTrip(t *testing.T) {
	m := mustMission(t, "m-pause")
	mustTransition(t, m, StateAnalyzing)
	mustTransition(t, m, StatePlanning)
	mustTransition(t, m, StateAwaitingApproval)
	mustTransition(t, m, StateExecuting)
	if _, err := m.Pause("op", "coffee", "", testNow()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if m.State != StateBlocked || m.PausedFrom != StateExecuting {
		t.Fatalf("paused = %q from %q, want blocked from executing", m.State, m.PausedFrom)
	}
	if _, err := m.Resume("op", "back", "", testNow()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if m.State != StateExecuting || m.PausedFrom != "" {
		t.Fatalf("resumed = %q bookmark %q, want executing with cleared bookmark", m.State, m.PausedFrom)
	}
}

func TestPauseResumeRefusals(t *testing.T) {
	m := mustMission(t, "m-pauser")
	if _, err := m.Pause("op", "x", "", testNow()); err == nil {
		t.Fatal("created must not pause")
	}
	if _, err := m.Resume("op", "x", "", testNow()); err == nil {
		t.Fatal("non-blocked must not resume")
	}
	mustTransition(t, m, StateAnalyzing)
	mustTransition(t, m, StatePlanning)
	if _, err := m.Transition(StateBlocked, "op", "park", "", testNow()); err != nil {
		t.Fatalf("direct advance to blocked: %v", err)
	}
	if _, err := m.Resume("op", "x", "", testNow()); err == nil {
		t.Fatal("blocked without a pause bookmark must refuse resume")
	} else if !strings.Contains(err.Error(), "bookmark") {
		t.Fatalf("must name the missing bookmark, got %q", err)
	}
	if _, err := m.Resume("op", "x", "", testNow()); err == nil {
		t.Fatal("second resume without a new pause must refuse")
	}
	// Resume clears the bookmark: a second resume is impossible.
	m2 := mustMission(t, "m-pauser2")
	mustTransition(t, m2, StateAnalyzing)
	if _, err := m2.Pause("op", "x", "", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := m2.Resume("op", "x", "", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := m2.Resume("op", "x", "", testNow()); err == nil {
		t.Fatal("resume after a completed resume must refuse")
	}
}

func TestCancelNeedsReasonAndCloses(t *testing.T) {
	m := mustMission(t, "m-cancel")
	if _, err := m.Cancel("op", "", "", testNow()); err == nil {
		t.Fatal("cancel without a reason must refuse")
	} else if !strings.Contains(err.Error(), "reason") {
		t.Fatalf("must name the missing reason, got %q", err)
	}
	mustTransition(t, m, StateAnalyzing)
	if _, err := m.Cancel("op", "out of scope", "", testNow()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if m.State != StateCancelled || m.CancelReason != "out of scope" {
		t.Fatalf("cancelled = %q reason %q", m.State, m.CancelReason)
	}
	if _, err := m.Transition(StateExecuting, "t", "x", "", testNow()); err == nil {
		t.Fatal("cancelled must not transition")
	}
	if _, err := m.Cancel("op", "again", "", testNow()); err == nil {
		t.Fatal("cancel of a terminal mission must refuse")
	}
}

func TestReopenIsExplicitAndAudited(t *testing.T) {
	m := mustMission(t, "m-reopen")
	mustTransition(t, m, StateAnalyzing)
	mustTransition(t, m, StatePlanning)
	mustTransition(t, m, StateAwaitingApproval)
	mustTransition(t, m, StateExecuting)
	mustTransition(t, m, StateIntegrating)
	mustTransition(t, m, StateValidating)
	mustTransition(t, m, StateFailed)
	if _, err := m.Reopen("op", "", "", testNow()); err == nil {
		t.Fatal("reopen without a reason must refuse")
	}
	before := len(m.History)
	if _, err := m.Reopen("op", "flake was environmental", "", testNow()); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if m.State != StateRecovering || m.ReopenCount != 1 {
		t.Fatalf("reopened = %q count %d, want recovering count 1", m.State, m.ReopenCount)
	}
	if len(m.History) != before+1 {
		t.Fatal("reopen must append exactly one audit event")
	}
	last := m.History[len(m.History)-1]
	if !strings.Contains(last.Reason, "flake was environmental") {
		t.Fatalf("reopen reason must stay in the audit trail, got %q", last.Reason)
	}
	live := mustMission(t, "m-reopen-live")
	if _, err := live.Reopen("op", "x", "", testNow()); err == nil {
		t.Fatal("reopen of a live mission must refuse")
	}
}

func TestIdempotentReplay(t *testing.T) {
	m := mustMission(t, "m-idem")
	mustTransition(t, m, StateAnalyzing)
	first, err := m.Transition(StatePlanning, "op", "go", "key-1", testNow())
	if err != nil {
		t.Fatal(err)
	}
	n := len(m.History)
	again, err := m.Transition(StatePlanning, "op", "go again", "key-1", testNow())
	if err != nil {
		t.Fatalf("idem replay must succeed: %v", err)
	}
	if again.EventID != first.EventID {
		t.Fatal("idem replay must return the stored event, not a new one")
	}
	if len(m.History) != n {
		t.Fatal("idem replay must not append history")
	}
	// A fresh key on the same edge is a new audited move.
	m2 := mustMission(t, "m-idem2")
	mustTransition(t, m2, StateAnalyzing)
	if _, err := m2.Transition(StatePlanning, "op", "go", "key-A", testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := m2.Transition(StatePlanning, "op", "go", "key-B", testNow()); err != nil {
		// planning -> planning is a no-op, always succeeds.
		t.Fatal(err)
	}
}

func TestAttachLinksDedupes(t *testing.T) {
	m := mustMission(t, "m-links")
	in := LinkInput{
		RunIDs: []string{"run_1", "run_2"}, TaskIDs: []string{"t1"},
		AgentIDs: []string{"a1"}, Worktrees: []string{"wt/m1"},
		Commits: []string{"abc123"}, Traces: []string{"run_1"},
		Receipts: []string{"receipts/m1.json"},
	}
	if err := m.AttachLinks(in); err != nil {
		t.Fatal(err)
	}
	if err := m.AttachLinks(in); err != nil {
		t.Fatalf("re-attaching the same ids must be a no-op: %v", err)
	}
	if len(m.Links.RunIDs) != 2 {
		t.Fatalf("run ids = %v, want 2 deduplicated", m.Links.RunIDs)
	}
	if m.Links.RunIDs[0] != "run_1" || m.Links.RunIDs[1] != "run_2" {
		t.Fatalf("links must stay sorted, got %v", m.Links.RunIDs)
	}
	bad := LinkInput{RunIDs: []string{"../escape"}}
	if err := m.AttachLinks(bad); err == nil {
		t.Fatal("hostile run id must refuse")
	}
	badRef := LinkInput{Worktrees: []string{"a/../../escape"}}
	if err := m.AttachLinks(badRef); err == nil {
		t.Fatal("traversal worktree must refuse")
	}
	if len(m.Links.Worktrees) != 1 {
		t.Fatal("refused attach must not mutate the stored set")
	}
}

func TestApplyEffectExactlyOnce(t *testing.T) {
	m := mustMission(t, "m-effect")
	calls := 0
	res, err := m.ApplyEffect("send-batch", "", testNow(), func() (string, error) {
		calls++
		return "batch-42", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "batch-42" || calls != 1 {
		t.Fatalf("first apply = %q calls %d", res.Output, calls)
	}
	res2, err := m.ApplyEffect("send-batch", "", testNow(), func() (string, error) {
		calls++
		return "batch-OTHER", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Output != "batch-42" || calls != 1 {
		t.Fatalf("replay must return the stored output without re-executing: %q calls %d", res2.Output, calls)
	}
	m.State = StateCompleted
	if _, err := m.ApplyEffect("late", "", testNow(), func() (string, error) { return "x", nil }); err == nil {
		t.Fatal("terminal missions must refuse new effects")
	}
}
