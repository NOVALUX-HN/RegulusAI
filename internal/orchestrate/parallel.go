// Parallel scheduler (F12.2/NOV-97): run isolated workers concurrently
// with budgets, cancellation and backpressure, then integrate through
// a controlled merge and gates.
//
// Reuse, never duplication:
//   - DAG/levels/collisions come from dag.go (same package);
//   - isolation (branch/path/workspace/scopes) from worktree.go with
//     F11.8 Delegate (workers inherit restricted scopes, never more);
//   - wall-vs-aggregate accounting follows the F11.3 tier discipline
//     (wall divides the aggregate by parallelism; ceilings, never
//     predictions);
//   - traceability rides the F11.1 exec log (run/task/delegation/gate
//     events, redacted pre-disk by RecordExec); dry-run computes the
//     plan and records nothing (F12.1 precedent).
//
// Failure isolation: one worker failing (or timing out, or the run
// being cancelled) never corrupts its siblings — every worker owns
// its worktree, dependents skip with a naming reason, the merge runs
// only when every worker passed, and conflicted merges stop for an
// integrator review with the worktrees preserved.
package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

// Task outcomes: the closed vocabulary.
const (
	TaskPassed    = "passed"
	TaskFailed    = "failed"
	TaskSkipped   = "skipped"
	TaskCancelled = "cancelled"
	TaskTimedOut  = "timed_out"
)

// ParallelBudgets bounds one parallel run: MaxConcurrency caps
// workers in flight (1..8, the F11.3 tier ceiling); PerTaskTimeoutMS
// bounds one worker (0 = unset); WallBudgetMS bounds the whole run
// (0 = unset); MaxQueued bounds admission (0 = MaxParallelTasks).
// Refusing an over-queued plan before anything runs IS the
// backpressure: the scheduler never holds more than one level in
// flight and never more than MaxConcurrency workers running.
type ParallelBudgets struct {
	MaxConcurrency   int   `json:"max_concurrency"`
	PerTaskTimeoutMS int64 `json:"per_task_timeout_ms"`
	WallBudgetMS     int64 `json:"wall_budget_ms"`
	MaxQueued        int   `json:"max_queued"`
}

// DefaultParallelBudgets is the documented policy: four workers
// (the tier default), no time budgets, admission capped at the plan
// maximum.
func DefaultParallelBudgets() ParallelBudgets {
	return ParallelBudgets{MaxConcurrency: 4, MaxQueued: MaxParallelTasks}
}

// ValidateParallelBudgets refuses unbounded or hostile budgets before
// anything runs.
func ValidateParallelBudgets(b ParallelBudgets, nTasks int) error {
	if b.MaxConcurrency < 1 || b.MaxConcurrency > 8 {
		return fmt.Errorf("orchestrate parallel: --max-concurrency %d outside 1..8", b.MaxConcurrency)
	}
	if b.PerTaskTimeoutMS < 0 {
		return fmt.Errorf("orchestrate parallel: --per-task-timeout-ms must not be negative, got %d", b.PerTaskTimeoutMS)
	}
	if b.WallBudgetMS < 0 {
		return fmt.Errorf("orchestrate parallel: --wall-budget-ms must not be negative, got %d", b.WallBudgetMS)
	}
	maxQ := b.MaxQueued
	if maxQ == 0 {
		maxQ = MaxParallelTasks
	}
	if maxQ < 1 {
		return fmt.Errorf("orchestrate parallel: --max-queued must not be negative, got %d", b.MaxQueued)
	}
	if nTasks > maxQ {
		return fmt.Errorf("orchestrate parallel: %d tasks exceed the queue bound %d (backpressure: split the plan)", nTasks, maxQ)
	}
	return nil
}

