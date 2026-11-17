// Sequential-vs-parallel benchmark (F13.3/NOV-106 deliverable):
// run the same wave twice — once sequential (max-concurrency 1),
// once parallel — and compare wall clocks with quality equivalence
// (same per-task outcomes). The benchmark reuses the F12.2
// scheduler for both modes: it measures scheduling, never a second
// engine. Production CLI runs it over simulated work (deterministic
// sleeps, in-memory worktrees); real-worker E2E lives in the
// package tests with fakes and, where git exists, real worktrees.
package execute

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
)

// BenchReport compares one wave in both modes.
type BenchReport struct {
	Tasks          []string          `json:"tasks"`
	SequentialWall int64             `json:"sequential_wall_ms"`
	ParallelWall   int64             `json:"parallel_wall_ms"`
	AggregateMS    int64             `json:"aggregate_ms"`
	Speedup        float64           `json:"speedup"`
	Equivalent     bool              `json:"equivalent"`
	Sequential     map[string]string `json:"sequential"`
	Parallel       map[string]string `json:"parallel"`
}

// BenchOptions carries one benchmark request: the wave tasks plus
// the work and worktree seams (nil work means instant no-op work).
type BenchOptions struct {
	Tasks      []orchestrate.ParallelTask
	Work       func(ctx context.Context, task orchestrate.ParallelTask, dir string) error
	Ops        orchestrate.WorktreeOps
	Now        func() time.Time
	ParallelN  int
	PerTaskMS  int64
	Record     func(observability.ExecEvent)
	RepoRoot   string
	BranchBase string
}

// benchWork defaults to instant no-op work.
func benchWork(o BenchOptions) func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
	if o.Work != nil {
		return o.Work
	}
	return func(ctx context.Context, task orchestrate.ParallelTask, dir string) error { return nil }
}

// runBenchMode runs one mode and returns wall clock plus outcomes.
func runBenchMode(ctx context.Context, o BenchOptions, concurrency int, runID string) (int64, map[string]string, int64, error) {
	now := o.Now
	if now == nil {
		now = time.Now
	}
	ops := o.Ops
	if ops == nil {
		ops = newMemOps()
	}
	root := o.RepoRoot
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	base := o.BranchBase
	if strings.TrimSpace(base) == "" {
		base = "HEAD"
	}
	n := o.ParallelN
	if concurrency == 1 || n <= 0 {
		n = concurrency
	}
	t0 := now()
	rep, err := orchestrate.RunParallel(ctx, orchestrate.ParallelRequest{
		Tasks: o.Tasks,
		Budgets: orchestrate.ParallelBudgets{
			MaxConcurrency: n, PerTaskTimeoutMS: o.PerTaskMS, MaxQueued: orchestrate.MaxParallelTasks,
		},
		Base: base, RunID: runID, BranchPrefix: "bench",
		WorktreeRoot: "mem-bench", RepoRoot: root,
		Parent: mcp.PrincipalFor(root), Ops: ops,
		Work: benchWork(o),
		Gate: func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
			return orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "bench: simulated gate"}
		},
		Record: o.Record, DryRun: false,
	})
	if err != nil {
		return 0, nil, 0, err
	}
	wall := now().Sub(t0).Milliseconds()
	if wall < 0 {
		wall = 0
	}
	out := map[string]string{}
	for _, r := range rep.Results {
		out[r.TaskID] = r.Outcome
	}
	return wall, out, rep.AggregateMS, nil
}

// Benchmark runs the wave sequential then parallel and compares:
// same tasks, same outcomes (quality equivalence), wall clocks and
// speedup. Outcomes differing between modes report Equivalent=false
// with the naming detail — never a passing claim over divergent
// quality.
func Benchmark(ctx context.Context, o BenchOptions) (BenchReport, error) {
	if err := orchestrate.ValidateParallelTasks(o.Tasks); err != nil {
		return BenchReport{}, err
	}
	ids := make([]string, 0, len(o.Tasks))
	for _, t := range o.Tasks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	rep := BenchReport{Tasks: ids, Sequential: map[string]string{}, Parallel: map[string]string{}}
	wall1, out1, agg, err := runBenchMode(ctx, o, 1, "bench-seq")
	if err != nil {
		return BenchReport{}, fmt.Errorf("execute bench sequential: %w", err)
	}
	n := o.ParallelN
	if n <= 1 {
		n = 4
	}
	wallN, outN, _, err := runBenchMode(ctx, o, n, "bench-par")
	if err != nil {
		return BenchReport{}, fmt.Errorf("execute bench parallel: %w", err)
	}
	rep.Sequential, rep.Parallel = out1, outN
	rep.SequentialWall, rep.ParallelWall, rep.AggregateMS = wall1, wallN, agg
	if wallN > 0 {
		rep.Speedup = float64(wall1) / float64(wallN)
	} else if wall1 == 0 {
		rep.Speedup = 1
	}
	rep.Equivalent = true
	for _, id := range ids {
		if out1[id] != outN[id] {
			rep.Equivalent = false
		}
	}
	if !rep.Equivalent {
		return rep, fmt.Errorf("execute bench: quality diverged between modes (sequential=%v parallel=%v)", out1, outN)
	}
	return rep, nil
}

// RenderBench renders one benchmark report as human text.
func RenderBench(rep BenchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bench: %d task(s) sequential=%dms parallel=%dms aggregate=%dms speedup=%.2fx\n",
		len(rep.Tasks), rep.SequentialWall, rep.ParallelWall, rep.AggregateMS, rep.Speedup)
	fmt.Fprintf(&b, "quality: %s\n", map[bool]string{true: "equivalent (same outcomes both modes)", false: "DIVERGED (see detail)"}[rep.Equivalent])
	for _, id := range rep.Tasks {
		fmt.Fprintf(&b, "- %s: sequential=%s parallel=%s\n", id, rep.Sequential[id], rep.Parallel[id])
	}
	return b.String()
}
