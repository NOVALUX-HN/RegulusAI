// Package verdict is the centralized verdict contract of the Regulus core
// (Phase 1, trust foundation; PRD R-G2 verdict authority).
//
// States: PASS, FAIL, BLOCKED, INCONCLUSIVE. The core is the sole authority:
// Adjudicate maps core-observed execution facts to a verdict. Agents and
// guardians produce evaluations, which Resolve records alongside the verdict
// but which can never change it — disagreement is flagged (Disputed), never
// applied.
package verdict

import (
	"fmt"
	"time"
)

// Status is the centralized verdict state.
type Status string

// Verdict states.
const (
	StatusPass         Status = "PASS"
	StatusFail         Status = "FAIL"
	StatusBlocked      Status = "BLOCKED"
	StatusInconclusive Status = "INCONCLUSIVE"
)

// Reason distinguishes why a verdict was reached.
type Reason string

// Verdict reasons.
const (
	ReasonOK                    Reason = "ok"
	ReasonDeterministicFailure  Reason = "deterministic_failure"
	ReasonPolicyViolation       Reason = "policy_violation"
	ReasonInsufficientEvidence  Reason = "insufficient_evidence"
	ReasonInfrastructureFailure Reason = "infrastructure_failure"
	ReasonEvaluatorDisagreement Reason = "evaluator_disagreement"
)

// Facts are the core-observed execution facts of one evaluated unit. Only
// the core constructs these; evaluators never supply them.
type Facts struct {
	// Executed reports whether the command ran (to completion or to a kill).
	Executed bool
	// TimedOut reports whether the run was killed by its deadline.
	TimedOut bool
	// SpawnError reports whether the command never ran (spawn failure).
	SpawnError bool
	// Refused reports whether execution policy refused before running.
	Refused bool
	// ResourceExhausted reports whether the run was killed for exceeding
	// its resource budget (output cap). The command did run.
	ResourceExhausted bool
	// ExitCode is the process exit code (-1 when no code exists).
	ExitCode int
	// MetricViolated reports a clean run whose acceptance metric was unmet.
	MetricViolated bool
	// EvidenceMissing reports a run whose evidence could not be recorded.
	EvidenceMissing bool
}

// Verdict is one adjudicated outcome.
type Verdict struct {
	Status Status
	Reason Reason
}

// Adjudicate maps execution facts to the authoritative verdict. Precedence:
// refusal beats everything (a refused command never runs); spawn/timeout
// failures are infrastructure; non-zero exits and unmet metrics are
// deterministic failures; runs without evidence are inconclusive.
// evaluator_disagreement is never adjudicated — it is only recorded by
// Resolve when external evaluations dispute the core.
func Adjudicate(f Facts) Verdict {
	switch {
	case f.Refused:
		return Verdict{StatusBlocked, ReasonPolicyViolation}
	case f.SpawnError:
		return Verdict{StatusBlocked, ReasonInfrastructureFailure}
	case f.TimedOut:
		return Verdict{StatusBlocked, ReasonInfrastructureFailure}
	case f.ResourceExhausted:
		return Verdict{StatusBlocked, ReasonInfrastructureFailure}
	case !f.Executed:
		return Verdict{StatusInconclusive, ReasonInsufficientEvidence}
	case f.ExitCode != 0:
		return Verdict{StatusFail, ReasonDeterministicFailure}
	case f.MetricViolated:
		return Verdict{StatusFail, ReasonDeterministicFailure}
	case f.EvidenceMissing:
		return Verdict{StatusInconclusive, ReasonInsufficientEvidence}
	default:
		return Verdict{StatusPass, ReasonOK}
	}
}

// ExitCode maps a verdict to the process exit code: 0 for PASS, 1 otherwise.
// Usage errors (exit 2) stay outside the verdict contract.
func (v Verdict) ExitCode() int {
	if v.Status == StatusPass {
		return 0
	}
	return 1
}

// Legacy maps a verdict to the historical pass/fail receipt vocabulary.
// New readers must use Status/Reason; Legacy exists only for backward
// compatibility with receipts and tooling written before Phase 1.
func (v Verdict) Legacy() string {
	if v.Status == StatusPass {
		return "pass"
	}
	return "fail"
}

// Evaluation is one external assessment (agent, guardian, or human tool).
// It carries an opinion, never authority.
type Evaluation struct {
	Evaluator string
	Status    Status
	Note      string
	At        time.Time
}

// Resolution is a core verdict with the evaluations recorded against it.
// Disputed is true when any evaluation disagrees with the core status; the
// verdict itself is unchanged by construction.
type Resolution struct {
	Verdict     Verdict
	Disputed    bool
	Evaluations []Evaluation
}

// Resolve records evaluations against an authoritative verdict. The verdict
// is returned untouched however unanimous the disagreement.
func Resolve(v Verdict, evals []Evaluation) Resolution {
	r := Resolution{Verdict: v, Evaluations: evals}
	for _, e := range evals {
		if e.Status != v.Status {
			r.Disputed = true
			break
		}
	}
	return r
}

// ParseStatus validates a status string.
func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusPass, StatusFail, StatusBlocked, StatusInconclusive:
		return Status(s), nil
	default:
		return "", fmt.Errorf("unknown verdict status %q", s)
	}
}

// ParseReason validates a reason string.
func ParseReason(s string) (Reason, error) {
	switch Reason(s) {
	case ReasonOK, ReasonDeterministicFailure, ReasonPolicyViolation,
		ReasonInsufficientEvidence, ReasonInfrastructureFailure,
		ReasonEvaluatorDisagreement:
		return Reason(s), nil
	default:
		return "", fmt.Errorf("unknown verdict reason %q", s)
	}
}
