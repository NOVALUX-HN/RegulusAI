// Parallel tests (F12.2/NOV-97): real concurrency of independent
// tasks, serialization of colliding tasks, worker-failure isolation,
// merge with gates and traceability, orphans and dry-run silence.
package orchestrate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

// fakeOps simulates git worktrees without a repo: Create makes the
// branch, Remove drops it, MergeBranch replays scripted conflicts.
type fakeOps struct {
	mu         sync.Mutex
	created    []string
	removed    []string
	deleted    []string
	branches   map[string]string // branch -> path
	conflicts  map[string][]string
	mergeErr   map[string]error
	mergeOrder []string
	pruned     int
	createErr  error
}

func newFakeOps() *fakeOps {
	return &fakeOps{branches: map[string]string{}, conflicts: map[string][]string{}, mergeErr: map[string]error{}}
}

func (f *fakeOps) Create(ctx context.Context, repoRoot, base, branch, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.branches[branch]; ok {
		return fmt.Errorf("branch %s already exists", branch)
	}
	f.branches[branch] = path
	f.created = append(f.created, branch)
	return nil
}

func (f *fakeOps) Remove(ctx context.Context, repoRoot, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, path)
	for b, p := range f.branches {
		if p == path {
			delete(f.branches, b)
		}
	}
	return nil
}

func (f *fakeOps) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, branch)
	delete(f.branches, branch)
	return nil
}

