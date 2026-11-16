// Validation comparison, decision and reproducible report (F11.6,
// NOV-92): baseline vs candidate with repetitions on governed splits,
// quality/latency/tokens separated with N and variability, the
// exploration verdict frozen before any holdout confirmation, human
// approval with an explicit inherited-change decision, and at least one
// real rejection as proof the evaluator says no.
package evolve

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/trace"
)

// ValidationArmStats aggregates one arm's exploration trials with the
// three F11.6 dimensions kept separate: quality (pass-rate over
// evaluable only — infra never dilutes), latency (mean/min/max over
// evaluable attempts), tokens (measured sources summed, unknown
// counted but never summed, per F11.5).
type ValidationArmStats struct {
	Arm            string
	Total          int
	Evaluable      int
	Pass           int
	Fail           int
	Infra          int
	PassRate       float64
	LatencyN       int
	LatencyMeanMS  float64
	LatencyMinMS   int64
	LatencyMaxMS   int64
	TokensMeasured int64
	UnknownTrials  int
	Sources        map[string]int
	Models         []string
	Seeds          []int64
}

// ValidationComparison is the frozen exploration verdict: per-arm
// stats, the descriptive gap, whether both arms ran comparable models,
// and the honesty note (significance is never claimed).
type ValidationComparison struct {
	Baseline         ValidationArmStats
	Candidate        ValidationArmStats
	GapPassRate      float64
	ModelsComparable bool
	Verdict          string
	Reason           string
	Honesty          string
}

// armStatsOf folds one arm's trials. Pass/fail drive quality;
// timeout/infra/non-evaluable sit in Infra and dilute nothing.
func armStatsOf(arm string, trials []ValidationTrial) ValidationArmStats {
	s := ValidationArmStats{Arm: arm, Sources: map[string]int{}}
	var latSum int64
	for _, t := range trials {
		s.Total++
		s.Sources[t.TokenSource]++
		switch t.Status {
		case ManifestPass:
			s.Pass++
		case ManifestFail:
			s.Fail++
		default:
			s.Infra++
			continue
		}
		s.LatencyN++
		latSum += t.LatencyMS
		if s.LatencyN == 1 || t.LatencyMS < s.LatencyMinMS {
			s.LatencyMinMS = t.LatencyMS
		}
		if t.LatencyMS > s.LatencyMaxMS {
			s.LatencyMaxMS = t.LatencyMS
		}
		if t.TokenSource == trace.SourceUnknown {
			s.UnknownTrials++
		} else {
			s.TokensMeasured += t.Tokens
		}
	}
	s.Evaluable = s.Pass + s.Fail
	if s.Evaluable > 0 {
		s.PassRate = float64(s.Pass) / float64(s.Evaluable)
	}
	if s.LatencyN > 0 {
		s.LatencyMeanMS = float64(latSum) / float64(s.LatencyN)
	}
	modelSet := map[string]bool{}
	seedSet := map[int64]bool{}
	for _, t := range trials {
		modelSet[t.Model] = true
		seedSet[t.Seed] = true
	}
	for m := range modelSet {
		s.Models = append(s.Models, m)
	}
	for v := range seedSet {
		s.Seeds = append(s.Seeds, v)
	}
	sort.Strings(s.Models)
	sort.Slice(s.Seeds, func(i, j int) bool { return s.Seeds[i] < s.Seeds[j] })
	return s
}

// honestyNote states n and the observed gap descriptively. Below the
// significance floor it disclaims significance; at/above it, it still
// withholds any significance claim — significance is never tested
// here (same rule as RobustSignificanceFloor).
func honestyNote(baseEval, candEval int, gap float64) string {
	min := baseEval
	if candEval < min {
		min = candEval
	}
	if min < RobustSignificanceFloor {
		return fmt.Sprintf("n=%d/%d below significance floor %d: no significance claimed; gap %.4f is descriptive only",
			baseEval, candEval, RobustSignificanceFloor, gap)
	}
	return fmt.Sprintf("n=%d/%d: gap %.4f descriptive; significance not tested, no significance claimed",
		baseEval, candEval, gap)
}