// EstimateParallelCost folds per-task wall estimates into ceilings:
// the aggregate sums them, the wall divides by parallelism
// (F11.3 discipline). All-zero estimates read as unknown, never as
// instant.
func EstimateParallelCost(tasks []ParallelTask, parallelism int) (wallMS, aggMS int64) {
	if parallelism <= 0 {
		parallelism = 4
	}
	for _, t := range tasks {
		if t.EstimateMS > 0 {
			aggMS += t.EstimateMS
		}
	}
	if aggMS == 0 {
		return 0, 0
	}
	return (aggMS + int64(parallelism) - 1) / int64(parallelism), aggMS
}

// TaskResult is one worker verdict: outcome, measured duration,
// isolation coordinates and the naming reason when it did not pass.
type TaskResult struct {
	TaskID     string   `json:"task_id"`
	Outcome    string   `json:"outcome"`
	DurationMS int64    `json:"duration_ms"`
	Reason     string   `json:"reason,omitempty"`
	Branch     string   `json:"branch"`
	Path       string   `json:"path"`
	Agent      string   `json:"agent"`
	Scopes     []string `json:"scopes"`
}

// MergeOutcome is one integration verdict: clean merge, conflict
// with the unmerged files and the integrator review, or blocked
// behind an earlier conflict.
type MergeOutcome struct {
	TaskID        string   `json:"task_id"`
	Branch        string   `json:"branch"`
	Merged        bool     `json:"merged"`
	Conflict      bool     `json:"conflict"`
	ConflictFiles []string `json:"conflict_files,omitempty"`
	NeedsReview   bool     `json:"needs_review,omitempty"`
	Reviewer      string   `json:"reviewer,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// GateOutcome is the post-merge gate verdict.
type GateOutcome struct {
	Ran    bool   `json:"ran"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
	Task   string `json:"task,omitempty"`
}

// ParallelReport is the run receipt: plan, per-worker verdicts,
// merges, gate, wall-vs-aggregate and trace link.
type ParallelReport struct {
	Version        string         `json:"version"`
	RunID          string         `json:"run_id"`
	Base           string         `json:"base"`
	BranchPrefix   string         `json:"branch_prefix"`
	Levels         [][]string     `json:"levels"`
	Serialized     [][2]string    `json:"serialized,omitempty"`
	Order          []string       `json:"order"`
	Results        []TaskResult   `json:"results"`
	Merges         []MergeOutcome `json:"merges,omitempty"`
	Gate           GateOutcome    `json:"gate"`
	WallMS         int64          `json:"wall_ms"`
	AggregateMS    int64          `json:"aggregate_ms"`
	Cancelled      bool           `json:"cancelled"`
	Failed         bool           `json:"failed"`
	DryRun         bool           `json:"dry_run"`
	TraceHint      string         `json:"trace_hint,omitempty"`
	OrphansRemoved int            `json:"orphans_removed"`
	CleanupErrors  []string       `json:"cleanup_errors,omitempty"`
}

// ParallelRequest carries everything RunParallel needs. Ops, Work and
// Gate must be set for a wet run (fail closed); a dry run touches
// none of them. Record is the F11.1 sink (nil records nothing but the
// trace hint is still computed); Now injects the clock for tests.
type ParallelRequest struct {
	Tasks        []ParallelTask
	Budgets      ParallelBudgets
	Base         string
	RunID        string
	BranchPrefix string
	WorktreeRoot string
	RepoRoot     string
	Parent       mcp.Principal
	Ops          WorktreeOps
	Work         func(ctx context.Context, task ParallelTask, dir string) error
	Gate         func(ctx context.Context, repoRoot string) GateOutcome
	Record       func(observability.ExecEvent)
	Now          func() time.Time
	DryRun       bool
}

// parallelPlan is the validated, leveled run: everything downstream
// is deterministic. Isolation coordinates are pure per (run, task)
// strings so the merge order and the orphan sweeper agree without
// extra state.
type parallelPlan struct {
	budgets    ParallelBudgets
	levels     [][]string
	serialized [][2]string
	order      []string
	byID       map[string]ParallelTask
	runID      string
	base       string
	prefix     string
	branchOf   map[string]string
	pathOf     map[string]string
	isoOf      map[string]WorkerIsolation
	now        func() time.Time
}

