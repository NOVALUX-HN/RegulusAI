package evolve

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"regulus-ai/internal/budget"
)

// An exhausted daily cap refuses the whole strategy run: no probe may
// fire and the refused-cap refusal lands in the ledger (S1).
func TestRunExperimentsRefusesWhenCapExhausted(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	for i := 0; i < budget.DefaultEvolutionCapPerDay; i++ {
		if err := budget.Record(dir, budget.Entry{
			Kind:      budget.KindExperiment,
			Timestamp: now,
			Outcome:   budget.OutcomeExperimentRun,
		}); err != nil {
			t.Fatal(err)
		}
	}
	exps := []Experiment{mkExp(t, dir, 1)}
	var calls atomic.Int32
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		calls.Add(1)
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	_, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe)
	if err == nil {
		t.Fatal("exhausted evolution cap must refuse the strategy run")
	}
	if !strings.Contains(err.Error(), "evolution cap reached") {
		t.Fatalf("refusal must name the evolution cap, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("refused run must fire zero probes, got %d", calls.Load())
	}
	entries, err := budget.Show(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != budget.DefaultEvolutionCapPerDay+1 {
		t.Fatalf("refusal must append exactly one ledger entry, got %d entries", len(entries))
	}
	last := entries[len(entries)-1]
	if last.Kind != budget.KindExperiment || last.Outcome != budget.OutcomeRefusedCap {
		t.Fatalf("last entry must be the experiment refusal, got %+v", last)
	}
	if last.Task != "" {
		t.Fatalf("experiment refusal carries no task (global cap), got %q", last.Task)
	}
	if !strings.Contains(last.Detail, "cap") {
		t.Fatalf("refusal detail must name the cap reason, got %q", last.Detail)
	}
}

// An allowed strategy run records exactly one experiment-run entry per
// invocation, then runs as today (S1).
func TestRunExperimentsRecordsOneExperimentRun(t *testing.T) {
	dir := t.TempDir()
	exps := []Experiment{mkExp(t, dir, 1), mkExp(t, dir, 1)}
	probe := func(exp Experiment, attempt int) (ProbeOutcome, error) {
		return ProbeOutcome{Pass: true, Detail: "ok"}, nil
	}
	out, err := RunExperiments(dir, StrategySequential, exps, RunCap{}, probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if !o.Pass {
			t.Fatalf("allowed run must run as today, got %+v", out)
		}
	}
	entries, err := budget.Show(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("one strategy run must record exactly one ledger entry, got %+v", entries)
	}
	e := entries[0]
	if e.Kind != budget.KindExperiment || e.Outcome != budget.OutcomeExperimentRun {
		t.Fatalf("entry must be kind experiment outcome experiment-run, got %+v", e)
	}
	if e.Task != "" {
		t.Fatalf("experiment runs carry no task (global cap only), got %q", e.Task)
	}
}
