// Executor tests (F13.3/NOV-106): ledger, waves, idempotency,
// concurrency, collisions, timeout, offline routing, cancel,
// budgets, deadlock, resume, supervised mode, dry-run, integration
// verification and the sequential-vs-parallel benchmark.
//
// The scheduler seam is the in-memory backend (no repo, no git):
// every test is deterministic and multi-host safe.
package execute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

func testMission(t *testing.T, root, id string) {
	t.Helper()
	now := time.Now().UTC()
	m, err := mission.NewMission(mission.NewInput{ID: id, Objective: "Given a mission When it executes Then every task integrates"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.CreateMission(root, m, now); err != nil {
		t.Fatal(err)
	}
	path := []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval, mission.StateExecuting}
	_, err = mission.UpdateMission(root, id, now, func(mm *mission.Mission) error {
		for _, s := range path {
			if _, err := mm.Transition(s, "test", "test advance", "", now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mkTask(id string, files, deps []string, autonomy string) planning.PlanTask {
	if autonomy == "" {
		autonomy = planning.AutonomyAutonomous
	}
	return planning.PlanTask{
		ID: id, Title: "task " + id, Objective: "do " + id, Deliverable: id + " done",
		DependsOn: deps, Scope: planning.Scope{Files: files, Components: []string{"pkg"}},
		Agent: "anchored", RequiredCapabilities: []string{"tools", "filesystem"},
		MinScopes:    []string{"spec:read", "memory:read"},
		Verification: planning.Verification{Strategy: "tests", Criteria: []string{"Given x When y Then z 1"}},
		Estimate:     planning.Estimate{Complexity: "low", Risk: "medium", TimeMS: 100, Tokens: 10, Note: planning.EstimateNote},
		Autonomy:     autonomy,
	}
}

func savePlan(t *testing.T, root, missionID string, version int, status string, tasks []planning.PlanTask) {
	t.Helper()
	p := &planning.Plan{
		Version: planning.PlanVersion, PlanVersion: version, MissionID: missionID,
		Objective: "Given a mission When it executes Then every task integrates",
		Kind:      planning.KindSimple, Status: status,
		CreatedAt: time.Now().UTC(), Tasks: tasks,
	}
	if _, err := planning.SavePlan(root, p); err != nil {
		t.Fatal(err)
	}
}

func testOpts(root, missionID string, ops *memOps) RunOptions {
	return RunOptions{
		Root: root, MissionID: missionID,
		MaxConcurrency: 4, MaxQueued: orchestrate.MaxParallelTasks,
		WorktreeRoot: "mem", Base: "HEAD",
		Ops:  ops,
		Work: func(ctx context.Context, task orchestrate.ParallelTask, dir string) error { return nil },
		Gate: func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
			return orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "test gate"}
		},
		Now: time.Now,
	}
}

func mustRun(t *testing.T, ctx context.Context, o RunOptions) WaveReport {
	t.Helper()
	rep, err := Run(ctx, o)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return rep
}

func TestTaskTransitionLaw(t *testing.T) {
	if err := CheckTaskTransition(TaskPending, TaskIntegrated); err == nil {
		t.Fatal("pending -> integrated must refuse (only through the wave)")
	}
	if err := CheckTaskTransition(TaskIntegrated, TaskReady); err == nil {
		t.Fatal("integrated must never reopen")
	}
	if err := CheckTaskTransition(TaskPending, TaskPending); err != nil {
		t.Fatalf("same-state must be a no-op: %v", err)
	}
	if err := CheckTaskTransition("bogus", TaskReady); err == nil {
		t.Fatal("unknown source must refuse")
	}
}

func TestLedgerSealedAndTamperEvident(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	if _, err := UpdateLedger(root, "msn_test", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "a"}, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLedger(root, "msn_test")
	if err != nil {
		t.Fatal(err)
	}
	if l.Tasks["t1"].State != TaskPending {
		t.Fatalf("seeded state = %q, want pending", l.Tasks["t1"].State)
	}
	dir, _ := mission.StoreDir(root, "msn_test")
	raw, _ := os.ReadFile(filepath.Join(dir, ExecutionFile))
	if err := os.WriteFile(filepath.Join(dir, ExecutionFile), append(raw, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLedger(root, "msn_test"); err == nil || !strings.Contains(err.Error(), "sha mismatch") {
		t.Fatalf("tampered ledger must fail closed with sha mismatch, got %v", err)
	}
}

func TestIndependentTasksRunParallel(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_par")
	savePlan(t, root, "msn_par", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, nil, ""),
	})
	var current, maxSeen atomic.Int64
	o := testOpts(root, "msn_par", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		c := current.Add(1)
		for {
			m := maxSeen.Load()
			if c <= m || maxSeen.CompareAndSwap(m, c) {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
		current.Add(-1)
		return nil
	}
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskIntegrated || rep.States["t2"] != TaskIntegrated {
		t.Fatalf("states = %v, want both integrated", rep.States)
	}
	if maxSeen.Load() < 2 {
		t.Fatalf("max concurrency = %d, want >= 2 (independent tasks must overlap)", maxSeen.Load())
	}
	_, m, _ := mission.LoadMission(root, "msn_par")
	if m.State != mission.StateValidating {
		t.Fatalf("mission = %s, want validating (everything integrated)", m.State)
	}
}

func TestDependentTasksWait(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_dep")
	savePlan(t, root, "msn_dep", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, []string{"t1"}, ""),
	})
	var mu sync.Mutex
	var events []string
	o := testOpts(root, "msn_dep", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		mu.Lock()
		events = append(events, "start:"+task.ID)
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		events = append(events, "end:"+task.ID)
		mu.Unlock()
		return nil
	}
	rep := mustRun(t, context.Background(), o)
	if rep.States["t2"] != TaskIntegrated {
		t.Fatalf("states = %v, want t2 integrated", rep.States)
	}
	pos := map[string]int{}
	for i, e := range events {
		pos[e] = i
	}
	if pos["end:t1"] > pos["start:t2"] {
		t.Fatalf("t2 started before t1 ended: %v", events)
	}
}

func TestCollisionSerializes(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_col")
	savePlan(t, root, "msn_col", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"pkg/foo.go"}, nil, ""),
		mkTask("t2", []string{"pkg/foo.go"}, nil, ""),
	})
	var mu sync.Mutex
	var events []string
	o := testOpts(root, "msn_col", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		mu.Lock()
		events = append(events, "start:"+task.ID)
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		events = append(events, "end:"+task.ID)
		mu.Unlock()
		return nil
	}
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskIntegrated || rep.States["t2"] != TaskIntegrated {
		t.Fatalf("states = %v, want both integrated (serialized, never conflicted silently)", rep.States)
	}
	pos := map[string]int{}
	for i, e := range events {
		pos[e] = i
	}
	if pos["end:t1"] > pos["start:t2"] && pos["end:t2"] > pos["start:t1"] {
		t.Fatalf("colliding tasks overlapped: %v", events)
	}
}

func TestTimeoutFailsAlone(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_to")
	savePlan(t, root, "msn_to", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, nil, ""),
	})
	o := testOpts(root, "msn_to", newMemOps())
	o.PerTaskTimeoutMS = 100
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		if task.ID == "t1" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		}
		return nil
	}
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskFailed {
		t.Fatalf("t1 = %s, want failed (timeout)", rep.States["t1"])
	}
	if rep.States["t2"] != TaskIntegrated {
		t.Fatalf("t2 = %s, want integrated (sibling untouched by the timeout)", rep.States["t2"])
	}
	if !strings.Contains(strings.ToLower(ledgerReason(t, root, "msn_to", "t1")), "timeout") {
		t.Fatal("timeout reason must name itself")
	}
	_, m, _ := mission.LoadMission(root, "msn_to")
	if m.State == mission.StateValidating {
		t.Fatal("partial wave must not advance the mission to validating")
	}
}

