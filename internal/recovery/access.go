// CLI-facing access helpers (F12.5/NOV-100): scope-subset
// enforcement (F11.8: record only what the caller grants, reassign
// only narrower), checkpoint lookup, and stale-running rotation.
package recovery

import (
	"fmt"
	"time"
)

// CheckScopesSubset refuses a scope set that widens the granted
// set: delegation only restricts, so a checkpoint may record at
// most what the caller holds.
func CheckScopesSubset(want, have []string) error {
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return fmt.Errorf("recovery: scope %q widens the granted %v (delegation only restricts)", w, have)
		}
	}
	return nil
}

// Lookup returns one checkpoint for CLI layers (plan defaults,
// reassignment defaults). Missing reads fail, never nil.
func (r *RunRecord) Lookup(taskID, agentID string) (*Checkpoint, error) {
	return r.find(taskID, agentID)
}

// RotateStaleRunning marks a running checkpoint whose last
// confirmation exceeds the attempt timeout as timed_out, so the
// resume that follows classifies the stall instead of silently
// restarting it. Fresh running checkpoints pass through.
func RotateStaleRunning(r *RunRecord, taskID, agentID string, now time.Time, p RetryPolicy) error {
	c, err := r.find(taskID, agentID)
	if err != nil {
		return err
	}
	if c.State != StateRunning {
		return nil
	}
	if !p.TimedOut(c.LastConfirm, now) {
		return nil
	}
	if err := CheckTransition(c.State, StateTimedOut); err != nil {
		return err
	}
	c.State = StateTimedOut
	c.LastClass = ClassTimeout
	c.LastConfirm = now.UTC()
	c.History = append(c.History, AttemptNote{
		Attempt: c.Attempt, State: StateTimedOut,
		Class: ClassTimeout, Detail: "stale running rotated by resume",
		At: now.UTC(),
	})
	return nil
}
