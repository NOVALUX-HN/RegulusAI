package evolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S6/C7: RunExperiments SPLIT leve + COVER of uncovered branches.
func TestRunExperimentsNilProbe(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1)}
	if _, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, nil); err == nil {
		t.Fatal("nil probe must be refused")
	} else if !strings.Contains(err.Error(), "needs a probe function") {
		t.Fatalf("refusal must name the probe, got %v", err)
	}
}

func TestCheckStrategyArgsDirect(t *testing.T) {
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true}, nil
	}
	for _, s := range []Strategy{StrategySequential, StrategyParallel, StrategyHybrid} {
		if err := checkStrategyArgs(s, probe); err != nil {
			t.Fatalf("checkStrategyArgs(%s) must hold: %v", s, err)
		}
	}
	if err := checkStrategyArgs(Strategy("NOPE"), probe); err == nil {
		t.Fatal("unknown strategy must be refused")
	}
	if err := checkStrategyArgs(StrategySequential, nil); err == nil {
		t.Fatal("nil probe must be refused")
	}
}

func TestRunExperimentsUnknownExperiment(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1)}
	exps[0].ID = "exp_missing"
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe)
	if err == nil {
		t.Fatalf("missing experiment must error, got %+v", out)
	}
	if len(out) != 1 || out[0].ID != "exp_missing" {
		t.Fatalf("outcome must cite the missing id, got %+v", out)
	}
}

func TestRunExperimentsWrongState(t *testing.T) {
	dir := t.TempDir()
	exp, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Stays PROPOSED: strategy runs only RUNNING.
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategySequential, []Experiment{exp}, RunCap{}, probe)
	if err == nil {
		t.Fatalf("non-RUNNING experiment must error, got %+v", out)
	}
	if !strings.Contains(err.Error(), "is PROPOSED") {
		t.Fatalf("error must name the state, got %v", err)
	}
	if len(out) != 1 || out[0].ID != exp.ID {
		t.Fatalf("outcome must cite the experiment, got %+v", out)
	}
}

func TestRunExperimentsProbeErrorRecorded(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 2)}
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{}, errors.New("boom")
	}
	out, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 outcome, got %+v", out)
	}
	if out[0].Pass {
		t.Fatalf("erroring probe must not pass, got %+v", out[0])
	}
	if out[0].Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2 (all retries fire)", out[0].Attempts)
	}
	if !strings.Contains(out[0].Detail, "probe error: boom") {
		t.Fatalf("detail must carry the probe error, got %q", out[0].Detail)
	}
}

func TestRunExperimentsCorruptBudget(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1)}
	// Malformed ledger: TodayEvolutionCount fails before any probe fires.
	if err := os.MkdirAll(filepath.Join(dir, ".regulus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".regulus", "budget.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	fired := 0
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		fired++
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	if _, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe); err == nil {
		t.Fatal("corrupt ledger must error")
	}
	if fired != 0 {
		t.Fatalf("corrupt ledger must fire zero probes, got %d", fired)
	}
}

func TestRunExperimentsRecordFailure(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1)}
	// Block the atomic rewrite: budget.json.tmp as a directory makes
	// Record's WriteFile fail.
	if err := os.MkdirAll(filepath.Join(dir, ".regulus", "budget.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	if _, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe); err == nil {
		t.Fatal("ledger write failure must error")
	}
}

func TestRunExperimentsParallelPropagatesError(t *testing.T) {
	dir := t.TempDir()
	good := mkExp(t, dir, 1)
	bad := good
	bad.ID = "exp_missing"
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	if _, err := RunExperiments(dir, StrategyParallel, []Experiment{good, bad}, RunCap{}, probe); err == nil {
		t.Fatal("parallel run with a missing experiment must error")
	}
}

func TestRunExperimentsHybridPropagatesGroupError(t *testing.T) {
	dir := t.TempDir()
	good := mkExp(t, dir, 1)
	bad := good
	bad.ID = "exp_missing"
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	if _, err := RunExperiments(dir, StrategyHybrid, []Experiment{good, bad}, RunCap{}, probe); err == nil {
		t.Fatal("hybrid group with a missing experiment must error")
	}
}

func TestEnforceExperimentBudgetDirect(t *testing.T) {
	dir := t.TempDir()
	if err := enforceExperimentBudget(dir, StrategySequential, 1); err != nil {
		t.Fatalf("allowed budget must hold: %v", err)
	}
	if err := enforceExperimentBudget(dir, Strategy("NOPE"), 0); err != nil {
		t.Fatalf("budget gate does not validate the strategy name: %v", err)
	}
}
