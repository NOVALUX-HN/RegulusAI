// Operator moves (F13.3/NOV-106): explicit approval and audited
// cancellation. Approval moves a held task forward; cancellation
// closes the mission and parks the live ledger.
package execute

import (
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/mission"
)

// ApproveTask records one operator approval: a blocked task reopens
// to ready with the approver named. Anything but blocked refuses:
// approval moves a held task forward, never rewrites sealed work.
func ApproveTask(root, missionID, taskID, actor string, now time.Time) error {
	_, err := UpdateLedger(root, missionID, now, func(l *Ledger) error {
		rec, ok := l.Tasks[taskID]
		if !ok {
			return fmt.Errorf("execute: mission %q holds no task %q (nothing to approve)", missionID, taskID)
		}
		if rec.State != TaskBlocked {
			return fmt.Errorf("execute: task %q is %s (only blocked tasks approve)", taskID, rec.State)
		}
		if err := moveTask(&rec, TaskReady, "approved by "+actor, now); err != nil {
			return err
		}
		rec.ApprovedBy = actor
		rec.UpdatedAt = now.UTC()
		l.Tasks[taskID] = rec
		return nil
	})
	return err
}

// CancelExecution closes the mission with the operator reason and
// parks every live ledger task as cancelled: the next scheduler
// wave refuses (terminal mission), and in-flight workers observe
// the cancelled context through the caller.
func CancelExecution(root, missionID, reason, actor string, now time.Time) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("execute: cancel needs a reason (audit)")
	}
	if _, err := mission.UpdateMission(root, missionID, now, func(m *mission.Mission) error {
		_, err := m.Cancel(actor, reason, "", now)
		return err
	}); err != nil {
		return err
	}
	_, err := UpdateLedger(root, missionID, now, func(l *Ledger) error {
		for id, rec := range l.Tasks {
			switch rec.State {
			case TaskPending, TaskReady, TaskDispatched, TaskRunning, TaskFailed, TaskBlocked, TaskSucceeded:
				_ = moveTask(&rec, TaskCancelled, "mission cancelled: "+reason, now)
				l.Tasks[id] = rec
			}
		}
		return nil
	})
	return err
}
