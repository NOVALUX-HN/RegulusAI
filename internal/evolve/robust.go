// Robust comparative evaluator (F7, NOV-59, f7-protocol-v1): baseline vs
// candidates with composition as a new candidate. Primary metrics drive
// the verdict; hard guardrails (functional, compliance, no-regression,
// adversarial, maintainability) can only reject, never promote; cost,
// latency and rework are reported and break ties only, never adopt.
// Holdout is confirmation-only behind an audited gate; missing and
// infra outcomes are never rewarded; every report carries n, dispersion
// and honest uncertainty without claiming significance.
//
// This file reuses the exploration taxonomy (ExplorePass/Fail/Timeout/
// Infra/NonEvaluable/Missing) and never redefines NOV-35 (bench tasks),
// NOV-39 (token economics), NOV-41 (adversarial corpus), NOV-42 (cycle
// adopt/reject/rollback), NOV-34 (run_id lineage) or F6 contracts.
package evolve

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Robust evaluator identity. Pinned by manifests; candidates never touch
// it (VerifyEvaluatorUntouched).
const (
	RobustEvaluatorName    = "robust-compare-v1"
	RobustEvaluatorVersion = "1.0.0"
)

// RobustMinDeltaDefault requires strict improvement by default: a tie
// keeps the baseline and lets cost break the tie only. Callers may raise
// MinDelta for a stronger bar; candidates cannot lower it
// (VerifyRobustThresholdsUntouched).
const RobustMinDeltaDefault = 0.0

// RobustSignificanceFloor is the honesty floor: below this evaluable n
// the report states significance cannot be claimed. It is a reporting
// rule, never a verdict input.
const RobustSignificanceFloor = 10

// Robust verdicts. Only ADOPT_CANDIDATE promotes; TIE keeps the baseline
// with a cost note; INSUFFICIENT and REJECT never promote.
const (
	RobustAdoptCandidate  = "ADOPT_CANDIDATE"
	RobustKeepBaseline    = "KEEP_BASELINE"
	RobustTie             = "TIE_COST_TIEBREAK"
	RobustInsufficient    = "INSUFFICIENT_EVIDENCE"
	RobustRejectGuardrail = "REJECT_GUARDRAIL"
)

// RobustSummary folds exploration outcomes without rewarding the unknown:
// pass/fail over evaluable only; timeout/infra/non-evaluable/missing sit
// in Infra and dilute nothing.
type RobustSummary struct {
	ID        string
	Total     int
	Evaluable int
	Pass      int
	Fail      int
	Infra     int
	PassRate  float64
	Tokens    int64
}

// SummarizeRobust folds outcomes deterministically (input order has no
// effect on counts). PassRate is pass/evaluable; with zero evaluable the
// rate is 0 and the verdict layer reports INSUFFICIENT.
func SummarizeRobust(id string, outcomes []ExplorationOutcome) RobustSummary {
	s := RobustSummary{ID: id, Total: len(outcomes)}
	for _, o := range outcomes {
		s.Tokens += o.Tokens
		switch o.Status {
		case ExplorePass:
			s.Pass++
		case ExploreFail:
			s.Fail++
		default:
			s.Infra++
		}
	}
	s.Evaluable = s.Pass + s.Fail
	if s.Evaluable > 0 {
		s.PassRate = float64(s.Pass) / float64(s.Evaluable)
	}
	return s
}

// RobustGuardrails are the hard gates. Every field must hold for an
// adoption; any failure rejects regardless of the primary metric.
// Cost/latency/rework live in RobustCost: informative, never gating.
type RobustGuardrails struct {
	Functional   bool
	Compliance   bool
	NoRegression bool
	Adversarial  bool
	Maintainable bool
}

// AllHardOK reports whether every hard guardrail holds.
func (g RobustGuardrails) AllHardOK() bool {
	return g.Functional && g.Compliance && g.NoRegression && g.Adversarial && g.Maintainable
}

// Failed names the failing guardrails in stable order.
func (g RobustGuardrails) Failed() []string {
	var out []string
	if !g.Functional {
		out = append(out, "functional")
	}
	if !g.Compliance {
		out = append(out, "compliance")
	}
	if !g.NoRegression {
		out = append(out, "no-regression")
	}
	if !g.Adversarial {
		out = append(out, "adversarial")
	}
	if !g.Maintainable {
		out = append(out, "maintainability")
	}
	return out
}

