package evolve

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Rejection is a terminal verdict with its reason recorded: harmful
// candidates are discarded, the baseline kept.
func TestRejectionPath(t *testing.T) {
	dir := t.TempDir()
	fresh, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{StateRunning, StateEvaluated, StateRejected} {
		if err := fresh.Advance(dir, to); err != nil {
			t.Fatalf("advance to %s must hold: %v", to, err)
		}
	}
	if err := fresh.Advance(dir, StateAdopted); err == nil {
		t.Fatal("terminal REJECTED must advance nowhere")
	}
	if warns := CheckExperimentIntegrity(fresh); len(warns) != 0 {
		t.Fatalf("rejected experiment must stay integral, got %v", warns)
	}
}

// Same evidence in, same results out: observation and metrics are
// deterministic functions of their inputs.
func TestReproducibility(t *testing.T) {
	dir := t.TempDir()
	writeReceipt(t, dir, "demo/R", "typecheck", "fail", 30)
	writeReceipt(t, dir, "demo/R", "lint", "pass", 10)
	first, err := Observe(dir, "demo/R")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Observe(dir, "demo/R")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Observe must be reproducible:\n%+v\n%+v", first, second)
	}
	trials := []Trial{{Pass: true, Cost: 2}, {Pass: false, Regressed: true, Cost: 1}}
	if !reflect.DeepEqual(Summarize(trials), Summarize(trials)) {
		t.Fatal("Summarize must be reproducible")
	}
}

// Full acceptance run (S12): formulate, isolate, compare, detect, reject
// the harmful, adopt the verified, revert — guarantees intact throughout.
func TestEndToEndAcceptance(t *testing.T) {
	dir := t.TempDir()
	writeReceipt(t, dir, "demo/A", "typecheck", "fail", 40)
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "evidence", "demo__A", "typecheck-2.json"),
		[]byte(`{"id":"rec_a2","task":"demo/A","gate":"typecheck","layer":1,"command":"echo ok","exit_code":1,"verdict":"fail","started_at":"2026-10-26T08:00:00Z","duration_ms":45}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReceipt(t, dir, "demo/A", "lint", "pass", 10)

	// 1. formulate + 2. isolated execution scaffolding via supported evidence
	obs, err := Observe(dir, "demo/A")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckSupport(obs); err != nil {
		t.Fatalf("repeated failure must support a hypothesis: %v", err)
	}
	h, err := Hypothesize(obs, WithProblem("p"), WithRootCause("r"), WithChange("c"), WithBenefit("b"), WithRegressions("rg"), WithCriteria("ev"))
	if err != nil {
		t.Fatal(err)
	}
	good, err := ProposeExperiment(dir, ExperimentSpec{Hypothesis: h,
		Baseline: "v0.9.0", Candidate: "anchor matcher to slugs", Dataset: "demo/A receipts",
		Domain: DomainINV, Budget: Budget{MaxRuns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := ProposeExperiment(dir, ExperimentSpec{Hypothesis: h,
		Baseline: "v0.9.0", Candidate: "skip typecheck on __ slugs", Dataset: "demo/A receipts",
		Domain: DomainINV, Budget: Budget{MaxRuns: 1}})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []Experiment{good, bad} {
		if err := e.Advance(dir, StateRunning); err != nil {
			t.Fatal(err)
		}
	}
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "dataset green"}, nil
	}
	// 2. isolated runs (strategy runner, capped)
	out, err := RunExperiments(dir, StrategySequential, []Experiment{good, bad}, RunCap{MaxTotal: 4}, probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if !o.Pass {
			t.Fatalf("probe must pass, got %+v", o)
		}
	}
	// baseline pins the stable guarantee (8. guarantees intact)
	base := Metrics{Trials: 4, SuccessRate: 1, AcceptanceRate: 1, PolicyCompliance: 1, EvidenceCompleteness: 1}
	if err := SetBaseline(dir, "v0.9.0", base); err != nil {
		t.Fatal(err)
	}
	trust := TrustCheck{Isolated: true, Provenance: "demo/A receipts", Validations: true, Regressions: true, Policies: true, Reversible: true}
	finish := func(e Experiment, results, analysis, decision string) Experiment {
		stored, err := LoadExperiment(dir, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := stored.AttestTrust(dir, trust); err != nil {
			t.Fatal(err)
		}
		stored, err = LoadExperiment(dir, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := stored.SetOutcome(results, analysis, decision); err != nil {
			t.Fatal(err)
		}
		if err := stored.Advance(dir, StateEvaluated); err != nil {
			t.Fatal(err)
		}
		return stored
	}
	goodEv := finish(good, "4/4 green", "no regressions", "adopt")
	badEv := finish(bad, "typecheck skipped: 4/4 pass, lint regressed", "lint regression", "reject")
	// 3-4. compare + detect: verified adopts, harmful is refused
	verified := []Trial{
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true},
	}
	v, err := Adopt(dir, goodEv.ID, Summarize(verified), "snap-good", "ana: verified")
	if err != nil {
		t.Fatalf("5. verified adoption must hold: %v", err)
	}
	regressing := base
	regressing.RegressionRate = 0.5
	if _, err := Adopt(dir, badEv.ID, regressing, "snap-bad", "ana: verified"); !errors.Is(err, ErrMandatoryRegression) {
		t.Fatalf("5. harmful improvement must be rejected, got %v", err)
	}
	// 6-7. revert the verified one; 8. history stays integral
	if _, err := Rollback(dir, v.N, "post-adopt lint drift"); err != nil {
		t.Fatalf("7. rollback must hold: %v", err)
	}
	for _, id := range []string{goodEv.ID, badEv.ID} {
		stored, err := LoadExperiment(dir, id)
		if err != nil {
			t.Fatal(err)
		}
		if warns := CheckExperimentIntegrity(stored); len(warns) != 0 {
			t.Fatalf("8. history must stay integral for %s: %v", id, warns)
		}
	}
	if vers, err := ListVersions(dir); err != nil || len(vers) != 1 || !vers[0].Superseded {
		t.Fatalf("audit trail must show the superseded version, got %+v %v", vers, err)
	}
}
