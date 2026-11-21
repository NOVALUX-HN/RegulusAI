package evolve

// F11.6 (NOV-92) validation protocol: pre-registered hypothesis,
// reversible surface, baseline, metrics and budget BEFORE any trial;
// seeds/repetitions/comparable models; governed holdout; discard
// registry with positive/negative controls; independent evaluator +
// human approval; canary/rollback proof; reproducible report with
// quality/latency/tokens separated (N + variability).
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/trace"
)

func valHypothesis() Hypothesis {
	return Hypothesis{
		Problem: "latency spikes on retry storms", RootCause: "unbounded retries",
		Change: "cap retries at 2 with backoff", Benefit: "lower p90 without pass-rate loss",
		PotentialRegressions: "flaky tasks may fail instead of retry-passing",
		EvaluationCriteria:   "pass-rate delta >= 0 and p90 down on exploration",
	}
}

func valSpec() ValidationPlanSpec {
	return ValidationPlanSpec{
		Hypothesis:       valHypothesis(),
		Surface:          ValidationSurface{Kind: "workflow", Description: "retry policy only", Reversible: true, RevertCommand: "regulus evolve rollback --exp <id> --reason <t>"},
		Baseline:         "retry-baseline-v1",
		Candidate:        "retry-cap-2",
		Dataset:          "agent-tasks-v3",
		Tier:             "L2",
		Models:           []string{"model-a", "model-b"},
		Seeds:            []int64{20261116, 20261117},
		Repetitions:      2,
		Primary:          "pass-rate",
		MinDelta:         0.0,
		Budget:           ValidationBudget{MaxRuns: 16, MaxWallTimeMS: 60000, MaxTokens: 100000},
		Implementer:      "sextus",
		Evaluator:        "andragon",
		EvaluatorVersion: "robust-compare-v1",
		Commands:         []string{"bin/regulus bench agent-tasks --reps 2 --split exploration"},
		Notes:            "pre-registered before any trial",
	}
}

func declareVal(t *testing.T, root string, mut func(*ValidationPlanSpec)) ValidationPlan {
	t.Helper()
	spec := valSpec()
	if mut != nil {
		mut(&spec)
	}
	p, err := DeclareValidationPlan(root, spec)
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	return p
}

func valTrial(arm, task string, seed int64, rep int, model, status string) ValidationTrial {
	return ValidationTrial{
		Arm: arm, TaskID: task, Seed: seed, Repetition: rep, Model: model,
		Split: ValidationSplitExploration, Status: status,
		LatencyMS: 120, Tokens: 40, TokenSource: trace.SourceMeasuredHost,
		ReceiptID: "rc_" + task, TraceID: "run_" + task,
	}
}

func TestValidationDeclareRequiresFullHypothesis(t *testing.T) {
	dir := t.TempDir()
	spec := valSpec()
	spec.Hypothesis.Benefit = ""
	if _, err := DeclareValidationPlan(dir, spec); err == nil {
		t.Fatal("empty benefit must refuse the declaration")
	} else if !strings.Contains(err.Error(), "hypothesis") {
		t.Fatalf("error must name the hypothesis, got %v", err)
	}
}

func TestValidationDeclareRefusesSelfEvaluation(t *testing.T) {
	dir := t.TempDir()
	spec := valSpec()
	spec.Evaluator = "  SEXTUS "
	if _, err := DeclareValidationPlan(dir, spec); err == nil {
		t.Fatal("evaluator == implementer must refuse (independent evaluator required)")
	} else if !strings.Contains(err.Error(), "independent") {
		t.Fatalf("error must name independence, got %v", err)
	}
	if _, err := DeclareValidationPlan(dir, func() ValidationPlanSpec { s := valSpec(); s.Evaluator = ""; return s }()); err == nil {
		t.Fatal("empty evaluator must refuse")
	}
}

func TestValidationDeclareRefusesIrreversibleSurface(t *testing.T) {
	dir := t.TempDir()
	for _, mut := range []func(*ValidationPlanSpec){
		func(s *ValidationPlanSpec) { s.Surface.Reversible = false },
		func(s *ValidationPlanSpec) { s.Surface.Kind = "" },
		func(s *ValidationPlanSpec) { s.Surface.RevertCommand = "" },
	} {
		spec := valSpec()
		mut(&spec)
		if _, err := DeclareValidationPlan(dir, spec); err == nil {
			t.Fatal("irreversible or undescribed surface must refuse")
		}
	}
}