// RobustCost is reported and tiebreaks only. A cheaper candidate never
// converts KEEP/TIE into ADOPT.
type RobustCost struct {
	Tokens     int64
	DurationMS int64
	Rework     string
}

// RobustOptions tunes the bar. MinDelta < 0 is refused.
type RobustOptions struct {
	MinDelta float64
}

// DefaultRobustOptions returns the strict-improvement default.
func DefaultRobustOptions() RobustOptions {
	return RobustOptions{MinDelta: RobustMinDeltaDefault}
}

// RobustDecision is the machine verdict plus its honest report.
type RobustDecision struct {
	Verdict      string
	Reason       string
	Baseline     RobustSummary
	Candidate    RobustSummary
	CostTiebreak string
	N            string
	Dispersion   string
	Uncertainty  string
}

// TiebreakByCost compares cost without promoting: the cheaper side is
// named, equality reports "equal". Never returns a verdict.
func TiebreakByCost(base, cand RobustCost) string {
	if cand.Tokens < base.Tokens {
		return "candidate-cheaper"
	}
	if cand.Tokens > base.Tokens {
		return "baseline-cheaper"
	}
	return "equal"
}

// CompareRobust applies the NOV-59 rules in order: guardrails first,
// then sufficiency, then non-regression, then minimum improvement.
// Cost only annotates ties. Deterministic and pure.
func CompareRobust(baseline, candidate RobustSummary, g RobustGuardrails, baseCost, candCost RobustCost, opts RobustOptions) (RobustDecision, error) {
	if opts.MinDelta < 0 {
		return RobustDecision{}, errors.New("evolve: robust MinDelta must not be negative")
	}
	d := RobustDecision{Baseline: baseline, Candidate: candidate}
	d.CostTiebreak = TiebreakByCost(baseCost, candCost)
	d.N = fmt.Sprintf("baseline n=%d (evaluable %d), candidate n=%d (evaluable %d)",
		baseline.Total, baseline.Evaluable, candidate.Total, candidate.Evaluable)
	d.Dispersion = fmt.Sprintf("baseline pass=%d fail=%d infra=%d rate=%.3f; candidate pass=%d fail=%d infra=%d rate=%.3f",
		baseline.Pass, baseline.Fail, baseline.Infra, baseline.PassRate,
		candidate.Pass, candidate.Fail, candidate.Infra, candidate.PassRate)
	d.Uncertainty = robustUncertainty(baseline.Evaluable, candidate.Evaluable, candidate.PassRate-baseline.PassRate)
	if !g.AllHardOK() {
		d.Verdict = RobustRejectGuardrail
		d.Reason = "guardrail failure: " + strings.Join(g.Failed(), ", ")
		return d, nil
	}
	if baseline.Evaluable == 0 || candidate.Evaluable == 0 {
		d.Verdict = RobustInsufficient
		d.Reason = "insufficient evaluable evidence: missing/infra outcomes are never scored as pass"
		return d, nil
	}
	diff := candidate.PassRate - baseline.PassRate
	if diff < 0 {
		d.Verdict = RobustKeepBaseline
		d.Reason = "candidate regresses the primary metric: non-regression rule keeps the baseline"
		return d, nil
	}
	if diff == 0 {
		d.Verdict = RobustTie
		d.Reason = "equal primary metric: baseline kept, cost tiebreak only (" + d.CostTiebreak + ")"
		return d, nil
	}
	if diff < opts.MinDelta {
		d.Verdict = RobustKeepBaseline
		d.Reason = fmt.Sprintf("improvement %.4f below minimum delta %.4f: baseline kept", diff, opts.MinDelta)
		return d, nil
	}
	d.Verdict = RobustAdoptCandidate
	d.Reason = fmt.Sprintf("improvement %.4f meets minimum delta %.4f with all guardrails holding", diff, opts.MinDelta)
	return d, nil
}

// robustUncertainty states n and the observed gap descriptively. Below
// the floor it disclaims significance; at/above it still withholds any
// significance claim (significance is never tested here).
func robustUncertainty(baseEval, candEval int, diff float64) string {
	min := baseEval
	if candEval < min {
		min = candEval
	}
	if min < RobustSignificanceFloor {
		return fmt.Sprintf("n=%d/%d below significance floor %d: no significance claimed; gap %.4f is descriptive only",
			baseEval, candEval, RobustSignificanceFloor, diff)
	}
	return fmt.Sprintf("n=%d/%d: gap %.4f descriptive; significance not tested, no significance claimed",
		baseEval, candEval, diff)
}

