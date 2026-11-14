// Optimization strategies for the PROBE stage (Phase 6, S5): sequential,
// parallel and hybrid execution of experiments over an injectable probe
// function. Strategies are selected by name and run progressively: the
// pilot-first hybrid exists so a cheap sequential check precedes fan-out.
// Every attempt is recorded in the experiment history (audit) and the
// total attempt count is capped by RunCap.
package evolve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// F7 exploration engine (NOV-58, f7-protocol-v1): hard-budgeted fan-out
// over lightweight candidates. INV and DOM share one protocol, one probe
// shape and one budget; sequential chains carry parent_id lineage without
// touching the baseline; the workspace is isolated (optional checkpoint
// dir) and never promotes (no Adopt/Advance path here); the fallback is
// inline goroutines — no subagents, no exec, hermetic under App Control.
// Statuses mirror the manifest taxonomy (pass/fail/timeout/infra/
// non-evaluable/missing) so metrics stay separable downstream.

// RunMode selects the exploration order.
type RunMode string

const (
	RunModeParallel   RunMode = "parallel"
	RunModeSequential RunMode = "sequential"
)

// F7 exploration statuses (protocol §5).
const (
	ExplorePass         = "pass"
	ExploreFail         = "fail"
	ExploreTimeout      = "timeout"
	ExploreInfra        = "infra"
	ExploreNonEvaluable = "non-evaluable"
	ExploreMissing      = "missing"
)

// ErrExplorationBudget stops a run when a hard budget fires. Partial
// outcomes collected so far accompany the error.
var ErrExplorationBudget = errors.New("evolve: exploration budget exhausted")

// RunBudget holds the hard F7 limits. Zero means uncapped except where
// noted; negative values are refused.
type RunBudget struct {
	MaxWallTime    time.Duration
	MaxTokens      int64
	MaxCandidates  int
	MaxConcurrency int
	MaxRetries     int
	MaxDiffBytes   int64
}

// RunOptions controls one exploration run.
type RunOptions struct {
	K         int
	Mode      RunMode
	Budget    RunBudget
	Seed      int64
	Workspace string
}

// ExplorationCandidate is one unit to explore: id + domain + lineage.
// Proposals carry no rubrics, answers or holdout content (anti-leakage).
type ExplorationCandidate struct {
	ID       string
	Domain   string
	ParentID string
}

// ExplorationOutcome is the per-candidate result, indexed by input order.
type ExplorationOutcome struct {
	ID       string
	Domain   string
	ParentID string
	Status   string
	Detail   string
	Attempts int
	Tokens   int64
}

// ExploreProbe runs one attempt. It must be deterministic in
// (seed, candidate, attempt) and must not depend on wall time. Returning
// a non-nil error records a fail ("probe error: ...") and retries per
// budget; only ctx cancellation, timeout and token/candidate budgets
// abort the whole run.
type ExploreProbe func(ctx context.Context, cand ExplorationCandidate, attempt int) (ExplorationOutcome, error)

// validExploreStatus reports protocol membership.
func validExploreStatus(s string) bool {
	switch s {
	case ExplorePass, ExploreFail, ExploreTimeout, ExploreInfra, ExploreNonEvaluable, ExploreMissing:
		return true
	}
	return false
}

// ValidateRunOptions checks K, mode, seed and budget signs without firing
// any probe.
func ValidateRunOptions(opts RunOptions, nCandidates int) error {
	if opts.K < 1 {
		return fmt.Errorf("evolve: exploration K must be >= 1, got %d", opts.K)
	}
	switch opts.Mode {
	case RunModeParallel, RunModeSequential:
	default:
		return fmt.Errorf("evolve: unknown exploration mode %q (want parallel or sequential)", opts.Mode)
	}
	if opts.Seed == 0 {
		return errors.New("evolve: exploration needs a non-zero seed for reproducibility")
	}
	if nCandidates < 0 {
		return errors.New("evolve: negative candidate count")
	}
	b := opts.Budget
	if b.MaxTokens < 0 || b.MaxCandidates < 0 || b.MaxConcurrency < 0 || b.MaxRetries < 0 || b.MaxDiffBytes < 0 || b.MaxWallTime < 0 {
		return errors.New("evolve: exploration budgets must not be negative")
	}
	return nil
}

