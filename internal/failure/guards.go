// Mission-level recovery guards (F13.5/NOV-108): the three
// preconditions a decision must hold before it names its action.
// Replan never invalidates confirmed results, rollback never
// leaves the mission-owned transactional unit, and reassignment
// only attests what F12.5 will enforce atomically when the
// operator runs `recovery resume --new-agent` (the enforcing
// authority stays there; this guard is the coordinator's
// check-then-propose, citing the same rules).
package failure

import (
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/recovery"
)

// CheckReplan refuses a replan that drops a confirmed task or
// modifies one: dependents may replan around confirmed results,
// never through them. Empty confirmed set passes (nothing to
// protect); a plan that omits a confirmed id or rewrites it
// fails with the offending ids.
func CheckReplan(confirmed, planTasks, modified []string) error {
	plan := map[string]bool{}
	for _, t := range planTasks {
		plan[strings.TrimSpace(t)] = true
	}
	mod := map[string]bool{}
	for _, t := range modified {
		mod[strings.TrimSpace(t)] = true
	}
	var dropped, rewritten []string
	for _, t := range confirmed {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if !plan[t] {
			dropped = append(dropped, t)
		} else if mod[t] {
			rewritten = append(rewritten, t)
		}
	}
	if len(dropped) > 0 {
		return fmt.Errorf("failure: replan drops confirmed task(s) %s (replan dependents, never repeat completed work)", strings.Join(dropped, ","))
	}
	if len(rewritten) > 0 {
		return fmt.Errorf("failure: replan modifies confirmed task(s) %s (confirmed results are never invalidated)", strings.Join(rewritten, ","))
	}
	return nil
}

// CheckRollbackScope confines a rollback to the mission-owned
// transactional unit: every compensation key must sit under one
// of the owned prefixes (mission id, run ids). An external key —
// work outside the mission — refuses the whole rollback before
// anything unwinds, so a partial undo can never start by
// reverting work outside the mission. Empty keys pass (nothing to unwind);
// empty owned with non-empty keys refuses (no provenance, no
// rollback).
func CheckRollbackScope(keys, owned []string) error {
	if len(keys) == 0 {
		return nil
	}
	if len(owned) == 0 {
		return fmt.Errorf("failure: rollback of %d compensation(s) with no owned prefixes (no provenance, no rollback)", len(keys))
	}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			return fmt.Errorf("failure: rollback with an empty compensation key (refusing an unnamed undo)")
		}
		hit := false
		for _, o := range owned {
			o = strings.TrimSpace(o)
			if o != "" && (k == o || strings.HasPrefix(k, o+"/")) {
				hit = true
				break
			}
		}
		if !hit {
			return fmt.Errorf("failure: compensation %q sits outside the mission-owned unit %v (external changes are never reverted)", k, owned)
		}
	}
	return nil
}

// VerifyReassignInput carries one reassignment attestation: the
// live F12.5 run record, the source checkpoint, the candidate
// target with its advertised capabilities/scopes/host, and the
// verified capability set (the F12.1 registry view) the target
// must cover.
type VerifyReassignInput struct {
	Rec          *recovery.RunRecord
	TaskID       string
	FromAgent    string
	ToAgent      string
	ToCaps       []string
	VerifiedCaps []string
	ToScopes     []string
	ToHost       string
	// WorktreeState attests the integration area: only "clean"
	// admits reassignment ("dirty" or "unknown" refuse — inspect
	// before moving work elsewhere).
	WorktreeState string
	Reason        string
}

// VerifyReassign attests a reassignment candidate without moving
// anything: worktree clean, checkpoint live and found, target id
// valid, every required capability advertised and verified,
// target scopes narrowing the stored ones (delegation only
// restricts, via recovery.CheckScopesSubset), and the target
// host circuit closed. The returned detail cites the evidence for
// the decision record; any refusal names the failing gate. The
// atomic enforcement stays in recovery.Reassign (run by
// `recovery resume --new-agent`), which re-validates at move
// time — this guard never replaces it.
func VerifyReassign(in VerifyReassignInput, now time.Time, p Policy) (string, error) {
	if in.Rec == nil {
		return "", fmt.Errorf("failure: reassignment needs the live F12.5 run record (never verify against a copy)")
	}
	if err := p.Validate(); err != nil {
		return "", err
	}
	if strings.ToLower(strings.TrimSpace(in.WorktreeState)) != "clean" {
		return "", fmt.Errorf("failure: worktree state %q is not clean (inspect the integration area before moving work)", in.WorktreeState)
	}
	src, err := in.Rec.Lookup(in.TaskID, in.FromAgent)
	if err != nil {
		return "", fmt.Errorf("failure: reassignment source unreadable: %w", err)
	}
	if err := mission.ValidateID(in.ToAgent); err != nil {
		return "", fmt.Errorf("failure: invalid reassignment target: %w", err)
	}
	verified := map[string]bool{}
	for _, v := range in.VerifiedCaps {
		verified[v] = true
	}
	have := map[string]bool{}
	for _, c := range in.ToCaps {
		have[c] = true
	}
	for _, need := range src.RequiredCaps {
		if !have[need] {
			return "", fmt.Errorf("failure: target %q lacks required capability %q (capabilities are verified, never assumed)", in.ToAgent, need)
		}
		if !verified[need] {
			return "", fmt.Errorf("failure: capability %q is not verified (never assign unverified capability)", need)
		}
	}
	if err := recovery.CheckScopesSubset(in.ToScopes, src.Scopes); err != nil {
		return "", fmt.Errorf("failure: reassignment %w", err)
	}
	if strings.TrimSpace(in.ToHost) != "" && in.Rec.CircuitOpen(in.ToHost, now, p.Retry) {
		return "", fmt.Errorf("failure: target host %q circuit is open (reassign elsewhere or wait out the cooldown)", in.ToHost)
	}
	return fmt.Sprintf("verified %s->%s caps=%v scopes=%v host=%s worktree=clean (%s)",
		in.FromAgent, in.ToAgent, in.ToCaps, in.ToScopes, in.ToHost, in.Reason), nil
}
