// Execution handlers (F13.3/NOV-106): `regulus execute ...`
// Moved-out-of-cli by construction (the F12.4 pattern): the logic
// lives here, internal/cli keeps only a thin forwarder plus init.
//
// Reuse, never duplication:
//   - waves: internal/execute (Mission Executor over the approved
//     plan and the F12.2 scheduler); this layer only parses flags,
//     wires production backends and prints;
//   - backends: orchestrate.GitWorktreeOps (real worktrees),
//     execute.ReceiptWork (scoped receipt + commit),
//     the parallel gate wiring (gauntlet task or diff-check);
//   - trace: the parallel recorder (F11.1) shares run/task ids;
//     dry-run records nothing.
package cmdx

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/execute"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

func executeUsage() string {
	return "usage: regulus execute run --id <mission> [--version N] [--approve] [--supervised] [--dry-run] [--max-concurrency N] [--per-task-timeout-ms N] [--wall-budget-ms N] [--max-queued N] [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] [--per-task-time-ms N] [--per-task-tokens N] [--per-task-cost N] [--worktree-root <dir>] [--base <b>] [--gate-task <t>] [--actor <a>] [--json] | regulus execute status --id <mission> [--json] | regulus execute approve --id <mission> --task <t> [--actor <a>] | regulus execute cancel --id <mission> --reason <r> [--actor <a>] | regulus execute bench --id <mission> [--version N] [--parallel N] [--work-ms N] [--json]"
}

// ExecuteCmd routes `regulus execute ...`.
func (io IO) ExecuteCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, executeUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	switch args[0] {
	case "run":
		return io.ExecuteRunCmd(args[1:])
	case "status":
		return io.ExecuteStatusCmd(args[1:])
	case "approve":
		return io.ExecuteApproveCmd(args[1:])
	case "cancel":
		return io.ExecuteCancelCmd(args[1:])
	case "bench":
		return io.ExecuteBenchCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "execute: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
}

// executeRunOptions carries the parsed `execute run` flags.
type executeRunOptions struct {
	id               string
	version          int
	approve          bool
	supervised       bool
	dryRun           bool
	maxConcurrency   int
	perTaskTimeoutMS int64
	wallBudgetMS     int64
	maxQueued        int
	budgetTimeMS     int64
	budgetTokens     int64
	budgetCost       float64
	perTaskTimeMS    int64
	perTaskTokens    int64
	perTaskCost      float64
	worktreeRoot     string
	base             string
	gateTask         string
	actor            string
	asJSON           bool
}

