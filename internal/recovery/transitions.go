// Live transitions (F12.5/NOV-100): record checkpoints, resume
// interrupted work, retry within budget and reassign across agents.
//
// Every transition is a pure record mutation plus validation: the
// caller persists with SaveRun after a successful call, so a crash
// between mutating and saving simply never happened (the next load
// sees the previous confirmed state and resumes from there).
package recovery

import (
	"fmt"
	"sort"
	"time"
)

// CheckpointInput carries one checkpoint write: durable state plus
// the inputs/outputs/artifacts/permissions/versions the spec
// requires persisted, and the undo steps for a later rollback.
type CheckpointInput struct {
	TaskID       string
	AgentID      string
	Host         string
	Version      string
	State        string
	Inputs       map[string]string
	Outputs      map[string]string
	Artifacts    map[string]string
	Scopes       []string
	Capabilities []string
	RequiredCaps []string
	Compensate   []Compensation
}

// RecordCheckpoint writes (or advances) one checkpoint. New
// checkpoints start at pending/running with attempt 0; existing
// ones must follow a legal edge and keep their attempt. Inputs,
// outputs, artifacts, scopes, capabilities and versions persist
// verbatim; scopes are normalized (sorted, deduped) so subset
// checks compare canonically.
func (r *RunRecord) RecordCheckpoint(in CheckpointInput, now time.Time) (*Checkpoint, error) {
	if in.State == "" {
		return nil, fmt.Errorf("recovery: checkpoint state is required")
	}
	if !validID(in.TaskID) {
		return nil, fmt.Errorf("recovery: invalid task id %q", in.TaskID)
	}
	if !validID(in.AgentID) {
		return nil, fmt.Errorf("recovery: invalid agent id %q", in.AgentID)
	}
	key := in.TaskID + "/" + in.AgentID
	prev, exists := r.Checks[key]
	from := StatePending
	if exists {
		from = prev.State
	}
	if err := CheckTransition(from, in.State); err != nil {
		return nil, err
	}
	cp := &Checkpoint{
		TaskID: in.TaskID, AgentID: in.AgentID,
		Host: in.Host, Version: in.Version, State: in.State,
		Inputs: copyMap(in.Inputs), Outputs: copyMap(in.Outputs),
		Artifacts: copyMap(in.Artifacts), Scopes: normList(in.Scopes),
		Capabilities: normList(in.Capabilities),
		RequiredCaps: normList(in.RequiredCaps),
		Compensate:   append([]Compensation(nil), in.Compensate...),
		LastConfirm:  now.UTC(),
	}
	if exists {
		cp.Attempt = prev.Attempt
		cp.Applied = prev.Applied
		// Scalar context carries over like the maps below: an
		// empty Host/Version means "unchanged", never "cleared"
		// (otherwise the circuit breaker would go blind).
		if cp.Host == "" {
			cp.Host = prev.Host
		}
		if cp.Version == "" {
			cp.Version = prev.Version
		}
		cp.History = append(append([]AttemptNote(nil), prev.History...), AttemptNote{
			Attempt: prev.Attempt, State: in.State, At: now.UTC(),
		})
		cp.Retries = prev.Retries
		cp.LastClass = prev.LastClass
		// Reassignment preserves context/evidence/permissions: a
		// checkpoint write never drops recorded inputs, outputs or
		// artifacts unless the caller replaces them explicitly.
		// Empty maps mean "unchanged", not "cleared".
		keepMaps(cp, prev)
	} else {
		cp.History = []AttemptNote{{Attempt: 0, State: in.State, At: now.UTC()}}
	}
	cp.IdemKey = IdempotencyKeyFor(r.RunID, cp.TaskID, cp.AgentID, cp.Attempt)
	if err := ValidateCheckpoint(cp); err != nil {
		return nil, err
	}
	r.Checks[key] = cp
	return cp, nil
}

// keepMaps preserves previously recorded context when a later
// checkpoint write carries empty maps: resumption must never lose
// evidence by omitting a field.
func keepMaps(cp, prev *Checkpoint) {
	if len(cp.Inputs) == 0 {
		cp.Inputs = prev.Inputs
	}
	if len(cp.Outputs) == 0 {
		cp.Outputs = prev.Outputs
	}
	if len(cp.Artifacts) == 0 {
		cp.Artifacts = prev.Artifacts
	}
	if len(cp.Scopes) == 0 {
		cp.Scopes = prev.Scopes
	}
	if len(cp.Capabilities) == 0 {
		cp.Capabilities = prev.Capabilities
	}
	if len(cp.RequiredCaps) == 0 {
		cp.RequiredCaps = prev.RequiredCaps
	}
	if len(cp.Compensate) == 0 {
		cp.Compensate = prev.Compensate
	}
}

