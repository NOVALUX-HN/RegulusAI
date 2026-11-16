package cmdx

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/gates"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
)

func parallelUsage() string {
	return "usage: regulus orchestrate parallel --tasks <file> [--base <branch>] [--worktree-root <dir>] [--branch-prefix <p>] [--max-concurrency N] [--per-task-timeout-ms N] [--wall-budget-ms N] [--max-queued N] [--gate-task <task>] [--dry-run] [--out <dir>] [--json] | regulus orchestrate orphans [--branch-prefix <p>] [--active <b,c>] [--prune] [--json]"
}

// orchestrateParallelOptions carries the parsed `parallel` flags.
type orchestrateParallelOptions struct {
	tasks            string
	base             string
	worktreeRoot     string
	branchPrefix     string
	maxConcurrency   int
	perTaskTimeoutMS int64
	wallBudgetMS     int64
	maxQueued        int
	gateTask         string
	dryRun           bool
	out              string
	asJSON           bool
}

// parseOrchestrateParallelFlags parses the `parallel` flags.
func parseOrchestrateParallelFlags(args []string, stderr io.Writer) (orchestrateParallelOptions, int, bool) {
	var opts orchestrateParallelOptions
	fs := flag.NewFlagSet("orchestrate parallel", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.tasks, "tasks", "", "JSON task file (array of parallel tasks, required)")
	fs.StringVar(&opts.base, "base", "HEAD", "base branch or commit workers branch from")
	fs.StringVar(&opts.worktreeRoot, "worktree-root", "", "worktree root (default <repo>/.regulus/orchestrate/worktrees)")
	fs.StringVar(&opts.branchPrefix, "branch-prefix", orchestrate.DefaultParallelBranchPrefix, "worker branch prefix (orphan sweeper only touches this prefix)")
	fs.IntVar(&opts.maxConcurrency, "max-concurrency", 4, "workers in flight 1..8")
	fs.Int64Var(&opts.perTaskTimeoutMS, "per-task-timeout-ms", 0, "per-worker wall ceiling in ms (0 = unset)")
	fs.Int64Var(&opts.wallBudgetMS, "wall-budget-ms", 0, "whole-run wall ceiling in ms (0 = unset)")
	fs.IntVar(&opts.maxQueued, "max-queued", 0, "admission bound (0 = plan maximum; smaller refuses over-queued plans)")
	fs.StringVar(&opts.gateTask, "gate-task", "", "post-merge gauntlet task (regulus gate run <task>); empty runs the lightweight merge gate (git diff --check)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "plan and print, create nothing, record nothing")
	fs.StringVar(&opts.out, "out", "", "write the report JSON into this directory (wet runs only)")
	fs.BoolVar(&opts.asJSON, "json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return orchestrateParallelOptions{}, 2, false
	}
	if fs.NArg() != 0 || opts.tasks == "" {
		fmt.Fprintln(stderr, parallelUsage())
		return orchestrateParallelOptions{}, 2, false
	}
	for _, f := range []string{opts.tasks, opts.out, opts.worktreeRoot} {
		if strings.HasPrefix(f, "-") {
			fmt.Fprintln(stderr, parallelUsage())
			return orchestrateParallelOptions{}, 2, false
		}
	}
	return opts, 0, true
}

// loadParallelTasks reads one tasks file.
func loadParallelTasks(path string, stderr io.Writer) ([]orchestrate.ParallelTask, int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: %v\n", err)
		return nil, 1, false
	}
	if len(raw) > 1024*1024 {
		fmt.Fprintln(stderr, "orchestrate parallel: tasks file too large (max 1MB)")
		return nil, 1, false
	}
	var tasks []orchestrate.ParallelTask
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tasks); err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: malformed tasks file: %v\n", err)
		return nil, 1, false
	}
	if err := orchestrate.ValidateParallelTasks(tasks); err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: %v\n", err)
		return nil, 1, false
	}
	return tasks, 0, true
}