// parallelEmit stamps the run id onto one exec event.
type parallelEmit func(observability.ExecEvent)

// makeParallelEmit builds the F11.1 sink: nil records nothing.
func makeParallelEmit(req ParallelRequest, runID string) parallelEmit {
	return func(e observability.ExecEvent) {
		if req.Record == nil {
			return
		}
		e.RunID = runID
		req.Record(e)
	}
}

// taskOutcomeStatus maps a worker outcome onto the F11.1 status
// vocabulary.
func taskOutcomeStatus(outcome string) string {
	switch outcome {
	case TaskPassed:
		return observability.ExecSucceeded
	case TaskTimedOut:
		return observability.ExecTimedOut
	case TaskCancelled, TaskSkipped:
		return observability.ExecCancelled
	default:
		return observability.ExecFailed
	}
}

// planParallelRun validates the request and levels the DAG: tasks,
// budgets, explicit plus collision order, ids and defaults. Pure:
// creates nothing, records nothing.
func planParallelRun(req ParallelRequest) (*parallelPlan, error) {
	if err := ValidateParallelTasks(req.Tasks); err != nil {
		return nil, err
	}
	budgets := req.Budgets
	if budgets.MaxConcurrency == 0 && budgets.PerTaskTimeoutMS == 0 &&
		budgets.WallBudgetMS == 0 && budgets.MaxQueued == 0 {
		budgets = DefaultParallelBudgets()
	}
	if err := ValidateParallelBudgets(budgets, len(req.Tasks)); err != nil {
		return nil, err
	}
	levels, serialized, err := PlanParallelLevels(req.Tasks)
	if err != nil {
		return nil, err
	}
	order := FlattenParallelLevels(levels)
	byID := map[string]ParallelTask{}
	for _, t := range req.Tasks {
		byID[t.ID] = t
	}
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		minted, merr := observability.MintRunID()
		if merr != nil {
			return nil, fmt.Errorf("orchestrate parallel: cannot mint run id: %v", merr)
		}
		runID = minted
	}
	if strings.ContainsAny(runID, "\n\r") || len(runID) > 128 {
		return nil, fmt.Errorf("orchestrate parallel: run id must be a single line under 128 chars")
	}
	base := strings.TrimSpace(req.Base)
	if base == "" {
		base = "HEAD"
	}
	prefix := strings.TrimSpace(req.BranchPrefix)
	if prefix == "" {
		prefix = DefaultParallelBranchPrefix
	}
	now := req.Now
	if now == nil {
		now = time.Now
	}
	plan := &parallelPlan{
		budgets: budgets, levels: levels, serialized: serialized,
		order: order, byID: byID, runID: runID, base: base,
		prefix: prefix, now: now,
		branchOf: map[string]string{}, pathOf: map[string]string{},
		isoOf: map[string]WorkerIsolation{},
	}
	for _, id := range order {
		br := ParallelBranch(prefix, runID, id)
		p := ParallelWorktreePath(req.WorktreeRoot, br)
		plan.branchOf[id], plan.pathOf[id] = br, p
		plan.isoOf[id] = IsolateParallelWorker(req.Parent, byID[id], br, p)
	}
	return plan, nil
}

// checkWetRequest refuses a wet run without its backends: worktree
// root, repo root, worktree ops, worker function and gate check.
func checkWetRequest(req ParallelRequest) error {
	if strings.TrimSpace(req.WorktreeRoot) == "" {
		return fmt.Errorf("orchestrate parallel: worktree root is required (pass --worktree-root)")
	}
	if strings.TrimSpace(req.RepoRoot) == "" {
		return fmt.Errorf("orchestrate parallel: repo root is required")
	}
	if req.Ops == nil {
		return fmt.Errorf("orchestrate parallel: no worktree backend configured")
	}
	if req.Work == nil {
		return fmt.Errorf("orchestrate parallel: no worker function configured")
	}
	if req.Gate == nil {
		return fmt.Errorf("orchestrate parallel: no gate check configured")
	}
	return nil
}