// sameModelSets reports whether both arms ran the same models.
func sameModelSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// explorationTrials returns the decision-driving subset: exploration
// split only. Holdout trials confirm; they never decide.
func explorationTrials(p ValidationPlan) []ValidationTrial {
	var out []ValidationTrial
	for _, t := range p.Trials {
		if t.Split == ValidationSplitExploration {
			out = append(out, t)
		}
	}
	return out
}

// holdoutTrials returns the confirmation-only subset.
func holdoutTrials(p ValidationPlan) []ValidationTrial {
	var out []ValidationTrial
	for _, t := range p.Trials {
		if t.Split == ValidationSplitHoldout {
			out = append(out, t)
		}
	}
	return out
}

// CompareValidation folds the exploration arms, derives the verdict
// with the declared MinDelta, and freezes it on the plan (COMPARED).
// Pure except for the freeze: the same trials always yield the same
// verdict. Holdout opens only after this freeze.
func CompareValidation(root, planID string) (ValidationComparison, error) {
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return ValidationComparison{}, err
	}
	if err := VerifyValidationPlan(p); err != nil {
		return ValidationComparison{}, err
	}
	if p.State != ValidationRunning && p.State != ValidationCompared {
		return ValidationComparison{}, fmt.Errorf("evolve: validation %s compares exploration trials while RUNNING, not %s", p.ID, p.State)
	}
	exp := explorationTrials(p)
	if len(exp) == 0 {
		return ValidationComparison{}, fmt.Errorf("evolve: validation %s has no exploration trials to compare", p.ID)
	}
	var baseT, candT []ValidationTrial
	for _, t := range exp {
		switch t.Arm {
		case ValidationArmBaseline:
			baseT = append(baseT, t)
		case ValidationArmCandidate:
			candT = append(candT, t)
		}
	}
	cmp := ValidationComparison{Baseline: armStatsOf(ValidationArmBaseline, baseT), Candidate: armStatsOf(ValidationArmCandidate, candT)}
	cmp.GapPassRate = cmp.Candidate.PassRate - cmp.Baseline.PassRate
	cmp.ModelsComparable = sameModelSets(cmp.Baseline.Models, cmp.Candidate.Models)
	cmp.Honesty = honestyNote(cmp.Baseline.Evaluable, cmp.Candidate.Evaluable, cmp.GapPassRate)
	switch {
	case cmp.Baseline.Evaluable == 0 || cmp.Candidate.Evaluable == 0:
		cmp.Verdict = RobustInsufficient
		cmp.Reason = "insufficient evaluable evidence: missing/infra outcomes are never scored as pass"
	case cmp.GapPassRate < 0:
		cmp.Verdict = RobustKeepBaseline
		cmp.Reason = "candidate regresses the primary metric: non-regression rule keeps the baseline"
	case cmp.GapPassRate == 0:
		cmp.Verdict = RobustTie
		cmp.Reason = "equal primary metric: baseline kept, cost/latency break ties only"
	case cmp.GapPassRate < p.MinDelta:
		cmp.Verdict = RobustKeepBaseline
		cmp.Reason = fmt.Sprintf("improvement %.4f below minimum delta %.4f: baseline kept", cmp.GapPassRate, p.MinDelta)
	default:
		cmp.Verdict = RobustAdoptCandidate
		cmp.Reason = fmt.Sprintf("improvement %.4f meets minimum delta %.4f; adoption still needs human approval, canary and rollback proof", cmp.GapPassRate, p.MinDelta)
	}
	if !cmp.ModelsComparable {
		cmp.Reason += "; models differ between arms (comparability caveat)"
	}
	p.ComparedAt = time.Now().UTC().Format(time.RFC3339)
	p.ComparedVerdict = cmp.Verdict
	p.ComparedReason = cmp.Reason
	p.ComparedTrials = len(exp)
	if p.State == ValidationRunning {
		p.History = append(p.History, ExpHistoryEvent{
			At: p.ComparedAt, Kind: "compared", From: p.State, To: ValidationCompared,
			By: p.Evaluator, Note: fmt.Sprintf("exploration verdict %s frozen over %d trials", cmp.Verdict, len(exp)),
		})
		p.State = ValidationCompared
	} else {
		p.History = append(p.History, ExpHistoryEvent{
			At: p.ComparedAt, Kind: "recompared", From: p.State, To: p.State,
			By: p.Evaluator, Note: fmt.Sprintf("exploration verdict refreshed: %s over %d trials", cmp.Verdict, len(exp)),
		})
	}
	if err := p.saveValidationPlan(root); err != nil {
		return ValidationComparison{}, err
	}
	return cmp, nil
}

