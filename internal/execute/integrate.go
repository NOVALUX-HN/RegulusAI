// Wave sealing and integration (F13.3/NOV-106): from scheduler
// verdicts to ledger states and verified integrations.
//
// The scheduler owns merging and the post-merge gate; this layer
// owns the ledger mapping and the integration checks that decide
// whether a succeeded task seals as integrated or parks as blocked
// for a human. Conflicts, dirty worktrees, out-of-scope files and
// failed gates never resolve silently: they block with the naming
// reason and the worktrees stay preserved for review.
package execute

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/gates"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

// outcomeToLedger maps scheduler outcomes onto ledger states:
// passed closes the attempt, timed-out and failed fail it,
// cancelled cancels it, skipped blocks it with the scheduler's
// naming reason (a dependency did not pass).
func outcomeToLedger(outcome string) string {
	switch outcome {
	case orchestrate.TaskPassed:
		return TaskSucceeded
	case orchestrate.TaskFailed, orchestrate.TaskTimedOut:
		return TaskFailed
	case orchestrate.TaskCancelled:
		return TaskCancelled
	default:
		return TaskBlocked
	}
}

// TouchedFiles carries what one worker reports touching: the
// receipt file the work wrote plus the commit sealing it. The
// F12.2 scheduler verdict carries no file list, so the wrapper
// reads it back from the isolated worktree after the work succeeds.
type TouchedFiles struct {
	Files  []string
	Commit string
}

// workWithLedger wraps the worker so the ledger tracks running
// while the task executes: dispatched -> running on entry. After
// the work succeeds it reads back the receipt the work wrote
// (files + commit) into the latch for sealWaveResults. The closing
// verdict lands in sealWaveResults after the scheduler returns, so
// a crash mid-wave reloads dispatched/running and the next run
// re-drives them (never sealed, never lost).
func workWithLedger(o RunOptions, work func(ctx context.Context, task orchestrate.ParallelTask, dir string) error, latch map[string]TouchedFiles) func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
	return func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		_, _ = UpdateLedger(o.Root, o.MissionID, nowOf(o)(), func(l *Ledger) error {
			rec := l.Tasks[task.ID]
			if rec.State == TaskDispatched {
				_ = moveTask(&rec, TaskRunning, "worker started in isolated worktree", nowOf(o)())
				l.Tasks[task.ID] = rec
			}
			return nil
		})
		if err := work(ctx, task, dir); err != nil {
			return err
		}
		if touched, ok := readReceipt(dir); ok {
			latch[task.ID] = touched
		}
		return nil
	}
}

// sealWaveResults maps every scheduler verdict onto the ledger under
// one generation-guarded write: passed -> succeeded (with the
// structured result plus the latched files/commit), failed/timed-out
// -> failed, cancelled -> cancelled, skipped -> blocked carrying the
// scheduler's reason. Tasks the wave never started keep their sealed
// state.
func sealWaveResults(root, missionID string, p *planning.Plan, parallel orchestrate.ParallelReport, latch map[string]TouchedFiles, now time.Time) (*Ledger, error) {
	byResult := map[string]orchestrate.TaskResult{}
	for _, r := range parallel.Results {
		byResult[r.TaskID] = r
	}
	return UpdateLedger(root, missionID, now, func(l *Ledger) error {
		for _, r := range parallel.Results {
			rec, ok := l.Tasks[r.TaskID]
			if !ok {
				continue
			}
			if TerminalTask(rec.State) {
				continue
			}
			rec.Result = TaskResult{
				Outcome: r.Outcome, DurationMS: r.DurationMS,
				Agent: r.Agent, Scopes: append([]string(nil), r.Scopes...),
				Reason: r.Reason, RunID: parallel.RunID,
			}
			if touched, ok := latch[r.TaskID]; ok {
				rec.Result.Files = append([]string(nil), touched.Files...)
				rec.Result.Commit = touched.Commit
			}
			to := outcomeToLedger(r.Outcome)
			reason := r.Reason
			if reason == "" {
				reason = "scheduler verdict " + r.Outcome + " in run " + parallel.RunID
			}
			if r.Outcome == orchestrate.TaskTimedOut {
				reason = "timeout: " + reason
			}
			if err := moveTask(&rec, to, reason, now); err != nil {
				// dispatched/running is the expected source; a wave
				// that never started the task (cancelled run) lands
				// from dispatched directly.
				if rec.State == TaskDispatched && to == TaskCancelled {
					rec.State = TaskCancelled
					rec.Reason = reason
					rec.UpdatedAt = now.UTC()
				} else {
					return fmt.Errorf("execute: cannot seal task %s: %w", r.TaskID, err)
				}
			}
			l.Tasks[r.TaskID] = rec
		}
		return nil
	})
}

