// Optimization strategies for the PROBE stage (Phase 6, S5): sequential,
// parallel and hybrid execution of experiments over an injectable probe
// function. Strategies are selected by name and run progressively: the
// pilot-first hybrid exists so a cheap sequential check precedes fan-out.
// Every attempt is recorded in the experiment history (audit) and the
// total attempt count is capped by RunCap.
package evolve

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/reflect"
)

// Strategy selects how experiments run.
type Strategy string

const (
	StrategySequential Strategy = "sequential"
	StrategyParallel   Strategy = "parallel"
	StrategyHybrid     Strategy = "hybrid"
)

// ErrBudgetExhausted stops a strategy run when the total attempt cap fires.
var ErrBudgetExhausted = errors.New("evolve: experiment run budget exhausted")

// ProbeOutcome is one probe attempt: pass with detail, or fail.
type ProbeOutcome struct {
	Pass   bool
	Detail string
}

// ProbeFunc runs one attempt of an experiment. Deterministic callers only:
// attempts are counted, so the function must not depend on wall time.
type ProbeFunc func(exp Experiment, attempt int) (ProbeOutcome, error)

// RunOutcome is the per-experiment result, indexed by input order.
type RunOutcome struct {
	ID       string
	Pass     bool
	Attempts int
	Detail   string
}

// RunCap bounds a strategy run. MaxTotal <= 0 means uncapped.
type RunCap struct {
	MaxTotal int
}

// RunExperiments runs exps under strategy. Outcomes follow input order for
// every strategy. Attempts beyond RunCap never fire: the run stops with
// ErrBudgetExhausted and the outcomes collected so far. The global
// evolution cap is enforced first (S1): an exhausted day refuses the
// whole run with no probe firing, and an allowed run records one
// experiment-run ledger entry before running.
// checkStrategyArgs validates the strategy name and probe before any
// ledger work: unknown strategies and a nil probe are refused with no
// side effects.
func checkStrategyArgs(strategy Strategy, probe ProbeFunc) error {
	switch strategy {
	case StrategySequential, StrategyParallel, StrategyHybrid:
	default:
		return fmt.Errorf("evolve: unknown strategy %q (want sequential, parallel or hybrid)", strategy)
	}
	if probe == nil {
		return errors.New("evolve: strategy run needs a probe function")
	}
	return nil
}

// enforceExperimentBudget enforces the global evolution cap (PRD R-E4,
// S1): strategy runs consume the daily ledger like the rest, mirroring
// reflect.Run. Experiments carry no task, so Task="" (global cap only);
// refusals are free and recorded. An allowed run records one
// experiment-run ledger entry before running.
func enforceExperimentBudget(root string, strategy Strategy, n int) error {
	now := time.Now().UTC()
	dailyCap := reflect.EvolutionCap(root)
	today, err := budget.TodayEvolutionCount(root, now)
	if err != nil {
		return err
	}
	if today >= dailyCap {
		if err := budget.Record(root, budget.Entry{
			Kind:      budget.KindExperiment,
			Timestamp: now,
			Task:      "",
			Outcome:   budget.OutcomeRefusedCap,
			Detail:    fmt.Sprintf("cap %d reached", dailyCap),
		}); err != nil {
			return err
		}
		return fmt.Errorf("evolve: evolution cap reached: %d run(s) today, cap %d (regulus.json evolution.max_per_day)", today, dailyCap)
	}
	if err := budget.Record(root, budget.Entry{
		Kind:      budget.KindExperiment,
		Timestamp: now,
		Task:      "",
		Outcome:   budget.OutcomeExperimentRun,
		Detail:    fmt.Sprintf("strategy %s: %d experiment(s)", strategy, n),
	}); err != nil {
		return err
	}
	return nil
}

// runSingleExperiment runs one experiment's attempts: it reloads from
// disk (the caller may hold a stale copy, and saving it back would
// clobber state transitions made after the copy), requires RUNNING,
// records every attempt in the history and stops on the first pass.
func runSingleExperiment(root string, strategy Strategy, id string, claim func() error, probe ProbeFunc) (RunOutcome, error) {
	exp, err := LoadExperiment(root, id)
	if err != nil {
		return RunOutcome{ID: id}, err
	}
	if exp.State != StateRunning {
		return RunOutcome{ID: exp.ID}, fmt.Errorf("evolve: strategy runs RUNNING experiments, %s is %s", exp.ID, exp.State)
	}
	res := RunOutcome{ID: exp.ID}
	for attempt := 1; attempt <= exp.Budget.MaxRuns; attempt++ {
		if err := claim(); err != nil {
			return res, err
		}
		got, perr := probe(exp, attempt)
		detail := got.Detail
		if perr != nil {
			detail = "probe error: " + perr.Error()
		}
		exp.History = append(exp.History, ExpHistoryEvent{
			Kind: "probe", From: exp.State, To: exp.State,
			By:   "strategy-" + string(strategy),
			Note: fmt.Sprintf("attempt %d/%d: %s", attempt, exp.Budget.MaxRuns, detail),
		})
		if serr := exp.save(root); serr != nil {
			return res, serr
		}
		res.Attempts = attempt
		res.Detail = detail
		if perr == nil && got.Pass {
			res.Pass = true
			break
		}
	}
	return res, nil
}

func RunExperiments(root string, strategy Strategy, exps []Experiment, cap RunCap, probe ProbeFunc) ([]RunOutcome, error) {
	if err := checkStrategyArgs(strategy, probe); err != nil {
		return nil, err
	}
	// Global evolution cap (PRD R-E4, S1): enforced first, before any
	// probe fires.
	if err := enforceExperimentBudget(root, strategy, len(exps)); err != nil {
		return nil, err
	}
	out := make([]RunOutcome, len(exps))
	var mu sync.Mutex
	used := 0
	claim := func() error {
		mu.Lock()
		defer mu.Unlock()
		if cap.MaxTotal > 0 && used >= cap.MaxTotal {
			return ErrBudgetExhausted
		}
		used++
		return nil
	}
	runOne := func(i int) error {
		res, err := runSingleExperiment(root, strategy, exps[i].ID, claim, probe)
		out[i] = res
		return err
	}
	switch strategy {
	case StrategySequential:
		for i := range exps {
			if err := runOne(i); err != nil {
				return out, err
			}
		}
	case StrategyParallel:
		var wg sync.WaitGroup
		errs := make([]error, len(exps))
		for i := range exps {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i] = runOne(i)
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return out, err
			}
		}
	case StrategyHybrid:
		if len(exps) > 0 {
			if err := runOne(0); err != nil {
				return out, err
			}
		}
		var wg sync.WaitGroup
		errs := make([]error, len(exps))
		for i := 1; i < len(exps); i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				errs[i] = runOne(i)
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				return out, err
			}
		}
	}
	return out, nil
}