func ledgerReason(t *testing.T, root, missionID, taskID string) string {
	t.Helper()
	l, err := LoadLedger(root, missionID)
	if err != nil {
		t.Fatal(err)
	}
	return l.Tasks[taskID].Reason
}

func TestUnroutableBlocksWithDependents(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_off")
	bad := mkTask("t1", []string{"a.go"}, nil, "")
	bad.RequiredCapabilities = []string{"no-such-capability"}
	savePlan(t, root, "msn_off", 1, planning.StatusReady, []planning.PlanTask{
		bad,
		mkTask("t2", []string{"b.go"}, []string{"t1"}, ""),
		mkTask("t3", []string{"c.go"}, nil, ""),
	})
	calls := 0
	o := testOpts(root, "msn_off", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		calls++
		return nil
	}
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskBlocked || rep.States["t2"] != TaskBlocked {
		t.Fatalf("states = %v, want t1+t2 blocked (offline routing propagates)", rep.States)
	}
	if rep.States["t3"] != TaskIntegrated {
		t.Fatalf("t3 = %s, want integrated (failure isolation)", rep.States["t3"])
	}
	if calls != 1 {
		t.Fatalf("work calls = %d, want 1 (blocked tasks never dispatch)", calls)
	}
}

func TestCancelReachesWorkers(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_cx")
	savePlan(t, root, "msn_cx", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, nil, ""),
	})
	ctx, cancel := context.WithCancel(context.Background())
	o := testOpts(root, "msn_cx", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	rep, err := Run(ctx, o)
	if err != nil {
		t.Fatalf("cancelled run must return a report, got %v", err)
	}
	for _, id := range []string{"t1", "t2"} {
		if rep.States[id] != TaskCancelled {
			t.Fatalf("%s = %s, want cancelled (cancel reaches workers and descendants)", id, rep.States[id])
		}
	}
}

