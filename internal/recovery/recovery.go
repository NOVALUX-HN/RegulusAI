// Package recovery implements F12.5/NOV-100: durable states and
// checkpoints per task and agent, idempotent resumption of
// interrupted executions, bounded retries with backoff/timeout and
// circuit breakers, error classification, capability/state-gated
// reassignment, idempotency keys or compensations, and
// cancellation with cleanup and partial rollback.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - traceability rides the F11.1 exec log by sharing run_id
//     (observability.RecordExec); this package never owns a trace
//     store, it only cites run/task/agent ids the cmdx layer emits.
//   - permission narrowing follows F11.8 (mcp.Principal.Delegate
//     only restricts): every scope set recorded here must be a
//     subset of the caller's, and a reassignment must narrow
//     further, never widen.
//   - agent capability evidence comes from the F12.1 registry
//     (orchestrate.RegistryFromManifests over the F11.7 matrix);
//     this package takes the verified set as an explicit parameter
//     so the rule stays pure and testable.
//   - worktree/merge isolation is F12.2's; recovery only records
//     where the isolated work lives (artifacts), it never moves it.
//
// Nothing here executes work: Checkpoint/Resume/Retry record durable
// intent and completion; the executor performs effects through
// ApplyEffect (idempotency-keyed, exactly-once per key) and confirms
// compensations through RollbackAll. Crash-safe means: a crash
// between applying an effect and acknowledging the checkpoint still
// resumes without re-executing the recorded effect.
package recovery

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RecoveryVersion pins the run-record document shape: readers
// compare only within one version.
const RecoveryVersion = "recovery-v1"

// Durable states. Terminal states close a checkpoint for good:
// succeeded, cancelled, rolled_back, rolled_back_partial. failed and
// timed_out stay resumable while the retry budget holds; pending,
// running and checkpointed are the live path.
const (
	StatePending           = "pending"
	StateRunning           = "running"
	StateCheckpointed      = "checkpointed"
	StateSucceeded         = "succeeded"
	StateFailed            = "failed"
	StateTimedOut          = "timed_out"
	StateCancelled         = "cancelled"
	StateRolledBack        = "rolled_back"
	StateRolledBackPartial = "rolled_back_partial"
)

// IsTerminal reports whether state closes the checkpoint: no resume,
// retry or reassignment may advance it afterwards.
func IsTerminal(state string) bool {
	switch state {
	case StateSucceeded, StateCancelled, StateRolledBack, StateRolledBackPartial:
		return true
	}
	return false
}

// legalTransitions is the allowed edge set. failed/timed_out may go
// back to running (resume/retry within budget); terminal states
// have no out-edges.
var legalTransitions = map[string]map[string]bool{
	StatePending:      {StateRunning: true, StateCancelled: true},
	StateRunning:      {StateCheckpointed: true, StateSucceeded: true, StateFailed: true, StateTimedOut: true, StateCancelled: true},
	StateCheckpointed: {StateRunning: true, StateSucceeded: true, StateFailed: true, StateCancelled: true},
	StateFailed:       {StateRunning: true, StateCancelled: true, StateRolledBack: true, StateRolledBackPartial: true},
	StateTimedOut:     {StateRunning: true, StateCancelled: true, StateRolledBack: true, StateRolledBackPartial: true},
	StateSucceeded:    {StateRolledBack: true, StateRolledBackPartial: true},
}

// CheckTransition refuses an illegal state edge before anything is
// recorded, so a crash can never be "recovered" into a state the
// live path could not reach.
func CheckTransition(from, to string) error {
	if from == to {
		return nil
	}
	if IsTerminal(from) && from != StateSucceeded {
		return fmt.Errorf("recovery: state %q is terminal (no transition to %q)", from, to)
	}
	nexts, ok := legalTransitions[from]
	if !ok {
		return fmt.Errorf("recovery: unknown state %q", from)
	}
	if !nexts[to] {
		return fmt.Errorf("recovery: illegal transition %q -> %q", from, to)
	}
	return nil
}

// Error classes (closed vocabulary). Retryability is conservative:
// only permanent and auth never retry; everything else is bounded
// by the retry budget, the timeout and the circuit breaker.
const (
	ClassCrash      = "crash"
	ClassNetwork    = "network"
	ClassHostOff    = "host_offline"
	ClassTimeout    = "timeout"
	ClassPartial    = "partial"
	ClassPermanent  = "permanent"
	ClassAuth       = "auth"
	ClassUnknown    = "unknown"
	ClassCancelledE = "cancelled"
)

// EffectResult is one exactly-once effect: the output recorded
// under its idempotency key. Re-applying the same key returns the
// stored output without re-executing.
type EffectResult struct {
	Key    string    `json:"key"`
	Output string    `json:"output"`
	At     time.Time `json:"at"`
}

// Compensation is one undo step for rollback: the key identifies
// the effect it undoes, Desc tells the operator what to do,
// Compensated records durable completion (never the act itself).
type Compensation struct {
	Key         string    `json:"key"`
	Desc        string    `json:"desc"`
	Compensated bool      `json:"compensated"`
	At          time.Time `json:"at,omitempty"`
}

// AttemptNote is one history line: what happened on an attempt and
// how it was classified. History is the audit trail the final
// state summarizes, never a second ledger.
type AttemptNote struct {
	Attempt int       `json:"attempt"`
	State   string    `json:"state"`
	Class   string    `json:"class,omitempty"`
	Detail  string    `json:"detail,omitempty"`
	At      time.Time `json:"at"`
}