// ValidationDecision is the sealed human decision: the exploration
// verdict it adopts, its owner and reason, plus the explicit call on
// inherited changes (adopt the legacy behavior, keep it, or revisit
// it in a new plan — never defaulted).
type ValidationDecision struct {
	Verdict         string `json:"verdict"`
	By              string `json:"by"`
	Reason          string `json:"reason"`
	At              string `json:"at"`
	Inherited       string `json:"inherited"`
	InheritedReason string `json:"inherited_reason"`
}

// ValidationEvidence pins the reproducible-report pointers: the
// commits the evidence ran on and the versioned manifests. Receipts
// and traces travel on the trials themselves.
type ValidationEvidence struct {
	Commits   []string `json:"commits"`
	Manifests []string `json:"manifests"`
}

// rejectionProof reports whether the plan carries a real rejection:
// a recorded discard, or a linked experiment whose live state is
// REJECTED or ROLLED_BACK. A later adoption cannot masquerade: the
// live state is loaded now, not trusted from link time.
func rejectionProof(root string, p ValidationPlan) (bool, []string) {
	var lines []string
	if len(p.Discards) > 0 {
		lines = append(lines, fmt.Sprintf("%d recorded discard(s)", len(p.Discards)))
	}
	live := 0
	for _, id := range p.LinkedExperiments {
		exp, err := LoadExperiment(root, id)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: missing (%v)", id, err))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", id, exp.State))
		if exp.State == StateRejected || exp.State == StateRolledBack {
			live++
		}
	}
	return len(p.Discards) > 0 || live > 0, lines
}

// FinalizeValidation seals the decision (FINAL). It enforces, in
// order: integrity, exploration verdict frozen and unchanged since,
// verdict equality (the holdout confirms, it never re-decides),
// human approval, explicit inherited-change call, evidence pins, a
// real rejection proof, holdout agreement on ADOPT, and the L3
// holdout promise.
func FinalizeValidation(root, planID string, d ValidationDecision, ev ValidationEvidence) error {
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return err
	}
	if err := VerifyValidationPlan(p); err != nil {
		return err
	}
	if p.State != ValidationCompared {
		return fmt.Errorf("evolve: validation %s finalizes from COMPARED, not %s", p.ID, p.State)
	}
	if len(explorationTrials(p)) != p.ComparedTrials {
		return errors.New("evolve: new exploration trials landed after the verdict: re-compare before finalizing")
	}
	if strings.TrimSpace(d.Verdict) != p.ComparedVerdict {
		return fmt.Errorf("evolve: finalize verdict %q must equal the frozen exploration verdict %q (holdout confirms, it never re-decides)",
			strings.TrimSpace(d.Verdict), p.ComparedVerdict)
	}
	if !validValidationApprover(d.By) {
		return errors.New("evolve: finalize needs a human approver (no self-approval, never auto/bot)")
	}
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("evolve: finalize needs a reason")
	}
	switch strings.TrimSpace(d.Inherited) {
	case ValidationInheritedAdopt, ValidationInheritedKeep, ValidationInheritedRevisit:
	default:
		return fmt.Errorf("evolve: inherited decision %q must be adopt, keep or revisit — explicit, never defaulted", strings.TrimSpace(d.Inherited))
	}
	if strings.TrimSpace(d.InheritedReason) == "" {
		return errors.New("evolve: inherited decision needs a reason")
	}
	commits, manifests := trimmedNonEmpty(ev.Commits), trimmedNonEmpty(ev.Manifests)
	if len(commits) == 0 {
		return errors.New("evolve: finalize needs at least one commit pin (reproducibility)")
	}
	if len(manifests) == 0 {
		return errors.New("evolve: finalize needs at least one evidence manifest pin")
	}
	ok, _ := rejectionProof(root, p)
	if !ok {
		return errors.New("evolve: finalize needs a real rejection — a recorded discard or a linked REJECTED/ROLLED_BACK experiment")
	}
	hold := holdoutTrials(p)
	if validationTierIncludesHoldout(p.Tier) && len(hold) == 0 {
		return errors.New("evolve: tier L3 promises full+holdout: no holdout trials recorded")
	}
	if p.ComparedVerdict == RobustAdoptCandidate && holdoutContradicts(hold) {
		return errors.New("evolve: holdout contradicts the exploration verdict: revisit in a new plan, do not adopt")
	}
	d.By = strings.TrimSpace(d.By)
	d.Reason = strings.TrimSpace(d.Reason)
	d.Inherited = strings.TrimSpace(d.Inherited)
	d.InheritedReason = strings.TrimSpace(d.InheritedReason)
	d.Verdict = strings.TrimSpace(d.Verdict)
	d.At = time.Now().UTC().Format(time.RFC3339)
	p.Decision = &d
	p.Evidence = &ValidationEvidence{Commits: commits, Manifests: manifests}
	p.History = append(p.History, ExpHistoryEvent{
		At: d.At, Kind: "finalized", From: p.State, To: ValidationFinal,
		By: d.By, Note: fmt.Sprintf("decision %s; inherited %s: %s", d.Verdict, d.Inherited, d.InheritedReason),
	})
	p.State = ValidationFinal
	return p.saveValidationPlan(root)
}

