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

// parallelRunFunc runs the scheduler; stubbed in tests so wet runs
// never touch git or shells.
var parallelRunFunc = func(ctx context.Context, req orchestrate.ParallelRequest) (orchestrate.ParallelReport, error) {
	return orchestrate.RunParallel(ctx, req)
}

// parallelMintRunID mints the wet-run id; stubbed in tests.
var parallelMintRunID = observability.MintRunID

// parallelNewOps builds the worktree backend; stubbed in tests.
var parallelNewOps = func() orchestrate.WorktreeOps { return orchestrate.GitWorktreeOps{} }

// parallelDiffCheckRun runs `git diff --check`; stubbed in tests.
var parallelDiffCheckRun = func(ctx context.Context, repoRoot string) (string, string, int, error) {
	return gates.ShRunner{Dir: repoRoot}.Run(ctx, "git diff --check")
}

// orphanOpsFunc builds the orphan backend; stubbed in tests so
// listing and pruning never touch git.
var orphanOpsFunc = func() orchestrate.WorktreeOps { return orchestrate.GitWorktreeOps{} }

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
	if parallelHasFlagValue(opts) {
		fmt.Fprintln(stderr, parallelUsage())
		return orchestrateParallelOptions{}, 2, false
	}
	return opts, 0, true
}

// parallelHasFlagValue refuses values that look like flags.
func parallelHasFlagValue(opts orchestrateParallelOptions) bool {
	for _, f := range []string{opts.tasks, opts.out, opts.worktreeRoot} {
		if strings.HasPrefix(f, "-") {
			return true
		}
	}
	return false
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

// resolveParallelWorktreeRoot defaults the worktree root.
func resolveParallelWorktreeRoot(repoRoot, opt string) string {
	if opt != "" {
		return opt
	}
	return filepath.Join(repoRoot, ".regulus", "orchestrate", "worktrees")
}

// resolveParallelBudgets defaults the admission bound.
func resolveParallelBudgets(opts orchestrateParallelOptions) orchestrate.ParallelBudgets {
	budgets := orchestrate.ParallelBudgets{
		MaxConcurrency: opts.maxConcurrency, PerTaskTimeoutMS: opts.perTaskTimeoutMS,
		WallBudgetMS: opts.wallBudgetMS, MaxQueued: opts.maxQueued,
	}
	if opts.maxQueued == 0 {
		budgets.MaxQueued = orchestrate.MaxParallelTasks
	}
	return budgets
}

// resolveParallelRunID names the run: dry runs plan only.
func resolveParallelRunID(dryRun bool, mint func() (string, error)) (string, error) {
	if dryRun {
		return "dry-run", nil
	}
	return mint()
}

// buildParallelRequest wires one scheduler request.
func buildParallelRequest(io IO, opts orchestrateParallelOptions, tasks []orchestrate.ParallelTask) (orchestrate.ParallelRequest, int, bool) {
	runID, err := resolveParallelRunID(opts.dryRun, parallelMintRunID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", err)
		return orchestrate.ParallelRequest{}, 1, false
	}
	req := orchestrate.ParallelRequest{
		Tasks: tasks, Budgets: resolveParallelBudgets(opts), Base: opts.base, RunID: runID,
		BranchPrefix: opts.branchPrefix, WorktreeRoot: resolveParallelWorktreeRoot(io.Dir, opts.worktreeRoot),
		RepoRoot: io.Dir, Parent: mcp.PrincipalFor(io.Dir), DryRun: opts.dryRun,
	}
	if opts.dryRun {
		return req, 0, true
	}
	req.Ops = parallelNewOps()
	req.Work = parallelShellWork
	req.Gate = io.parallelGate(opts.gateTask)
	req.Record = io.parallelRecorder(io.Dir)
	return req, 0, true
}

// shouldWriteParallelReport reports whether the report persists.
func shouldWriteParallelReport(opts orchestrateParallelOptions) bool {
	return !opts.dryRun && opts.out != ""
}

// finishParallelReport persists, prints and maps the exit.
func (io IO) finishParallelReport(rep orchestrate.ParallelReport, opts orchestrateParallelOptions) int {
	if shouldWriteParallelReport(opts) {
		if code := writeParallelReport(opts.out, rep, io.Stderr); code != 0 {
			return code
		}
		fmt.Fprintf(io.Stdout, "orchestrate: parallel report recorded in %s\n", opts.out)
	}
	printParallelReport(io, rep, opts.asJSON)
	return parallelExitFor(io, rep)
}

// parallelExitFor maps a finished report to its exit code.
func parallelExitFor(io IO, rep orchestrate.ParallelReport) int {
	if len(rep.CleanupErrors) > 0 {
		fmt.Fprintf(io.Stderr, "orchestrate parallel: %d worktree(s) left for `regulus orchestrate orphans --prune`\n", len(rep.CleanupErrors))
	}
	if rep.Failed {
		fmt.Fprintln(io.Stderr, "orchestrate parallel: run failed (worktrees preserved for review)")
		return 1
	}
	return 0
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
	req, code, ok := buildParallelRequest(io, opts, tasks)
	if !ok {
		return code
	}
	rep, err := parallelRunFunc(context.Background(), req)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate parallel: %v\n", err)
		return 1
	}
	return io.finishParallelReport(rep, opts)
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
			return runParallelDiffCheck(ctx, repoRoot)
		}
		return runParallelGauntlet(ctx, repoRoot, gateTask, version, runner)
	}
}

