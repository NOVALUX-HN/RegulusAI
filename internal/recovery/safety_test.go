// Safety validation for recovery (F17.4 / NOV-141).
//
// The task asks to validate cancellation, checkpoint/resume, idempotency,
// rollback and safety, and to prevent duplicated intervention, arbitrary
// termination and unauthorised actions. These tests try to break those
// guarantees rather than confirm them.
package recovery

import (
	"os"
	"strings"
	"testing"
	"time"
)

func safetyRun(t *testing.T) *RunRecord {
	t.Helper()
	r, err := NewRun("run_safety", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	return r
}

func safetyCP(r *RunRecord, task, agent, state string) *Checkpoint {
	c := &Checkpoint{TaskID: task, AgentID: agent, Host: "opencode", State: state, LastConfirm: time.Unix(0, 0)}
	r.Checks[c.Key()] = c
	return c
}

// TestCancelRequiresAReason: an intervention without a written reason is
// refused, so nothing is terminated silently.
func TestCancelRequiresAReason(t *testing.T) {
	r := safetyRun(t)
	c := safetyCP(r, "t1", "a1", StateRunning)
	if _, err := r.Cancel("t1", "a1", "", time.Unix(1, 0)); err == nil {
		t.Fatal("cancel without a reason must be refused")
	}
	if c.State != StateRunning {
		t.Fatalf("a refused cancel must not change state, got %q", c.State)
	}
}

// TestCancelIsIdempotent: a second cancel has nothing left to close. It reports
// that instead of silently succeeding, and changes nothing — no duplicated
// history, no rewritten reason. Duplicated intervention is what the task
// forbids, and an error is the safe way to say "already done".
func TestCancelIsIdempotent(t *testing.T) {
	r := safetyRun(t)
	c := safetyCP(r, "t1", "a1", StateRunning)

	closed, err := r.Cancel("t1", "a1", "operator stop", time.Unix(1, 0))
	if err != nil || len(closed) != 1 {
		t.Fatalf("first cancel: closed=%v err=%v", closed, err)
	}
	history, state := len(c.History), c.State

	if _, err := r.Cancel("t1", "a1", "operator stop again", time.Unix(2, 0)); err == nil {
		t.Fatal("a second cancel must report that nothing is cancellable")
	}
	if c.State != state {
		t.Fatalf("second cancel changed state: %q -> %q", state, c.State)
	}
	if len(c.History) != history {
		t.Fatalf("second cancel duplicated history: %d -> %d", history, len(c.History))
	}
	if c.CancelReason != "operator stop" {
		t.Fatalf("the first reason must survive, got %q", c.CancelReason)
	}
}

// TestCancelNeverReopensTerminal: a terminal checkpoint is refused, so a
// finished success cannot be turned into a failure after the fact.
func TestCancelNeverReopensTerminal(t *testing.T) {
	for _, state := range []string{StateSucceeded, StateRolledBack, StateRolledBackPartial} {
		r := safetyRun(t)
		c := safetyCP(r, "t1", "a1", state)
		if _, err := r.Cancel("t1", "a1", "stop", time.Unix(1, 0)); err == nil {
			t.Fatalf("%s: cancelling a terminal checkpoint must be refused", state)
		}
		if c.State != state {
			t.Fatalf("%s must stay terminal, got %q", state, c.State)
		}
	}
}

// TestApplyEffectIsExactlyOnce: the same key runs the effect once, and the
// replay returns the stored result instead of re-executing it.
func TestApplyEffectIsExactlyOnce(t *testing.T) {
	r := safetyRun(t)
	safetyCP(r, "t1", "a1", StateRunning)

	calls := 0
	fn := func() (string, error) { calls++; return "applied", nil }

	first, err := r.ApplyEffect("t1", "a1", "k1", "opencode", "", time.Unix(1, 0), fn)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := r.ApplyEffect("t1", "a1", "k1", "opencode", "", time.Unix(2, 0), fn)
	if err != nil {
		t.Fatalf("replay apply: %v", err)
	}
	if calls != 1 {
		t.Fatalf("the effect ran %d times, want exactly 1", calls)
	}
	if first != second {
		t.Fatalf("the replay must return the stored result: %+v vs %+v", first, second)
	}
}

// TestApplyEffectRefusedWhenTerminal: a finished checkpoint cannot be mutated,
// which is what stops a late intervention from resurrecting work.
func TestApplyEffectRefusedWhenTerminal(t *testing.T) {
	r := safetyRun(t)
	safetyCP(r, "t1", "a1", StateSucceeded)
	calls := 0
	_, err := r.ApplyEffect("t1", "a1", "k1", "opencode", "", time.Unix(1, 0), func() (string, error) {
		calls++
		return "", nil
	})
	if err == nil {
		t.Fatal("an effect on a terminal checkpoint must be refused")
	}
	if calls != 0 {
		t.Fatalf("a refused effect must not run, ran %d times", calls)
	}
}

// TestReassignRefusesUnsafeTargets covers the four refusals that keep
// detection from becoming authorisation.
func TestReassignRefusesUnsafeTargets(t *testing.T) {
	p := DefaultRetryPolicy()
	now := time.Unix(10, 0)

	mk := func() (*RunRecord, *Checkpoint) {
		r := safetyRun(t)
		c := safetyCP(r, "t1", "a1", StateRunning)
		c.RequiredCaps = []string{"tools"}
		c.Scopes = []string{"read", "write"}
		return r, c
	}

	cases := []struct {
		name string
		in   ReassignInput
	}{
		{
			"capability not present on the target",
			ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToCaps: []string{"gate"}, ToScopes: []string{"read"}, VerifiedCaps: []string{"tools"}, Reason: "r"},
		},
		{
			"capability present but unverified",
			ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToCaps: []string{"tools"}, ToScopes: []string{"read"}, VerifiedCaps: nil, Reason: "r"},
		},
		{
			"scopes widen the source",
			ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToCaps: []string{"tools"}, ToScopes: []string{"read", "write", "admin"}, VerifiedCaps: []string{"tools"}, Reason: "r"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := mk()
			if _, err := r.Reassign(tc.in, now, p); err == nil {
				t.Fatalf("%s must be refused", tc.name)
			}
		})
	}

	t.Run("terminal source", func(t *testing.T) {
		r, c := mk()
		c.State = StateSucceeded
		in := ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToCaps: []string{"tools"}, ToScopes: []string{"read"}, VerifiedCaps: []string{"tools"}, Reason: "r"}
		if _, err := r.Reassign(in, now, p); err == nil {
			t.Fatal("a terminal source must be refused")
		}
	})

	t.Run("target host circuit open", func(t *testing.T) {
		r, _ := mk()
		for i := 0; i < 3; i++ {
			r.RecordFailure("codex", now, p)
		}
		in := ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToHost: "codex", ToCaps: []string{"tools"}, ToScopes: []string{"read"}, VerifiedCaps: []string{"tools"}, Reason: "r"}
		if _, err := r.Reassign(in, now, p); err == nil {
			t.Fatal("an open target circuit must be refused")
		}
	})
}

