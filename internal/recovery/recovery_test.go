// Behavior tests for F12.5/NOV-100: crash resumption without
// duplicate effects, bounded retries, context-preserving
// reassignment, network/host-offline/timeout/partial handling,
// cancellation and rollback.
package recovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testNow() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func testRun(t *testing.T, id string) *RunRecord {
	t.Helper()
	r, err := NewRun(id, testNow())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustCheckpoint(t *testing.T, r *RunRecord, task, agent, state string) *Checkpoint {
	t.Helper()
	c, err := r.RecordCheckpoint(CheckpointInput{
		TaskID: task, AgentID: agent, Host: "h1",
		State:        state,
		Inputs:       map[string]string{"goal": "ship"},
		Outputs:      map[string]string{"draft": "v1"},
		Artifacts:    map[string]string{"patch": "sha-1"},
		Scopes:       []string{"memory:read", "memory:write"},
		RequiredCaps: []string{"code-review"},
		Capabilities: []string{"code-review"},
		Compensate: []Compensation{
			{Key: "e1", Desc: "revert patch"},
			{Key: "e2", Desc: "drop temp branch"},
		},
	}, testNow())
	if err != nil {
		t.Fatalf("RecordCheckpoint %s: %v", state, err)
	}
	return c
}

// Crash between applying an effect and acknowledging the
// checkpoint must resume without re-executing the effect.
func TestCrashResumesWithoutDuplicatingEffects(t *testing.T) {
	dir := t.TempDir()
	r := testRun(t, "run-crash")
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	calls := 0
	if _, err := r.ApplyEffect("t1", "a1", "send-batch", "h1", "", testNow(), func() (string, error) {
		calls++
		return "batch-42", nil
	}); err != nil {
		t.Fatal(err)
	}
	// Crash: persist the applied effect but never advance the
	// state to checkpointed (the acknowledgement is lost).
	if _, err := SaveRun(dir, r, testNow()); err != nil {
		t.Fatal(err)
	}
	// "Restart": reload from disk, resume, replay the effect list.
	_, r2, err := LoadRun(dir, "run-crash")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := r2.Resume("t1", "a1", []string{"send-batch", "notify"}, testNow(), DefaultRetryPolicy())
	if err != nil {
		t.Fatalf("Resume after crash: %v", err)
	}
	if len(plan.Skip) != 1 || plan.Skip[0] != "send-batch" {
		t.Fatalf("resume must skip the applied effect, got %+v", plan)
	}
	if len(plan.Run) != 1 || plan.Run[0] != "notify" {
		t.Fatalf("resume must still run the pending effect, got %+v", plan)
	}
	if got, err := r2.ApplyEffect("t1", "a1", "send-batch", "h1", "", testNow(), func() (string, error) {
		calls++
		return "batch-42-DUP", nil
	}); err != nil || got.Output != "batch-42" {
		t.Fatalf("re-apply must return the stored output without executing, got %+v err=%v", got, err)
	}
	if calls != 1 {
		t.Fatalf("effect executed %d times, want exactly 1", calls)
	}
	if _, err := SaveRun(dir, r2, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadRun(dir, "run-crash"); err != nil {
		t.Fatalf("reloaded run must stay valid: %v", err)
	}
}

// Tampered stores and illegal edges fail closed.
func TestStoreAndTransitionsFailClosed(t *testing.T) {
	dir := t.TempDir()
	r := testRun(t, "run-guard")
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	if _, err := SaveRun(dir, r, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateSucceeded}, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveRun(dir, r, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resume("t1", "a1", nil, testNow(), DefaultRetryPolicy()); err == nil {
		t.Fatal("resume on succeeded must be refused")
	}
	if _, err := r.Reassign(ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2"}, testNow(), DefaultRetryPolicy()); err == nil {
		t.Fatal("reassign on terminal must be refused")
	}
	if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateRunning}, testNow()); err == nil {
		t.Fatal("succeeded -> running must be refused")
	}
	if _, err := NewRun("../escape", testNow()); err == nil {
		t.Fatal("hostile run id must be refused")
	}
	// Tamper with the bytes: the sha must catch it.
	raw := mustRead(t, filepath.Join(dir, ".regulus", "recovery", "run-guard", RunFile))
	raw = append(raw, '\n')
	mustWrite(t, filepath.Join(dir, ".regulus", "recovery", "run-guard", RunFile), raw)
	if _, _, err := LoadRun(dir, "run-guard"); err == nil || !strings.Contains(err.Error(), "sha mismatch") {
		t.Fatalf("edited store must fail on sha, got %v", err)
	}
}

// Retries are bounded, backoff is deterministic, non-retryable
// classes refuse immediately.
func TestRetriesAreBounded(t *testing.T) {
	r := testRun(t, "run-retry")
	p := DefaultRetryPolicy()
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	if got := p.BackoffDelay(1); got != 1000 {
		t.Fatalf("backoff(1) = %d, want 1000", got)
	}
	if got := p.BackoffDelay(2); got != 2000 {
		t.Fatalf("backoff(2) = %d, want 2000", got)
	}
	if got := p.BackoffDelay(99); got != 30000 {
		t.Fatalf("backoff(99) = %d, want cap 30000", got)
	}
	for _, class := range []string{ClassPermanent, ClassAuth, ClassPartial, ClassCancelledE} {
		if _, err := r.AttemptRetry("t1", "a1", class, "nope", testNow(), p); err == nil {
			t.Fatalf("class %q must never retry", class)
		}
	}
	for i := 0; i < p.MaxRetries; i++ {
		if _, err := r.AttemptRetry("t1", "a1", ClassNetwork, "blip", testNow(), p); err != nil {
			t.Fatalf("retry %d: %v", i+1, err)
		}
	}
	c, _ := r.find("t1", "a1")
	if c.Retries != p.MaxRetries || c.Attempt != p.MaxRetries {
		t.Fatalf("budget position wrong: %+v", c)
	}
	if _, err := r.AttemptRetry("t1", "a1", ClassNetwork, "blip", testNow(), p); err == nil {
		t.Fatal("budget exhaustion must refuse")
	}
	if c.State != StateFailed {
		t.Fatalf("exhausted checkpoint must land on failed, got %q", c.State)
	}
}

// Reassignment preserves context/evidence/permissions and refuses
// capability or scope violations.
func TestReassignPreservesContextAndNarrows(t *testing.T) {
	r := testRun(t, "run-move")
	p := DefaultRetryPolicy()
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateFailed}, testNow()); err != nil {
		t.Fatal(err)
	}
	got, err := r.Reassign(ReassignInput{
		TaskID: "t1", FromAgent: "a1", ToAgent: "a2",
		ToCaps:   []string{"code-review", "extra"},
		ToScopes: []string{"memory:read"},
		ToHost:   "h2", VerifiedCaps: []string{"code-review", "extra"},
		Reason: "h1 flaky",
	}, testNow(), p)
	if err != nil {
		t.Fatalf("Reassign: %v", err)
	}
	if got.Inputs["goal"] != "ship" || got.Outputs["draft"] != "v1" || got.Artifacts["patch"] != "sha-1" {
		t.Fatalf("reassign lost context/evidence: %+v", got)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "memory:read" {
		t.Fatalf("reassign must carry the narrowed scopes, got %v", got.Scopes)
	}
	if _, ok := r.Checks["t1/a1"]; ok {
		t.Fatal("source checkpoint must move, not copy")
	}
	// Unverified capability: refused even when advertised.
	mustCheckpoint(t, r, "t2", "a1", StateRunning)
	if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t2", AgentID: "a1", State: StateFailed}, testNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reassign(ReassignInput{
		TaskID: "t2", FromAgent: "a1", ToAgent: "a9",
		ToCaps: []string{"code-review"}, ToScopes: []string{"memory:read"},
		VerifiedCaps: []string{"other-cap"}, Reason: "x",
	}, testNow(), p); err == nil {
		t.Fatal("unverified capability must refuse reassignment")
	}
	// Scope widening: refused (F11.8 delegation only restricts).
	if _, err := r.Reassign(ReassignInput{
		TaskID: "t2", FromAgent: "a1", ToAgent: "a9",
		ToCaps: []string{"code-review"}, ToScopes: []string{"memory:read", "memory:write", "gate:execute"},
		VerifiedCaps: []string{"code-review"}, Reason: "x",
	}, testNow(), p); err == nil {
		t.Fatal("scope widening must refuse reassignment")
	}
}