// ComposedLineage records a composition as a new candidate identity with
// parent lineage. It carries no score: the composed candidate must be
// executed and evaluated on its own evidence, never auto-adopted.
type ComposedLineage struct {
	ID      string
	Parents []string
}

// ComposeLineage validates a composition: a non-empty new id plus at
// least two distinct parents, sorted for determinism.
func ComposeLineage(newID string, parentIDs ...string) (ComposedLineage, error) {
	if strings.TrimSpace(newID) == "" {
		return ComposedLineage{}, errors.New("evolve: composition needs a new candidate id")
	}
	seen := map[string]bool{}
	var parents []string
	for _, p := range parentIDs {
		if strings.TrimSpace(p) == "" {
			return ComposedLineage{}, errors.New("evolve: composition parents must be non-empty ids")
		}
		if !seen[p] {
			seen[p] = true
			parents = append(parents, p)
		}
	}
	if len(parents) < 2 {
		return ComposedLineage{}, errors.New("evolve: composition needs at least two distinct parents")
	}
	sort.Strings(parents)
	return ComposedLineage{ID: newID, Parents: parents}, nil
}

// EvaluatorPin pins the evaluator a candidate was scored with. Candidates
// never mutate it; any drift fails verification.
type EvaluatorPin struct {
	Name    string
	Version string
	SHA256  string
}

// VerifyEvaluatorUntouched refuses any evaluator drift between scoring and
// decision time.
func VerifyEvaluatorUntouched(pinned, current EvaluatorPin) error {
	if pinned != current {
		return fmt.Errorf("evolve: evaluator touched by candidate: pinned %+v, current %+v", pinned, current)
	}
	if strings.TrimSpace(pinned.Name) == "" || strings.TrimSpace(pinned.Version) == "" {
		return errors.New("evolve: evaluator pin needs a name and version")
	}
	return nil
}

// VerifyRobustThresholdsUntouched refuses threshold changes between
// scoring and decision: candidates never lower the bar.
func VerifyRobustThresholdsUntouched(pinnedDelta, currentDelta float64) error {
	if pinnedDelta != currentDelta {
		return fmt.Errorf("evolve: thresholds touched by candidate: pinned delta %.4f, current %.4f", pinnedDelta, currentDelta)
	}
	if currentDelta < 0 {
		return errors.New("evolve: robust MinDelta must not be negative")
	}
	return nil
}

// ConfirmHoldoutWithAudit is the only holdout path: confirmation of a
// decision already made on exploration evidence, behind an audited gate.
// Without audited=true and a non-empty audit reference it refuses; it
// never searches the holdout.
func ConfirmHoldoutWithAudit(baseline, candidate RobustSummary, g RobustGuardrails, baseCost, candCost RobustCost, opts RobustOptions, audited bool, auditRef string) (RobustDecision, error) {
	if !audited || strings.TrimSpace(auditRef) == "" {
		return RobustDecision{}, errors.New("evolve: holdout is confirmation-only behind an audited gate (audited=true with an audit reference)")
	}
	d, err := CompareRobust(baseline, candidate, g, baseCost, candCost, opts)
	if err != nil {
		return RobustDecision{}, err
	}
	d.Reason += "; holdout confirmation audited via " + auditRef
	return d, nil
}

// ContainsHoldoutLeak reports whether candidate text names a holdout id.
// Exploration proposals carry ids only (protocol §3); naming a holdout id
// is a leak and must be rejected upstream.
func ContainsHoldoutLeak(candidateText string, holdoutIDs []string) bool {
	lower := strings.ToLower(candidateText)
	for _, id := range holdoutIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(id)) {
			return true
		}
	}
	return false
}

// adversarialMarkers are prompt-injection / manipulation signals. The
// evaluator flags; it never executes candidate instructions.
var adversarialMarkers = []string{
	"ignore previous instructions",
	"ignore all instructions",
	"system:",
	"jailbreak",
	"exfiltrate",
	"self-grade",
	"override the evaluator",
	"lower the threshold",
}

// FlagsAdversarial reports whether text carries an injection or
// manipulation signal. Flagged content must fail the adversarial
// guardrail (Adversarial=false) and therefore REJECT.
func FlagsAdversarial(text string) bool {
	lower := strings.ToLower(text)
	for _, m := range adversarialMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}
