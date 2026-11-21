// Wave admission (F13.3/NOV-106): from a seeded ledger to the
// runnable task set. Every stage seals before the next proceeds on
// wet runs and folds in memory on dry-run, so a crash replays from
// the last confirmed ledger and dry-run seals nothing.
package execute

import (
	"fmt"
	"sort"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

// applyMarks moves every marked task to blocked: the shared fold
// behind the seal-every-stage path (commit seals it wet, folds it
// in memory dry).
func applyMarks(l *Ledger, marks map[string]string, now time.Time) error {
	for id, reason := range marks {
		rec := l.Tasks[id]
		if err := moveTask(&rec, TaskBlocked, reason, now); err != nil {
			return err
		}
		l.Tasks[id] = rec
	}
	return nil
}

func MapWaveTasks(p *planning.Plan, l *Ledger) []orchestrate.ParallelTask {
	done := map[string]bool{}
	for id, rec := range l.Tasks {
		if rec.State == TaskIntegrated || rec.State == TaskSucceeded {
			done[id] = true
		}
	}
	byID := map[string]planning.PlanTask{}
	for _, t := range p.Tasks {
		byID[t.ID] = t
	}
	var out []orchestrate.ParallelTask
	for _, t := range p.Tasks {
		if done[t.ID] {
			continue
		}
		if rec, ok := l.Tasks[t.ID]; ok && TerminalTask(rec.State) {
			continue
		}
		// Held tasks never reach the scheduler: approval, conflict
		// review and routing refusals park as blocked, and the gate
		// below propagates the block to their dependents.
		if rec, ok := l.Tasks[t.ID]; ok && rec.State == TaskBlocked {
			continue
		}
		var deps []string
		for _, d := range t.DependsOn {
			if done[d] {
				continue
			}
			if _, known := byID[d]; known {
				deps = append(deps, d)
			}
		}
		out = append(out, orchestrate.ParallelTask{
			ID: t.ID, Title: t.Title, Files: append([]string(nil), t.Scope.Files...),
			DependsOn: deps, Agent: t.Agent, Scopes: append([]string(nil), t.MinScopes...),
			EstimateMS: t.Estimate.TimeMS, EstimateTokens: t.Estimate.Tokens,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// holdForApproval marks needs-approval (and, in supervised mode,
// every unapproved) tasks blocked before dispatch: the scheduler
// never sees them, and the report names the approver gate.
func holdForApproval(l *Ledger, p *planning.Plan, supervised bool, now time.Time) []string {
	byPlan := map[string]planning.PlanTask{}
	for _, t := range p.Tasks {
		byPlan[t.ID] = t
	}
	var held []string
	for id, rec := range l.Tasks {
		if rec.State != TaskPending && rec.State != TaskFailed && rec.State != TaskBlocked {
			continue
		}
		pt, ok := byPlan[id]
		if !ok {
			continue
		}
		needs := pt.Autonomy == planning.AutonomyNeedsApproval || pt.Autonomy == planning.AutonomyNeedsUser
		if supervised && rec.ApprovedBy == "" {
			needs = true
		}
		if !needs || rec.ApprovedBy != "" {
			continue
		}
		rec.State = TaskBlocked
		rec.Reason = "awaiting approval (autonomy " + pt.Autonomy + ": approve explicitly, never auto-approved)"
		rec.UpdatedAt = now.UTC()
		rec.History = append(rec.History, "held -> blocked: awaiting approval")
		l.Tasks[id] = rec
		held = append(held, id)
	}
	sort.Strings(held)
	return held
}

// gateWaveDependencies holds tasks whose dependencies are not
// satisfied by sealed work: a failed/blocked/cancelled dependency
// propagates the block with the naming reason; a dependency still
// live in the ledger (pending/ready/dispatched/running outside
// this wave) keeps the dependent pending behind it.
func gateWaveDependencies(wave []orchestrate.ParallelTask, ledger *Ledger, now time.Time) ([]orchestrate.ParallelTask, map[string]string) {
	blocked := map[string]string{}
	inWave := map[string]bool{}
	for _, t := range wave {
		inWave[t.ID] = true
	}
	changed := true
	for changed {
		changed = false
		var keep []orchestrate.ParallelTask
		for _, t := range wave {
			if _, out := blocked[t.ID]; out {
				continue
			}
			reason := ""
			for _, d := range t.DependsOn {
				if !inWave[d] {
					continue
				}
				if _, out := blocked[d]; out {
					reason = "dependency " + d + " did not pass (propagated block, never silent)"
					break
				}
			}
			if reason == "" {
				for _, d := range t.DependsOn {
					if inWave[d] {
						continue
					}
					rec, ok := ledger.Tasks[d]
					if !ok || (rec.State != TaskIntegrated && rec.State != TaskSucceeded) {
						st := stateOf(rec, ok)
						switch st {
						case TaskBlocked, TaskFailed, TaskCancelled:
							reason = "dependency " + d + " is " + st + " (propagated block, never silent)"
						default:
							reason = "dependency " + d + " is " + st + " (waits, never jumps the DAG)"
						}
						break
					}
				}
			}
			if reason != "" {
				blocked[t.ID] = reason
				changed = true
				continue
			}
			keep = append(keep, t)
		}
		wave = keep
	}
	return wave, blocked
}

// stateOf names a ledger state for gating messages.
func stateOf(rec TaskRecord, ok bool) string {
	if !ok {
		return "unknown"
	}
	return rec.State
}

// exceptTask drops one task from the wave.
func exceptTask(wave []orchestrate.ParallelTask, id string) []orchestrate.ParallelTask {
	out := wave[:0]
	for _, t := range wave {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

// propagateRoutingBlocks parks every task depending (transitively) on
// a routing-blocked task: a dependent with no runnable dependency
// never reaches the scheduler, it blocks with the naming reason.
// Admitted tasks keep only edges onto runnable tasks.
func propagateRoutingBlocks(admitted []orchestrate.ParallelTask, blocked map[string]string, ledger *Ledger, now time.Time) ([]orchestrate.ParallelTask, map[string]string) {
	out := map[string]string{}
	for id, reason := range blocked {
		out[id] = reason
	}
	inWave := map[string]bool{}
	for _, t := range admitted {
		inWave[t.ID] = true
	}
	changed := true
	for changed {
		changed = false
		for _, t := range admitted {
			if _, isOut := out[t.ID]; isOut {
				continue
			}
			for _, d := range t.DependsOn {
				if reason, bad := out[d]; bad {
					if inWave[d] {
						out[t.ID] = "dependency " + d + " blocked (" + reason + ")"
					} else if rec, ok := ledger.Tasks[d]; !ok || (rec.State != TaskIntegrated && rec.State != TaskSucceeded) {
						out[t.ID] = "dependency " + d + " blocked (" + reason + ")"
					}
					changed = true
					break
				}
			}
		}
	}
	var keep []orchestrate.ParallelTask
	for _, t := range admitted {
		if _, isOut := out[t.ID]; isOut {
			continue
		}
		var deps []string
		for _, d := range t.DependsOn {
			if _, bad := out[d]; !bad {
				deps = append(deps, d)
			}
		}
		t.DependsOn = deps
		keep = append(keep, t)
	}
	return keep, out
}

// prepareWave runs the admission stages and returns the runnable
// wave plus the confirmed ledger: seed (idempotent), approval
// holds, dependency pre-check, routing re-validation with block
// propagation, budget gates and the F12.2 DAG order. A cycle names
// its loop and refuses the whole wave before anything dispatches.
func prepareWave(o RunOptions, m *mission.Mission, p *planning.Plan, now time.Time) ([]orchestrate.ParallelTask, *Ledger, error) {
	ledger, err := LoadLedger(o.Root, o.MissionID)
	if err != nil {
		return nil, nil, err
	}
	// commit seals one stage: a generation-guarded write on wet
	// runs, a pure memory fold on dry-run (which seals nothing:
	// not the ledger, not the mission, not a trace event).
	commit := func(fn func(*Ledger) error) (*Ledger, error) {
		if o.DryRun {
			if err := fn(ledger); err != nil {
				return nil, err
			}
			return ledger, nil
		}
		updated, err := UpdateLedger(o.Root, o.MissionID, now, fn)
		if err != nil {
			return nil, err
		}
		return updated, nil
	}
	// Stage 1: seed every plan task (idempotent), so every later
	// stage reads confirmed ledger instead of memory.
	ledger, err = commit(func(l *Ledger) error {
		SeedLedger(l, planTaskIDs(p), p.PlanVersion, agentIndex(p), now)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	// Stage 2: hold approvals before anything dispatches.
	held := holdForApproval(ledger, p, o.Supervised, now)
	if len(held) > 0 {
		ledger, err = commit(func(l *Ledger) error {
			for _, id := range held {
				rec := l.Tasks[id]
				if err := moveTask(&rec, TaskBlocked, "awaiting approval (approve explicitly, never auto-approved)", now); err != nil {
					return err
				}
				l.Tasks[id] = rec
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	wave := MapWaveTasks(p, ledger)
	// Stage 3: ledger pre-check. A task whose dependency did not
	// succeed in a previous wave (or is blocked now) never reaches
	// the scheduler: it blocks here with the naming reason, and its
	// own dependents stay pending behind it. No deadlocks: cycles
	// fail closed in PlanParallelLevels below.
	admit, preblocked := gateWaveDependencies(wave, ledger, now)
	if len(preblocked) > 0 {
		ledger, err = commit(func(l *Ledger) error {
			return applyMarks(l, preblocked, now)
		})
		if err != nil {
			return nil, nil, err
		}
	}
	admitted, offline := RevalidateRouting(admit, p, m)
	wave, routed := propagateRoutingBlocks(admitted, offline, ledger, now)
	if len(routed) > 0 {
		ledger, err = commit(func(l *Ledger) error {
			return applyMarks(l, routed, now)
		})
		if err != nil {
			return nil, nil, err
		}
	}

	if len(wave) == 0 {
		// Nothing runnable: every task is sealed, blocked or waiting.
		// The caller short-circuits without touching the scheduler,
		// so a second run never re-dispatches sealed work and the
		// empty DAG never reaches the level planner.
		return wave, ledger, nil
	}

	global := m.Budget
	if o.GlobalTimeMS > 0 || o.GlobalTokensMS > 0 || o.GlobalCost > 0 {
		global = mission.Budget{TimeMS: o.GlobalTimeMS, Tokens: o.GlobalTokensMS, Cost: o.GlobalCost}
	}
	perTaskBudget := mission.Budget{TimeMS: o.PerTaskTimeMS, Tokens: o.PerTaskTokens, Cost: o.PerTaskCost}
	if err := CheckWaveBudgets(wave, estimateIndex(p), global, perTaskBudget); err != nil {
		return nil, nil, err
	}
	// DAG order, collision serialisation and cycle refusal all come
	// from the F12.2 planner: a cycle names its loop and refuses the
	// whole wave before anything dispatches (deadlock prevention).
	if _, _, err := orchestrate.PlanParallelLevels(wave); err != nil {
		return nil, nil, fmt.Errorf("execute: %w", err)
	}
	return wave, ledger, nil
}