func TestValidationDeclareEnforcesBudgetCoversMatrix(t *testing.T) {
	dir := t.TempDir()
	spec := valSpec() // 2 seeds x 2 reps x 2 models = 8 per arm
	spec.Budget.MaxRuns = 7
	if _, err := DeclareValidationPlan(dir, spec); err == nil {
		t.Fatal("budget below the declared seed/rep/model matrix must refuse")
	} else if !strings.Contains(err.Error(), "budget") {
		t.Fatalf("error must name the budget, got %v", err)
	}
	spec.Budget.MaxRuns = 8
	if _, err := DeclareValidationPlan(dir, spec); err != nil {
		t.Fatalf("budget exactly covering the matrix must hold: %v", err)
	}
}

func TestValidationDeclareRejectsBadMatrix(t *testing.T) {
	dir := t.TempDir()
	cases := []func(*ValidationPlanSpec){
		func(s *ValidationPlanSpec) { s.Seeds = nil },
		func(s *ValidationPlanSpec) { s.Seeds = []int64{0} },
		func(s *ValidationPlanSpec) { s.Models = nil },
		func(s *ValidationPlanSpec) { s.Repetitions = 0 },
		func(s *ValidationPlanSpec) { s.Tier = "L9" },
		func(s *ValidationPlanSpec) { s.MinDelta = -0.1 },
		func(s *ValidationPlanSpec) { s.Commands = nil },
		func(s *ValidationPlanSpec) { s.Budget.MaxWallTimeMS = 0 },
	}
	for i, mut := range cases {
		spec := valSpec()
		mut(&spec)
		if _, err := DeclareValidationPlan(dir, spec); err == nil {
			t.Fatalf("case %d must refuse", i)
		}
	}
}

func TestValidationDeclareStoresFrozenPlan(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	if p.State != ValidationDeclared {
		t.Fatalf("state = %q, want DECLARED", p.State)
	}
	if p.PlanSHA256 == "" || p.DeclaredAt == "" {
		t.Fatal("frozen plan needs a sha and a declaration timestamp")
	}
	loaded, err := LoadValidationPlan(dir, p.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := VerifyValidationPlan(loaded); err != nil {
		t.Fatalf("fresh plan must verify: %v", err)
	}
	// Tamper with the frozen content: verification must fail, never
	// silently compare against moved content.
	raw, _ := os.ReadFile(filepath.Join(ValidationDir(dir), p.ID+".json"))
	tampered := strings.Replace(string(raw), "retry-cap-2", "retry-cap-9", 1)
	if err := os.WriteFile(filepath.Join(ValidationDir(dir), p.ID+".json"), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if bad, err := LoadValidationPlan(dir, p.ID); err != nil {
		t.Fatalf("tampered file still loads (integrity fails at verify): %v", err)
	} else if err := VerifyValidationPlan(bad); err == nil {
		t.Fatal("tampered plan must fail verification")
	}
}

func TestValidationTrialFlowEnforcesMatrix(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	if _, err := RecordValidationTrial(dir, p.ID, valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass)); err != nil {
		t.Fatalf("first trial must record: %v", err)
	}
	loaded, _ := LoadValidationPlan(dir, p.ID)
	if loaded.State != ValidationRunning {
		t.Fatalf("first trial must flip DECLARED -> RUNNING, got %q", loaded.State)
	}
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT2", 999, 0, "model-a", ManifestPass),        // unknown seed
		valTrial(ValidationArmBaseline, "AT2", 20261116, 0, "model-z", ManifestPass),   // unknown model
		valTrial(ValidationArmBaseline, "AT2", 20261116, 9, "model-a", ManifestPass),   // rep out of range
		valTrial(ValidationArmBaseline, "AT2", 20261116, 0, "model-a", "bogus-status"), // bad status
		valTrial("sideways", "AT2", 20261116, 0, "model-a", ManifestPass),              // bad arm
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err == nil {
			t.Fatalf("out-of-matrix trial %+v must refuse", tr)
		}
	}
	// Budget exhaustion refuses instead of silently dropping trials.
	small := declareVal(t, dir, func(s *ValidationPlanSpec) {
		s.Seeds = []int64{7}
		s.Models = []string{"m"}
		s.Repetitions = 1
		s.Budget = ValidationBudget{MaxRuns: 1, MaxWallTimeMS: 10, MaxTokens: 0}
	})
	tr := valTrial(ValidationArmBaseline, "AT1", 7, 0, "m", ManifestPass)
	if _, err := RecordValidationTrial(dir, small.ID, tr); err != nil {
		t.Fatalf("trial within budget: %v", err)
	}
	if _, err := RecordValidationTrial(dir, small.ID, tr); err == nil {
		t.Fatal("trial past MaxRuns must refuse")
	}
}