// holdoutContradicts reports whether holdout evidence evaluable on
// both arms shows the candidate worse than the baseline.
func holdoutContradicts(hold []ValidationTrial) bool {
	var baseT, candT []ValidationTrial
	for _, t := range hold {
		switch t.Arm {
		case ValidationArmBaseline:
			baseT = append(baseT, t)
		case ValidationArmCandidate:
			candT = append(candT, t)
		}
	}
	base, cand := armStatsOf(ValidationArmBaseline, baseT), armStatsOf(ValidationArmCandidate, candT)
	if base.Evaluable == 0 || cand.Evaluable == 0 {
		return false
	}
	return cand.PassRate < base.PassRate
}

// trimmedNonEmpty returns the trimmed non-empty entries in order.
func trimmedNonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// RenderValidationReport prints the reproducible F11.6 report: the
// frozen declaration (manifest pin, declaration hash, commands), the
// per-arm evidence with quality/latency/tokens separated (each with N
// and variability), controls, discards and linked rejections, the
// independent evaluator and human approval, the explicit
// inherited-change decision, and the verdict with its next step.
// Only FINAL plans render: the report seals a decision, never a draft.
func RenderValidationReport(root, planID string) (string, error) {
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return "", err
	}
	if err := VerifyValidationPlan(p); err != nil {
		return "", err
	}
	if p.State != ValidationFinal || p.Decision == nil || p.Evidence == nil {
		return "", fmt.Errorf("evolve: validation %s renders its report from FINAL, not %s", p.ID, p.State)
	}
	var b strings.Builder
	renderValidationHead(&b, p)
	renderValidationArms(&b, p)
	renderValidationGovernance(&b, root, p)
	renderValidationEvidence(&b, p)
	fmt.Fprintf(&b, "\nverdict: %s (%s)\n", p.Decision.Verdict, p.ComparedReason)
	fmt.Fprintf(&b, "honesty: %s\n", honestyNote(explorationEvaluable(p, ValidationArmBaseline), explorationEvaluable(p, ValidationArmCandidate), validationGap(p)))
	fmt.Fprintf(&b, "next: regulus evolve adopt --exp <id> --approved-by <human> (canary path), or regulus evolve rollback --exp <id> --reason <text> on drift\n")
	return b.String(), nil
}

// explorationEvaluable counts evaluable exploration trials of one arm.
func explorationEvaluable(p ValidationPlan, arm string) int {
	n := 0
	for _, t := range p.Trials {
		if t.Split == ValidationSplitExploration && t.Arm == arm && (t.Status == ManifestPass || t.Status == ManifestFail) {
			n++
		}
	}
	return n
}

// validationGap recomputes the descriptive exploration gap for the
// honesty line (same fold as CompareValidation, read-only).
func validationGap(p ValidationPlan) float64 {
	var baseT, candT []ValidationTrial
	for _, t := range explorationTrials(p) {
		if t.Arm == ValidationArmBaseline {
			baseT = append(baseT, t)
		} else {
			candT = append(candT, t)
		}
	}
	return armStatsOf(ValidationArmCandidate, candT).PassRate - armStatsOf(ValidationArmBaseline, baseT).PassRate
}