// failParallelRun closes a failed report: flag, run-end event with
// the naming detail, trace hint. Worktrees stay preserved.
func failParallelRun(rep *ParallelReport, emit parallelEmit, runID, detail string) {
	rep.Failed = true
	emit(observability.ExecEvent{
		Type: observability.ExecRunEnd, Status: statusForEnd(*rep),
		Payload: map[string]any{
			"command": "orchestrate parallel",
			"failed":  true, "detail": detail,
		},
	})
	rep.TraceHint = bench.TraceHintFor(runID)
}

// statusForEnd maps a failed/cancelled report onto the run-end
// status: cancellation reads as cancelled, every other failure as
// failed.
func statusForEnd(rep ParallelReport) string {
	if rep.Cancelled {
		return observability.ExecCancelled
	}
	return observability.ExecFailed
}

// RunParallel executes one parallel plan: validate, level, run each
// level concurrently (bounded by MaxConcurrency, one level at a
// time), then merge every worker branch in deterministic order and
// run the gate. Dry-run plans and estimates without side effects.
func RunParallel(ctx context.Context, req ParallelRequest) (ParallelReport, error) {
	plan, err := planParallelRun(req)
	if err != nil {
		return ParallelReport{}, err
	}
	rep := ParallelReport{
		Version: ParallelVersion, RunID: plan.runID, Base: plan.base,
		BranchPrefix: plan.prefix, Levels: plan.levels,
		Serialized: plan.serialized, Order: plan.order,
	}
	if req.DryRun {
		rep.DryRun = true
		wall, agg := EstimateParallelCost(req.Tasks, plan.budgets.MaxConcurrency)
		rep.WallMS, rep.AggregateMS = wall, agg
		return rep, nil
	}
	if err := checkWetRequest(req); err != nil {
		return ParallelReport{}, err
	}
	emit := makeParallelEmit(req, plan.runID)
	runCtx := ctx
	cancelWall := func() {}
	if plan.budgets.WallBudgetMS > 0 {
		runCtx, cancelWall = context.WithDeadline(ctx, time.Now().Add(time.Duration(plan.budgets.WallBudgetMS)*time.Millisecond))
		defer cancelWall()
	}
	started := plan.now()
	emit(observability.ExecEvent{
		Type: observability.ExecRunStart, Status: observability.ExecStarted,
		Payload: map[string]any{
			"command": "orchestrate parallel", "tasks": len(req.Tasks),
			"base": plan.base, "levels": len(plan.levels),
			"max_concurrency": plan.budgets.MaxConcurrency,
		},
	})
	results, failed := runParallelLevels(runCtx, req, plan, emit)
	var agg int64
	for _, id := range plan.order {
		agg += results[id].DurationMS
	}
	rep.AggregateMS = agg
	rep.WallMS = plan.now().Sub(started).Milliseconds()
	if rep.WallMS < 0 {
		rep.WallMS = 0
	}
	rep.Cancelled = ctx.Err() != nil || runCtx.Err() != nil
	ordered := make([]TaskResult, 0, len(plan.order))
	for _, id := range plan.order {
		ordered = append(ordered, *results[id])
	}
	rep.Results = ordered
	if failed {
		failParallelRun(&rep, emit, plan.runID,
			"a worker did not pass; merge skipped, worktrees preserved for review")
		return rep, nil
	}
	merges, conflicted := mergeParallelBranches(runCtx, req, plan, emit)
	rep.Merges = merges
	if conflicted {
		failParallelRun(&rep, emit, plan.runID,
			"merge conflict: integrator review required, worktrees preserved")
		return rep, nil
	}
	if passed := gateParallelRun(runCtx, req, plan, emit, &rep); !passed {
		failParallelRun(&rep, emit, plan.runID,
			"post-merge gate failed, worktrees preserved for review")
		return rep, nil
	}
	cleanupParallelWorktrees(runCtx, req, plan, emit, &rep)
	emit(observability.ExecEvent{
		Type: observability.ExecRunEnd, Status: observability.ExecSucceeded,
		Payload: map[string]any{"command": "orchestrate parallel", "merged": len(plan.order)},
	})
	rep.TraceHint = bench.TraceHintFor(plan.runID)
	return rep, nil
}