// effectiveSelection caps the run: first N in input order where
// N = min(K, len, MaxCandidates when > 0). Input order is stable so
// resume stays idempotent under the same seed and candidate list.
func effectiveSelection(opts RunOptions, cands []ExplorationCandidate) []ExplorationCandidate {
	n := len(cands)
	if opts.K < n {
		n = opts.K
	}
	if opts.Budget.MaxCandidates > 0 && opts.Budget.MaxCandidates < n {
		n = opts.Budget.MaxCandidates
	}
	return cands[:n]
}

// truncateDiff enforces the MaxDiffBytes hard ceiling by truncation.
func truncateDiff(detail string, max int64) string {
	if max > 0 && int64(len(detail)) > max {
		return detail[:max]
	}
	return detail
}

// retryable reports whether another attempt may help. Pass,
// non-evaluable and missing are terminal; fail/timeout/infra retry
// while attempts remain.
func retryable(status string) bool {
	switch status {
	case ExploreFail, ExploreTimeout, ExploreInfra:
		return true
	}
	return false
}

// checkpointPath confines resume state to the isolated workspace.
func checkpointPath(workspace string) string {
	return filepath.Join(workspace, "checkpoint.json")
}

// loadCheckpoint reads previously completed outcomes from the workspace.
// A missing workspace or checkpoint reads as empty, never an error.
func loadCheckpoint(workspace string) map[string]ExplorationOutcome {
	done := map[string]ExplorationOutcome{}
	if strings.TrimSpace(workspace) == "" {
		return done
	}
	data, err := os.ReadFile(checkpointPath(workspace))
	if err != nil {
		return done
	}
	var outs []ExplorationOutcome
	if err := json.Unmarshal(data, &outs); err != nil {
		return map[string]ExplorationOutcome{}
	}
	for _, o := range outs {
		done[o.ID] = o
	}
	return done
}