// Error signals map to the documented classes.
func TestClassifyCoversSpecSignals(t *testing.T) {
	cases := map[string]string{
		"connection refused by peer":     ClassNetwork,
		"TLS handshake timeout":          ClassNetwork,
		"host offline, node unreachable": ClassHostOff,
		"deadline exceeded":              ClassTimeout,
		"context deadline exceeded":      ClassTimeout,
		"partial: some effects applied":  ClassPartial,
		"panic: runtime error":           ClassCrash,
		"process died (exit 137)":        ClassCrash,
		"policy denied for tool x":       ClassAuth,
		"malformed checkpoint":           ClassPermanent,
		"something nobody named":         ClassUnknown,
	}
	for signal, want := range cases {
		if got := ClassifyError(signal); got != want {
			t.Errorf("ClassifyError(%q) = %q, want %q", signal, got, want)
		}
	}
	if Retryable(ClassPermanent) || Retryable(ClassAuth) {
		t.Fatal("permanent/auth must never retry")
	}
	if !Retryable(ClassNetwork) || !Retryable(ClassCrash) || !Retryable(ClassUnknown) {
		t.Fatal("transient classes must stay retryable (bounded)")
	}
	if SameHostAllowed(ClassHostOff) {
		t.Fatal("host_offline must move hosts")
	}
}