// ExecuteRunCmd implements `regulus execute run`.
func (io IO) ExecuteRunCmd(args []string) int {
	var opts executeRunOptions
	fs := flag.NewFlagSet("execute run", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	fs.StringVar(&opts.id, "id", "", "mission id (required)")
	fs.IntVar(&opts.version, "version", 0, "plan revision (default: latest)")
	fs.BoolVar(&opts.approve, "approve", false, "record the operator approval for a needs-approval plan")
	fs.BoolVar(&opts.supervised, "supervised", false, "every task needs an explicit approval before dispatch")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "level, check and print without dispatching or sealing anything")
	fs.IntVar(&opts.maxConcurrency, "max-concurrency", 4, "workers in flight 1..8")
	fs.Int64Var(&opts.perTaskTimeoutMS, "per-task-timeout-ms", 0, "per-worker wall ceiling in ms (0 = unset)")
	fs.Int64Var(&opts.wallBudgetMS, "wall-budget-ms", 0, "whole-wave wall ceiling in ms (0 = unset)")
	fs.IntVar(&opts.maxQueued, "max-queued", 0, "admission bound (0 = plan maximum)")
	fs.Int64Var(&opts.budgetTimeMS, "budget-time-ms", 0, "global wall ceiling in ms (0 = mission budget)")
	fs.Int64Var(&opts.budgetTokens, "budget-tokens", 0, "global token ceiling (0 = mission budget)")
	fs.Float64Var(&opts.budgetCost, "budget-cost", 0, "global money ceiling (0 = mission budget)")
	fs.Int64Var(&opts.perTaskTimeMS, "per-task-time-ms", 0, "per-task wall ceiling in ms (0 = unset)")
	fs.Int64Var(&opts.perTaskTokens, "per-task-tokens", 0, "per-task token ceiling (0 = unset)")
	fs.Float64Var(&opts.perTaskCost, "per-task-cost", 0, "per-task money ceiling (0 = unset)")
	fs.StringVar(&opts.worktreeRoot, "worktree-root", "", "worktree root (default <repo>/.regulus/execute/worktrees)")
	fs.StringVar(&opts.base, "base", "HEAD", "merge base for worker branches")
	fs.StringVar(&opts.gateTask, "gate-task", "", "post-merge gauntlet task (empty runs git diff --check)")
	fs.StringVar(&opts.actor, "actor", "", "operator ordering this run (audit)")
	fs.BoolVar(&opts.asJSON, "json", false, "print the wave report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || opts.id == "" || strings.HasPrefix(opts.id, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	_, plan, err := execute.LoadApprovedPlan(io.Dir, opts.id, opts.version, opts.approve)
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute run: %v\n", err)
		return 1
	}
	root := filepath.Join(io.Dir, ".regulus", "execute", "worktrees")
	if opts.worktreeRoot != "" {
		root = opts.worktreeRoot
	}
	rep, err := execute.Run(context.Background(), execute.RunOptions{
		Root: io.Dir, MissionID: opts.id, PlanVersion: plan.PlanVersion,
		ApproveAll: opts.approve, Supervised: opts.supervised, DryRun: opts.dryRun,
		Actor: opts.actor, MaxConcurrency: opts.maxConcurrency,
		PerTaskTimeoutMS: opts.perTaskTimeoutMS, WallBudgetMS: opts.wallBudgetMS,
		MaxQueued: opts.maxQueued, GlobalTimeMS: opts.budgetTimeMS,
		GlobalTokensMS: opts.budgetTokens, GlobalCost: opts.budgetCost,
		PerTaskTimeMS: opts.perTaskTimeMS, PerTaskTokens: opts.perTaskTokens,
		PerTaskCost: opts.perTaskCost, WorktreeRoot: root, Base: opts.base,
		GateTask: opts.gateTask, Ops: orchestrate.GitWorktreeOps{},
		Work: execute.ReceiptWork(io.Dir, opts.id, plan.PlanVersion),
		Gate: io.parallelGate(opts.gateTask), Record: io.parallelRecorder(io.Dir),
		Now: time.Now,
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute run: %v\n", err)
		return 1
	}
	if opts.asJSON {
		raw, merr := json.MarshalIndent(rep, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "execute run: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return waveExitFor(rep)
	}
	fmt.Fprint(io.Stdout, renderWaveReport(rep))
	return waveExitFor(rep)
}

// waveExitFor maps a wave report to an exit code without laundering:
// an empty wave with blocked tasks still reports them and exits 1;
// a dry-run exits 0 (it refused nothing, it only planned).
func waveExitFor(rep execute.WaveReport) int {
	if rep.DryRun {
		return 0
	}
	for _, st := range rep.States {
		switch st {
		case execute.TaskFailed, execute.TaskBlocked, execute.TaskCancelled:
			return 1
		}
	}
	return 0
}

// renderWaveReport renders one wave receipt as human text.
func renderWaveReport(rep execute.WaveReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "execute: mission %s plan v%d", rep.MissionID, rep.PlanVersion)
	if rep.DryRun {
		fmt.Fprintf(&b, " dry-run (nothing dispatched, nothing sealed)\n")
	} else {
		fmt.Fprintf(&b, " runs [%s]\n", strings.Join(rep.RunIDs, ", "))
	}
	ids := make([]string, 0, len(rep.States))
	for id := range rep.States {
		ids = append(ids, id)
	}
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s: %s\n", id, rep.States[id])
	}
	if len(rep.Integrated) > 0 {
		fmt.Fprintf(&b, "integrated: %s\n", strings.Join(rep.Integrated, ", "))
	}
	if len(rep.Blocked) > 0 {
		fmt.Fprintf(&b, "blocked (review first): %s\n", strings.Join(rep.Blocked, ", "))
	}
	if rep.DryRun && rep.AggregateMS > 0 {
		fmt.Fprintf(&b, "ceiling: wall ~%dms aggregate ~%dms\n", rep.WallMS, rep.AggregateMS)
	} else if !rep.DryRun {
		fmt.Fprintf(&b, "receipt: wall %dms aggregate %dms spent-tokens(est) %d\n", rep.WallMS, rep.AggregateMS, rep.SpentTokens)
	}
	if rep.TraceHint != "" {
		fmt.Fprintf(&b, "traza: %s\n", rep.TraceHint)
	}
	return b.String()
}