// OrchestrateParallelCmd implements `regulus orchestrate parallel`.
func (io IO) OrchestrateParallelCmd(args []string) int {
	opts, code, ok := parseOrchestrateParallelFlags(args, io.Stderr)
	if !ok {
		return code
	}
	tasks, code, ok := loadParallelTasks(opts.tasks, io.Stderr)
	if !ok {
		return code
	}
	repoRoot := io.Dir
	worktreeRoot := opts.worktreeRoot
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(repoRoot, ".regulus", "orchestrate", "worktrees")
	}
	runID := "dry-run"
	if !opts.dryRun {
		minted, merr := observability.MintRunID()
		if merr != nil {
			fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", merr)
			return 1
		}
		runID = minted
	}
	budgets := orchestrate.ParallelBudgets{
		MaxConcurrency: opts.maxConcurrency, PerTaskTimeoutMS: opts.perTaskTimeoutMS,
		WallBudgetMS: opts.wallBudgetMS, MaxQueued: opts.maxQueued,
	}
	if opts.maxQueued == 0 {
		budgets.MaxQueued = orchestrate.MaxParallelTasks
	}
	req := orchestrate.ParallelRequest{
		Tasks: tasks, Budgets: budgets, Base: opts.base, RunID: runID,
		BranchPrefix: opts.branchPrefix, WorktreeRoot: worktreeRoot,
		RepoRoot: repoRoot, Parent: mcp.PrincipalFor(repoRoot), DryRun: opts.dryRun,
	}
	if opts.dryRun {
		rep, err := orchestrate.RunParallel(context.Background(), req)
		if err != nil {
			fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", err)
			return 1
		}
		printParallelReport(io, rep, opts.asJSON)
		return 0
	}
	req.Ops = orchestrate.GitWorktreeOps{}
	req.Work = parallelShellWork
	req.Gate = io.parallelGate(opts.gateTask)
	req.Record = io.parallelRecorder(repoRoot)
	rep, err := orchestrate.RunParallel(context.Background(), req)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", err)
		return 1
	}
	if opts.out != "" {
		if code := writeParallelReport(opts.out, rep, io.Stderr); code != 0 {
			return code
		}
		fmt.Fprintf(io.Stdout, "orchestrate: parallel report recorded in %s\n", opts.out)
	}
	printParallelReport(io, rep, opts.asJSON)
	if len(rep.CleanupErrors) > 0 {
		fmt.Fprintf(io.Stderr, "orchestrate parallel: %d worktree(s) left for `regulus orchestrate orphans --prune`\n", len(rep.CleanupErrors))
	}
	if rep.Failed {
		fmt.Fprintln(io.Stderr, "orchestrate parallel: run failed (worktrees preserved for review)")
		return 1
	}
	return 0
}

// parallelShellWork runs one worker command inside its worktree.
func parallelShellWork(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
	if strings.TrimSpace(task.Command) == "" {
		return nil
	}
	out, errb, exit, err := gates.ShRunner{Dir: dir}.Run(ctx, task.Command)
	if err != nil {
		return fmt.Errorf("task %s: %v", task.ID, err)
	}
	if exit != 0 {
		return fmt.Errorf("task %s: command exited %d (stdout %.500q stderr %.500q)", task.ID, exit, out, errb)
	}
	return nil
}

