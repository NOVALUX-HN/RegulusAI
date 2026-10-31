package evolve

import (
	"strings"
	"testing"
)

// Every hypothesis explains all six points; empty fields are refused.
func TestHypothesisRequiresAllFields(t *testing.T) {
	full := Hypothesis{
		Problem: "gate typecheck fails twice on demo/T9", RootCause: "rule X misfires on __ slugs",
		Change: "anchor the matcher to slugs", Benefit: "fewer false failures",
		PotentialRegressions: "slower matching", EvaluationCriteria: "typecheck passes on T9, suite stays green",
	}
	if err := full.Validate(); err != nil {
		t.Fatalf("full hypothesis must validate: %v", err)
	}
	broken := full
	broken.RootCause = "  "
	if err := broken.Validate(); err == nil {
		t.Fatal("hypothesis without root cause must be refused")
	}
}

// A hypothesis built on isolated observations is refused: one failure is
// not a general rule.
func TestHypothesisRefusesIsolatedObservations(t *testing.T) {
	iso := Observation{Source: SourceFailedExecutions, Task: "demo/T10", Summary: "once", Count: 1}
	if _, err := Hypothesize([]Observation{iso}); err == nil {
		t.Fatal("Hypothesize must refuse isolated-only support")
	}
	rep := Observation{Source: SourceRepeatedFailures, Task: "demo/T9", Summary: "twice", Count: 2}
	h, err := Hypothesize([]Observation{rep}, WithProblem("p"), WithRootCause("r"), WithChange("c"), WithBenefit("b"), WithRegressions("rg"), WithCriteria("ev"))
	if err != nil {
		t.Fatalf("Hypothesize with repeated support must hold: %v", err)
	}
	if h.Problem != "p" || h.RootCause != "r" {
		t.Fatalf("Hypothesize must carry the given fields, got %+v", h)
	}
}

// Experiments walk PROPOSED -> RUNNING -> EVALUATED -> ADOPTED/REJECTED with
// validated transitions, exclusive ids and re-checkable integrity.
func TestExperimentLifecycle(t *testing.T) {
	dir := t.TempDir()
	exp, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "matcher anchored to slugs", Dataset: "demo/T9 receipts",
		Model: "n/a (deterministic)", Runtime: "go test ./internal/...", Domain: DomainINV,
		Parameters: map[string]string{"scope": "typecheck"}, Budget: Budget{MaxRuns: 3},
	})
	if err != nil {
		t.Fatalf("ProposeExperiment must hold: %v", err)
	}
	if !strings.HasPrefix(exp.ID, "exp_") || exp.State != StateProposed {
		t.Fatalf("new experiment must be exp_* PROPOSED, got %+v", exp)
	}
	if err := exp.Advance(dir, StateEvaluated); err == nil {
		t.Fatal("PROPOSED -> EVALUATED skip must be refused")
	}
	for _, to := range []string{StateRunning, StateEvaluated, StateAdopted} {
		if err := exp.Advance(dir, to); err != nil {
			t.Fatalf("advance to %s must hold: %v", to, err)
		}
	}
	if err := exp.Advance(dir, StateRejected); err == nil {
		t.Fatal("terminal ADOPTED must advance nowhere")
	}
	if warns := CheckExperimentIntegrity(exp); len(warns) != 0 {
		t.Fatalf("stored experiment must be integral, got %v", warns)
	}
	if _, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "other", Dataset: "d", Domain: DomainINV, ID: exp.ID,
	}); err == nil {
		t.Fatal("reused experiment id must be refused, never overwritten")
	}
}