// runParallelDiffCheck runs the lightweight merge gate.
func runParallelDiffCheck(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
	out, errb, exit, err := parallelDiffCheckRun(ctx, repoRoot)
	return diffCheckOutcome(out, errb, exit, err)
}

// diffCheckOutcome maps a diff-check run onto its verdict.
func diffCheckOutcome(out, errb string, exit int, err error) orchestrate.GateOutcome {
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

// runParallelGauntlet runs the named gauntlet task.
func runParallelGauntlet(ctx context.Context, repoRoot, gateTask, version string, runner gates.CmdRunner) orchestrate.GateOutcome {
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

// orphansOptions carries the parsed `orphans` flags.
type orphansOptions struct {
	prefix string
	active string
	prune  bool
	asJSON bool
}

// parseOrphansFlags parses the `orphans` flags.
func parseOrphansFlags(args []string, stderr io.Writer) (orphansOptions, int, bool) {
	var opts orphansOptions
	fs := flag.NewFlagSet("orchestrate orphans", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.prefix, "branch-prefix", orchestrate.DefaultParallelBranchPrefix, "worker branch prefix to sweep")
	fs.StringVar(&opts.active, "active", "", "comma-separated worker branches to protect (in-flight runs)")
	fs.BoolVar(&opts.prune, "prune", false, "remove orphans (list only by default)")
	fs.BoolVar(&opts.asJSON, "json", false, "print orphans as JSON")
	if err := fs.Parse(args); err != nil {
		return orphansOptions{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, parallelUsage())
		return orphansOptions{}, 2, false
	}
	return opts, 0, true
}

// activeBranchSet protects in-flight branches.
func activeBranchSet(active string) map[string]bool {
	keep := map[string]bool{}
	for _, b := range splitList(active) {
		keep[b] = true
	}
	return keep
}

// listParallelOrphans lists abandoned worker checkouts.
func (io IO) listParallelOrphans(ops orchestrate.WorktreeOps, keep map[string]bool, prefix string) ([]orchestrate.WorktreeEntry, int, bool) {
	entries, err := ops.List(context.Background(), io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", err)
		return nil, 1, false
	}
	return orchestrate.FindParallelOrphans(entries, keep, prefix), 0, true
}

// printOrphanList prints the list-only report.
func printOrphanList(io IO, orphans []orchestrate.WorktreeEntry, prefix string, asJSON bool) int {
	if asJSON {
		raw, merr := json.MarshalIndent(orphans, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "orphans: %d worktree(s) under %q\n", len(orphans), prefix+"/")
	for _, o := range orphans {
		fmt.Fprintf(io.Stdout, "- %s branch=%s\n", o.Path, o.Branch)
	}
	return 0
}

// pruneParallelOrphans removes abandoned worker checkouts.
func (io IO) pruneParallelOrphans(ops orchestrate.WorktreeOps, keep map[string]bool, prefix string) ([]string, int, bool) {
	removed, err := orchestrate.PruneParallelOrphans(context.Background(), ops, io.Dir, keep, prefix)
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate orphans: %v\n", err)
		return nil, 1, false
	}
	return removed, 0, true
}

// printPrunedOrphans prints the prune report.
func printPrunedOrphans(io IO, removed []string, asJSON bool) int {
	if asJSON {
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

// OrchestrateOrphansCmd implements `regulus orchestrate orphans`.
func (io IO) OrchestrateOrphansCmd(args []string) int {
	opts, code, ok := parseOrphansFlags(args, io.Stderr)
	if !ok {
		return code
	}
	ops := orphanOpsFunc()
	keep := activeBranchSet(opts.active)
	if !opts.prune {
		orphans, code, ok := io.listParallelOrphans(ops, keep, opts.prefix)
		if !ok {
			return code
		}
		return printOrphanList(io, orphans, opts.prefix, opts.asJSON)
	}
	removed, code, ok := io.pruneParallelOrphans(ops, keep, opts.prefix)
	if !ok {
		return code
	}
	return printPrunedOrphans(io, removed, opts.asJSON)
}