// parallelGate wires the post-merge gate.
func (io IO) parallelGate(gateTask string) func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
	version := io.Version
	runner := io.Runner
	return func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
		if strings.TrimSpace(gateTask) == "" {
			out, errb, exit, err := gates.ShRunner{Dir: repoRoot}.Run(ctx, "git diff --check")
			if err != nil {
				return orchestrate.GateOutcome{Ran: true, Detail: fmt.Sprintf("diff-check could not run: %v", err)}
			}
			if exit != 0 {
				detail := strings.TrimSpace(out + "\n" + errb)
				if len(detail) > 500 {
					detail = detail[:500]
				}
				return orchestrate.GateOutcome{Ran: true, Detail: "diff-check failed: " + detail}
			}
			return orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "diff-check clean"}
		}
		cfg, err := gates.Load(filepath.Join(repoRoot, "regulus.json"))
		if err != nil {
			return orchestrate.GateOutcome{Ran: true, Task: gateTask, Detail: fmt.Sprintf("gate load: %v", err)}
		}
		cfgBytes, err := os.ReadFile(filepath.Join(repoRoot, "regulus.json"))
		if err != nil {
			return orchestrate.GateOutcome{Ran: true, Task: gateTask, Detail: fmt.Sprintf("gate read: %v", err)}
		}
		res, _, err := gates.RunTask(ctx, cfg, cfgBytes, repoRoot, gateTask, version, "parallel", "", runner)
		if err != nil {
			return orchestrate.GateOutcome{Ran: true, Task: gateTask, Detail: fmt.Sprintf("gate run: %v", err)}
		}
		if res.Failed {
			return orchestrate.GateOutcome{Ran: true, Task: gateTask,
				Detail: fmt.Sprintf("gauntlet failed at %q (%d/%d passed)", res.FailedGate, res.Passed, res.Executed)}
		}
		return orchestrate.GateOutcome{Ran: true, Passed: true, Task: gateTask,
			Detail: fmt.Sprintf("gauntlet %d/%d passed", res.Passed, res.Executed)}
	}
}

// parallelRecorder links the run to its F11.1 timeline.
func (io IO) parallelRecorder(repoRoot string) func(observability.ExecEvent) {
	stderr := io.Stderr
	return func(e observability.ExecEvent) {
		if _, _, err := observability.RecordExec(repoRoot, e, time.Now().UTC()); err != nil {
			fmt.Fprintf(stderr, "orchestrate parallel: evento de traza no registrado (%v)\n", err)
		}
	}
}

// printParallelReport prints one report.
func printParallelReport(io IO, rep orchestrate.ParallelReport, asJSON bool) {
	if asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", err)
			return
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return
	}
	fmt.Fprint(io.Stdout, orchestrate.RenderParallelReport(rep))
}

// writeParallelReport stores one report as JSON.
func writeParallelReport(dir string, rep orchestrate.ParallelReport, stderr io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: %v\n", err)
		return 1
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "parallel-report.json"), append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "orchestrate parallel: %v\n", err)
		return 1
	}
	return 0
}

// OrchestrateOrphansCmd implements `regulus orchestrate orphans`.
func (io IO) OrchestrateOrphansCmd(args []string) int {
	fs := flag.NewFlagSet("orchestrate orphans", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	prefix := fs.String("branch-prefix", orchestrate.DefaultParallelBranchPrefix, "worker branch prefix to sweep")
	active := fs.String("active", "", "comma-separated worker branches to protect (in-flight runs)")
	prune := fs.Bool("prune", false, "remove orphans (list only by default)")
	asJSON := fs.Bool("json", false, "print orphans as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, parallelUsage())
		return 2
	}
	keep := map[string]bool{}
	for _, b := range splitList(*active) {
		keep[b] = true
	}
	ops := orchestrate.GitWorktreeOps{}
	entries, err := ops.List(context.Background(), io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", err)
		return 1
	}
	orphans := orchestrate.FindParallelOrphans(entries, keep, *prefix)
	if !*prune {
		if *asJSON {
			raw, merr := json.MarshalIndent(orphans, "", "  ")
			if merr != nil {
				fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", merr)
				return 1
			}
			fmt.Fprintln(io.Stdout, string(raw))
			return 0
		}
		fmt.Fprintf(io.Stdout, "orphans: %d worktree(s) under %q\n", len(orphans), *prefix+"/")
		for _, o := range orphans {
			fmt.Fprintf(io.Stdout, "- %s branch=%s\n", o.Path, o.Branch)
		}
		return 0
	}
	removed, err := orchestrate.PruneParallelOrphans(context.Background(), ops, io.Dir, keep, *prefix)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(removed, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "orphans: pruned %d worktree(s)\n", len(removed))
	for _, p := range removed {
		fmt.Fprintf(io.Stdout, "- %s\n", p)
	}
	return 0
}
