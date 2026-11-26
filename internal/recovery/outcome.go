// Recovery outcome evidence (F17.3 / NOV-140).
//
// The F17.1 audit found that neither a Checkpoint nor a failure DecisionRecord
// carried any before/after evidence: the system knew what it proposed and that
// it finished, never whether the action worked, what it cost, or how many
// retries it took. DecisionRecord.Result is a proposed command string, which is
// a plan, not a measurement.
//
// This file adds that record as a sibling document with its own version pin,
// deliberately NOT as new fields on Checkpoint or RunRecord: those two are
// pinned to recovery-v1 and extending them would change a persisted contract
// for every existing reader. A StrategyOutcome is written next to a run, read by
// whoever wants the measured history, and ignored by readers that do not.
package recovery

import (
	"fmt"
	"strings"
	"time"
)

// OutcomeVersion pins the strategy-outcome document shape.
const OutcomeVersion = "recovery-outcome-v1"

// Recovery strategies. The vocabulary is closed: a new strategy is a contract
// change, and an unknown one is refused rather than recorded as free text.
const (
	StrategyRetry    = "retry"
	StrategyReassign = "reassign"
	StrategyReplan   = "replan"
	StrategyRollback = "rollback"
	StrategyEscalate = "escalate"
)

// strategyNames is the allowlist Validate enforces.
var strategyNames = map[string]bool{
	StrategyRetry: true, StrategyReassign: true, StrategyReplan: true,
	StrategyRollback: true, StrategyEscalate: true,
}

// Evidence is the measured state of a checkpoint at one instant. It is always
// captured from a Checkpoint — never typed by hand — so a proposal can never be
// mistaken for an observation.
type Evidence struct {
	State                string `json:"state"`
	Retries              int    `json:"retries"`
	EffectsApplied       int    `json:"effects_applied"`
	CompensationsPending int    `json:"compensations_pending"`
	CircuitOpen          bool   `json:"circuit_open"`
}

// EvidenceOf snapshots a checkpoint. Pending compensations are counted from the
// undo list, not from a caller-supplied number, and the circuit comes from the
// run record because it is host-scoped there, not checkpoint-scoped.
func EvidenceOf(c Checkpoint, circuit Circuit) Evidence {
	pending := 0
	for _, comp := range c.Compensate {
		if !comp.Compensated {
			pending++
		}
	}
	return Evidence{
		State:                c.State,
		Retries:              c.Retries,
		EffectsApplied:       len(c.Applied),
		CompensationsPending: pending,
		CircuitOpen:          !circuit.OpenedAt[c.Host].IsZero(),
	}
}

// StrategyOutcome records one recovery action with measured before/after
// evidence. Every field is an observation except Strategy and Class, which name
// what was done and what was concluded.
type StrategyOutcome struct {
	Version   string    `json:"version"`
	TaskID    string    `json:"task_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	Strategy  string    `json:"strategy"`
	Before    Evidence  `json:"before"`
	After     Evidence  `json:"after"`
	Effective bool      `json:"effective"`
	CostMS    int64     `json:"cost_ms"`
	Retries   int       `json:"retries"`
	Escalated bool      `json:"escalated"`
	Class     string    `json:"progress_class,omitempty"`
	Reasons   []string  `json:"reasons,omitempty"`
	At        time.Time `json:"at"`
}

// NewStrategyOutcome builds the record from measured checkpoints. The circuits
// carry the host-scoped state, since a Checkpoint does not own one. cost is the
// measured elapsed time of the action; a negative cost is refused because it
// would be a fabricated measurement.
func NewStrategyOutcome(strategy string, before, after Checkpoint, beforeCircuit, afterCircuit Circuit, cost time.Duration, now time.Time) (StrategyOutcome, error) {
	b := EvidenceOf(before, beforeCircuit)
	a := EvidenceOf(after, afterCircuit)
	o := StrategyOutcome{
		Version:   OutcomeVersion,
		TaskID:    before.TaskID,
		AgentID:   before.AgentID,
		Strategy:  strategy,
		Before:    b,
		After:     a,
		Effective: Effective(b, a),
		CostMS:    cost.Milliseconds(),
		Retries:   b.Retries,
		Escalated: strategy == StrategyEscalate || (a.CircuitOpen && !b.CircuitOpen),
		At:        now,
	}
	o.Reasons = o.measuredReasons()
	if err := o.Validate(); err != nil {
		return StrategyOutcome{}, err
	}
	return o, nil
}

// Effective reports whether the action produced an observable advance. It never
// consults time or retry counts: a retry that changed nothing is not effective
// even if it took an hour.
func Effective(before, after Evidence) bool {
	if after.State == StateSucceeded && before.State != StateSucceeded {
		return true
	}
	if after.EffectsApplied > before.EffectsApplied {
		return true
	}
	if after.CompensationsPending < before.CompensationsPending {
		return true
	}
	return false
}

// measuredReasons states the evidence behind the verdict, so a reader can
// disagree with the conclusion without guessing at it.
func (o StrategyOutcome) measuredReasons() []string {
	var reasons []string
	if o.Before.State != o.After.State {
		reasons = append(reasons, fmt.Sprintf("state %s -> %s", o.Before.State, o.After.State))
	}
	if d := o.After.EffectsApplied - o.Before.EffectsApplied; d != 0 {
		reasons = append(reasons, fmt.Sprintf("effects applied %+d", d))
	}
	if d := o.After.CompensationsPending - o.Before.CompensationsPending; d != 0 {
		reasons = append(reasons, fmt.Sprintf("compensations pending %+d", d))
	}
	if !o.Before.CircuitOpen && o.After.CircuitOpen {
		reasons = append(reasons, "the host circuit opened")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no observed state changed")
	}
	if !o.Effective {
		reasons = append(reasons, "not effective: nothing advanced")
	}
	return reasons
}

// Validate refuses a record that could not have come from a measurement, and a
// record whose version is not the one this code writes.
func (o StrategyOutcome) Validate() error {
	if o.Version != OutcomeVersion {
		return fmt.Errorf("recovery: outcome version %q, want %q", o.Version, OutcomeVersion)
	}
	if strings.TrimSpace(o.TaskID) == "" {
		return fmt.Errorf("recovery: outcome without a task id")
	}
	if !strategyNames[o.Strategy] {
		return fmt.Errorf("recovery: unknown strategy %q", o.Strategy)
	}
	if o.CostMS < 0 {
		return fmt.Errorf("recovery: negative cost %dms", o.CostMS)
	}
	if !knownState(o.Before.State) || !knownState(o.After.State) {
		return fmt.Errorf("recovery: outcome with unknown state (%q -> %q)", o.Before.State, o.After.State)
	}
	if o.Escalated && o.Strategy != StrategyEscalate && !o.After.CircuitOpen {
		return fmt.Errorf("recovery: escalated outcome without an escalation strategy or an open circuit")
	}
	return nil
}

// Delta renders the measured change in one line for a receipt or a report.
func (o StrategyOutcome) Delta() string {
	verdict := "no advance"
	if o.Effective {
		verdict = "advanced"
	}
	return fmt.Sprintf("%s on %s (%s): %s -> %s, %d effect(s), %d retry(ies), %dms, %s",
		o.Strategy, o.TaskID, o.AgentID, o.Before.State, o.After.State,
		o.After.EffectsApplied, o.Retries, o.CostMS, verdict)
}