// saveCheckpoint rewrites the workspace checkpoint atomically (temp+rename).
func saveCheckpoint(workspace string, outs []ExplorationOutcome) {
	if strings.TrimSpace(workspace) == "" {
		return
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(outs, "", "  ")
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(workspace, "checkpoint-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	os.Rename(tmpName, checkpointPath(workspace))
}

// RunExploration runs opts.K candidates under opts with hard budgets.
// Outcomes follow input order; partial failures never abort the run —
// only ctx cancellation, wall-time timeout and token/candidate budgets
// do, returning the partial outcomes with the cause. The baseline is
// never touched (no root, no store writes outside Workspace) and nothing
// here can promote: adoption lives in the NOV-42 cycle, not in exploration.
func RunExploration(ctx context.Context, opts RunOptions, cands []ExplorationCandidate, probe ExploreProbe) ([]ExplorationOutcome, error) {
	return RunExplorationResume(ctx, opts, cands, nil, probe)
}

// RunExplorationResume resumes idempotently: ids present in done (or in
// the workspace checkpoint when Workspace is set) are reused without
// re-firing the probe. Repeating the same call with the outcomes of the
// previous attempt fires zero new probes for completed ids.
func RunExplorationResume(ctx context.Context, opts RunOptions, cands []ExplorationCandidate, done map[string]ExplorationOutcome, probe ExploreProbe) ([]ExplorationOutcome, error) {
	if err := ValidateRunOptions(opts, len(cands)); err != nil {
		return nil, err
	}
	if probe == nil {
		return nil, errors.New("evolve: exploration needs a probe function")
	}
	for _, c := range cands {
		if strings.TrimSpace(c.ID) == "" {
			return nil, errors.New("evolve: exploration candidate needs an id")
		}
		if c.Domain != DomainINV && c.Domain != DomainDOM {
			return nil, fmt.Errorf("evolve: exploration domain %q must be INV or DOM", c.Domain)
		}
	}
	if opts.Workspace != "" {
		if err := os.MkdirAll(opts.Workspace, 0o755); err != nil {
			return nil, err
		}
		for id, o := range loadCheckpoint(opts.Workspace) {
			if done == nil {
				done = map[string]ExplorationOutcome{}
			}
			if _, ok := done[id]; !ok {
				done[id] = o
			}
		}
	}
	if opts.Budget.MaxWallTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Budget.MaxWallTime)
		defer cancel()
	}
	sel := effectiveSelection(opts, cands)
	out := make([]ExplorationOutcome, len(sel))
	doneCount := 0
	for i, c := range sel {
		if d, ok := done[c.ID]; ok {
			out[i] = d
			doneCount++
			continue
		}
		out[i] = ExplorationOutcome{ID: c.ID, Domain: c.Domain, ParentID: c.ParentID, Status: ExploreMissing}
	}
	if doneCount == len(sel) {
		return out, nil
	}
	var mu sync.Mutex
	var totalTokens int64
	for _, d := range done {
		totalTokens += d.Tokens
	}
	claimTokens := func(t int64) error {
		mu.Lock()
		defer mu.Unlock()
		if opts.Budget.MaxTokens > 0 && totalTokens+t > opts.Budget.MaxTokens {
			return ErrExplorationBudget
		}
		totalTokens += t
		return nil
	}
	// attemptOne fires a single probe attempt and folds budget/detail rules.
	attemptOne := func(cand ExplorationCandidate, attempt int) (ExplorationOutcome, bool, error) {
		if err := ctx.Err(); err != nil {
			return ExplorationOutcome{}, false, err
		}
		got, err := probe(ctx, cand, attempt)
		if err != nil {
			got = ExplorationOutcome{ID: cand.ID, Domain: cand.Domain, ParentID: cand.ParentID, Status: ExploreFail, Detail: "probe error: " + err.Error()}
		}
		if strings.TrimSpace(got.ID) == "" {
			got.ID = cand.ID
		}
		got.Domain = cand.Domain
		got.ParentID = cand.ParentID
		if !validExploreStatus(got.Status) {
			got.Status = ExploreFail
		}
		got.Detail = truncateDiff(got.Detail, opts.Budget.MaxDiffBytes)
		if got.Attempts == 0 {
			got.Attempts = attempt
		}
		if err := claimTokens(got.Tokens); err != nil {
			return got, false, err
		}
		return got, true, nil
	}
	runCandidate := func(i int) error {
		cand := sel[i]
		if _, ok := done[cand.ID]; ok {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		maxAttempts := 1 + opts.Budget.MaxRetries
		var last ExplorationOutcome
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			got, ok, err := attemptOne(cand, attempt)
			if err != nil {
				if errors.Is(err, ErrExplorationBudget) {
					last = got
					last.Attempts = attempt
					mu.Lock()
					out[i] = last
					mu.Unlock()
					return ErrExplorationBudget
				}
				return err
			}
			if !ok {
				return ctx.Err()
			}
			got.Attempts = attempt
			last = got
			if !retryable(got.Status) {
				break
			}
			if got.Status == ExplorePass {
				break
			}
		}
		mu.Lock()
		out[i] = last
		completed := append([]ExplorationOutcome(nil), out...)
		mu.Unlock()
		saveCheckpoint(opts.Workspace, completed)
		return nil
	}
	switch opts.Mode {
	case RunModeSequential:
		for i := range sel {
			if _, ok := done[sel[i].ID]; ok {
				continue
			}
			if err := runCandidate(i); err != nil {
				return out, err
			}
		}
	case RunModeParallel:
		maxConc := opts.Budget.MaxConcurrency
		if maxConc <= 0 {
			maxConc = len(sel)
		}
		sem := make(chan struct{}, maxConc)
		var wg sync.WaitGroup
		errs := make([]error, len(sel))
		for i := range sel {
			if _, ok := done[sel[i].ID]; ok {
				continue
			}
			if err := ctx.Err(); err != nil {
				for j := range errs {
					if errs[j] == nil {
						errs[j] = err
					}
				}
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(i int) {
				defer wg.Done()
				defer func() { <-sem }()
				errs[i] = runCandidate(i)
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