// runParallelLevels runs every level in order, each level's runnable
// workers concurrently (bounded by MaxConcurrency). A task whose
// explicit or collision predecessor did not pass never starts: it
// skips with a naming reason. Cancellation leaves queued work
// unstarted.
func runParallelLevels(runCtx context.Context, req ParallelRequest, plan *parallelPlan, emit parallelEmit) (map[string]*TaskResult, bool) {
	results := map[string]*TaskResult{}
	for _, id := range plan.order {
		iso := plan.isoOf[id]
		results[id] = &TaskResult{
			TaskID: id, Outcome: TaskFailed, Branch: plan.branchOf[id],
			Path: plan.pathOf[id], Agent: iso.Agent, Scopes: iso.Scopes,
		}
	}
	depOf := map[string][]string{}
	for _, t := range req.Tasks {
		depOf[t.ID] = append([]string(nil), t.DependsOn...)
	}
	// Collision predecessors that already order a task behind an
	// earlier worker: the waiter skips when its predecessor did not
	// pass (shared files, broken base).
	collPred := map[string][]string{}
	for _, p := range plan.serialized {
		collPred[p[1]] = append(collPred[p[1]], p[0])
	}
	failed := false
	sem := make(chan struct{}, plan.budgets.MaxConcurrency)
	for _, lv := range plan.levels {
		runnable := map[string]bool{}
		for _, id := range lv {
			if skipBy := skipReason(id, depOf, collPred, results); skipBy != "" {
				results[id].Outcome = TaskSkipped
				results[id].Reason = skipBy
				failed = true
				emit(observability.ExecEvent{
					TaskID: "parallel/" + id, Type: observability.ExecTaskEnd,
					Status:  observability.ExecCancelled,
					Payload: map[string]any{"task": id, "skipped": skipBy},
				})
				continue
			}
			if runCtx.Err() != nil {
				results[id].Outcome = TaskCancelled
				results[id].Reason = "run cancelled before start"
				failed = true
				continue
			}
			runnable[id] = true
		}
		var wg sync.WaitGroup
		for _, id := range lv {
			if !runnable[id] {
				continue
			}
			wg.Add(1)
			go func(taskID string) {
				defer wg.Done()
				runParallelWorker(runCtx, req, plan, emit, sem, results[taskID])
			}(id)
		}
		wg.Wait()
		for _, id := range lv {
			if results[id].Outcome != TaskPassed {
				failed = true
			}
		}
	}
	return results, failed
}

// skipReason names why a task must not start: an explicit or
// serialized predecessor did not pass. Empty means runnable.
func skipReason(id string, depOf, collPred map[string][]string, results map[string]*TaskResult) string {
	for _, d := range depOf[id] {
		if r := results[d]; r == nil || r.Outcome != TaskPassed {
			return "dependency " + d + " did not pass"
		}
	}
	for _, p := range collPred[id] {
		if r := results[p]; r == nil || r.Outcome != TaskPassed {
			return "serialized predecessor " + p + " did not pass (shared files)"
		}
	}
	return ""
}