func TestValidationHoldoutSealedUntilCompared(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, func(s *ValidationPlanSpec) { s.Tier = "L3" })
	hold := valTrial(ValidationArmCandidate, "AT5", 20261116, 0, "model-a", ManifestPass)
	hold.Split = ValidationSplitHoldout
	if _, err := RecordValidationTrial(dir, p.ID, hold); err == nil {
		t.Fatal("holdout via the exploration path must refuse while sealed")
	} else if !strings.Contains(err.Error(), "sealed") {
		t.Fatalf("error must name the seal, got %v", err)
	}
	if _, err := RecordHoldoutTrial(dir, p.ID, "", hold); err == nil {
		t.Fatal("holdout without an audit reference must refuse")
	}
	// Exploration evidence first, then the governed holdout path.
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass),
		valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass),
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("exploration trial: %v", err)
		}
	}
	if _, err := CompareValidation(dir, p.ID); err != nil {
		t.Fatalf("compare: %v", err)
	}
	if _, err := RecordHoldoutTrial(dir, p.ID, "audit: human reviewed exploration verdict", hold); err != nil {
		t.Fatalf("audited holdout after comparison must record: %v", err)
	}
}

func TestValidationControlsAndDiscards(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	if err := RecordValidationControl(dir, p.ID, ValidationControl{Kind: "sideways", Name: "x", Expectation: "e", Outcome: "o"}); err == nil {
		t.Fatal("control kind must be positive|negative")
	}
	if err := RecordValidationControl(dir, p.ID, ValidationControl{Kind: "positive", Name: "aa-baseline", Expectation: RobustTie, Outcome: RobustTie, Held: true, Detail: "A/A calibrates noise"}); err != nil {
		t.Fatalf("positive control: %v", err)
	}
	if err := RecordValidationControl(dir, p.ID, ValidationControl{Kind: "negative", Name: "sabotage", Expectation: RobustKeepBaseline, Outcome: RobustKeepBaseline, Held: true, Detail: "broken candidate rejected"}); err != nil {
		t.Fatalf("negative control: %v", err)
	}
	if err := DiscardValidationCandidate(dir, p.ID, ValidationDiscard{CandidateID: "c1", Reason: "", By: "andragon"}); err == nil {
		t.Fatal("discard without reason must refuse")
	}
	if err := DiscardValidationCandidate(dir, p.ID, ValidationDiscard{CandidateID: "c1", Reason: "regresses pass-rate", By: "andragon"}); err != nil {
		t.Fatalf("discard: %v", err)
	}
}

