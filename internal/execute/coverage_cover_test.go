// Coverage tests for the executor leftovers (F13.3/NOV-106):
// ledger law, admission stages, routing re-validation, budget
// gates, wave sealing/integration, operator moves, mission
// advancement and the benchmark. All in-process on t.TempDir()
// with the memory worktree backend: no shells, no network,
// portable Linux+Windows.
package execute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

func covNow() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func TestCovCheckTaskTransitionTable(t *testing.T) {
	legal := [][2]string{
		{TaskPending, TaskReady}, {TaskPending, TaskBlocked}, {TaskPending, TaskCancelled},
		{TaskReady, TaskDispatched}, {TaskReady, TaskBlocked},
		{TaskDispatched, TaskRunning}, {TaskRunning, TaskSucceeded},
		{TaskRunning, TaskFailed}, {TaskRunning, TaskBlocked},
		{TaskSucceeded, TaskIntegrated}, {TaskSucceeded, TaskReady},
		{TaskFailed, TaskReady}, {TaskBlocked, TaskReady}, {TaskCancelled, TaskReady},
	}
	for _, e := range legal {
		if err := CheckTaskTransition(e[0], e[1]); err != nil {
			t.Fatalf("legal edge %q -> %q refused: %v", e[0], e[1], err)
		}
	}
	for _, e := range [][2]string{
		{TaskPending, TaskDispatched}, {TaskPending, TaskSucceeded},
		{TaskReady, TaskSucceeded}, {TaskDispatched, TaskSucceeded},
		{TaskSucceeded, TaskCancelled}, {TaskFailed, TaskIntegrated},
	} {
		if err := CheckTaskTransition(e[0], e[1]); err == nil {
			t.Fatalf("illegal edge %q -> %q allowed", e[0], e[1])
		}
	}
	if err := CheckTaskTransition(TaskIntegrated, TaskReady); err == nil {
		t.Fatal("integrated must never reopen")
	} else if !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("must name terminality, got %v", err)
	}
	if err := CheckTaskTransition("bogus", TaskReady); err == nil {
		t.Fatal("unknown source must refuse")
	}
	if err := CheckTaskTransition(TaskPending, "bogus"); err == nil {
		t.Fatal("unknown target must refuse")
	}
}

func TestCovOpKeys(t *testing.T) {
	if got := OpKeyFor("m", 2, "t", 3); got != "exec/m/v2/t/a3" {
		t.Fatalf("op key drifted: %q", got)
	}
	if got := IntegrateKeyFor("m", 2, "t"); got != "integrate/m/v2/t" {
		t.Fatalf("integrate key drifted: %q", got)
	}
	if OpKeyFor("m", 1, "t", 1) == OpKeyFor("m", 1, "t", 2) {
		t.Fatal("attempts must key separately (resume never double-dispatches)")
	}
	if IntegrateKeyFor("m", 1, "t") != IntegrateKeyFor("m", 1, "t") {
		t.Fatal("integration keys are attempt-independent (retries never re-integrate)")
	}
}

