package evolve

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func mkExp(t *testing.T, root string, maxRuns int) Experiment {
	t.Helper()
	exp, err := ProposeExperiment(root, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: maxRuns},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Advance(root, StateRunning); err != nil {
		t.Fatal(err)
	}
	return exp
}

// Sequential runs experiments in input order and reports in input order.
func TestSequentialRunsInOrder(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1), mkExp(t, dir, 1), mkExp(t, dir, 1)}
	var mu sync.Mutex
	var order []string
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		mu.Lock()
		order = append(order, exp.ID)
		mu.Unlock()
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || len(order) != 3 {
		t.Fatalf("want 3 outcomes in order, got %+v", out)
	}
	for i := range out {
		if out[i].ID != exps[i].ID || order[i] != exps[i].ID {
			t.Fatalf("sequential must follow input order, got outcomes %+v order %v", out, order)
		}
		if !out[i].Pass || out[i].Attempts != 1 {
			t.Fatalf("want pass in 1 attempt, got %+v", out[i])
		}
	}
}

// Parallel runs every experiment; outcomes stay indexed by input.
func TestParallelRunsEveryExperiment(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 2), mkExp(t, dir, 2), mkExp(t, dir, 2), mkExp(t, dir, 2)}
	var calls atomic.Int32
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		calls.Add(1)
		if attempt < 2 {
			return ProbeOutcome{Pass: false, Detail: "retry"}, nil
		}
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategyParallel, exps, RunCap{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	for i := range out {
		if out[i].ID != exps[i].ID {
			t.Fatalf("parallel outcomes must stay indexed by input, got %+v", out)
		}
		if !out[i].Pass || out[i].Attempts != 2 {
			t.Fatalf("want pass after retry (2 attempts), got %+v", out[i])
		}
	}
	if calls.Load() != 8 {
		t.Fatalf("want 4 exps x 2 attempts = 8 probes, got %d", calls.Load())
	}
}

// Hybrid runs the pilot fully before the parallel group starts.
func TestHybridPilotsFirst(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1), mkExp(t, dir, 1), mkExp(t, dir, 1)}
	var mu sync.Mutex
	events := []string{}
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		mu.Lock()
		events = append(events, exp.ID)
		mu.Unlock()
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategyHybrid, exps, RunCap{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 outcomes, got %+v", out)
	}
	if events[0] != exps[0].ID {
		t.Fatalf("pilot %s must run before the group, events %v", exps[0].ID, events)
	}
}

// Unknown strategies and exhausted caps are refused; the cap stops early
// and experiments beyond it never run.
func TestStrategyRefusalsAndCap(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1), mkExp(t, dir, 1)}
	if _, err := RunExperiments(dir, Strategy("NOPE"), exps, RunCap{}, nil); err == nil {
		t.Fatal("unknown strategy must be refused")
	}
	var ran []string
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		ran = append(ran, exp.ID)
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategySequential, exps, RunCap{MaxTotal: 1}, probe)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("exhausted cap must return ErrBudgetExhausted, got %v / %+v", err, out)
	}
	if len(ran) != 1 {
		t.Fatalf("cap stops the run: only 1 probe may fire, got %v", ran)
	}
	if fmt.Sprint(out) == "" {
		t.Fatal("unreachable")
	}
}