// ResumePlan is the idempotent work list for one resumption: Skip
// holds effect keys already applied (never re-executed), Run holds
// the keys still pending, and Key is the attempt's idempotency key
// the executor acknowledges against.
type ResumePlan struct {
	TaskID  string   `json:"task_id"`
	AgentID string   `json:"agent_id"`
	Attempt int      `json:"attempt"`
	Key     string   `json:"idempotency_key"`
	State   string   `json:"state"`
	Skip    []string `json:"skip_effects"`
	Run     []string `json:"run_effects,omitempty"`
}

// Resume continues an interrupted checkpoint without restarting:
// failed/timed_out/checkpointed go back to running on the same
// attempt and the same idempotency key, so a crashed
// acknowledgement replays to stored results. Terminal states,
// exhausted budgets and open circuits refuse with the reason.
func (r *RunRecord) Resume(taskID, agentID string, effects []string, now time.Time, p RetryPolicy) (ResumePlan, error) {
	c, err := r.find(taskID, agentID)
	if err != nil {
		return ResumePlan{}, err
	}
	if IsTerminal(c.State) {
		return ResumePlan{}, fmt.Errorf("recovery: checkpoint %q is %s (terminal, resume refused)", c.Key(), c.State)
	}
	if err := CheckTransition(c.State, StateRunning); err != nil {
		return ResumePlan{}, err
	}
	if c.Retries > p.MaxRetries {
		return ResumePlan{}, fmt.Errorf("recovery: checkpoint %q exhausted its retry budget (%d > %d)", c.Key(), c.Retries, p.MaxRetries)
	}
	if c.Host != "" && r.CircuitOpen(c.Host, now, p) {
		return ResumePlan{}, fmt.Errorf("recovery: host %q circuit is open (reassign elsewhere or wait out the cooldown)", c.Host)
	}
	want := IdempotencyKeyFor(r.RunID, c.TaskID, c.AgentID, c.Attempt)
	if c.IdemKey != "" && c.IdemKey != want {
		return ResumePlan{}, fmt.Errorf("recovery: checkpoint %q key drift (stored %q, want %q)", c.Key(), c.IdemKey, want)
	}
	c.State = StateRunning
	c.IdemKey = want
	c.LastConfirm = now.UTC()
	c.History = append(c.History, AttemptNote{Attempt: c.Attempt, State: StateRunning, Detail: "resume", At: now.UTC()})
	return planFor(c, effects), nil
}

// planFor splits known effects into already-applied (skip) and
// pending (run), sorted for stable output.
func planFor(c *Checkpoint, effects []string) ResumePlan {
	plan := ResumePlan{
		TaskID: c.TaskID, AgentID: c.AgentID,
		Attempt: c.Attempt, Key: c.IdemKey, State: c.State,
	}
	for _, e := range effects {
		if _, done := c.Applied[e]; done {
			plan.Skip = append(plan.Skip, e)
		} else {
			plan.Run = append(plan.Run, e)
		}
	}
	sort.Strings(plan.Skip)
	sort.Strings(plan.Run)
	return plan
}

// AttemptRetry spends one retry of the budget on a classified
// failure: non-retryable classes, exhausted budgets, unmet backoff
// and same-host refusals fail with the reason; otherwise the
// attempt advances with a fresh idempotency key.
func (r *RunRecord) AttemptRetry(taskID, agentID, class, detail string, now time.Time, p RetryPolicy) (delayMS int64, err error) {
	c, err := r.find(taskID, agentID)
	if err != nil {
		return 0, err
	}
	if IsTerminal(c.State) {
		return 0, fmt.Errorf("recovery: checkpoint %q is %s (terminal, retry refused)", c.Key(), c.State)
	}
	if !Retryable(class) {
		return 0, fmt.Errorf("recovery: class %q never retries (resume or compensate instead)", class)
	}
	if c.Retries >= p.MaxRetries {
		c.State = StateFailed
		c.LastClass = class
		c.LastConfirm = now.UTC()
		return 0, fmt.Errorf("recovery: checkpoint %q exhausted its retry budget (%d/%d)", c.Key(), c.Retries, p.MaxRetries)
	}
	delayMS = p.BackoffDelay(c.Retries + 1)
	c.Retries++
	c.Attempt++
	c.State = StateRunning
	c.LastClass = class
	c.IdemKey = IdempotencyKeyFor(r.RunID, c.TaskID, c.AgentID, c.Attempt)
	c.LastConfirm = now.UTC()
	c.History = append(c.History, AttemptNote{
		Attempt: c.Attempt, State: StateRunning,
		Class: class, Detail: detail, At: now.UTC(),
	})
	return delayMS, nil
}