// renderValidationHead prints the frozen declaration: identity, pin,
// hypothesis, surface, matrix, evaluator and commands.
func renderValidationHead(b *strings.Builder, p ValidationPlan) {
	fmt.Fprintf(b, "# Evolution validation report %s\n\n", p.ID)
	fmt.Fprintf(b, "state: %s (declared %s, compared %s, finalized %s)\n", p.State, p.DeclaredAt, p.ComparedAt, p.Decision.At)
	fmt.Fprintf(b, "declaration sha256: %s\n", p.PlanSHA256)
	fmt.Fprintf(b, "evaluator: %s (%s) — independent of implementer %s\n", p.Evaluator, p.EvaluatorVersion, p.Implementer)
	fmt.Fprintf(b, "\n## Hypothesis (pre-registered)\n\n")
	fmt.Fprintf(b, "- problem: %s\n- root cause: %s\n- change: %s\n", p.Hypothesis.Problem, p.Hypothesis.RootCause, p.Hypothesis.Change)
	fmt.Fprintf(b, "- benefit: %s\n- regressions: %s\n- criteria: %s\n", p.Hypothesis.Benefit, p.Hypothesis.PotentialRegressions, p.Hypothesis.EvaluationCriteria)
	fmt.Fprintf(b, "\n## Surface (reversible)\n\n")
	fmt.Fprintf(b, "- kind: %s, reversible: %v, critical: %v\n- description: %s\n- revert: %s\n",
		p.Surface.Kind, p.Surface.Reversible, p.Surface.Critical, p.Surface.Description, p.Surface.RevertCommand)
	fmt.Fprintf(b, "\n## Matrix\n\n")
	fmt.Fprintf(b, "- baseline: %s | candidate: %s | dataset: %s | tier: %s\n", p.Baseline, p.Candidate, p.Dataset, p.Tier)
	fmt.Fprintf(b, "- models: %s | seeds: %v | repetitions: %d\n", strings.Join(p.Models, ", "), p.Seeds, p.Repetitions)
	fmt.Fprintf(b, "- primary: %s (MinDelta %.4f)\n", p.Primary, p.MinDelta)
	fmt.Fprintf(b, "- budget: runs<=%d wall<=%dms tokens<=%d\n", p.Budget.MaxRuns, p.Budget.MaxWallTimeMS, p.Budget.MaxTokens)
	fmt.Fprintf(b, "\n## Commands (exact)\n\n")
	for _, c := range p.Commands {
		fmt.Fprintf(b, "- %s\n", c)
	}
}

// renderValidationArms prints the three separated dimensions per arm
// plus the descriptive gap and the budget adherence.
func renderValidationArms(b *strings.Builder, p ValidationPlan) {
	var baseT, candT, holdT []ValidationTrial
	for _, t := range p.Trials {
		switch {
		case t.Split == ValidationSplitHoldout:
			holdT = append(holdT, t)
		case t.Arm == ValidationArmBaseline:
			baseT = append(baseT, t)
		default:
			candT = append(candT, t)
		}
	}
	base, cand := armStatsOf(ValidationArmBaseline, baseT), armStatsOf(ValidationArmCandidate, candT)
	fmt.Fprintf(b, "\n## Exploration evidence (decision basis)\n\n")
	for _, s := range []ValidationArmStats{base, cand} {
		fmt.Fprintf(b, "### %s: quality pass-rate %.4f (pass=%d fail=%d evaluable=%d infra=%d total=%d)\n",
			s.Arm, s.PassRate, s.Pass, s.Fail, s.Evaluable, s.Infra, s.Total)
		fmt.Fprintf(b, "- latency: mean %.1fms min %dms max %dms (N=%d evaluable attempts)\n", s.LatencyMeanMS, s.LatencyMinMS, s.LatencyMaxMS, s.LatencyN)
		fmt.Fprintf(b, "- tokens: measured total %d (unknown trials %d, never summed; sources %s) (models %s)\n",
			s.TokensMeasured, s.UnknownTrials, formatSources(s.Sources), strings.Join(s.Models, ", "))
	}
	fmt.Fprintf(b, "- gap (candidate - baseline): %.4f descriptive; models comparable: %v\n", cand.PassRate-base.PassRate, sameModelSets(base.Models, cand.Models))
	var latSum, tokSum int64
	for _, t := range p.Trials {
		latSum += t.LatencyMS
		if t.TokenSource != trace.SourceUnknown {
			tokSum += t.Tokens
		}
	}
	fmt.Fprintf(b, "- budget: trials %d/%d, wall %dms/%dms, measured tokens %d/%d\n", len(p.Trials), p.Budget.MaxRuns, latSum, p.Budget.MaxWallTimeMS, tokSum, p.Budget.MaxTokens)
	fmt.Fprintf(b, "\n## Holdout confirmation (confirmation-only)\n\n")
	if len(holdT) == 0 {
		fmt.Fprintf(b, "- holdout: not run (sealed; tier %s)\n", p.Tier)
		return
	}
	var hbase, hcand []ValidationTrial
	for _, t := range holdT {
		if t.Arm == ValidationArmBaseline {
			hbase = append(hbase, t)
		} else {
			hcand = append(hcand, t)
		}
	}
	hs, hc := armStatsOf(ValidationArmBaseline, hbase), armStatsOf(ValidationArmCandidate, hcand)
	fmt.Fprintf(b, "- holdout baseline: pass-rate %.4f (evaluable %d) | candidate: %.4f (evaluable %d)\n", hs.PassRate, hs.Evaluable, hc.PassRate, hc.Evaluable)
	fmt.Fprintf(b, "- audits: %s\n", strings.Join(p.HoldoutAudits, "; "))
	if holdoutContradicts(holdT) {
		fmt.Fprintf(b, "- agreement: HOLDOUT CONTRADICTS exploration (adoption refused)\n")
	} else {
		fmt.Fprintf(b, "- agreement: holdout does not contradict the exploration verdict\n")
	}
}

