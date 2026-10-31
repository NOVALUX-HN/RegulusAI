// Pipeline stages of controlled self-improvement (Phase 6, S1):
// OBSERVE → HYPOTHESIZE → PROPOSE → PROBE → EVALUATE → ADOPT / REJECT.
// Transitions are total and validated: stages advance one step, terminal
// stages (ADOPTED, REJECTED) advance nowhere, and every stage carries an
// explicit input/output contract.
package evolve

import "fmt"

// Stage is one pipeline step. ADOPTED and REJECTED are terminal.
type Stage string

const (
	StageObserve     Stage = "OBSERVE"
	StageHypothesize Stage = "HYPOTHESIZE"
	StagePropose     Stage = "PROPOSE"
	StageProbe       Stage = "PROBE"
	StageEvaluate    Stage = "EVALUATE"
	StageAdopted     Stage = "ADOPTED"
	StageRejected    Stage = "REJECTED"
)

// next maps each non-terminal stage to its single legal successor.
// EVALUATE is the only fork: ADOPT or REJECT.
var next = map[Stage][]Stage{
	StageObserve:     {StageHypothesize},
	StageHypothesize: {StagePropose},
	StagePropose:     {StageProbe},
	StageProbe:       {StageEvaluate},
	StageEvaluate:    {StageAdopted, StageRejected},
}

// Advance validates one pipeline step. Skips, backward moves, unknown
// stages and any move out of a terminal stage are refused.
func Advance(from, to Stage) error {
	want, ok := next[from]
	if !ok {
		return fmt.Errorf("evolve: stage %s is terminal or unknown, advances nowhere", from)
	}
	for _, n := range want {
		if n == to {
			return nil
		}
	}
	return fmt.Errorf("evolve: stage %s advances only to %v, not %s", from, want, to)
}

// Terminal reports whether the stage ends the pipeline.
func (s Stage) Terminal() bool {
	_, ok := next[s]
	return !ok
}

// StageContract is the explicit contract of one stage: what it consumes
// and what it produces (S1: cada etapa debe tener contratos explícitos).
type StageContract struct {
	Stage  Stage
	Input  string
	Output string
}

// contracts declares every stage contract in pipeline order.
var contracts = []StageContract{
	{StageObserve, "receipts, traces, guardian findings, gauntlet results, costs, memory insights, task outcomes", "supported observations (generalizable or isolated)"},
	{StageHypothesize, "supported observations (at least one generalizable)", "hypothesis: problem, root cause, change, benefit, regressions, criteria"},
	{StagePropose, "hypothesis", "stored experiment proposal with baseline, dataset, budget"},
	{StageProbe, "proposal + isolated runner", "candidate results under isolation (no harness mutation)"},
	{StageEvaluate, "candidate results + baseline results + metrics", "comparison: ADOPT or REJECT verdict with regression analysis"},
	{StageAdopted, "ADOPT verdict + human approval + reversibility proof", "versioned adoption recorded in history and audit trail"},
	{StageRejected, "REJECT verdict with reason", "recorded rejection; candidate discarded, baseline kept"},
}

// Contract returns the explicit contract of a stage.
func Contract(s Stage) StageContract {
	for _, c := range contracts {
		if c.Stage == s {
			return c
		}
	}
	return StageContract{Stage: s}
}