// integrateWave verifies every succeeded task and seals what passes
// as integrated through the mission exactly-once ledger: the op key
// integrate/<mission>/v<plan>/<task> replays the stored output
// instead of re-merging, so resume after a crash between sealing
// and integrating never integrates twice. Anything failing
// verification blocks with the naming reason.
func integrateWave(root string, m *mission.Mission, p *planning.Plan, ledger *Ledger, parallel orchestrate.ParallelReport, now time.Time) (integrated, blocked []string) {
	byMerge := map[string]orchestrate.MergeOutcome{}
	for _, mg := range parallel.Merges {
		byMerge[mg.TaskID] = mg
	}
	byPlan := map[string]planning.PlanTask{}
	for _, t := range p.Tasks {
		byPlan[t.ID] = t
	}
	for _, id := range SortedTaskIDs(ledger) {
		rec := ledger.Tasks[id]
		if rec.State != TaskSucceeded {
			continue
		}
		pt, ok := byPlan[id]
		if !ok {
			continue
		}
		reason := VerifyIntegration(pt, byMerge[id], rec, parallel.Gate)
		if reason != "" {
			_, _ = UpdateLedger(root, m.ID, now, func(l *Ledger) error {
				r := l.Tasks[id]
				_ = moveTask(&r, TaskBlocked, reason, now)
				l.Tasks[id] = r
				return nil
			})
			blocked = append(blocked, id)
			continue
		}
		key := IntegrateKeyFor(m.ID, p.PlanVersion, id)
		_, _ = mission.UpdateMission(root, m.ID, now, func(mm *mission.Mission) error {
			_, err := mm.ApplyEffect(key, "integrated "+id+" plan v"+fmt.Sprint(p.PlanVersion), now, func() (string, error) {
				return "integrated " + id, nil
			})
			return err
		})
		_, _ = UpdateLedger(root, m.ID, now, func(l *Ledger) error {
			r := l.Tasks[id]
			_ = moveTask(&r, TaskIntegrated, "verified: clean merge, scope-owned files, gate passed", now)
			l.Tasks[id] = r
			return nil
		})
		integrated = append(integrated, id)
	}
	sort.Strings(integrated)
	sort.Strings(blocked)
	return integrated, blocked
}

// VerifyIntegration checks one succeeded task before it seals:
// merge conflict or merge error parks for integrator review (never
// auto-resolved); a failed post-merge gate parks with the gate
// detail; files outside the task scope park for review (shared
// files serialize through the DAG, they never merge silently).
// Empty means integrated.
func VerifyIntegration(pt planning.PlanTask, merge orchestrate.MergeOutcome, rec TaskRecord, gate orchestrate.GateOutcome) string {
	if merge.Conflict {
		return fmt.Sprintf("merge conflict in %s (files [%s]): integrator review required, worktrees preserved",
			merge.Branch, strings.Join(merge.ConflictFiles, ", "))
	}
	if merge.Error != "" && !merge.Merged {
		return fmt.Sprintf("merge did not run for %s: %s (integrator review first)", pt.ID, merge.Error)
	}
	if gate.Ran && !gate.Passed {
		return fmt.Sprintf("post-merge gate failed for %s: %s (worktrees preserved for review)", pt.ID, gate.Detail)
	}
	if out := outOfScope(rec.Result.Files, pt.Scope.Files); len(out) > 0 {
		return fmt.Sprintf("task %s touched files outside its scope [%s] (ownership check: review required, never merged silently)",
			pt.ID, strings.Join(out, ", "))
	}
	return ""
}