// runParallelWorker executes one isolated worker: bounded admission,
// delegation/task events, worktree creation, the command with the
// per-task timeout, and the closing verdict. A worktree failure, a
// command failure, a timeout and a cancellation each name
// themselves; sibling worktrees are untouched.
func runParallelWorker(runCtx context.Context, req ParallelRequest, plan *parallelPlan, emit parallelEmit, sem chan struct{}, res *TaskResult) {
	task := plan.byID[res.TaskID]
	iso := plan.isoOf[res.TaskID]
	select {
	case sem <- struct{}{}:
	case <-runCtx.Done():
		res.Outcome = TaskCancelled
		res.Reason = "run cancelled while queued (backpressure: bounded concurrency)"
		return
	}
	defer func() { <-sem }()
	dlg := "dlg_" + plan.runID + "_" + sanitizeParallelSegment(res.TaskID, false)
	agentExec := "agent_" + plan.runID + "_" + sanitizeParallelSegment(res.TaskID, false)
	emit(observability.ExecEvent{
		AgentExecutionID: agentExec, DelegationID: dlg,
		Type: observability.ExecDelegationRequest, Status: observability.ExecRunning,
		Payload: map[string]any{
			"to": iso.Agent, "task": res.TaskID,
			"scopes": iso.Scopes, "branch": iso.Branch, "path": iso.Path,
		},
	})
	emit(observability.ExecEvent{
		TaskID: "parallel/" + res.TaskID, AgentExecutionID: agentExec,
		Type: observability.ExecTaskStart, Status: observability.ExecStarted,
		Payload: map[string]any{"task": res.TaskID, "branch": iso.Branch},
	})
	t0 := plan.now()
	outcome, reason := runOneWorker(runCtx, req, task, iso)
	dur := plan.now().Sub(t0).Milliseconds()
	if dur < 0 {
		dur = 0
	}
	res.Outcome, res.Reason, res.DurationMS = outcome, reason, dur
	st := taskOutcomeStatus(outcome)
	emit(observability.ExecEvent{
		TaskID: "parallel/" + res.TaskID, AgentExecutionID: agentExec,
		Type: observability.ExecTaskEnd, Status: st,
		Payload:      map[string]any{"task": res.TaskID, "outcome": outcome},
		ErrorDetails: reason,
	})
	emit(observability.ExecEvent{
		AgentExecutionID: agentExec, DelegationID: dlg,
		Type: observability.ExecDelegationResult, Status: st,
		Payload: map[string]any{"to": iso.Agent, "task": res.TaskID, "outcome": outcome},
	})
}

// runOneWorker executes one isolated worker: create its worktree,
// run its command inside it with the per-task timeout, and classify
// the verdict. A worktree failure, a command failure, a timeout and
// a cancellation each name themselves; sibling worktrees are
// untouched.
func runOneWorker(ctx context.Context, req ParallelRequest, task ParallelTask, iso WorkerIsolation) (string, string) {
	if err := req.Ops.Create(ctx, req.RepoRoot, req.Base, iso.Branch, iso.Path); err != nil {
		if ctx.Err() != nil {
			return TaskCancelled, "cancelled before worktree creation: " + ctx.Err().Error()
		}
		return TaskFailed, "worktree: " + err.Error()
	}
	workCtx := ctx
	cancel := func() {}
	if req.Budgets.PerTaskTimeoutMS > 0 {
		workCtx, cancel = context.WithTimeout(ctx, time.Duration(req.Budgets.PerTaskTimeoutMS)*time.Millisecond)
		defer cancel()
	}
	err := req.Work(workCtx, task, iso.Path)
	if err == nil {
		return TaskPassed, ""
	}
	if workCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		return TaskTimedOut, fmt.Sprintf("timeout after %dms: %v", req.Budgets.PerTaskTimeoutMS, err)
	}
	if ctx.Err() != nil || workCtx.Err() == context.Canceled {
		return TaskCancelled, "cancelled: " + err.Error()
	}
	return TaskFailed, err.Error()
}