// Checkpoint is the durable per-task/per-agent record: inputs,
// outputs, artifacts, permissions, versions and the last confirmed
// state, plus the idempotency ledger (Applied), the undo list
// (Compensations) and the audit history.
type Checkpoint struct {
	TaskID       string                  `json:"task_id"`
	AgentID      string                  `json:"agent_id"`
	Host         string                  `json:"host,omitempty"`
	Version      string                  `json:"version,omitempty"`
	State        string                  `json:"state"`
	Attempt      int                     `json:"attempt"`
	IdemKey      string                  `json:"idempotency_key"`
	Inputs       map[string]string       `json:"inputs,omitempty"`
	Outputs      map[string]string       `json:"outputs,omitempty"`
	Artifacts    map[string]string       `json:"artifacts,omitempty"`
	Scopes       []string                `json:"scopes,omitempty"`
	Capabilities []string                `json:"capabilities,omitempty"`
	RequiredCaps []string                `json:"required_caps,omitempty"`
	Compensate   []Compensation          `json:"compensations,omitempty"`
	Applied      map[string]EffectResult `json:"applied_effects,omitempty"`
	History      []AttemptNote           `json:"history,omitempty"`
	LastConfirm  time.Time               `json:"last_confirmed"`
	Retries      int                     `json:"retries"`
	LastClass    string                  `json:"last_error_class,omitempty"`
	CancelReason string                  `json:"cancel_reason,omitempty"`
}

// Circuit tracks consecutive per-host failures for one run: after
// Threshold failures the host is open (refused) until CooldownMS
// passes. Success on any checkpoint resets the host count.
type Circuit struct {
	Failures map[string]int       `json:"failures,omitempty"`
	OpenedAt map[string]time.Time `json:"opened_at,omitempty"`
}

// RunRecord is one recoverable execution: every checkpoint plus
// the run-level circuit and the F11.1 trace link (shared run_id).
type RunRecord struct {
	Version    string                 `json:"version"`
	RunID      string                 `json:"run_id"`
	TraceRunID string                 `json:"trace_run_id"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Checks     map[string]*Checkpoint `json:"checkpoints"`
	Circuit    Circuit                `json:"circuit"`
}

// Key scopes one checkpoint inside a run.
func (c *Checkpoint) Key() string { return c.TaskID + "/" + c.AgentID }

// IdempotencyKeyFor mints the exactly-once key for an attempt:
// same run/task/agent/attempt always yields the same key, so a
// crashed acknowledgement replays to the stored result instead of
// re-executing.
func IdempotencyKeyFor(runID, taskID, agentID string, attempt int) string {
	return fmt.Sprintf("%s/%s/%s#%d", runID, taskID, agentID, attempt)
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// validID accepts store ids usable as file names without traversal:
// alphanumeric start, conservative charset, no "..", bounded.
func validID(s string) bool {
	return idRe.MatchString(s) && !strings.Contains(s, "..")
}

// ValidateRunID refuses hostile run ids before they touch the store.
func ValidateRunID(runID string) error {
	if !validID(runID) {
		return fmt.Errorf("recovery: invalid run id %q (alphanumeric start, [A-Za-z0-9._/-], no .., max 128)", runID)
	}
	return nil
}

// ValidateCheckpoint refuses empty task/agent ids, unknown states
// knownState reports whether s is one of the durable checkpoint states. There
// is exactly one allowlist: ValidateCheckpoint and the strategy-outcome record
// both use it, so the two can never disagree about what a valid state is.
func knownState(s string) bool {
	switch s {
	case StatePending, StateRunning, StateCheckpointed, StateSucceeded,
		StateFailed, StateTimedOut, StateCancelled,
		StateRolledBack, StateRolledBackPartial:
		return true
	}
	return false
}

// and relative-path-hostile values before recording.
func ValidateCheckpoint(c *Checkpoint) error {
	if !validID(c.TaskID) {
		return fmt.Errorf("recovery: invalid task id %q", c.TaskID)
	}
	if !validID(c.AgentID) {
		return fmt.Errorf("recovery: invalid agent id %q", c.AgentID)
	}
	if knownState(c.State) {
		return nil
	}
	return fmt.Errorf("recovery: unknown state %q", c.State)
}

// NewRun starts one run record. TraceRunID defaults to RunID so
// checkpoints link to the F11.1 exec log by the shared id without
// a second store.
func NewRun(runID string, now time.Time) (*RunRecord, error) {
	if err := ValidateRunID(runID); err != nil {
		return nil, err
	}
	return &RunRecord{
		Version: RecoveryVersion, RunID: runID, TraceRunID: runID,
		CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
		Checks: map[string]*Checkpoint{}, Circuit: Circuit{},
	}, nil
}

// find locates a checkpoint or reports it missing (never a silent
// nil: callers distinguish "no checkpoint" from "terminal").
func (r *RunRecord) find(taskID, agentID string) (*Checkpoint, error) {
	c, ok := r.Checks[taskID+"/"+agentID]
	if !ok {
		return nil, fmt.Errorf("recovery: no checkpoint for task %q agent %q in run %q", taskID, agentID, r.RunID)
	}
	return c, nil
}
