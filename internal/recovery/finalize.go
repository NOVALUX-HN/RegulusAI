// Idempotent effects, reassignment, cancellation and rollback
// (F12.5/NOV-100).
//
// Exactly-once per key: ApplyEffect executes fn only when key has
// no stored result; a crash between executing and checkpointing
// still resumes cleanly because the stored result survives in the
// run record and the next plan lists the key under Skip.
package recovery

import (
	"fmt"
	"sort"
	"time"
)

// ApplyEffect runs fn for key once: a stored result returns without
// calling fn (the no-duplicate guarantee the crash test pins); a
// fresh key executes, stores the output on success, and records the
// host success for the circuit. Failures leave no record so the
// next resume retries the same key.
func (r *RunRecord) ApplyEffect(taskID, agentID, key, host, output string, now time.Time, fn func() (string, error)) (EffectResult, error) {
	c, err := r.find(taskID, agentID)
	if err != nil {
		return EffectResult{}, err
	}
	if IsTerminal(c.State) {
		return EffectResult{}, fmt.Errorf("recovery: checkpoint %q is %s (terminal, effect refused)", c.Key(), c.State)
	}
	if prev, done := c.Applied[key]; done {
		return prev, nil
	}
	out, ferr := fn()
	if ferr != nil {
		return EffectResult{}, ferr
	}
	if output != "" {
		out = output
	}
	res := EffectResult{Key: key, Output: out, At: now.UTC()}
	if c.Applied == nil {
		c.Applied = map[string]EffectResult{}
	}
	c.Applied[key] = res
	c.LastConfirm = now.UTC()
	r.RecordSuccess(host)
	return res, nil
}