// TestReassignMovesInsteadOfCopying: reassignment must MOVE the checkpoint. A
// surviving source would leave two live records for one task, which is exactly
// the duplicated intervention the task forbids.
func TestReassignMovesInsteadOfCopying(t *testing.T) {
	r := safetyRun(t)
	c := safetyCP(r, "t1", "a1", StateRunning)
	c.RequiredCaps = []string{"tools"}
	c.Scopes = []string{"read"}
	in := ReassignInput{TaskID: "t1", FromAgent: "a1", ToAgent: "a2", ToHost: "opencode", ToCaps: []string{"tools"}, ToScopes: []string{"read"}, VerifiedCaps: []string{"tools"}, Reason: "r"}

	first, err := r.Reassign(in, time.Unix(1, 0), DefaultRetryPolicy())
	if err != nil {
		t.Fatalf("first reassign: %v", err)
	}
	if first.IdemKey != IdempotencyKeyFor(r.RunID, "t1", "a2", first.Attempt) {
		t.Fatalf("unexpected idempotency key %q", first.IdemKey)
	}
	if _, ok := r.Checks["t1/a1"]; ok {
		t.Fatal("the source must not survive: two live records would duplicate the intervention")
	}
	if _, ok := r.Checks["t1/a2"]; !ok {
		t.Fatal("the target checkpoint must exist after a reassignment")
	}
}

// TestRecoveryHasNoProcessTermination pins S9 structurally: nothing in this
// package may reach for a process to kill. Cancellation records state; it never
// terminates anything, so an observation-only host is safe by construction.
func TestRecoveryHasNoProcessTermination(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	forbidden := []string{"os/exec", "os.Process", ".Kill(", "Process.Signal", "syscall.Kill"}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(raw)
		for _, f := range forbidden {
			if strings.Contains(src, f) {
				t.Fatalf("%s contains %q: recovery must never terminate a process (S9)", name, f)
			}
		}
	}
}

// TestClassifierCannotAuthorize pins the same rule for the F17.2 classifier: a
// verdict is data, and classifying must never become acting.
func TestClassifierCannotAuthorize(t *testing.T) {
	raw, err := os.ReadFile("progress.go")
	if err != nil {
		t.Fatalf("read progress.go: %v", err)
	}
	src := string(raw)
	for _, f := range []string{"os/exec", "os.Process", ".Kill(", "syscall."} {
		if strings.Contains(src, f) {
			t.Fatalf("progress.go contains %q: classification must not act on its verdict", f)
		}
	}
}