// formatSources renders the per-source trial counts deterministically.
func formatSources(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

// renderValidationGovernance prints controls, discards, linked
// rejections, approval and the inherited-change decision.
func renderValidationGovernance(b *strings.Builder, root string, p ValidationPlan) {
	fmt.Fprintf(b, "\n## Controls (+/-)\n\n")
	if len(p.Controls) == 0 {
		fmt.Fprintf(b, "- none registered\n")
	}
	for _, c := range p.Controls {
		fmt.Fprintf(b, "- [%s] %s: expected %s, observed %s (held=%v) %s\n", c.Kind, c.Name, c.Expectation, c.Outcome, c.Held, c.Detail)
	}
	fmt.Fprintf(b, "\n## Discards and linked rejections\n\n")
	if len(p.Discards) == 0 {
		fmt.Fprintf(b, "- discards: none\n")
	}
	for _, d := range p.Discards {
		fmt.Fprintf(b, "- discarded %s by %s at %s: %s\n", d.CandidateID, d.By, d.At, d.Reason)
	}
	_, proof := rejectionProof(root, p)
	for _, line := range proof {
		fmt.Fprintf(b, "- linked: %s\n", line)
	}
	fmt.Fprintf(b, "\n## Approval and inherited changes\n\n")
	fmt.Fprintf(b, "- approved by %s at %s: %s\n", p.Decision.By, p.Decision.At, p.Decision.Reason)
	fmt.Fprintf(b, "- inherited changes: %s — %s\n", p.Decision.Inherited, p.Decision.InheritedReason)
}

// renderValidationEvidence prints commits, manifests and the
// receipt/trace pointers cited by every trial.
func renderValidationEvidence(b *strings.Builder, p ValidationPlan) {
	fmt.Fprintf(b, "\n## Reproducibility\n\n")
	fmt.Fprintf(b, "- commits: %s\n", strings.Join(p.Evidence.Commits, ", "))
	fmt.Fprintf(b, "- manifests: %s\n", strings.Join(p.Evidence.Manifests, ", "))
	receipts, traces := map[string]bool{}, map[string]bool{}
	for _, t := range p.Trials {
		if strings.TrimSpace(t.ReceiptID) != "" {
			receipts[t.ReceiptID] = true
		}
		if strings.TrimSpace(t.TraceID) != "" {
			traces[t.TraceID] = true
		}
	}
	fmt.Fprintf(b, "- receipts: %s\n", strings.Join(sortedKeys(receipts), ", "))
	fmt.Fprintf(b, "- traces: %s\n", strings.Join(sortedKeys(traces), ", "))
}

// sortedKeys returns the map keys in order for deterministic output.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