// mergeParallelBranches integrates every worker branch in
// deterministic order, one at a time. The first conflict (or merge
// error) stops the line: the outcome names the review state and
// later branches stay blocked and named.
func mergeParallelBranches(runCtx context.Context, req ParallelRequest, plan *parallelPlan, emit parallelEmit) ([]MergeOutcome, bool) {
	merges := make([]MergeOutcome, 0, len(plan.order))
	blocked := ""
	for _, id := range plan.order {
		m := MergeOutcome{TaskID: id, Branch: plan.branchOf[id]}
		if blocked != "" {
			m.Error = "blocked by conflict in " + blocked + " (integrator review first)"
			merges = append(merges, m)
			continue
		}
		conflicts, merr := req.Ops.MergeBranch(runCtx, req.RepoRoot, plan.branchOf[id], plan.base)
		if merr != nil {
			m.Error = merr.Error()
			merges = append(merges, m)
			blocked = id + " (merge error)"
			emit(observability.ExecEvent{
				TaskID: "parallel/" + id, Type: observability.ExecGate,
				Status:       observability.ExecFailed,
				Payload:      map[string]any{"merge": plan.branchOf[id], "base": plan.base},
				ErrorDetails: m.Error,
			})
			continue
		}
		if len(conflicts) > 0 {
			m.Conflict, m.NeedsReview, m.Reviewer = true, true, "integrator"
			m.ConflictFiles = append([]string(nil), conflicts...)
			merges = append(merges, m)
			blocked = id
			emit(observability.ExecEvent{
				TaskID: "parallel/" + id, Type: observability.ExecGate,
				Status: observability.ExecFailed,
				Payload: map[string]any{
					"merge": plan.branchOf[id], "base": plan.base,
					"conflicts": conflicts, "reviewer": "integrator",
				},
				ErrorDetails: "merge conflict: integrator review required",
			})
			continue
		}
		m.Merged = true
		merges = append(merges, m)
		emit(observability.ExecEvent{
			TaskID: "parallel/" + id, Type: observability.ExecGate,
			Status:  observability.ExecSucceeded,
			Payload: map[string]any{"merge": plan.branchOf[id], "base": plan.base},
		})
	}
	conflicted := false
	for _, m := range merges {
		if m.Conflict || m.Error != "" {
			conflicted = true
		}
	}
	return merges, conflicted
}

// gateParallelRun runs the post-merge gate and records its verdict
// on the report. False keeps the worktrees for review.
func gateParallelRun(runCtx context.Context, req ParallelRequest, plan *parallelPlan, emit parallelEmit, rep *ParallelReport) bool {
	gate := req.Gate(runCtx, req.RepoRoot)
	rep.Gate = gate
	emit(observability.ExecEvent{
		TaskID: "parallel/merge", Type: observability.ExecGate,
		Status:  map[bool]string{true: observability.ExecSucceeded, false: observability.ExecFailed}[gate.Passed],
		Payload: map[string]any{"gate": gate.Detail, "task": gate.Task},
	})
	return gate.Passed
}

// cleanupParallelWorktrees drops every worker checkout and prunes
// stale metadata. Best-effort with a naming note: merge+gate already
// passed, so leftovers become orphans for `orphans --prune`, never a
// silent loss.
func cleanupParallelWorktrees(runCtx context.Context, req ParallelRequest, plan *parallelPlan, emit parallelEmit, rep *ParallelReport) {
	for _, id := range plan.order {
		if err := req.Ops.Remove(runCtx, req.RepoRoot, plan.pathOf[id]); err != nil {
			rep.CleanupErrors = append(rep.CleanupErrors, id+": "+err.Error())
			continue
		}
		_ = req.Ops.DeleteBranch(runCtx, req.RepoRoot, plan.branchOf[id])
		rep.OrphansRemoved++
	}
	if err := req.Ops.Prune(runCtx, req.RepoRoot); err != nil {
		rep.CleanupErrors = append(rep.CleanupErrors, "prune: "+err.Error())
	}
	emit(observability.ExecEvent{
		Type: observability.ExecArtifact, Status: observability.ExecSucceeded,
		Payload: map[string]any{
			"wall_ms": rep.WallMS, "aggregate_ms": rep.AggregateMS,
			"merged": len(plan.order), "gate": rep.Gate.Detail,
			"orphans_removed": rep.OrphansRemoved,
		},
	})
}