func TestValidationCompareSeparatesDimensions(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	trials := []ValidationTrial{
		{Arm: ValidationArmBaseline, TaskID: "AT1", Seed: 20261116, Repetition: 0, Model: "model-a", Split: ValidationSplitExploration, Status: ManifestPass, LatencyMS: 100, Tokens: 50, TokenSource: trace.SourceMeasuredHost},
		{Arm: ValidationArmBaseline, TaskID: "AT2", Seed: 20261116, Repetition: 0, Model: "model-a", Split: ValidationSplitExploration, Status: ManifestFail, LatencyMS: 300, Tokens: 70, TokenSource: trace.SourceMeasuredHost},
		{Arm: ValidationArmBaseline, TaskID: "AT3", Seed: 20261116, Repetition: 0, Model: "model-a", Split: ValidationSplitExploration, Status: ManifestInfra, LatencyMS: 9000, Tokens: 0, TokenSource: trace.SourceUnknown},
		{Arm: ValidationArmCandidate, TaskID: "AT1", Seed: 20261116, Repetition: 0, Model: "model-a", Split: ValidationSplitExploration, Status: ManifestPass, LatencyMS: 80, Tokens: 30, TokenSource: trace.SourceMeasuredHost},
		{Arm: ValidationArmCandidate, TaskID: "AT2", Seed: 20261116, Repetition: 0, Model: "model-a", Split: ValidationSplitExploration, Status: ManifestPass, LatencyMS: 90, Tokens: 0, TokenSource: trace.SourceUnknown},
	}
	for _, tr := range trials {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if cmp.Baseline.Evaluable != 2 || cmp.Baseline.PassRate != 0.5 {
		t.Fatalf("baseline quality wrong: %+v", cmp.Baseline)
	}
	if cmp.Baseline.Infra != 1 {
		t.Fatalf("infra must stay out of the pass-rate: %+v", cmp.Baseline)
	}
	if cmp.Baseline.LatencyMeanMS != 200 || cmp.Baseline.LatencyMinMS != 100 || cmp.Baseline.LatencyMaxMS != 300 {
		t.Fatalf("baseline latency wrong: %+v", cmp.Baseline)
	}
	if cmp.Candidate.TokensMeasured != 30 || cmp.Candidate.UnknownTrials != 1 {
		t.Fatalf("unknown tokens must be counted, never summed: %+v", cmp.Candidate)
	}
	if cmp.Baseline.TokensMeasured != 120 {
		t.Fatalf("measured tokens must sum: %+v", cmp.Baseline)
	}
	if cmp.Verdict != RobustAdoptCandidate {
		t.Fatalf("verdict = %q, want ADOPT_CANDIDATE (gap 0.5 meets MinDelta 0)", cmp.Verdict)
	}
	if !strings.Contains(cmp.Honesty, "no significance") {
		t.Fatalf("comparison must disclaim significance, got %q", cmp.Honesty)
	}
}

func TestValidationCompareInsufficientWithoutEvaluable(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	tr := valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestInfra)
	if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
		t.Fatalf("record: %v", err)
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare with no evaluable must still report: %v", err)
	}
	if cmp.Verdict != RobustInsufficient {
		t.Fatalf("verdict = %q, want INSUFFICIENT_EVIDENCE", cmp.Verdict)
	}
}

func TestValidationFinalizeRequiresHumanAndInherited(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass),
		valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass),
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if err := DiscardValidationCandidate(dir, p.ID, ValidationDiscard{CandidateID: "retry-cap-3", Reason: "infra noise, not signal", By: "andragon"}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	ev := ValidationEvidence{Commits: []string{"abc1234"}, Manifests: []string{"bench/evidence/v/manifest.json"}}
	bad := []ValidationDecision{
		{Verdict: cmp.Verdict, By: "bot", Reason: "auto", Inherited: "keep", InheritedReason: "r"},
		{Verdict: cmp.Verdict, By: "andragon", Reason: "", Inherited: "keep", InheritedReason: "r"},
		{Verdict: cmp.Verdict, By: "andragon", Reason: "ok", Inherited: "", InheritedReason: ""},
		{Verdict: cmp.Verdict, By: "andragon", Reason: "ok", Inherited: "keep", InheritedReason: ""},
		{Verdict: RobustAdoptCandidate, By: "andragon", Reason: "ok", Inherited: "keep", InheritedReason: "r"},
	}
	for i, d := range bad {
		if err := FinalizeValidation(dir, p.ID, d, ev); err == nil {
			t.Fatalf("decision %d must refuse (bot/empty/mismatch/undecided)", i)
		}
	}
	if err := FinalizeValidation(dir, p.ID, ValidationDecision{Verdict: cmp.Verdict, By: "andragon", Reason: "tie keeps baseline; no regression adopted", Inherited: "keep", InheritedReason: "NOV-71 trims stay; no inherited change touched"}, ev); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	loaded, _ := LoadValidationPlan(dir, p.ID)
	if loaded.State != ValidationFinal {
		t.Fatalf("state = %q, want FINAL", loaded.State)
	}
	rep, err := RenderValidationReport(dir, p.ID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, want := range []string{
		"pass-rate", "latency", "tokens", "evaluable",
		"retry-cap-3", "andragon", "keep",
		"abc1234", "manifest.json", cmp.Verdict,
		"no significance",
	} {
		if !strings.Contains(rep, want) {
			t.Fatalf("report must contain %q\n---\n%s", want, rep)
		}
	}
}

func TestValidationFinalizeNeedsRejectionProof(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass),
		valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass),
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	ev := ValidationEvidence{Commits: []string{"abc1234"}, Manifests: []string{"m.json"}}
	d := ValidationDecision{Verdict: cmp.Verdict, By: "andragon", Reason: "ok", Inherited: "keep", InheritedReason: "none touched"}
	if err := FinalizeValidation(dir, p.ID, d, ev); err == nil {
		t.Fatal("finalize without any discard or linked rejection must refuse: the report needs a real rejection")
	}
}

