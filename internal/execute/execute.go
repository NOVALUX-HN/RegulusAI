// Mission execution waves (F13.3/NOV-106): from an approved plan to
// scheduler invocations, with ledger readiness driving every wave.
//
// A wave is one orchestrate.RunParallel call over the tasks the
// ledger marks ready. After the wave the executor seals task
// outcomes, integrates what merged cleanly, blocks what needs a
// human, and stops: the next wave (resume, approvals, retries) is a
// new invocation that skips everything already sealed. No wave ever
// re-dispatches a sealed op key, so resume never duplicates
// execution or integration.
package execute

import (
	"context"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

// RunOptions carries one execution request. Ops/Work/Gate/Record are
// seams: production wires git + shell + trace, tests inject fakes.
// A nil Ops on a wet run fails closed; dry-run touches none of them.
type RunOptions struct {
	Root        string
	MissionID   string
	PlanVersion int // 0 = latest stored revision
	ApproveAll  bool
	Supervised  bool
	DryRun      bool
	Actor       string

	MaxConcurrency   int
	PerTaskTimeoutMS int64
	WallBudgetMS     int64
	MaxQueued        int

	GlobalTimeMS   int64
	GlobalTokensMS int64
	GlobalCost     float64
	PerTaskTimeMS  int64
	PerTaskTokens  int64
	PerTaskCost    float64

	WorktreeRoot string
	Base         string
	GateTask     string

	Ops    orchestrate.WorktreeOps
	Work   func(ctx context.Context, task orchestrate.ParallelTask, dir string) error
	Gate   func(ctx context.Context, repoRoot string) orchestrate.GateOutcome
	Record func(observability.ExecEvent)
	Now    func() time.Time
}

// WaveReport is the receipt of one execution invocation: the wave
// run ids, per-task ledger states, budget accounting and trace link.
type WaveReport struct {
	MissionID   string            `json:"mission_id"`
	PlanVersion int               `json:"plan_version"`
	RunIDs      []string          `json:"run_ids"`
	States      map[string]string `json:"states"`
	Integrated  []string          `json:"integrated"`
	Blocked     []string          `json:"blocked"`
	WallMS      int64             `json:"wall_ms"`
	AggregateMS int64             `json:"aggregate_ms"`
	SpentTimeMS int64             `json:"spent_time_ms"`
	SpentTokens int64             `json:"spent_tokens"`
	DryRun      bool              `json:"dry_run"`
	TraceHint   string            `json:"trace_hint,omitempty"`
}

// nowOf returns the injected clock or the real one.
func nowOf(o RunOptions) func() time.Time {
	if o.Now != nil {
		return o.Now
	}
	return time.Now
}

// LoadApprovedPlan loads the plan revision and refuses anything that
// is not executable: refused and clarification-needing plans never
// dispatch; needs-approval plans dispatch only with an explicit
// operator approval (--approve). Terminal missions refuse: reopen
// explicitly before executing.
func LoadApprovedPlan(root, missionID string, version int, approveAll bool) (*mission.Mission, *planning.Plan, error) {
	_, m, err := mission.LoadMission(root, missionID)
	if err != nil {
		return nil, nil, err
	}
	if mission.IsTerminal(m.State) {
		return nil, nil, fmt.Errorf("execute: mission %q is %s (terminal: reopen explicitly with a reason before executing)", missionID, m.State)
	}
	var p *planning.Plan
	if version <= 0 {
		p, err = planning.LatestPlan(root, missionID)
	} else {
		p, err = planning.LoadPlan(root, missionID, version)
	}
	if err != nil {
		return nil, nil, err
	}
	switch p.Status {
	case planning.StatusReady:
	case planning.StatusNeedsApproval:
		if !approveAll {
			return nil, nil, fmt.Errorf("execute: plan v%d needs-approval (re-run with --approve to record the operator approval, never auto-approved)", p.PlanVersion)
		}
	default:
		return nil, nil, fmt.Errorf("execute: plan v%d status %q is not executable (ready or approved only; refused=%q clarification=%q)",
			p.PlanVersion, p.Status, planning.StatusRefused, planning.StatusNeedsClarification)
	}
	return m, p, nil
}

// MapWaveTasks maps plan tasks onto scheduler tasks for one wave:
// integrated and succeeded ledger states are skipped (resume never
// re-dispatches sealed work); DependsOn edges onto skipped tasks
// are satisfied and dropped; edges onto tasks that failed, block or
// wait stay, so the scheduler (and the ledger pre-check) holds the
// dependents. Tasks already terminal in the ledger never map.

// shortCircuitEmpty fills the report states and reports whether the
// wave short-circuits without touching the scheduler. Empty waves
// seal nothing and return the confirmed states; dry-runs estimate
// cost, seal nothing and return the confirmed states.
func shortCircuitEmpty(rep *WaveReport, wave []orchestrate.ParallelTask, ledger *Ledger, o RunOptions) bool {
	if len(wave) == 0 {
		// Nothing runnable: every task is sealed, blocked or waiting.
		// Resume short-circuits without touching the scheduler, so a
		// second run never re-dispatches sealed work.
		for id, rec := range ledger.Tasks {
			rep.States[id] = rec.State
		}
		return true
	}
	if o.DryRun {
		wall, agg := orchestrate.EstimateParallelCost(wave, concurrencyOf(o))
		rep.WallMS, rep.AggregateMS = wall, agg
		for id, rec := range ledger.Tasks {
			rep.States[id] = rec.State
		}
		// Dry-run seals nothing: not the ledger, not the mission,
		// not a trace event.
		return true
	}
	return false
}

// sealAdmission seals readiness: every admitted task moves
// pending/ready -> dispatched with its idempotency key. A task
// already carrying a sealed op key for this attempt is never
// re-keyed: resume reuses the key instead of dispatching twice.
func sealAdmission(root, missionID string, planVersion int, wave []orchestrate.ParallelTask, now time.Time) (*Ledger, error) {
	return UpdateLedger(root, missionID, now, func(l *Ledger) error {
		for _, t := range wave {
			rec := l.Tasks[t.ID]
			if rec.OpKey != "" && (rec.State == TaskDispatched || rec.State == TaskRunning) {
				continue
			}
			rec.Attempt++
			rec.OpKey = OpKeyFor(missionID, planVersion, t.ID, rec.Attempt)
			rec.Agent = t.Agent
			// Legal admission path: pending/failed/blocked reopen
			// through ready, then dispatch. Anything else fails
			// closed instead of jumping the ledger.
			if rec.State == TaskPending || rec.State == TaskFailed || rec.State == TaskBlocked {
				if err := moveTask(&rec, TaskReady, "admitted to wave", now); err != nil {
					return err
				}
			}
			if err := moveTask(&rec, TaskDispatched, "admitted to wave (op "+rec.OpKey+")", now); err != nil {
				return err
			}
			l.Tasks[t.ID] = rec
		}
		return nil
	})
}

// buildParallelRequest assembles one scheduler invocation: run id,
// budgets, default work/gate, principal, latch and the parallel
// request. Order matches the inlined version so fakes observe the
// same sequence.
func buildParallelRequest(o RunOptions, wave []orchestrate.ParallelTask, planVersion int) (string, orchestrate.ParallelRequest, map[string]TouchedFiles) {
	runID, _ := observability.MintRunID()
	budgets := orchestrate.ParallelBudgets{
		MaxConcurrency: concurrencyOf(o), PerTaskTimeoutMS: o.PerTaskTimeoutMS,
		WallBudgetMS: o.WallBudgetMS, MaxQueued: o.MaxQueued,
	}
	if budgets.MaxConcurrency == 0 && budgets.PerTaskTimeoutMS == 0 && budgets.WallBudgetMS == 0 && budgets.MaxQueued == 0 {
		budgets = orchestrate.DefaultParallelBudgets()
	}
	work := o.Work
	if work == nil {
		work = ReceiptWork(o.Root, o.MissionID, planVersion)
	}
	gate := o.Gate
	if gate == nil {
		gate = DiffCheckGate()
	}
	parent := mcp.PrincipalFor(o.Root)
	latch := map[string]TouchedFiles{}
	preq := orchestrate.ParallelRequest{
		Tasks: wave, Budgets: budgets, Base: baseOf(o), RunID: runID,
		BranchPrefix: "mission-" + sanitizeSegment(o.MissionID),
		WorktreeRoot: worktreeRootOf(o), RepoRoot: o.Root, Parent: parent,
		Ops: o.Ops, Work: workWithLedger(o, work, latch), Gate: gate,
		Record: o.Record, DryRun: false,
	}
	return runID, preq, latch
}

// sealAndIntegrate seals scheduler verdicts, integrates what verified
// cleanly, reloads the confirmed ledger, folds report accounting and
// advances the mission plus the trace link. It mirrors the inlined
// tail of Run byte for byte.
func sealAndIntegrate(ctx context.Context, o RunOptions, m *mission.Mission, p *planning.Plan, ledger *Ledger, parallel orchestrate.ParallelReport, latch map[string]TouchedFiles, rep *WaveReport, now, started time.Time) error {
	rep.WallMS, rep.AggregateMS = parallel.WallMS, parallel.AggregateMS
	rep.TraceHint = parallel.TraceHint
	if ctx.Err() != nil {
		parallel.Cancelled = true
	}
	sealed, err := sealWaveResults(o.Root, o.MissionID, p, parallel, latch, now)
	if err != nil {
		return err
	}
	ledger = sealed
	integrated, blocked := integrateWave(o.Root, m, p, ledger, parallel, now)
	rep.Integrated = integrated
	rep.Blocked = blocked
	// Reload: integration sealed states after the seal above.
	if fresh, lerr := LoadLedger(o.Root, o.MissionID); lerr == nil {
		ledger = fresh
	}
	for id, rec := range ledger.Tasks {
		rep.States[id] = rec.State
	}
	rep.SpentTimeMS = nowOf(o)().Sub(started).Milliseconds()
	rep.SpentTokens = spentTokens(ledger, p)
	advanceMission(o.Root, o.MissionID, ledger, p, o.Actor, now)
	emitWaveEvent(o, *rep, parallel.Cancelled)
	return nil
}

// Run consumes the approved plan and dispatches one wave through
// the F12.2 scheduler. Dry-run levels the DAG, checks budgets and
// routing, prints the ceilings and seals nothing. A wet run seals
// the ledger, advances the mission executing -> integrating on first
// integration and integrating -> validating when everything the
// wave knew is integrated.
func Run(ctx context.Context, o RunOptions) (WaveReport, error) {
	now := nowOf(o)()
	m, p, err := LoadApprovedPlan(o.Root, o.MissionID, o.PlanVersion, o.ApproveAll)
	if err != nil {
		return WaveReport{}, err
	}
	rep := WaveReport{MissionID: o.MissionID, PlanVersion: p.PlanVersion, States: map[string]string{}, DryRun: o.DryRun}
	wave, ledger, err := prepareWave(o, m, p, now)
	if err != nil {
		return WaveReport{}, err
	}
	if shortCircuitEmpty(&rep, wave, ledger, o) {
		return rep, nil
	}
	if o.Ops == nil {
		return WaveReport{}, fmt.Errorf("execute: no worktree backend configured (pass Ops; dry-run needs none)")
	}
	if err := ensureExecuting(o.Root, o.MissionID, o.Actor, now); err != nil {
		return WaveReport{}, err
	}
	ledger, err = sealAdmission(o.Root, o.MissionID, p.PlanVersion, wave, now)
	if err != nil {
		return WaveReport{}, err
	}
	runID, preq, latch := buildParallelRequest(o, wave, p.PlanVersion)
	rep.RunIDs = []string{runID}
	started := nowOf(o)()
	parallel, err := orchestrate.RunParallel(ctx, preq)
	if err != nil {
		return WaveReport{}, err
	}
	if err := sealAndIntegrate(ctx, o, m, p, ledger, parallel, latch, &rep, now, started); err != nil {
		return WaveReport{}, err
	}
	return rep, nil
}

// concurrencyOf defaults the worker bound to the scheduler policy.
func concurrencyOf(o RunOptions) int {
	if o.MaxConcurrency > 0 {
		return o.MaxConcurrency
	}
	return orchestrate.DefaultParallelBudgets().MaxConcurrency
}

// baseOf defaults the merge base to HEAD.
func baseOf(o RunOptions) string {
	if strings.TrimSpace(o.Base) != "" {
		return o.Base
	}
	return "HEAD"
}

// worktreeRootOf defaults the isolation root under the project.
func worktreeRootOf(o RunOptions) string {
	if strings.TrimSpace(o.WorktreeRoot) != "" {
		return o.WorktreeRoot
	}
	return ".regulus/execute/worktrees"
}

// sanitizeSegment folds text onto the branch alphabet.
func sanitizeSegment(s string) string {
	var b strings.Builder
	for _, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-.")
	if out == "" {
		return "mission"
	}
	return out
}

// ensureExecuting moves awaiting_approval -> executing once the
// approved plan starts dispatching; live states continue untouched.
func ensureExecuting(root, missionID, actor string, now time.Time) error {
	_, err := mission.UpdateMission(root, missionID, now, func(m *mission.Mission) error {
		if m.State == mission.StateAwaitingApproval {
			_, err := m.Transition(mission.StateExecuting, actor, "executor: approved plan started dispatching", "", now)
			return err
		}
		switch m.State {
		case mission.StateExecuting, mission.StateIntegrating, mission.StateRecovering:
			return nil
		default:
			return fmt.Errorf("execute: mission %q is %s (execute runs from executing/integrating/recovering; pause/resume or advance explicitly)", m.ID, m.State)
		}
	})
	return err
}

// spentTokens sums the labelled estimates of sealed tasks: an
// accounting estimate, never a measurement.
func spentTokens(ledger *Ledger, p *planning.Plan) int64 {
	est := estimateIndex(p)
	var out int64
	for id, rec := range ledger.Tasks {
		switch rec.State {
		case TaskSucceeded, TaskIntegrated:
			out += est[id].Tokens
		}
	}
	return out
}

// advanceMission moves the mission after a wave: first integration
// executing -> integrating; everything the plan knows integrated
// (and nothing live left) integrating -> validating. Partial waves
// stay where they are for resume/approve/retry.
func advanceMission(root, missionID string, ledger *Ledger, p *planning.Plan, actor string, now time.Time) {
	_, _ = mission.UpdateMission(root, missionID, now, func(m *mission.Mission) error {
		allDone := true
		anyIntegrated := false
		for _, t := range p.Tasks {
			rec, ok := ledger.Tasks[t.ID]
			if !ok || rec.State != TaskIntegrated {
				allDone = false
			} else {
				anyIntegrated = true
			}
		}
		switch {
		case allDone && (m.State == mission.StateIntegrating || m.State == mission.StateExecuting):
			_, err := m.Transition(mission.StateValidating, actor, "executor: every plan task integrated", "", now)
			return err
		case anyIntegrated && m.State == mission.StateExecuting:
			_, err := m.Transition(mission.StateIntegrating, actor, "executor: first task integrated", "", now)
			return err
		default:
			return nil
		}
	})
}

// emitWaveEvent links the wave to the F11.1 timeline under the
// mission run id. Best-effort: a trace failure never fails the wave.
func emitWaveEvent(o RunOptions, rep WaveReport, cancelled bool) {
	if o.Record == nil {
		return
	}
	status := observability.ExecSucceeded
	evType := observability.ExecRunEnd
	if cancelled {
		status = observability.ExecCancelled
	}
	runID := rep.MissionID
	if len(rep.RunIDs) > 0 {
		runID = rep.RunIDs[0]
	}
	o.Record(observability.ExecEvent{
		RunID: runID, TaskID: o.MissionID, Type: evType, Status: status,
		Payload: map[string]any{
			"command": "execute run", "mission_id": o.MissionID,
			"plan_version": rep.PlanVersion, "integrated": rep.Integrated,
			"blocked": rep.Blocked, "wall_ms": rep.WallMS,
		},
	})
}