// The circuit opens after Threshold consecutive failures and a
// resume on the open host refuses; success closes it.
func TestCircuitBreakerOpensAndCloses(t *testing.T) {
	r := testRun(t, "run-circuit")
	p := DefaultRetryPolicy()
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	now := testNow()
	for i := 0; i < p.CircuitThreshold; i++ {
		r.RecordFailure("h1", now, p)
		if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateFailed}, now); err != nil {
			t.Fatal(err)
		}
		if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateRunning}, now); err != nil {
			t.Fatal(err)
		}
	}
	if !r.CircuitOpen("h1", now, p) {
		t.Fatal("circuit must be open after Threshold consecutive failures")
	}
	if _, err := r.Resume("t1", "a1", nil, now, p); err == nil {
		t.Fatal("resume on an open-circuit host must be refused")
	}
	// Cooldown passes: the host serves again.
	later := now.Add(10 * time.Minute)
	if r.CircuitOpen("h1", later, p) {
		t.Fatal("circuit must close after cooldown")
	}
	r.RecordSuccess("h1")
	if r.CircuitOpen("h1", now, p) {
		t.Fatal("success must close the circuit")
	}
}

// Timeout marks the attempt; partial resumes through the ledger,
// never as a blind retry.
func TestTimeoutAndPartial(t *testing.T) {
	p := DefaultRetryPolicy()
	if !p.TimedOut(testNow(), testNow().Add(61*time.Second)) {
		t.Fatal("61s must exceed the 60s attempt timeout")
	}
	if p.TimedOut(testNow(), testNow().Add(59*time.Second)) {
		t.Fatal("59s must stay inside the 60s attempt timeout")
	}
	r := testRun(t, "run-partial")
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	if _, err := r.ApplyEffect("t1", "a1", "e1", "h1", "", testNow(), func() (string, error) { return "ok", nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateFailed}, testNow()); err != nil {
		t.Fatal(err)
	}
	c, _ := r.find("t1", "a1")
	c.LastClass = ClassPartial
	if _, err := r.AttemptRetry("t1", "a1", ClassPartial, "half-applied", testNow(), p); err == nil {
		t.Fatal("partial must not blind-retry")
	}
	plan, err := r.Resume("t1", "a1", []string{"e1", "e2"}, testNow(), p)
	if err != nil {
		t.Fatalf("partial must resume through the ledger: %v", err)
	}
	if len(plan.Skip) != 1 || len(plan.Run) != 1 {
		t.Fatalf("partial resume must skip e1 and run e2, got %+v", plan)
	}
}

// Cancel is terminal and audited; rollback unwinds LIFO with a
// partial state when one compensation fails.
func TestCancelAndRollback(t *testing.T) {
	r := testRun(t, "run-end")
	mustCheckpoint(t, r, "t1", "a1", StateRunning)
	closed, err := r.Cancel("t1", "a1", "operator stop", testNow())
	if err != nil || len(closed) != 1 {
		t.Fatalf("Cancel: %v %v", closed, err)
	}
	c, _ := r.find("t1", "a1")
	if c.State != StateCancelled || c.CancelReason == "" {
		t.Fatalf("cancel must be terminal with a reason: %+v", c)
	}
	if _, err := r.Resume("t1", "a1", nil, testNow(), DefaultRetryPolicy()); err == nil {
		t.Fatal("resume after cancel must be refused")
	}
	// Rollback order: last effect unwinds first.
	r2 := testRun(t, "run-rb")
	mustCheckpoint(t, r2, "t1", "a1", StateRunning)
	if _, err := r2.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateFailed}, testNow()); err != nil {
		t.Fatal(err)
	}
	var order []string
	done, err := r2.RollbackAll("t1", "a1", testNow(), func(s Compensation) error {
		order = append(order, s.Key)
		return nil
	})
	if err != nil || len(done) != 2 || order[0] != "e2" || order[1] != "e1" {
		t.Fatalf("rollback must unwind LIFO, got %v err=%v", order, err)
	}
	c2, _ := r2.find("t1", "a1")
	if c2.State != StateRolledBack {
		t.Fatalf("full rollback must land on rolled_back, got %q", c2.State)
	}
	// Partial: a failing compensation stops the run mid-list.
	r3 := testRun(t, "run-partial-rb")
	mustCheckpoint(t, r3, "t1", "a1", StateRunning)
	if _, err := r3.RecordCheckpoint(CheckpointInput{TaskID: "t1", AgentID: "a1", State: StateFailed}, testNow()); err != nil {
		t.Fatal(err)
	}
	_, err = r3.RollbackAll("t1", "a1", testNow(), func(s Compensation) error {
		if s.Key == "e2" {
			return errors.New("branch already deleted")
		}
		return nil
	})
	if err == nil {
		t.Fatal("failing compensation must surface partial rollback")
	}
	c3, _ := r3.find("t1", "a1")
	if c3.State != StateRolledBackPartial {
		t.Fatalf("must land on rolled_back_partial, got %q", c3.State)
	}
	// Live work never rolls back directly: cancel first.
	r4 := testRun(t, "run-live")
	mustCheckpoint(t, r4, "t1", "a1", StateRunning)
	if _, err := r4.RollbackAll("t1", "a1", testNow(), func(Compensation) error { return nil }); err == nil {
		t.Fatal("live checkpoint must refuse rollback")
	}
}