func TestValidationLinkedRejectionSatisfiesProof(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, nil)
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass),
		valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass),
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	// A stored experiment that really rejected satisfies the proof.
	exp, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: valHypothesis(), Baseline: "b", Candidate: "c",
		Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if err := exp.Advance(dir, StateRunning); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := exp.SetOutcome("trials=1", "regresses", "reject: pass-rate dropped"); err != nil {
		t.Fatalf("outcome: %v", err)
	}
	if err := exp.Advance(dir, StateEvaluated); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := exp.Advance(dir, StateRejected); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := LinkValidationExperiment(dir, p.ID, exp.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	ev := ValidationEvidence{Commits: []string{"abc1234"}, Manifests: []string{"m.json"}}
	d := ValidationDecision{Verdict: cmp.Verdict, By: "andragon", Reason: "ok", Inherited: "keep", InheritedReason: "none touched"}
	if err := FinalizeValidation(dir, p.ID, d, ev); err != nil {
		t.Fatalf("linked REJECTED experiment must satisfy the proof: %v", err)
	}
	rep, err := RenderValidationReport(dir, p.ID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(rep, exp.ID) || !strings.Contains(rep, StateRejected) {
		t.Fatalf("report must cite the linked rejection, got:\n%s", rep)
	}
}

func TestValidationL3RequiresHoldout(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, func(s *ValidationPlanSpec) { s.Tier = "L3" })
	for _, tr := range []ValidationTrial{
		valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass),
		valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass),
	} {
		if _, err := RecordValidationTrial(dir, p.ID, tr); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	cmp, err := CompareValidation(dir, p.ID)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if err := DiscardValidationCandidate(dir, p.ID, ValidationDiscard{CandidateID: "c0", Reason: "probe", By: "andragon"}); err != nil {
		t.Fatalf("discard: %v", err)
	}
	ev := ValidationEvidence{Commits: []string{"abc1234"}, Manifests: []string{"m.json"}}
	d := ValidationDecision{Verdict: cmp.Verdict, By: "andragon", Reason: "ok", Inherited: "keep", InheritedReason: "none touched"}
	if err := FinalizeValidation(dir, p.ID, d, ev); err == nil {
		t.Fatal("L3 without holdout trials must refuse: the tier promises full+holdout")
	}
	hold := valTrial(ValidationArmCandidate, "AT1", 20261116, 0, "model-a", ManifestPass)
	hold.Split = ValidationSplitHoldout
	if _, err := RecordHoldoutTrial(dir, p.ID, "audit: L3 pre-release gate", hold); err != nil {
		t.Fatalf("holdout: %v", err)
	}
	holdB := valTrial(ValidationArmBaseline, "AT1", 20261116, 0, "model-a", ManifestPass)
	holdB.Split = ValidationSplitHoldout
	if _, err := RecordHoldoutTrial(dir, p.ID, "audit: L3 pre-release gate", holdB); err != nil {
		t.Fatalf("holdout: %v", err)
	}
	if err := FinalizeValidation(dir, p.ID, d, ev); err != nil {
		t.Fatalf("L3 with agreeing holdout must finalize: %v", err)
	}
}

func TestValidationRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	p := declareVal(t, dir, func(s *ValidationPlanSpec) {
		s.Notes = "token sk-live_abc123 must never persist"
	})
	loaded, err := LoadValidationPlan(dir, p.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if strings.Contains(loaded.Notes, "sk-live_abc123") {
		t.Fatalf("notes must be redacted before the store, got %q", loaded.Notes)
	}
}