func TestBudgetsRefuseBeforeDispatch(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	// Routing admits agents on their declared estimates (60s/4000
	// tokens): the mission budget fits them but not the wave
	// aggregate, so the refusal names the executor budget gate.
	m, err := mission.NewMission(mission.NewInput{ID: "msn_bud", Objective: "Given x When y Then z 1", Budget: mission.Budget{TimeMS: 70000, Tokens: 5000}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.CreateMission(root, m, now); err != nil {
		t.Fatal(err)
	}
	_, err = mission.UpdateMission(root, "msn_bud", now, func(mm *mission.Mission) error {
		for _, s := range []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval, mission.StateExecuting} {
			if _, err := mm.Transition(s, "test", "t", "", now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	big := mkTask("t1", []string{"a.go"}, nil, "")
	big.Estimate.TimeMS = 100000
	savePlan(t, root, "msn_bud", 1, planning.StatusReady, []planning.PlanTask{big})
	calls := 0
	o := testOpts(root, "msn_bud", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		calls++
		return nil
	}
	if _, err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("over-budget wave must refuse naming the budget, got %v", err)
	}
	if calls != 0 {
		t.Fatal("refused wave must dispatch nothing")
	}
}

func TestCycleRefusesWholeWave(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_cyc")
	savePlan(t, root, "msn_cyc", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, []string{"t2"}, ""),
		mkTask("t2", []string{"b.go"}, []string{"t1"}, ""),
	})
	calls := 0
	o := testOpts(root, "msn_cyc", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		calls++
		return nil
	}
	if _, err := Run(context.Background(), o); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic plan must refuse naming the cycle, got %v", err)
	}
	if calls != 0 {
		t.Fatal("deadlocked wave must dispatch nothing")
	}
}

func TestResumeNeverDuplicates(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_re")
	savePlan(t, root, "msn_re", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, nil, ""),
	})
	var mu sync.Mutex
	counts := map[string]int{}
	o := testOpts(root, "msn_re", newMemOps())
	o.Work = func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		mu.Lock()
		counts[task.ID]++
		n := counts[task.ID]
		mu.Unlock()
		if task.ID == "t2" && n == 1 {
			return errors.New("simulated worker failure")
		}
		return nil
	}
	rep1 := mustRun(t, context.Background(), o)
	if rep1.States["t1"] != TaskIntegrated || rep1.States["t2"] != TaskFailed {
		t.Fatalf("wave1 = %v, want t1 integrated + t2 failed", rep1.States)
	}
	rep2 := mustRun(t, context.Background(), o)
	if rep2.States["t2"] != TaskIntegrated {
		t.Fatalf("wave2 = %v, want t2 integrated after retry", rep2.States)
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["t1"] != 1 {
		t.Fatalf("t1 executed %d times, want 1 (resume never re-dispatches sealed work)", counts["t1"])
	}
	if counts["t2"] != 2 {
		t.Fatalf("t2 executed %d times, want 2 (one retry, new attempt)", counts["t2"])
	}
	l, _ := LoadLedger(root, "msn_re")
	if l.Tasks["t2"].Attempt != 2 {
		t.Fatalf("t2 attempt = %d, want 2", l.Tasks["t2"].Attempt)
	}
}