// outOfScope lists touched files neither equal to nor under a scope
// file (same containment rule the F12.2 DAG collides on: equal or
// slash-boundary prefix shares state). Empty scope with touched
// files reads as a violation, never as vacuous approval.
func outOfScope(touched, scope []string) []string {
	if len(touched) == 0 {
		return nil
	}
	norm := func(f string) string {
		p := strings.TrimSpace(strings.ReplaceAll(f, "\\", "/"))
		p = path.Clean(p)
		return strings.TrimPrefix(p, "./")
	}
	var scoped []string
	for _, s := range scope {
		if n := norm(s); n != "." && n != "" {
			scoped = append(scoped, n)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range touched {
		n := norm(t)
		owned := false
		for _, s := range scoped {
			if n == s || strings.HasPrefix(n, s+"/") || strings.HasPrefix(s, n+"/") {
				owned = true
				break
			}
		}
		if !owned && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// receiptFileName is the structured receipt one worker leaves in
// its isolated worktree: the wrapper reads it back for the
// integration ownership check.
const receiptFileName = ".regulus-task-receipt.json"

// ReceiptWork is the default worker: it refuses empty scope entries
// (fail closed, never guessed), writes the structured receipt into
// the isolated worktree and commits it, capturing files, commit,
// agent and scopes for the integration check. A worktree without
// git (or a failing commit) fails the task with the reason.
func ReceiptWork(root, missionID string, planVersion int) func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
	return func(ctx context.Context, task orchestrate.ParallelTask, dir string) error {
		for _, f := range task.Files {
			if strings.TrimSpace(f) == "" {
				return fmt.Errorf("task %s: empty scope entry (fail closed, never guessed)", task.ID)
			}
		}
		files := append([]string(nil), task.Files...)
		sort.Strings(files)
		raw, err := json.Marshal(map[string]any{
			"mission_id": missionID, "plan_version": planVersion,
			"task_id": task.ID, "agent": task.Agent,
			"scopes": task.Scopes, "files": files,
		})
		if err != nil {
			return fmt.Errorf("task %s: cannot encode receipt: %v", task.ID, err)
		}
		if err := os.WriteFile(filepath.Join(dir, receiptFileName), append(raw, '\n'), 0o644); err != nil {
			return fmt.Errorf("task %s: cannot write receipt: %v", task.ID, err)
		}
		runner := gates.ShRunner{Dir: dir}
		if _, _, _, err := runner.Run(ctx, "git add "+receiptFileName); err != nil {
			return fmt.Errorf("task %s: cannot stage receipt: %v", task.ID, err)
		}
		msg := fmt.Sprintf("mission %s task %s receipt", missionID, task.ID)
		if _, errb, exit, err := runner.Run(ctx, "git commit -m "+fmt.Sprintf("%q", msg)); err != nil || exit != 0 {
			return fmt.Errorf("task %s: cannot commit receipt: %v (%s)", task.ID, err, strings.TrimSpace(errb))
		}
		return nil
	}
}

// readReceipt reads back one worker receipt: the scope files the
// task claims plus the commit sealing them. False when the work
// wrote no receipt (custom workers): ownership then checks nothing
// and the integration still requires clean merge and gate.
func readReceipt(dir string) (TouchedFiles, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, receiptFileName))
	if err != nil {
		return TouchedFiles{}, false
	}
	var rec struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		return TouchedFiles{}, false
	}
	commit := ""
	runner := gates.ShRunner{Dir: dir}
	if out, _, exit, err := runner.Run(context.Background(), "git rev-parse HEAD"); err == nil && exit == 0 {
		commit = strings.TrimSpace(out)
	}
	return TouchedFiles{Files: rec.Files, Commit: commit}, true
}

// DiffCheckGate is the default post-merge gate: git diff --check on
// the repo root (clean tree reads as passed).
func DiffCheckGate() func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
	return func(ctx context.Context, repoRoot string) orchestrate.GateOutcome {
		runner := gates.ShRunner{Dir: repoRoot}
		out, errb, exit, err := runner.Run(ctx, "git diff --check")
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
}