// ExecuteStatusCmd implements `regulus execute status`.
func (io IO) ExecuteStatusCmd(args []string) int {
	fs := flag.NewFlagSet("execute status", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	asJSON := fs.Bool("json", false, "print the ledger as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute status: %v\n", err)
		return 1
	}
	ledger, err := execute.LoadLedger(io.Dir, *id)
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute status: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(ledger, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "execute status: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "execute: mission %s state=%s generation=%d ledger-generation=%d\n",
		m.ID, m.State, m.Generation, ledger.Generation)
	for _, tid := range execute.SortedTaskIDs(ledger) {
		rec := ledger.Tasks[tid]
		fmt.Fprintf(io.Stdout, "- %s: %s attempt=%d agent=%s op=%s\n",
			tid, rec.State, rec.Attempt, rec.Agent, rec.OpKey)
		if rec.Reason != "" {
			fmt.Fprintf(io.Stdout, "  reason: %s\n", rec.Reason)
		}
	}
	return 0
}

// ExecuteApproveCmd implements `regulus execute approve`.
func (io IO) ExecuteApproveCmd(args []string) int {
	fs := flag.NewFlagSet("execute approve", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	task := fs.String("task", "", "task id to approve (required)")
	actor := fs.String("actor", "", "operator approving (audit)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *task == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	if err := execute.ApproveTask(io.Dir, *id, *task, *actor, time.Now().UTC()); err != nil {
		fmt.Fprintf(io.Stderr, "execute approve: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "execute: %s task %s approved (re-run `execute run` to dispatch)\n", *id, *task)
	return 0
}

// ExecuteCancelCmd implements `regulus execute cancel`.
func (io IO) ExecuteCancelCmd(args []string) int {
	fs := flag.NewFlagSet("execute cancel", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	reason := fs.String("reason", "", "audit reason (required)")
	actor := fs.String("actor", "", "operator cancelling (audit)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || *reason == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	if strings.HasPrefix(*reason, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	if err := execute.CancelExecution(io.Dir, *id, *reason, *actor, time.Now().UTC()); err != nil {
		fmt.Fprintf(io.Stderr, "execute cancel: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "execute: %s cancelled (workers and descendants park as cancelled)\n", *id)
	return 0
}

// ExecuteBenchCmd implements `regulus execute bench`: the sequential
// vs parallel benchmark over the plan wave with simulated work
// (deterministic sleeps, in-memory worktrees). It measures the
// scheduling shape, never real worker output: quality equivalence
// means the same outcomes in both modes.
func (io IO) ExecuteBenchCmd(args []string) int {
	fs := flag.NewFlagSet("execute bench", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	id := fs.String("id", "", "mission id (required)")
	version := fs.Int("version", 0, "plan revision (default: latest)")
	parallel := fs.Int("parallel", 4, "parallel workers 1..8")
	workMS := fs.Int64("work-ms", 100, "simulated work per task in ms")
	asJSON := fs.Bool("json", false, "print the benchmark as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *id == "" || strings.HasPrefix(*id, "-") {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	if *parallel < 1 || *parallel > 8 || *workMS < 0 {
		fmt.Fprintln(io.Stderr, executeUsage())
		return 2
	}
	var plan *planning.Plan
	var err error
	if *version <= 0 {
		plan, err = planning.LatestPlan(io.Dir, *id)
	} else {
		plan, err = planning.LoadPlan(io.Dir, *id, *version)
	}
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute bench: %v\n", err)
		return 1
	}
	tasks := make([]orchestrate.ParallelTask, 0, len(plan.Tasks))
	for _, t := range plan.Tasks {
		tasks = append(tasks, orchestrate.ParallelTask{
			ID: t.ID, Title: t.Title, Files: append([]string(nil), t.Scope.Files...),
			DependsOn: append([]string(nil), t.DependsOn...),
		})
	}
	rep, err := execute.Benchmark(context.Background(), execute.BenchOptions{
		Tasks: tasks, ParallelN: *parallel, Now: time.Now,
		Work: func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(*workMS) * time.Millisecond):
				return nil
			}
		},
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "execute bench: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(rep, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "execute bench: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, execute.RenderBench(rep))
	return 0
}