// RenderParallelReport renders one parallel run as human text:
// plan, per-worker verdicts, merges, gate and the wall-vs-aggregate
// receipt. Deterministic: same report, same bytes (the trace hint
// prints last as recording metadata).
func RenderParallelReport(rep ParallelReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "parallel %s: run %s base %s\n", rep.Version, rep.RunID, rep.Base)
	fmt.Fprintf(&b, "plan: %d task(s) in %d level(s)", len(rep.Order), len(rep.Levels))
	for i, lv := range rep.Levels {
		fmt.Fprintf(&b, " L%d[%s]", i+1, strings.Join(lv, ","))
	}
	fmt.Fprintf(&b, "\n")
	if len(rep.Serialized) > 0 {
		pairs := make([]string, 0, len(rep.Serialized))
		for _, p := range rep.Serialized {
			pairs = append(pairs, p[0]+"~"+p[1])
		}
		fmt.Fprintf(&b, "serialized (shared files): %s\n", strings.Join(pairs, ", "))
	}
	for _, r := range rep.Results {
		fmt.Fprintf(&b, "- %s: %s %dms branch=%s", r.TaskID, r.Outcome, r.DurationMS, r.Branch)
		if r.Reason != "" {
			fmt.Fprintf(&b, " (%s)", r.Reason)
		}
		fmt.Fprintf(&b, "\n")
	}
	for _, m := range rep.Merges {
		switch {
		case m.Merged:
			fmt.Fprintf(&b, "merge %s: ok (%s)\n", m.TaskID, m.Branch)
		case m.Conflict:
			fmt.Fprintf(&b, "merge %s: CONFLICT %s files=[%s] reviewer=%s\n",
				m.TaskID, m.Branch, strings.Join(m.ConflictFiles, ", "), m.Reviewer)
		default:
			fmt.Fprintf(&b, "merge %s: skipped (%s)\n", m.TaskID, m.Error)
		}
	}
	if rep.Gate.Ran {
		fmt.Fprintf(&b, "gate: %s (%s)\n",
			map[bool]string{true: "pass", false: "FAIL"}[rep.Gate.Passed], rep.Gate.Detail)
	} else if !rep.DryRun {
		fmt.Fprintf(&b, "gate: not run\n")
	}
	if rep.DryRun {
		if rep.AggregateMS == 0 {
			fmt.Fprintf(&b, "ceiling: sin estimaciones (wall=? aggregate=?)\n")
		} else {
			fmt.Fprintf(&b, "ceiling: wall ~%dms aggregate ~%dms over %d task(s)\n",
				rep.WallMS, rep.AggregateMS, len(rep.Order))
		}
		fmt.Fprintf(&b, "dry-run: plan no ejecutado, nada registrado\n")
	} else {
		speedup := 0.0
		if rep.WallMS > 0 {
			speedup = float64(rep.AggregateMS) / float64(rep.WallMS)
		}
		fmt.Fprintf(&b, "receipt: wall %dms aggregate %dms speedup %.2fx\n", rep.WallMS, rep.AggregateMS, speedup)
	}
	if len(rep.CleanupErrors) > 0 {
		fmt.Fprintf(&b, "cleanup: %d note(s): %s (prune with: regulus orchestrate orphans --prune)\n",
			len(rep.CleanupErrors), strings.Join(rep.CleanupErrors, "; "))
	} else if !rep.DryRun && !rep.Failed {
		fmt.Fprintf(&b, "cleanup: %d worktree(s) removed\n", rep.OrphansRemoved)
	}
	if rep.Cancelled {
		fmt.Fprintf(&b, "cancelled: run cancelled (dependents skipped, worktrees preserved)\n")
	}
	if rep.Failed {
		fmt.Fprintf(&b, "result: FAILED (worktrees preserved for review)\n")
	} else if !rep.DryRun {
		fmt.Fprintf(&b, "result: ok\n")
	}
	if rep.TraceHint != "" {
		fmt.Fprintf(&b, "traza: %s\n", rep.TraceHint)
	}
	return b.String()
}