func TestCovLedgerValidationAndSeeding(t *testing.T) {
	root := t.TempDir()
	if _, err := UpdateLedger(root, "msn_cov", covNow(), func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "a"}, covNow())
		l.Tasks["bad"] = TaskRecord{TaskID: "bad", State: "bogus", PlanVersion: 1}
		return nil
	}); err == nil {
		t.Fatal("unknown ledger states must fail the seal")
	} else if !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("must name the unknown state, got %v", err)
	}
	if _, err := UpdateLedger(root, "msn_cov", covNow(), func(l *Ledger) error {
		SeedLedger(l, []string{"t1", "t2"}, 1, map[string]string{"t1": "a", "t2": "b"}, covNow())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateLedger(root, "msn_cov", covNow(), func(l *Ledger) error {
		rec := l.Tasks["t1"]
		if err := moveTask(&rec, TaskReady, "admit", covNow()); err != nil {
			return err
		}
		l.Tasks["t1"] = rec
		SeedLedger(l, []string{"t1", "t2"}, 2, map[string]string{"t1": "a", "t2": "b"}, covNow())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLedger(root, "msn_cov")
	if err != nil {
		t.Fatal(err)
	}
	if l.Tasks["t1"].State != TaskReady || l.Tasks["t1"].PlanVersion != 2 {
		t.Fatalf("replan keeps live states and tracks the newest revision: %+v", l.Tasks["t1"])
	}
	found := false
	for _, h := range l.Tasks["t1"].History {
		if strings.Contains(h, "replan v2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("replan must audit: %+v", l.Tasks["t1"].History)
	}
	if got := SortedTaskIDs(l); len(got) != 2 || got[0] != "t1" || got[1] != "t2" {
		t.Fatalf("ledger order must be deterministic, got %v", got)
	}
	if _, err := LoadLedger(root, "../escape"); err == nil {
		t.Fatal("hostile mission ids must refuse before touching the store")
	}
	fresh, err := LoadLedger(root, "msn_missing")
	if err != nil {
		t.Fatalf("missing ledger loads empty: %v", err)
	}
	if fresh.Version != ExecutionVersion || len(fresh.Tasks) != 0 {
		t.Fatalf("empty ledger shape drifted: %+v", fresh)
	}
}

func TestCovLedgerMalformedFailsClosed(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_mal")
	dir, err := mission.StoreDir(root, "msn_mal")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("{not json")
	man := ExecutionManifest{
		Version: ExecutionVersion, MissionID: "msn_mal", LedgerFile: ExecutionFile,
		Generation: 1, UpdatedAt: covNow(),
	}
	sum := sha256.Sum256(raw)
	man.LedgerSHA = hex.EncodeToString(sum[:])
	manRaw := `{"version":"` + ExecutionVersion + `","mission_id":"msn_mal","ledger_file":"` + ExecutionFile + `","ledger_sha256":"` + man.LedgerSHA + `","generation":1,"updated_at":"2026-11-16T12:00:00Z"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, ExecutionFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ExecutionMeta), []byte(manRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLedger(root, "msn_mal"); err == nil {
		t.Fatal("malformed ledger must fail closed")
	} else if !strings.Contains(err.Error(), "malformed ledger") {
		t.Fatalf("must name the malformed ledger, got %v", err)
	}
}

func TestCovMapWaveTasksSkips(t *testing.T) {
	p := &planning.Plan{Tasks: []planning.PlanTask{
		mkTask("done", []string{"a.go"}, nil, ""),
		mkTask("blocked", []string{"b.go"}, nil, ""),
		mkTask("t1", []string{"c.go"}, []string{"done", "ghost"}, ""),
	}}
	l := &Ledger{Tasks: map[string]TaskRecord{
		"done":    {TaskID: "done", State: TaskSucceeded},
		"blocked": {TaskID: "blocked", State: TaskBlocked},
		"t1":      {TaskID: "t1", State: TaskPending},
	}}
	wave := MapWaveTasks(p, l)
	if len(wave) != 1 || wave[0].ID != "t1" {
		t.Fatalf("sealed and held tasks never map, got %v", wave)
	}
	if len(wave[0].DependsOn) != 0 {
		t.Fatalf("satisfied and unknown deps drop, got %v", wave[0].DependsOn)
	}
	if wave[0].Agent != "anchored" || len(wave[0].Files) != 1 {
		t.Fatalf("scheduler task carries agent and files: %+v", wave[0])
	}
}

func TestCovHoldForApproval(t *testing.T) {
	plan := &planning.Plan{Tasks: []planning.PlanTask{
		mkTask("need", []string{"a.go"}, nil, planning.AutonomyNeedsApproval),
		mkTask("auto", []string{"b.go"}, nil, planning.AutonomyAutonomous),
	}}
	newLedger := func() *Ledger {
		return &Ledger{Tasks: map[string]TaskRecord{
			"need": {TaskID: "need", State: TaskPending},
			"auto": {TaskID: "auto", State: TaskPending},
		}}
	}
	l := newLedger()
	held := holdForApproval(l, plan, false, covNow())
	if len(held) != 1 || held[0] != "need" || l.Tasks["need"].State != TaskBlocked {
		t.Fatalf("needs-approval parks before dispatch: %v %+v", held, l.Tasks)
	}
	if l.Tasks["auto"].State != TaskPending {
		t.Fatalf("autonomous tasks stay runnable: %+v", l.Tasks["auto"])
	}
	l2 := newLedger()
	held2 := holdForApproval(l2, plan, true, covNow())
	if len(held2) != 2 {
		t.Fatalf("supervised mode holds every unapproved task, got %v", held2)
	}
	l2.Tasks["auto"] = TaskRecord{TaskID: "auto", State: TaskPending, ApprovedBy: "op"}
	held3 := holdForApproval(l2, plan, true, covNow())
	for _, id := range held3 {
		if id == "auto" {
			t.Fatal("approved tasks are never held")
		}
	}
}

func TestCovGateWaveDependencies(t *testing.T) {
	ledger := &Ledger{Tasks: map[string]TaskRecord{
		"done": {TaskID: "done", State: TaskSucceeded},
		"bad":  {TaskID: "bad", State: TaskFailed},
		"live": {TaskID: "live", State: TaskPending},
	}}
	wave := []orchestrate.ParallelTask{
		{ID: "t1", Title: "t1", DependsOn: []string{"done"}},
		{ID: "t2", Title: "t2", DependsOn: []string{"bad"}},
		{ID: "t3", Title: "t3", DependsOn: []string{"live"}},
		{ID: "t4", Title: "t4", DependsOn: []string{"ghost"}},
		{ID: "t5", Title: "t5", DependsOn: []string{"t2"}},
	}
	keep, blocked := gateWaveDependencies(wave, ledger, covNow())
	if len(keep) != 1 || keep[0].ID != "t1" {
		t.Fatalf("only satisfied deps stay runnable, kept %v blocked %v", keep, blocked)
	}
	if !strings.Contains(blocked["t2"], "is failed (propagated block") {
		t.Fatalf("failed deps propagate: %v", blocked)
	}
	if !strings.Contains(blocked["t3"], "is pending (waits") {
		t.Fatalf("live deps wait behind the DAG: %v", blocked)
	}
	if !strings.Contains(blocked["t4"], "is unknown (waits") {
		t.Fatalf("unknown deps wait: %v", blocked)
	}
	if !strings.Contains(blocked["t5"], "dependency t2 did not pass") {
		t.Fatalf("blocks propagate transitively in-wave: %v", blocked)
	}
}

func TestCovPropagateRoutingBlocksAndHelpers(t *testing.T) {
	admitted := []orchestrate.ParallelTask{{ID: "t1"}, {ID: "t2", DependsOn: []string{"t1", "ok"}}}
	keep, out := propagateRoutingBlocks(admitted, map[string]string{"t1": "offline"}, &Ledger{Tasks: map[string]TaskRecord{}}, covNow())
	if len(keep) != 0 || out["t2"] == "" || !strings.Contains(out["t2"], "dependency t1 blocked") {
		t.Fatalf("routing blocks propagate with reasons: keep=%v out=%v", keep, out)
	}
	admitted2 := []orchestrate.ParallelTask{{ID: "t3", DependsOn: []string{"t9"}}}
	keep2, out2 := propagateRoutingBlocks(admitted2, map[string]string{"t9": "offline"}, &Ledger{Tasks: map[string]TaskRecord{}}, covNow())
	if len(keep2) != 0 || !strings.Contains(out2["t3"], "dependency t9 blocked") {
		t.Fatalf("off-wave blocked deps propagate: keep=%v out=%v", keep2, out2)
	}
	if got := exceptTask(admitted, "t1"); len(got) != 1 || got[0].ID != "t2" {
		t.Fatalf("exceptTask must drop one task, got %v", got)
	}
	if got := stateOf(TaskRecord{State: TaskFailed}, true); got != TaskFailed {
		t.Fatalf("stateOf names known states, got %q", got)
	}
	if got := stateOf(TaskRecord{}, false); got != "unknown" {
		t.Fatalf("stateOf names missing ledgers, got %q", got)
	}
}

func TestCovRevalidateRoutingBranches(t *testing.T) {
	m := &mission.Mission{Risk: mission.RiskLow, Budget: mission.Budget{TimeMS: 70000, Tokens: 5000}}
	good := mkTask("t1", []string{"a.go"}, nil, "")
	good.Deliverable = "t1 done"
	p := &planning.Plan{Tasks: []planning.PlanTask{good}}
	admitted, blocked := RevalidateRouting([]orchestrate.ParallelTask{{ID: "t1", Title: "t1"}}, p, m)
	if len(admitted) != 1 || len(blocked) != 0 {
		t.Fatalf("routable tasks admit: %v %v", admitted, blocked)
	}
	admitted, blocked = RevalidateRouting([]orchestrate.ParallelTask{{ID: "ghost", Title: "g"}}, p, m)
	if len(admitted) != 0 || !strings.Contains(blocked["ghost"], "vanished") {
		t.Fatalf("vanished tasks block: %v %v", admitted, blocked)
	}
	bad := mkTask("t1", []string{"a.go"}, nil, "")
	bad.RequiredCapabilities = []string{"no-such-capability"}
	pb := &planning.Plan{Tasks: []planning.PlanTask{bad}}
	_, blocked = RevalidateRouting([]orchestrate.ParallelTask{{ID: "t1", Title: "t1"}}, pb, m)
	if !strings.Contains(blocked["t1"], "routing imposible") {
		t.Fatalf("undescribable tasks block as impossible: %v", blocked)
	}
	if got := taskComplexity(6); got != orchestrate.ComplexityHigh {
		t.Fatalf("complexity buckets: 6 files = %q", got)
	}
	if got := taskComplexity(3); got != orchestrate.ComplexityMedium {
		t.Fatalf("complexity buckets: 3 files = %q", got)
	}
	if got := taskComplexity(1); got != orchestrate.ComplexityLow {
		t.Fatalf("complexity buckets: 1 file = %q", got)
	}
	crit := mkTask("t1", []string{"a.go"}, nil, "")
	crit.Rationale = "toca paquete crítico del núcleo"
	if got := missionRisk(&mission.Mission{Risk: mission.RiskLow}, crit); got != orchestrate.RiskHigh {
		t.Fatalf("critical packages raise low risk to high, got %q", got)
	}
	plain := mkTask("t1", []string{"a.go"}, nil, "")
	if got := missionRisk(&mission.Mission{}, plain); got != orchestrate.RiskMedium {
		t.Fatalf("empty mission risk defaults to medium, got %q", got)
	}
	if got := missionRisk(&mission.Mission{Risk: mission.RiskHigh}, plain); got != orchestrate.RiskHigh {
		t.Fatalf("mission risk passes through, got %q", got)
	}
}

func TestCovCheckWaveBudgetsTable(t *testing.T) {
	tasks := []orchestrate.ParallelTask{{ID: "t1", Title: "t1"}}
	est := map[string]planning.Estimate{"t1": {TimeMS: 100, Tokens: 10, Cost: 5, CostKnown: true}}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{}, mission.Budget{TimeMS: 50}); err == nil {
		t.Fatal("per-task time ceilings must gate")
	} else if !strings.Contains(err.Error(), "per-task ceiling") {
		t.Fatalf("must name the per-task ceiling, got %v", err)
	}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{}, mission.Budget{Tokens: 5}); err == nil {
		t.Fatal("per-task token ceilings must gate")
	}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{}, mission.Budget{Cost: 1}); err == nil {
		t.Fatal("per-task known cost must gate")
	}
	unknown := map[string]planning.Estimate{"t1": {TimeMS: 100, Tokens: 10, Cost: 9999}}
	if err := CheckWaveBudgets(tasks, unknown, mission.Budget{}, mission.Budget{Cost: 1}); err != nil {
		t.Fatalf("unknown money never gates: %v", err)
	}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{TimeMS: 50}, mission.Budget{}); err == nil {
		t.Fatal("global time ceilings must gate")
	} else if !strings.Contains(err.Error(), "global budget") {
		t.Fatalf("must name the global budget, got %v", err)
	}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{Tokens: 5}, mission.Budget{}); err == nil {
		t.Fatal("global token ceilings must gate")
	}
	if err := CheckWaveBudgets(tasks, est, mission.Budget{}, mission.Budget{}); err != nil {
		t.Fatalf("unset budgets never gate: %v", err)
	}
}

func TestCovLoadApprovedPlanBranches(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_term")
	savePlan(t, root, "msn_term", 1, planning.StatusRefused, nil)
	if _, _, err := LoadApprovedPlan(root, "msn_term", 0, true); err == nil {
		t.Fatal("refused plans never dispatch")
	}
	savePlan(t, root, "msn_term", 2, planning.StatusNeedsClarification, []planning.PlanTask{mkTask("t1", []string{"a.go"}, nil, "")})
	if _, _, err := LoadApprovedPlan(root, "msn_term", 0, true); err == nil {
		t.Fatal("clarification plans are not executable")
	} else if !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("must name executability, got %v", err)
	}
	savePlan(t, root, "msn_term", 3, planning.StatusReady, []planning.PlanTask{mkTask("t1", []string{"a.go"}, nil, "")})
	_, p, err := LoadApprovedPlan(root, "msn_term", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if p.PlanVersion != 3 {
		t.Fatalf("explicit versions load, got v%d", p.PlanVersion)
	}
	if _, _, err := LoadApprovedPlan(root, "msn_term", 99, false); err == nil {
		t.Fatal("missing revisions must fail")
	}
	// Terminal missions refuse before plans even load.
	if _, err := mission.UpdateMission(root, "msn_term", covNow(), func(m *mission.Mission) error {
		for _, s := range []string{mission.StateIntegrating, mission.StateValidating, mission.StateCompleted} {
			if _, err := m.Transition(s, "test", "t", "", covNow()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadApprovedPlan(root, "msn_term", 0, true); err == nil {
		t.Fatal("terminal missions never execute")
	} else if !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("must name terminality, got %v", err)
	}
}

func TestCovRunEmptyWaveShortCircuits(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_empty")
	savePlan(t, root, "msn_empty", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
	})
	now := covNow()
	if _, err := UpdateLedger(root, "msn_empty", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "anchored"}, now)
		rec := l.Tasks["t1"]
		for _, next := range []string{TaskReady, TaskDispatched, TaskRunning, TaskSucceeded, TaskIntegrated} {
			if err := moveTask(&rec, next, "test seal", now); err != nil {
				return err
			}
		}
		l.Tasks["t1"] = rec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	o := testOpts(root, "msn_empty", newMemOps())
	rep, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RunIDs) != 0 || rep.States["t1"] != TaskIntegrated {
		t.Fatalf("sealed waves short-circuit the scheduler: %+v", rep)
	}
}

func TestCovRunNeedsOps(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_ops")
	savePlan(t, root, "msn_ops", 1, planning.StatusReady, []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
	})
	o := RunOptions{Root: root, MissionID: "msn_ops", MaxConcurrency: 2, Now: time.Now}
	if _, err := Run(context.Background(), o); err == nil {
		t.Fatal("wet runs without a backend must fail closed")
	} else if !strings.Contains(err.Error(), "no worktree backend") {
		t.Fatalf("must name the missing backend, got %v", err)
	}
}

func TestCovEnsureExecuting(t *testing.T) {
	root := t.TempDir()
	now := covNow()
	m, err := mission.NewMission(mission.NewInput{ID: "msn_ens", Objective: "Given x When y Then z 1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.CreateMission(root, m, now); err != nil {
		t.Fatal(err)
	}
	if err := ensureExecuting(root, "msn_ens", "op", now); err == nil {
		t.Fatal("created missions do not execute")
	}
	_, err = mission.UpdateMission(root, "msn_ens", now, func(mm *mission.Mission) error {
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
	if err := ensureExecuting(root, "msn_ens", "op", now); err != nil {
		t.Fatal(err)
	}
	_, loaded, err := mission.LoadMission(root, "msn_ens")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != mission.StateExecuting {
		t.Fatalf("approved plans start executing, got %s", loaded.State)
	}
	if err := ensureExecuting(root, "msn_ens", "op", now); err != nil {
		t.Fatalf("live missions continue untouched: %v", err)
	}
}

func TestCovConcurrencyBaseRootsSanitize(t *testing.T) {
	if concurrencyOf(RunOptions{}) != 4 {
		t.Fatal("zero concurrency means the scheduler default")
	}
	if concurrencyOf(RunOptions{MaxConcurrency: 2}) != 2 {
		t.Fatal("explicit concurrency wins")
	}
	if baseOf(RunOptions{}) != "HEAD" || baseOf(RunOptions{Base: "main"}) != "main" {
		t.Fatal("merge base defaults to HEAD")
	}
	if worktreeRootOf(RunOptions{}) != ".regulus/execute/worktrees" {
		t.Fatalf("worktree root default drifted: %q", worktreeRootOf(RunOptions{}))
	}
	if sanitizeSegment("a b!") != "a-b" {
		t.Fatalf("segments fold to the branch alphabet, got %q", sanitizeSegment("a b!"))
	}
	if sanitizeSegment("") != "mission" {
		t.Fatal("empty segments fold to mission")
	}
}

func TestCovOutcomeToLedgerTable(t *testing.T) {
	for outcome, want := range map[string]string{
		orchestrate.TaskPassed:    TaskSucceeded,
		orchestrate.TaskFailed:    TaskFailed,
		orchestrate.TaskTimedOut:  TaskFailed,
		orchestrate.TaskCancelled: TaskCancelled,
		orchestrate.TaskSkipped:   TaskBlocked,
		"bogus":                   TaskBlocked,
	} {
		if got := outcomeToLedger(outcome); got != want {
			t.Fatalf("outcome %q maps to %q, want %q", outcome, got, want)
		}
	}
}

func TestCovVerifyIntegrationBranches(t *testing.T) {
	pt := mkTask("t1", []string{"pkg/a.go"}, nil, "")
	rec := TaskRecord{TaskID: "t1", State: TaskSucceeded, Result: TaskResult{Files: []string{"pkg/a.go"}}}
	merged := orchestrate.MergeOutcome{TaskID: "t1", Branch: "b", Merged: true}
	gate := orchestrate.GateOutcome{Ran: true, Passed: true}
	if reason := VerifyIntegration(pt, merged, rec, gate); reason != "" {
		t.Fatalf("clean integrations seal, got %q", reason)
	}
	if reason := VerifyIntegration(pt, merged, rec, orchestrate.GateOutcome{}); reason != "" {
		t.Fatalf("unrun gates never block: %q", reason)
	}
	broken := orchestrate.MergeOutcome{TaskID: "t1", Branch: "b", Error: "disk gone"}
	if reason := VerifyIntegration(pt, broken, rec, gate); !strings.Contains(reason, "did not run") {
		t.Fatalf("merge errors park with a name: %q", reason)
	}
	emptyRec := TaskRecord{TaskID: "t1", State: TaskSucceeded}
	emptyPT := mkTask("t1", nil, nil, "")
	if reason := VerifyIntegration(emptyPT, merged, emptyRec, gate); reason != "" {
		t.Fatalf("receipt-less custom workers check merge and gate only: %q", reason)
	}
	if out := outOfScope([]string{"pkg/a.go", "pkg/a.go", "other/e.go"}, []string{"pkg"}); len(out) != 1 || out[0] != "other/e.go" {
		t.Fatalf("ownership check dedupes and sorts: %v", out)
	}
	if out := outOfScope([]string{"pkg"}, []string{"pkg/a.go"}); len(out) != 0 {
		t.Fatalf("parent paths own their scope: %v", out)
	}
	if out := outOfScope([]string{"x.go"}, nil); len(out) != 1 {
		t.Fatalf("empty scope with touched files is a violation: %v", out)
	}
}

func TestCovSealWaveCancelledFromDispatched(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_seal")
	now := covNow()
	if _, err := UpdateLedger(root, "msn_seal", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1", "t2"}, 1, map[string]string{"t1": "a", "t2": "a"}, now)
		for _, id := range []string{"t1", "t2"} {
			rec := l.Tasks[id]
			for _, next := range []string{TaskReady, TaskDispatched} {
				if err := moveTask(&rec, next, "test", now); err != nil {
					return err
				}
			}
			l.Tasks[id] = rec
		}
		done := l.Tasks["t2"]
		for _, next := range []string{TaskRunning, TaskSucceeded, TaskIntegrated} {
			if err := moveTask(&done, next, "test", now); err != nil {
				return err
			}
		}
		l.Tasks["t2"] = done
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	parallel := orchestrate.ParallelReport{RunID: "run_s", Results: []orchestrate.TaskResult{
		{TaskID: "t1", Outcome: orchestrate.TaskCancelled, Reason: "halted"},
		{TaskID: "t2", Outcome: orchestrate.TaskPassed},
	}}
	sealed, err := sealWaveResults(root, "msn_seal", &planning.Plan{}, parallel, map[string]TouchedFiles{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Tasks["t1"].State != TaskCancelled {
		t.Fatalf("dispatched cancellations land directly: %+v", sealed.Tasks["t1"])
	}
	if sealed.Tasks["t2"].State != TaskIntegrated {
		t.Fatalf("terminal tasks are never resealed: %+v", sealed.Tasks["t2"])
	}
	if _, err := sealWaveResults(root, "msn_seal", &planning.Plan{}, orchestrate.ParallelReport{Results: []orchestrate.TaskResult{
		{TaskID: "t1", Outcome: orchestrate.TaskPassed},
	}}, map[string]TouchedFiles{}, now); err == nil {
		t.Fatal("illegal seals fail instead of jumping the ledger")
	} else if !strings.Contains(err.Error(), "cannot seal task") {
		t.Fatalf("must name the unsealable task, got %v", err)
	}
}

func TestCovIntegrateWaveSealsAndParks(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_int")
	now := covNow()
	pt := mkTask("t1", []string{"a.go"}, nil, "")
	pt2 := mkTask("t2", []string{"b.go"}, nil, "")
	p := &planning.Plan{PlanVersion: 1, Tasks: []planning.PlanTask{pt, pt2}}
	if _, err := UpdateLedger(root, "msn_int", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1", "t2"}, 1, map[string]string{"t1": "a", "t2": "a"}, now)
		for _, id := range []string{"t1", "t2"} {
			rec := l.Tasks[id]
			for _, next := range []string{TaskReady, TaskDispatched, TaskRunning, TaskSucceeded} {
				if err := moveTask(&rec, next, "test", now); err != nil {
					return err
				}
			}
			rec.Result = TaskResult{Files: []string{map[string]string{"t1": "a.go", "t2": "b.go"}[id]}}
			l.Tasks[id] = rec
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, m, err := mission.LoadMission(root, "msn_int")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := LoadLedger(root, "msn_int")
	if err != nil {
		t.Fatal(err)
	}
	parallel := orchestrate.ParallelReport{
		RunID: "run_i",
		Merges: []orchestrate.MergeOutcome{
			{TaskID: "t1", Branch: "b1", Merged: true},
			{TaskID: "t2", Branch: "b2", Conflict: true, ConflictFiles: []string{"b.go"}, NeedsReview: true, Reviewer: "integrator"},
		},
		Gate: orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "clean"},
	}
	integrated, blocked := integrateWave(root, m, p, ledger, parallel, now)
	if len(integrated) != 1 || integrated[0] != "t1" || len(blocked) != 1 || blocked[0] != "t2" {
		t.Fatalf("clean seals, conflicted parks: integrated=%v blocked=%v", integrated, blocked)
	}
	after, err := LoadLedger(root, "msn_int")
	if err != nil {
		t.Fatal(err)
	}
	if after.Tasks["t1"].State != TaskIntegrated || after.Tasks["t2"].State != TaskBlocked {
		t.Fatalf("ledger mirrors integration: %+v", after.Tasks)
	}
	_, reloaded, err := mission.LoadMission(root, "msn_int")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Effects[IntegrateKeyFor("msn_int", 1, "t1")]; !ok {
		t.Fatalf("integration seals exactly-once: %v", reloaded.Effects)
	}
}

func TestCovApproveTaskBranches(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_appr")
	now := covNow()
	if err := ApproveTask(root, "msn_appr", "ghost", "op", now); err == nil {
		t.Fatal("unknown tasks have nothing to approve")
	} else if !strings.Contains(err.Error(), "holds no task") {
		t.Fatalf("must name the missing task, got %v", err)
	}
	if _, err := UpdateLedger(root, "msn_appr", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "a"}, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := ApproveTask(root, "msn_appr", "t1", "op", now); err == nil {
		t.Fatal("only blocked tasks approve")
	}
}

func TestCovCancelExecutionBranches(t *testing.T) {
	root := t.TempDir()
	testMission(t, root, "msn_cx3")
	if err := CancelExecution(root, "msn_cx3", "", "op", covNow()); err == nil {
		t.Fatal("cancel needs a reason")
	}
	now := covNow()
	if _, err := UpdateLedger(root, "msn_cx3", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "a"}, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := CancelExecution(root, "msn_cx3", "halt", "op", now); err != nil {
		t.Fatal(err)
	}
	l, _ := LoadLedger(root, "msn_cx3")
	if l.Tasks["t1"].State != TaskCancelled {
		t.Fatalf("cancel parks live tasks: %+v", l.Tasks["t1"])
	}
	_, m, _ := mission.LoadMission(root, "msn_cx3")
	if m.State != mission.StateCancelled {
		t.Fatalf("mission closes cancelled, got %s", m.State)
	}
}

func TestCovSpentAdvanceEmit(t *testing.T) {
	p := &planning.Plan{PlanVersion: 1, Tasks: []planning.PlanTask{
		mkTask("t1", []string{"a.go"}, nil, ""),
		mkTask("t2", []string{"b.go"}, nil, ""),
	}}
	p.Tasks[0].Estimate.Tokens = 100
	p.Tasks[1].Estimate.Tokens = 50
	l := &Ledger{Tasks: map[string]TaskRecord{
		"t1": {TaskID: "t1", State: TaskSucceeded},
		"t2": {TaskID: "t2", State: TaskIntegrated},
	}}
	if got := spentTokens(l, p); got != 150 {
		t.Fatalf("spent sums sealed estimates, got %d", got)
	}
	root := t.TempDir()
	testMission(t, root, "msn_adv")
	now := covNow()
	full := &Ledger{Tasks: map[string]TaskRecord{
		"t1": {TaskID: "t1", State: TaskIntegrated},
		"t2": {TaskID: "t2", State: TaskIntegrated},
	}}
	advanceMission(root, "msn_adv", full, p, "op", now)
	_, m, _ := mission.LoadMission(root, "msn_adv")
	if m.State != mission.StateValidating {
		t.Fatalf("everything integrated validates, got %s", m.State)
	}
	testMission(t, root, "msn_adv2")
	part := &Ledger{Tasks: map[string]TaskRecord{
		"t1": {TaskID: "t1", State: TaskIntegrated},
		"t2": {TaskID: "t2", State: TaskFailed},
	}}
	advanceMission(root, "msn_adv2", part, p, "op", now)
	_, m2, _ := mission.LoadMission(root, "msn_adv2")
	if m2.State != mission.StateIntegrating {
		t.Fatalf("first integration moves to integrating, got %s", m2.State)
	}
	var events []observability.ExecEvent
	emitWaveEvent(RunOptions{}, WaveReport{MissionID: "m", PlanVersion: 1}, false)
	emitWaveEvent(RunOptions{Record: func(e observability.ExecEvent) { events = append(events, e) }},
		WaveReport{MissionID: "m", RunIDs: []string{"run_1"}, PlanVersion: 1}, true)
	if len(events) != 1 || events[0].Status != observability.ExecCancelled {
		t.Fatalf("cancelled waves link cancelled: %+v", events)
	}
	emitWaveEvent(RunOptions{Record: func(e observability.ExecEvent) { events = append(events, e) }},
		WaveReport{MissionID: "m", PlanVersion: 1}, false)
	if events[1].Status != observability.ExecSucceeded || events[1].RunID != "m" {
		t.Fatalf("clean waves link succeeded under the mission id: %+v", events[1])
	}
}

func TestCovWorkWithLedgerTracksRunning(t *testing.T) {
	root := t.TempDir()
	now := covNow()
	if _, err := UpdateLedger(root, "msn_wl", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "a"}, now)
		rec := l.Tasks["t1"]
		for _, next := range []string{TaskReady, TaskDispatched} {
			if err := moveTask(&rec, next, "test", now); err != nil {
				return err
			}
		}
		l.Tasks["t1"] = rec
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	latch := map[string]TouchedFiles{}
	wrapped := workWithLedger(RunOptions{Root: root, MissionID: "msn_wl", Now: time.Now},
		func(ctx context.Context, task orchestrate.ParallelTask, dir string) error { return nil }, latch)
	if err := wrapped(context.Background(), orchestrate.ParallelTask{ID: "t1", Title: "t1"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	l, _ := LoadLedger(root, "msn_wl")
	if l.Tasks["t1"].State != TaskRunning {
		t.Fatalf("workers move to running on entry: %+v", l.Tasks["t1"])
	}
	if len(latch) != 0 {
		t.Fatalf("receipt-less work latches nothing: %v", latch)
	}
	failing := workWithLedger(RunOptions{Root: root, MissionID: "msn_wl", Now: time.Now},
		func(ctx context.Context, task orchestrate.ParallelTask, dir string) error { return errors.New("boom") }, latch)
	if err := failing(context.Background(), orchestrate.ParallelTask{ID: "t1", Title: "t1"}, t.TempDir()); err == nil {
		t.Fatal("work errors propagate")
	}
}

func TestCovBenchDivergedAndDefaults(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	rep, err := Benchmark(context.Background(), BenchOptions{
		Tasks: []orchestrate.ParallelTask{
			{ID: "t1", Title: "t1"},
			{ID: "t2", Title: "t2"},
		},
		ParallelN: 2, Now: time.Now,
		Work: func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n <= 2 {
				return errors.New("sequential mode fails")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("divergent quality must fail the benchmark")
	} else if !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("must name the divergence, got %v", err)
	}
	out := RenderBench(rep)
	for _, want := range []string{"bench:", "quality:", "DIVERGED", "- t1:", "- t2:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("bench render must quote %q:\n%s", want, out)
		}
	}
	def, err := Benchmark(context.Background(), BenchOptions{
		Tasks: []orchestrate.ParallelTask{{ID: "t1", Title: "t1"}},
	})
	if err != nil {
		t.Fatalf("default bench work is instant no-op: %v", err)
	}
	if !def.Equivalent || len(def.Tasks) != 1 {
		t.Fatalf("default bench must read equivalent: %+v", def)
	}
	if _, err := Benchmark(context.Background(), BenchOptions{
		Tasks: []orchestrate.ParallelTask{{ID: "../x", Title: "T"}},
	}); err == nil {
		t.Fatal("hostile bench plans must refuse")
	}
}