func (f *fakeOps) List(ctx context.Context, repoRoot string) ([]WorktreeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []WorktreeEntry
	for b, p := range f.branches {
		out = append(out, WorktreeEntry{Path: p, Branch: b, Commit: "abc"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (f *fakeOps) MergeBranch(ctx context.Context, repoRoot, branch, base string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mergeOrder = append(f.mergeOrder, branch)
	if err, ok := f.mergeErr[branch]; ok {
		return nil, err
	}
	return append([]string(nil), f.conflicts[branch]...), nil
}

func (f *fakeOps) Prune(ctx context.Context, repoRoot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned++
	return nil
}

func testParent() mcp.Principal {
	return mcp.Principal{
		Agent: "tester", Host: "h", User: "u", Workspace: "/repo",
		Scopes: map[string]bool{
			"memory:read": true, "memory:write": true, "gate:execute": true,
		},
	}
}

func passGate() GateOutcome { return GateOutcome{Ran: true, Passed: true, Detail: "diff-check clean"} }

func okWork(ctx context.Context, task ParallelTask, dir string) error { return nil }

// Two independent tasks must overlap in flight: each waits until the
// other has started before finishing.
func TestParallelIndependentOverlap(t *testing.T) {
	ops := newFakeOps()
	var started atomic.Int32
	release := make(chan struct{})
	work := func(ctx context.Context, task ParallelTask, dir string) error {
		if started.Add(1) == 2 {
			close(release)
		} else {
			select {
			case <-release:
			case <-time.After(5 * time.Second):
				return errors.New("sibling never started: no real parallelism")
			}
		}
		return nil
	}
	var mu sync.Mutex
	var events []observability.ExecEvent
	req := ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A", Files: []string{"a.txt"}},
			{ID: "b", Title: "b", Files: []string{"b.txt"}},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "HEAD",
		RunID:        "run_overlap",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         work,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
		Record:       func(e observability.ExecEvent) { mu.Lock(); events = append(events, e); mu.Unlock() },
	}
	rep, err := RunParallel(context.Background(), req)
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if rep.Failed {
		t.Fatalf("independent tasks must pass: %+v", rep)
	}
	if len(rep.Levels) != 1 || len(rep.Levels[0]) != 2 {
		t.Fatalf("independent tasks share one level, got %v", rep.Levels)
	}
	for _, r := range rep.Results {
		if r.Outcome != TaskPassed {
			t.Fatalf("task %s: outcome = %s (%s)", r.TaskID, r.Outcome, r.Reason)
		}
		if !strings.HasPrefix(r.Branch, "parallel/run_overlap/") {
			t.Fatalf("branch must carry prefix/run/task, got %q", r.Branch)
		}
	}
	if len(rep.Merges) != 2 {
		t.Fatalf("both workers must merge, got %v", rep.Merges)
	}
	if !rep.Gate.Passed {
		t.Fatalf("gate must pass: %+v", rep.Gate)
	}
	if rep.AggregateMS < 0 || rep.WallMS < 0 {
		t.Fatalf("wall/aggregate must be measured, got %+v", rep)
	}
	// Traceability: one run id everywhere, task ends for both.
	seen := map[string]bool{}
	for _, e := range events {
		if e.RunID != "run_overlap" {
			t.Fatalf("every event carries the run id, got %q", e.RunID)
		}
		if e.Type == observability.ExecTaskEnd {
			seen[e.TaskID] = true
		}
	}
	if !seen["parallel/a"] || !seen["parallel/b"] {
		t.Fatalf("both workers must close a task event, got %v", seen)
	}
	// Workers inherit restricted scopes, never more than the parent.
	for _, r := range rep.Results {
		for _, s := range r.Scopes {
			if !testParent().Scopes[s] {
				t.Fatalf("worker scope %q widens the parent (F11.8)", s)
			}
		}
	}
	if ops.pruned != 1 || len(ops.removed) != 2 {
		t.Fatalf("success removes every worktree and prunes, removed=%v pruned=%d", ops.removed, ops.pruned)
	}
}

// Colliding tasks serialize: one level per task in sorted id order,
// and the second never overlaps the first.
func TestParallelCollisionsSerialize(t *testing.T) {
	ops := newFakeOps()
	var mu sync.Mutex
	var order []string
	work := func(ctx context.Context, task ParallelTask, dir string) error {
		mu.Lock()
		order = append(order, "start-"+task.ID)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		order = append(order, "end-"+task.ID)
		mu.Unlock()
		return nil
	}
	req := ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "b", Title: "B", Files: []string{"shared.txt"}},
			{ID: "a", Title: "A", Files: []string{"shared.txt"}},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "HEAD",
		RunID:        "run_serial",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         work,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
	}
	rep, err := RunParallel(context.Background(), req)
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if rep.Failed {
		t.Fatalf("serialized tasks must pass: %+v", rep)
	}
	if len(rep.Levels) != 2 {
		t.Fatalf("colliding tasks need two levels, got %v", rep.Levels)
	}
	if rep.Levels[0][0] != "a" || rep.Levels[1][0] != "b" {
		t.Fatalf("collision order is sorted id order, got %v", rep.Levels)
	}
	if len(rep.Serialized) != 1 || rep.Serialized[0] != [2]string{"a", "b"} {
		t.Fatalf("report must name the serialized pair, got %v", rep.Serialized)
	}
	want := []string{"start-a", "end-a", "start-b", "end-b"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("no overlap allowed, got %v want %v", order, want)
	}
}

// Explicit dependencies order levels even without collisions.
func TestParallelExplicitDepsOrder(t *testing.T) {
	levels, _, err := PlanParallelLevels([]ParallelTask{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B", DependsOn: []string{"a"}},
		{ID: "c", Title: "C", DependsOn: []string{"b"}},
	})
	if err != nil {
		t.Fatalf("PlanParallelLevels: %v", err)
	}
	if len(levels) != 3 || levels[0][0] != "a" || levels[1][0] != "b" || levels[2][0] != "c" {
		t.Fatalf("chain must level one per task, got %v", levels)
	}
}

// Cycles fail closed naming the loop.
func TestParallelCycleRefused(t *testing.T) {
	_, _, err := PlanParallelLevels([]ParallelTask{
		{ID: "a", Title: "A", DependsOn: []string{"b"}},
		{ID: "b", Title: "B", DependsOn: []string{"a"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle must fail naming the loop, got %v", err)
	}
	if _, err := RunParallel(context.Background(), ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A", DependsOn: []string{"nope"}},
		},
		Budgets: DefaultParallelBudgets(),
		RunID:   "run_bad",
		DryRun:  true,
	}); err == nil {
		t.Fatal("unknown dependency must fail")
	}
}

// A failing worker never corrupts its sibling: the sibling still
// passes, dependents skip with a naming reason, and the merge never
// runs (worktrees preserved for review).
func TestParallelFailureIsolated(t *testing.T) {
	ops := newFakeOps()
	work := func(ctx context.Context, task ParallelTask, dir string) error {
		if task.ID == "bad" {
			return errors.New("boom")
		}
		return nil
	}
	req := ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "bad", Title: "Bad", Files: []string{"bad.txt"}},
			{ID: "good", Title: "Good", Files: []string{"good.txt"}},
			{ID: "child", Title: "Child", DependsOn: []string{"bad"}},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "HEAD",
		RunID:        "run_fail",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         work,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
	}
	rep, err := RunParallel(context.Background(), req)
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if !rep.Failed {
		t.Fatalf("a worker failure must fail the run: %+v", rep)
	}
	byID := map[string]TaskResult{}
	for _, r := range rep.Results {
		byID[r.TaskID] = r
	}
	if byID["good"].Outcome != TaskPassed {
		t.Fatalf("sibling must still pass, got %+v", byID["good"])
	}
	if byID["bad"].Outcome != TaskFailed {
		t.Fatalf("failing worker must fail, got %+v", byID["bad"])
	}
	if byID["child"].Outcome != TaskSkipped || !strings.Contains(byID["child"].Reason, "bad") {
		t.Fatalf("dependent must skip naming the failed dependency, got %+v", byID["child"])
	}
	if len(rep.Merges) != 0 {
		t.Fatalf("no merge after a worker failure, got %v", rep.Merges)
	}
	if rep.Gate.Ran {
		t.Fatal("gate must not run after a worker failure")
	}
	if len(ops.removed) != 0 {
		t.Fatalf("worktrees stay preserved for review, removed=%v", ops.removed)
	}
}