func TestDispatchedOpKeyNeverRekeyed(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_key")
	savePlan(t, root, "msn_key", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
	})
	now := time.Now().UTC()
	_, err := UpdateLedger(root, "msn_key", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "anchored"}, now)
		rec := l.Tasks["t1"]
		if err := moveTask(&rec, TaskReady, "test", now); err != nil {
			return err
		}
		rec.Attempt = 1
		rec.OpKey = OpKeyFor("msn_key", 1, "t1", 1)
		if err := moveTask(&rec, TaskDispatched, "test dispatch (simulated crash before run)", now); err != nil {
			return err
		}
		l.Tasks["t1"] = rec
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	o := testOpts(root, "msn_key", newMemOps())
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskIntegrated {
		t.Fatalf("t1 = %s, want integrated (crash re-drive completes)", rep.States["t1"])
	}
	l, _ := LoadLedger(root, "msn_key")
	if got := l.Tasks["t1"].OpKey; got != OpKeyFor("msn_key", 1, "t1", 1) {
		t.Fatalf("opkey = %q, want the sealed key (never re-keyed, never double-dispatched)", got)
	}
	if l.Tasks["t1"].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1 (same attempt re-driven)", l.Tasks["t1"].Attempt)
	}
}

func TestSupervisedApprovalGate(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_sup")
	savePlan(t, root, "msn_sup", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, planning.AutonomyNeedsApproval),
	})
	o := testOpts(root, "msn_sup", newMemOps())
	o.Supervised = true
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskBlocked {
		t.Fatalf("t1 = %s, want blocked awaiting approval", rep.States["t1"])
	}
	if err := ApproveTask(root, "msn_sup", "t1", "operator", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rep2 := mustRun(t, context.Background(), o)
	if rep2.States["t1"] != TaskIntegrated {
		t.Fatalf("t1 = %s, want integrated after approval", rep2.States["t1"])
	}
	if err := ApproveTask(root, "msn_sup", "t1", "operator", time.Now().UTC()); err == nil {
		t.Fatal("approving an integrated task must refuse")
	}
}

