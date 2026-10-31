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
// ErrBudgetExhausted and the outcomes collected so far.
func RunExperiments(root string, strategy Strategy, exps []Experiment, cap RunCap, probe ProbeFunc) ([]RunOutcome, error) {
	switch strategy {
	case StrategySequential, StrategyParallel, StrategyHybrid:
	default:
		return nil, fmt.Errorf("evolve: unknown strategy %q (want sequential, parallel or hybrid)", strategy)
	}
	if probe == nil {
		return nil, errors.New("evolve: strategy run needs a probe function")
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
		exp := exps[i]
		res := RunOutcome{ID: exp.ID}
		for attempt := 1; attempt <= exp.Budget.MaxRuns; attempt++ {
			if err := claim(); err != nil {
				out[i] = res
				return err
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
				out[i] = res
				return serr
			}
			res.Attempts = attempt
			res.Detail = detail
			if perr == nil && got.Pass {
				res.Pass = true
				break
			}
		}
		out[i] = res
		return nil
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