// A merge conflict stops the line for an integrator review: the gate
// never runs and later branches stay blocked and named.
func TestParallelConflictNeedsReview(t *testing.T) {
	ops := newFakeOps()
	rep, err := RunParallel(context.Background(), ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A"},
			{ID: "b", Title: "B"},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "main",
		RunID:        "run_conflict",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         okWork,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
	})
	_ = rep
	_ = err
	// Script the conflict on the second merge in deterministic order.
	ops2 := newFakeOps()
	second := ParallelBranch("parallel", "run_conflict2", "b")
	ops2.conflicts[second] = []string{"shared.txt"}
	gateRan := false
	rep2, err := RunParallel(context.Background(), ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A"},
			{ID: "b", Title: "B"},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "main",
		RunID:        "run_conflict2",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops2,
		Work:         okWork,
		Gate: func(ctx context.Context, root string) GateOutcome {
			gateRan = true
			return passGate()
		},
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if !rep2.Failed {
		t.Fatalf("conflict must fail the run: %+v", rep2)
	}
	if gateRan {
		t.Fatal("gate must not run after a merge conflict")
	}
	byID := map[string]MergeOutcome{}
	for _, m := range rep2.Merges {
		byID[m.TaskID] = m
	}
	if !byID["a"].Merged {
		t.Fatalf("first branch merges clean, got %+v", byID["a"])
	}
	got := byID["b"]
	if !got.Conflict || !got.NeedsReview || got.Reviewer != "integrator" {
		t.Fatalf("conflict needs integrator review, got %+v", got)
	}
	if len(got.ConflictFiles) != 1 || got.ConflictFiles[0] != "shared.txt" {
		t.Fatalf("conflict must name files, got %+v", got)
	}
}