func TestDryRunSealsNothing(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	m, _ := mission.NewMission(mission.NewInput{ID: "msn_dry", Objective: "Given x When y Then z 1"}, now)
	if _, err := mission.CreateMission(root, m, now); err != nil {
		t.Fatal(err)
	}
	_, err := mission.UpdateMission(root, "msn_dry", now, func(mm *mission.Mission) error {
		for _, s := range []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval} {
			if _, err := mm.Transition(s, "test", "t", "", now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	savePlan(t, root, "msn_dry", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
	})
	o := testOpts(root, "msn_dry", newMemOps())
	o.DryRun = true
	rep := mustRun(t, context.Background(), o)
	if !rep.DryRun {
		t.Fatal("report must carry the dry-run receipt")
	}
	l, err := LoadLedger(root, "msn_dry")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Tasks) != 0 {
		t.Fatalf("dry-run sealed %d ledger tasks, want 0", len(l.Tasks))
	}
	_, mm, _ := mission.LoadMission(root, "msn_dry")
	if mm.State != mission.StateAwaitingApproval {
		t.Fatalf("dry-run moved the mission to %s (records nothing)", mm.State)
	}
}

func TestConflictParksForReview(t *testing.T) {
	pt := mkTask("t1", []string{"pkg/a.go"}, nil, "")
	rec := TaskRecord{TaskID: "t1", State: TaskSucceeded, Result: TaskResult{Files: []string{"pkg/a.go"}}}
	merge := orchestrate.MergeOutcome{TaskID: "t1", Branch: "b", Conflict: true, ConflictFiles: []string{"pkg/a.go"}, NeedsReview: true, Reviewer: "integrator"}
	if reason := VerifyIntegration(pt, merge, rec, orchestrate.GateOutcome{Ran: true, Passed: true}); !strings.Contains(reason, "integrator") {
		t.Fatalf("conflict must park for integrator review, got %q", reason)
	}
}

func TestOwnershipViolationParks(t *testing.T) {
	pt := mkTask("t1", []string{"pkg/a.go"}, nil, "")
	rec := TaskRecord{TaskID: "t1", State: TaskSucceeded, Result: TaskResult{Files: []string{"other/evil.go"}}}
	merge := orchestrate.MergeOutcome{TaskID: "t1", Branch: "b", Merged: true}
	if reason := VerifyIntegration(pt, merge, rec, orchestrate.GateOutcome{Ran: true, Passed: true}); !strings.Contains(reason, "outside its scope") {
		t.Fatalf("out-of-scope files must park for review, got %q", reason)
	}
}

func TestGateFailureParks(t *testing.T) {
	pt := mkTask("t1", []string{"pkg/a.go"}, nil, "")
	rec := TaskRecord{TaskID: "t1", State: TaskSucceeded, Result: TaskResult{Files: []string{"pkg/a.go"}}}
	merge := orchestrate.MergeOutcome{TaskID: "t1", Branch: "b", Merged: true}
	gate := orchestrate.GateOutcome{Ran: true, Passed: false, Detail: "diff-check failed"}
	if reason := VerifyIntegration(pt, merge, rec, gate); !strings.Contains(reason, "gate failed") {
		t.Fatalf("failed gate must park, got %q", reason)
	}
}

func TestApprovedPlanRefusals(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_ref")
	savePlan(t, root, "msn_ref", 1, planning.StatusRefused, nil)
	if _, _, err := LoadApprovedPlan(root, "msn_ref", 0, true); err == nil {
		t.Fatal("refused plan must never dispatch")
	}
	savePlan(t, root, "msn_ref", 2, planning.StatusNeedsApproval, []planning.PlanTask{mkTask("t1", []string{"a.go"}, nil, "")})
	if _, _, err := LoadApprovedPlan(root, "msn_ref", 2, false); err == nil {
		t.Fatal("needs-approval plan without --approve must refuse")
	}
	if _, _, err := LoadApprovedPlan(root, "msn_ref", 2, true); err != nil {
		t.Fatalf("approved plan must load: %v", err)
	}
}

func TestBenchEquivalent(t *testing.T) {
	tasks := []orchestrate.ParallelTask{
		{ID: "t1", Title: "t1", Files: []string{"a.go"}},
		{ID: "t2", Title: "t2", Files: []string{"b.go"}},
		{ID: "t3", Title: "t3", Files: []string{"c.go"}},
	}
	rep, err := Benchmark(context.Background(), BenchOptions{
		Tasks: tasks, ParallelN: 3, Now: time.Now,
		Work: func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(60 * time.Millisecond):
				return nil
			}
		},
	})
	if err != nil {
		t.Fatalf("Benchmark: %v", err)
	}
	if !rep.Equivalent {
		t.Fatal("same work both modes must read as equivalent")
	}
	if rep.ParallelWall > rep.SequentialWall {
		t.Fatalf("parallel=%dms over sequential=%dms (no speedup measured)", rep.ParallelWall, rep.SequentialWall)
	}
	t.Logf("bench: %s", RenderBench(rep))
}

func TestCancelExecutionParksLedger(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_cx2")
	savePlan(t, root, "msn_cx2", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
	})
	o := testOpts(root, "msn_cx2", newMemOps())
	rep := mustRun(t, context.Background(), o)
	if rep.States["t1"] != TaskIntegrated {
		t.Fatalf("t1 = %s, want integrated", rep.States["t1"])
	}
	if err := CancelExecution(root, "msn_cx2", "operator halt", "op", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, m, _ := mission.LoadMission(root, "msn_cx2")
	if m.State != mission.StateCancelled {
		t.Fatalf("mission = %s, want cancelled", m.State)
	}
	l, _ := LoadLedger(root, "msn_cx2")
	if l.Tasks["t1"].State != TaskIntegrated {
		t.Fatalf("integrated work must survive cancel, got %s", l.Tasks["t1"].State)
	}
}