// PendingEffects lists applied keys in stable order (audit view).
func (c *Checkpoint) PendingEffects() []string {
	out := make([]string, 0, len(c.Applied))
	for k := range c.Applied {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ReassignInput carries a reassignment request: the target agent
// with its advertised capabilities/scopes/host, and the verified
// capability set (the F12.1 registry view) the target must cover.
type ReassignInput struct {
	TaskID       string
	FromAgent    string
	ToAgent      string
	ToCaps       []string
	ToScopes     []string
	ToHost       string
	VerifiedCaps []string
	Reason       string
}

// Reassign moves a checkpoint across agents only when
// capabilities and state allow: the checkpoint must be live, the
// target must advertise every required capability with each one
// present in the verified set, the target scopes must narrow (F11.8:
// subset, never widen) and the target host circuit must be closed.
// Context, evidence and permissions carry over — reassignment
// preserves them, it never re-collects.
func (r *RunRecord) Reassign(in ReassignInput, now time.Time, p RetryPolicy) (*Checkpoint, error) {
	src, err := r.find(in.TaskID, in.FromAgent)
	if err != nil {
		return nil, err
	}
	if IsTerminal(src.State) {
		return nil, fmt.Errorf("recovery: checkpoint %q is %s (terminal, reassign refused)", src.Key(), src.State)
	}
	if !validID(in.ToAgent) {
		return nil, fmt.Errorf("recovery: invalid target agent %q", in.ToAgent)
	}
	verified := map[string]bool{}
	for _, v := range in.VerifiedCaps {
		verified[v] = true
	}
	have := map[string]bool{}
	for _, c := range in.ToCaps {
		have[c] = true
	}
	for _, need := range src.RequiredCaps {
		if !have[need] {
			return nil, fmt.Errorf("recovery: target %q lacks required capability %q", in.ToAgent, need)
		}
		if !verified[need] {
			return nil, fmt.Errorf("recovery: capability %q is not verified (never assign unverified capability)", need)
		}
	}
	if !isSubset(normList(in.ToScopes), src.Scopes) {
		return nil, fmt.Errorf("recovery: target scopes %v widen %v (delegation only restricts)", in.ToScopes, src.Scopes)
	}
	if in.ToHost != "" && r.CircuitOpen(in.ToHost, now, p) {
		return nil, fmt.Errorf("recovery: target host %q circuit is open", in.ToHost)
	}
	dst := &Checkpoint{
		TaskID: src.TaskID, AgentID: in.ToAgent,
		Host: in.ToHost, Version: src.Version, State: src.State,
		Attempt: src.Attempt, Retries: src.Retries,
		Inputs: copyMap(src.Inputs), Outputs: copyMap(src.Outputs),
		Artifacts: copyMap(src.Artifacts),
		Scopes:    normList(in.ToScopes), Capabilities: normList(in.ToCaps),
		RequiredCaps: append([]string(nil), src.RequiredCaps...),
		Compensate:   append([]Compensation(nil), src.Compensate...),
		Applied:      copyApplied(src.Applied),
		History: append(append([]AttemptNote(nil), src.History...), AttemptNote{
			Attempt: src.Attempt, State: src.State,
			Detail: "reassign " + src.AgentID + "->" + in.ToAgent + ": " + in.Reason,
			At:     now.UTC(),
		}),
		LastConfirm: now.UTC(), LastClass: src.LastClass,
	}
	dst.IdemKey = IdempotencyKeyFor(r.RunID, dst.TaskID, dst.AgentID, dst.Attempt)
	if err := ValidateCheckpoint(dst); err != nil {
		return nil, err
	}
	delete(r.Checks, src.Key())
	r.Checks[dst.Key()] = dst
	return dst, nil
}

// Cancel closes one checkpoint (or every checkpoint of the run
// when taskID is "") as cancelled with the reason. Cancelled is
// terminal: no resume, retry or reassign afterwards. Cleanup —
// removing worktree/branch references the checkpoint cites — stays
// the operator's (F12.2 owns the sweep); cancel records the intent
// durably so the sweep can cite it.
func (r *RunRecord) Cancel(taskID, agentID, reason string, now time.Time) ([]string, error) {
	if reason == "" {
		return nil, fmt.Errorf("recovery: cancel needs a reason (audit)")
	}
	var closed []string
	for key, c := range r.Checks {
		if taskID != "" && c.TaskID != taskID {
			continue
		}
		if agentID != "" && c.AgentID != agentID {
			continue
		}
		if IsTerminal(c.State) {
			continue
		}
		c.State = StateCancelled
		c.CancelReason = reason
		c.LastConfirm = now.UTC()
		c.History = append(c.History, AttemptNote{
			Attempt: c.Attempt, State: StateCancelled,
			Detail: reason, At: now.UTC(),
		})
		closed = append(closed, key)
	}
	if taskID != "" && len(closed) == 0 {
		return nil, fmt.Errorf("recovery: nothing cancellable for task %q agent %q", taskID, agentID)
	}
	sort.Strings(closed)
	return closed, nil
}

// RollbackAll compensates pending undo steps in reverse order
// (LIFO: the last effect unwinds first). apply performs one undo;
// a failing apply stops the run and the checkpoint lands on
// rolled_back_partial naming what remains, otherwise rolled_back.
// Only failed/timed_out/succeeded checkpoints roll back: live work
// cancels first, terminal rollbacks never re-run.
func (r *RunRecord) RollbackAll(taskID, agentID string, now time.Time, apply func(Compensation) error) ([]string, error) {
	c, err := r.find(taskID, agentID)
	if err != nil {
		return nil, err
	}
	switch c.State {
	case StateFailed, StateTimedOut, StateSucceeded:
	default:
		return nil, fmt.Errorf("recovery: checkpoint %q is %s (only failed/timed_out/succeeded roll back)", c.Key(), c.State)
	}
	var done []string
	for i := len(c.Compensate) - 1; i >= 0; i-- {
		step := c.Compensate[i]
		if step.Compensated {
			continue
		}
		if err := apply(step); err != nil {
			c.State = StateRolledBackPartial
			c.LastConfirm = now.UTC()
			c.History = append(c.History, AttemptNote{
				Attempt: c.Attempt, State: StateRolledBackPartial,
				Detail: "rollback stopped at " + step.Key + ": " + err.Error(),
				At:     now.UTC(),
			})
			return done, fmt.Errorf("recovery: rollback partial after %d step(s), stuck at %q: %w", len(done), step.Key, err)
		}
		step.Compensated = true
		step.At = now.UTC()
		c.Compensate[i] = step
		done = append(done, step.Key)
	}
	c.State = StateRolledBack
	c.LastConfirm = now.UTC()
	c.History = append(c.History, AttemptNote{
		Attempt: c.Attempt, State: StateRolledBack,
		Detail: fmt.Sprintf("%d compensation(s) applied", len(done)),
		At:     now.UTC(),
	})
	return done, nil
}

// helpers

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyApplied(m map[string]EffectResult) map[string]EffectResult {
	if m == nil {
		return nil
	}
	out := make(map[string]EffectResult, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func normList(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func isSubset(want, have []string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