// A failing post-merge gate fails the run and preserves worktrees.
func TestParallelGateFailurePreserves(t *testing.T) {
	ops := newFakeOps()
	rep, err := RunParallel(context.Background(), ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A"},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 1, MaxQueued: 64},
		Base:         "HEAD",
		RunID:        "run_gatefail",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         okWork,
		Gate: func(ctx context.Context, root string) GateOutcome {
			return GateOutcome{Ran: true, Passed: false, Detail: "gofmt dirty"}
		},
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if !rep.Failed || !rep.Gate.Ran || rep.Gate.Passed {
		t.Fatalf("gate failure must fail the run: %+v", rep)
	}
	if len(ops.removed) != 0 {
		t.Fatalf("worktrees stay preserved when the gate fails, removed=%v", ops.removed)
	}
}

// Dry-run plans without side effects: no worktree calls, no trace
// records, deterministic bytes.
func TestParallelDryRunSilent(t *testing.T) {
	ops := newFakeOps()
	recorded := 0
	mk := func() (ParallelReport, error) {
		return RunParallel(context.Background(), ParallelRequest{
			Tasks: []ParallelTask{
				{ID: "a", Title: "A", Files: []string{"a.txt"}, EstimateMS: 1000},
				{ID: "b", Title: "B", Files: []string{"b.txt"}, EstimateMS: 1000},
			},
			Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
			Base:         "HEAD",
			RunID:        "run_dry",
			BranchPrefix: "parallel",
			WorktreeRoot: "/wt",
			RepoRoot:     "/repo",
			Parent:       testParent(),
			Ops:          ops,
			Work:         okWork,
			Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
			Record:       func(e observability.ExecEvent) { recorded++ },
			DryRun:       true,
		})
	}
	a, err := mk()
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	b, err := mk()
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if RenderParallelReport(a) != RenderParallelReport(b) {
		t.Fatal("dry-run reports must be byte-identical")
	}
	if !a.DryRun || a.TraceHint != "" {
		t.Fatalf("dry-run records nothing, got %+v", a)
	}
	if recorded != 0 || len(ops.created) != 0 {
		t.Fatalf("dry-run must not touch worktrees or traces (ops=%d records=%d)", len(ops.created), recorded)
	}
	if a.WallMS != 1000 || a.AggregateMS != 2000 {
		t.Fatalf("dry-run ceilings divide the aggregate by parallelism, got %+v", a)
	}
}

// Budgets: per-task timeouts time out, tiny queues refuse admission,
// wild concurrency fails closed.
func TestParallelBudgetsEnforced(t *testing.T) {
	slow := func(ctx context.Context, task ParallelTask, dir string) error {
		select {
		case <-time.After(5 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rep, err := RunParallel(context.Background(), ParallelRequest{
		Tasks:        []ParallelTask{{ID: "a", Title: "A"}},
		Budgets:      ParallelBudgets{MaxConcurrency: 1, MaxQueued: 64, PerTaskTimeoutMS: 50},
		Base:         "HEAD",
		RunID:        "run_timeout",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          newFakeOps(),
		Work:         slow,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if rep.Results[0].Outcome != TaskTimedOut {
		t.Fatalf("slow worker must time out, got %+v", rep.Results[0])
	}
	if _, err := RunParallel(context.Background(), ParallelRequest{
		Tasks: []ParallelTask{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}},
		Budgets: ParallelBudgets{
			MaxConcurrency: 1, MaxQueued: 1,
		},
		RunID:  "run_q",
		DryRun: true,
	}); err == nil || !strings.Contains(err.Error(), "queue") {
		t.Fatalf("over-queued plans must refuse (backpressure), got %v", err)
	}
	if _, err := RunParallel(context.Background(), ParallelRequest{
		Tasks:   []ParallelTask{{ID: "a", Title: "A"}},
		Budgets: ParallelBudgets{MaxConcurrency: 99, MaxQueued: 64},
		RunID:   "run_c",
		DryRun:  true,
	}); err == nil {
		t.Fatal("wild concurrency must fail closed")
	}
}

// Cancellation stops the run: queued workers never start and the
// report says so.
func TestParallelCancelStopsQueue(t *testing.T) {
	ops := newFakeOps()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep, err := RunParallel(ctx, ParallelRequest{
		Tasks: []ParallelTask{
			{ID: "a", Title: "A"},
			{ID: "b", Title: "B", DependsOn: []string{"a"}},
		},
		Budgets:      ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		Base:         "HEAD",
		RunID:        "run_cancel",
		BranchPrefix: "parallel",
		WorktreeRoot: "/wt",
		RepoRoot:     "/repo",
		Parent:       testParent(),
		Ops:          ops,
		Work:         okWork,
		Gate:         func(ctx context.Context, root string) GateOutcome { return passGate() },
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}
	if !rep.Cancelled || !rep.Failed {
		t.Fatalf("cancelled run must say so: %+v", rep)
	}
}

// Orphans: only our prefix without an active branch is swept.
func TestParallelOrphansSwept(t *testing.T) {
	entries := []WorktreeEntry{
		{Path: "/w/old-a", Branch: "parallel/run1/a"},
		{Path: "/w/keep", Branch: "parallel/run2/a"},
		{Path: "/w/user", Branch: "feature/x"},
		{Path: "/w/detached"},
	}
	active := map[string]bool{"parallel/run2/a": true}
	got := FindParallelOrphans(entries, active, "parallel")
	if len(got) != 1 || got[0].Path != "/w/old-a" {
		t.Fatalf("only abandoned workers are orphans, got %v", got)
	}
	if got := FindParallelOrphans(entries, map[string]bool{}, ""); len(got) != 2 {
		t.Fatalf("empty prefix means the default, got %v", got)
	}
}

// Hostile plans fail validation, never routing.
func TestParallelValidateHostile(t *testing.T) {
	bad := [][]ParallelTask{
		{{ID: "../x", Title: "T"}},
		{{ID: "a", Title: "T", Files: []string{"/abs"}}},
		{{ID: "a", Title: "T", Files: []string{"a/../../b"}}},
		{{ID: "a", Title: ""}},
		{{ID: "a", Title: "T", DependsOn: []string{"a"}}},
	}
	for i, tasks := range bad {
		if err := ValidateParallelTasks(tasks); err == nil {
			t.Fatalf("hostile plan %d must fail validation", i)
		}
	}
}
