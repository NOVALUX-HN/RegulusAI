// Split directo del ejecutor (CRAP>30 sin cambiar comportamiento):
// helpers puros más tabla del orquestador delgado. Todo en proceso
// con el backend en memoria, portable Linux+Windows: sin red ni git.
package execute

import (
	"context"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

func splitNow() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }

func splitMission(t *testing.T, root, id string) {
	t.Helper()
	now := splitNow()
	m, err := mission.NewMission(mission.NewInput{ID: id, Objective: "Given a mission When it executes Then every task integrates"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mission.CreateMission(root, m, now); err != nil {
		t.Fatal(err)
	}
	_, err = mission.UpdateMission(root, id, now, func(mm *mission.Mission) error {
		for _, s := range []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval, mission.StateExecuting} {
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

func splitTask(id string, files, deps []string) planning.PlanTask {
	return planning.PlanTask{
		ID: id, Title: "task " + id, Objective: "do " + id, Deliverable: id + " done",
		DependsOn: deps, Scope: planning.Scope{Files: files, Components: []string{"pkg"}},
		Agent: "anchored", RequiredCapabilities: []string{"tools", "filesystem"},
		MinScopes:    []string{"spec:read", "memory:read"},
		Verification: planning.Verification{Strategy: "tests", Criteria: []string{"Given x When y Then z 1"}},
		Estimate:     planning.Estimate{Complexity: "low", Risk: "medium", TimeMS: 100, Tokens: 10, Note: planning.EstimateNote},
		Autonomy:     planning.AutonomyAutonomous,
	}
}

func splitSavePlan(t *testing.T, root, missionID string, version int, tasks []planning.PlanTask) {
	t.Helper()
	p := &planning.Plan{
		Version: planning.PlanVersion, PlanVersion: version, MissionID: missionID,
		Objective: "Given a mission When it executes Then every task integrates",
		Kind:      planning.KindSimple, Status: planning.StatusReady,
		CreatedAt: splitNow(), Tasks: tasks,
	}
	if _, err := planning.SavePlan(root, p); err != nil {
		t.Fatal(err)
	}
}

func TestSplitShortCircuitEmptyTable(t *testing.T) {
	ledger := &Ledger{Tasks: map[string]TaskRecord{
		"t1": {TaskID: "t1", State: TaskBlocked},
	}}
	wave := []orchestrate.ParallelTask{{ID: "t1", Title: "t1"}}
	for _, tc := range []struct {
		name      string
		wave      []orchestrate.ParallelTask
		opts      RunOptions
		wantShort bool
		wantWall  bool
	}{
		{name: "empty", wave: nil, opts: RunOptions{}, wantShort: true},
		{name: "dry-run", wave: wave, opts: RunOptions{DryRun: true}, wantShort: true, wantWall: true},
		{name: "wet", wave: wave, opts: RunOptions{}, wantShort: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := &WaveReport{States: map[string]string{}}
			if got := shortCircuitEmpty(rep, tc.wave, ledger, tc.opts); got != tc.wantShort {
				t.Fatalf("shortCircuitEmpty = %v, want %v", got, tc.wantShort)
			} else if got {
				if rep.States["t1"] != TaskBlocked {
					t.Fatalf("states must mirror the ledger: %+v", rep)
				}
				if tc.wantWall && rep.WallMS <= 0 && rep.AggregateMS <= 0 {
					t.Fatalf("dry-run must estimate cost: %+v", rep)
				}
			}
		})
	}
}

func TestSplitSealAdmissionSealsAndPreservesKey(t *testing.T) {
	root := t.TempDir()
	splitMission(t, root, "msn_split_seal")
	splitSavePlan(t, root, "msn_split_seal", 1, []planning.PlanTask{splitTask("t1", []string{"a.go"}, nil)})
	now := splitNow()
	if _, err := UpdateLedger(root, "msn_split_seal", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1"}, 1, map[string]string{"t1": "anchored"}, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wave := []orchestrate.ParallelTask{{ID: "t1", Title: "t1", Agent: "anchored"}}
	sealed, err := sealAdmission(root, "msn_split_seal", 1, wave, now)
	if err != nil {
		t.Fatal(err)
	}
	rec := sealed.Tasks["t1"]
	if rec.State != TaskDispatched || rec.Attempt != 1 || rec.OpKey != OpKeyFor("msn_split_seal", 1, "t1", 1) {
		t.Fatalf("admission must dispatch with opkey: %+v", rec)
	}
	// Resume con la misma attempt nunca re-clavea.
	again, err := sealAdmission(root, "msn_split_seal", 1, wave, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Tasks["t1"].OpKey; got != OpKeyFor("msn_split_seal", 1, "t1", 1) {
		t.Fatalf("opkey = %q, want the sealed key", got)
	}
	if again.Tasks["t1"].Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", again.Tasks["t1"].Attempt)
	}
}

func TestSplitBuildParallelRequestDefaultsTable(t *testing.T) {
	wave := []orchestrate.ParallelTask{{ID: "t1", Title: "t1"}}
	for _, tc := range []struct {
		name        string
		opts        RunOptions
		wantBase    string
		wantRoot    string
		wantPrefix  string
		wantMaxConc int
	}{
		{
			name: "defaults", opts: RunOptions{Root: "r", MissionID: "m"},
			wantBase: "HEAD", wantRoot: ".regulus/execute/worktrees",
			wantPrefix: "mission-m", wantMaxConc: 4,
		},
		{
			name: "explicit", opts: RunOptions{Root: "r", MissionID: "a b!", Base: "main", WorktreeRoot: "wt", MaxConcurrency: 2},
			wantBase: "main", wantRoot: "wt",
			wantPrefix: "mission-a-b", wantMaxConc: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runID, preq, latch := buildParallelRequest(tc.opts, wave, 1)
			if runID == "" {
				t.Fatal("run id must mint")
			}
			if preq.Base != tc.wantBase {
				t.Fatalf("base = %q, want %q", preq.Base, tc.wantBase)
			}
			if preq.WorktreeRoot != tc.wantRoot {
				t.Fatalf("worktree root = %q, want %q", preq.WorktreeRoot, tc.wantRoot)
			}
			if preq.BranchPrefix != tc.wantPrefix {
				t.Fatalf("branch prefix = %q, want %q", preq.BranchPrefix, tc.wantPrefix)
			}
			if preq.Budgets.MaxConcurrency != tc.wantMaxConc {
				t.Fatalf("concurrency = %d, want %d", preq.Budgets.MaxConcurrency, tc.wantMaxConc)
			}
			if preq.RunID != runID || len(preq.Tasks) != 1 || latch == nil {
				t.Fatalf("request must carry run/tasks/latch: %+v", preq)
			}
			if preq.Work == nil || preq.Gate == nil {
				t.Fatal("work/gate default when nil")
			}
		})
	}
}

func TestSplitSealAndIntegrateSealsAccounts(t *testing.T) {
	root := t.TempDir()
	splitMission(t, root, "msn_split_sealint")
	pt1 := splitTask("t1", []string{"a.go"}, nil)
	pt2 := splitTask("t2", []string{"b.go"}, nil)
	splitSavePlan(t, root, "msn_split_sealint", 1, []planning.PlanTask{pt1, pt2})
	now := splitNow()
	if _, err := UpdateLedger(root, "msn_split_sealint", now, func(l *Ledger) error {
		SeedLedger(l, []string{"t1", "t2"}, 1, map[string]string{"t1": "anchored", "t2": "anchored"}, now)
		for _, id := range []string{"t1", "t2"} {
			rec := l.Tasks[id]
			for _, next := range []string{TaskReady, TaskDispatched, TaskRunning, TaskSucceeded} {
				if err := moveTask(&rec, next, "test seal", now); err != nil {
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
	_, m, err := mission.LoadMission(root, "msn_split_sealint")
	if err != nil {
		t.Fatal(err)
	}
	p := &planning.Plan{PlanVersion: 1, Tasks: []planning.PlanTask{pt1, pt2}}
	ledger, err := LoadLedger(root, "msn_split_sealint")
	if err != nil {
		t.Fatal(err)
	}
	parallel := orchestrate.ParallelReport{
		RunID: "run_split", WallMS: 7, AggregateMS: 9, TraceHint: "trace-split",
		Merges: []orchestrate.MergeOutcome{
			{TaskID: "t1", Branch: "b1", Merged: true},
			{TaskID: "t2", Branch: "b2", Conflict: true, ConflictFiles: []string{"b.go"}, NeedsReview: true, Reviewer: "integrator"},
		},
		Gate: orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "clean"},
	}
	var events []observability.ExecEvent
	o := RunOptions{
		Root: root, MissionID: "msn_split_sealint", Actor: "op",
		Record: func(e observability.ExecEvent) { events = append(events, e) },
		Now:    func() time.Time { return splitNow() },
	}
	rep := &WaveReport{MissionID: o.MissionID, PlanVersion: 1, States: map[string]string{}}
	started := splitNow()
	if err := sealAndIntegrate(context.Background(), o, m, p, ledger, parallel, map[string]TouchedFiles{}, rep, now, started); err != nil {
		t.Fatal(err)
	}
	if rep.WallMS != 7 || rep.TraceHint != "trace-split" {
		t.Fatalf("report must carry scheduler accounting: %+v", rep)
	}
	if len(rep.Integrated) != 1 || rep.Integrated[0] != "t1" || len(rep.Blocked) != 1 || rep.Blocked[0] != "t2" {
		t.Fatalf("clean seals, conflicted parks: %+v", rep)
	}
	if rep.States["t1"] != TaskIntegrated || rep.States["t2"] != TaskBlocked {
		t.Fatalf("states must mirror the sealed ledger: %+v", rep.States)
	}
	if len(events) != 1 || events[0].Status != observability.ExecSucceeded {
		t.Fatalf("trace must link succeeded: %+v", events)
	}
}

func TestSplitRunThinOrchestratorTable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dryRun  bool
		withOps bool
		wantErr string
	}{
		{name: "dry-run-seals-nothing", dryRun: true, withOps: false},
		{name: "wet-needs-ops", dryRun: false, withOps: false, wantErr: "no worktree backend"},
		{name: "wet-dispatches", dryRun: false, withOps: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			id := "msn_split_run_" + strings.ReplaceAll(tc.name, "-", "_")
			now := splitNow()
			m, err := mission.NewMission(mission.NewInput{ID: id, Objective: "Given x When y Then z 1"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mission.CreateMission(root, m, now); err != nil {
				t.Fatal(err)
			}
			path := []string{mission.StateAnalyzing, mission.StatePlanning, mission.StateAwaitingApproval}
			if !tc.dryRun || tc.withOps {
				// Wet paths need executing; dry-run stays awaiting.
				if !tc.dryRun {
					path = append(path, mission.StateExecuting)
				}
			}
			_, err = mission.UpdateMission(root, id, now, func(mm *mission.Mission) error {
				for _, s := range path {
					if _, err := mm.Transition(s, "test", "t", "", now); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			splitSavePlan(t, root, id, 1, []planning.PlanTask{splitTask("t1", []string{"a.go"}, nil)})
			o := RunOptions{
				Root: root, MissionID: id, DryRun: tc.dryRun,
				MaxConcurrency: 2, MaxQueued: orchestrate.MaxParallelTasks,
				WorktreeRoot: "mem", Base: "HEAD", Now: time.Now,
				Work: func(ctx context.Context, task orchestrate.ParallelTask, dir string) error { return nil },
				Gate: func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
					return orchestrate.GateOutcome{Ran: true, Passed: true, Detail: "test gate"}
				},
			}
			if tc.withOps {
				o.Ops = newMemOps()
			}
			rep, err := Run(context.Background(), o)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rep.DryRun != tc.dryRun {
				t.Fatalf("dryRun = %v, want %v", rep.DryRun, tc.dryRun)
			}
			if tc.dryRun {
				l, _ := LoadLedger(root, id)
				if len(l.Tasks) != 0 {
					t.Fatalf("dry-run sealed %d tasks", len(l.Tasks))
				}
			} else {
				if rep.States["t1"] != TaskIntegrated {
					t.Fatalf("wet run must integrate: %+v", rep.States)
				}
			}
		})
	}
}
