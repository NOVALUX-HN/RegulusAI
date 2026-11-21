// Recovery decisions (F13.5/NOV-108): select exactly one of the
// seven actions for a classified signal, with the mission-level
// budgets enforced and every loser recorded as discarded.
//
// Decision matrix (class -> decision, budgets in Policy):
//
//	transient        -> retry while RetriesUsed < MaxRetries,
//	                    else pause_for_approval (escalate the
//	                    exhausted budget, never spin silently)
//	agent_offline /
//	host_offline     -> reassign when the bid is verified,
//	                    else isolate (quarantine, never continue
//	                    blind on a missing worker)
//	merge_conflict   -> isolate (serialize/manual; retries cannot
//	                    fix a deterministic conflict)
//	validation_failed-> replan while ReplansUsed < MaxReplans and
//	                    the confirmed-set guard holds, else
//	                    pause_for_approval
//	budget_exceeded  -> pause_for_approval (ceilings never retry)
//	permanent        -> rollback when compensations are pending
//	                    and mission-owned, else fail_closed
//	                    (suggesting failed: definitive stop)
//	permission_denied/
//	corrupted_state /
//	unknown          -> fail_closed (suggesting blocked: park and
//	                    ask; nothing proceeds, nothing is marked
//	                    done)
//
// Rollback is also selected for validation_failed with an
// exhausted replan budget when compensations are pending and
// mission-owned; forward recovery refused plus owned undo steps
// means unwind, not stall.
package failure

import (
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/mission"
)

// Decide classifies the signal and selects one recovery decision
// under the policy. Pure: no store, no mutation — the caller
// persists the returned record with AppendDecision. A policy or
// classification refusal returns an error (fail closed before
// anything is recorded).
func Decide(sig Signal, bid ReassignBid, p Policy, now time.Time) (DecisionRecord, error) {
	if err := p.Validate(); err != nil {
		return DecisionRecord{}, err
	}
	class, detail, err := Classify(sig)
	if err != nil {
		return DecisionRecord{}, err
	}
	id, err := MintDecisionID()
	if err != nil {
		return DecisionRecord{}, err
	}
	rec := DecisionRecord{
		ID: id, At: now.UTC(),
		MissionID:    strings.TrimSpace(sig.MissionID),
		MissionState: strings.TrimSpace(sig.MissionState),
		Cause:        causeOf(sig),
		Class:        class,
		ClassDetail:  detail,
		Evidence:     evidenceOf(sig),
		Actor:        sig.Actor,
		IdemKey:      sig.IdemKey,
	}
	if rec.MissionID == "" {
		return DecisionRecord{}, fmt.Errorf("failure: mission id is required (every decision is evidence for one mission)")
	}
	decision, reason, discarded, backoff, result := selectDecision(sig, bid, p, class)
	rec.Decision = decision
	rec.Reason = reason
	rec.Discarded = discarded
	rec.BackoffMS = backoff
	rec.Result = result
	if bid.Present {
		rec.ReassignDetail = bid.Detail
	}
	rec.SuggestedMissionTo = suggestMissionEdge(rec.MissionState, decision, class)
	return rec, nil
}

// selectDecision applies the matrix. Guards (replan confirmed-set,
// rollback scope) are enforced here so a decision can never name
// an action its preconditions refuse: violations degrade to the
// safe fallback with the reason recorded.
func selectDecision(sig Signal, bid ReassignBid, p Policy, class string) (decision, reason string, discarded []string, backoff int64, result string) {
	switch class {
	case ClassTransient:
		if sig.BudgetExhausted {
			return DecisionPauseForApproval,
				"transient signal but an exhausted budget flag is set: ceilings never retry, a human owns the next spend",
				[]string{"retry (budget flag exhausted)", "reassign (host healthy, worker present)", "rollback (forward path open)"},
				0,
				"proposed: mission pause, human approves the next spend"
		}
		if sig.RetriesUsed < p.Retry.MaxRetries {
			backoff = p.Retry.BackoffDelay(sig.RetriesUsed + 1)
			return DecisionRetry,
				fmt.Sprintf("transient failure within budget (attempt %d of %d, deterministic backoff %dms): resume through the F12.5 ledger, skipping applied effects, never re-executing them",
					sig.RetriesUsed+1, p.Retry.MaxRetries, backoff),
				[]string{"reassign (worker and host healthy)", "replan (no plan defect evidenced)", "rollback (forward path open)"},
				backoff,
				"proposed: recovery retry/resume on the same idempotency key"
		}
		return DecisionPauseForApproval,
			fmt.Sprintf("transient failure with the retry budget spent (%d/%d): stop spinning, escalate with the evidence attached",
				sig.RetriesUsed, p.Retry.MaxRetries),
			[]string{"retry (budget exhausted)", "reassign (no worker/host defect)", "fail_closed (recovery may still be possible with approval)"},
			0,
			"proposed: mission pause, human approves or extends the budget"
	case ClassAgentOffline, ClassHostOffline:
		who := "worker"
		if class == ClassHostOffline {
			who = "host"
		}
		if bid.Present && bid.Verified {
			return DecisionReassign,
				fmt.Sprintf("%s is the failure (%s), and the %s bid is verified (capabilities, context, worktree state, scopes): move the checkpoint, never re-collect evidence",
					who, class, bid.ToAgent),
				[]string{"retry (the " + who + " is gone, retries would hit the same wall)", "replan (the plan is sound, the executor is missing)", "fail_closed (a verified target exists)"},
				0,
				fmt.Sprintf("proposed: recovery resume --new-agent %s (narrowed scopes, verified caps)", bid.ToAgent)
		}
		why := "no reassignment candidate was offered"
		if bid.Present {
			why = "the reassignment bid failed verification: " + bid.Detail
		}
		return DecisionIsolate,
			fmt.Sprintf("%s is the failure (%s) but %s: quarantine the affected tasks so nothing else schedules onto the missing %s",
				who, class, why, who),
			[]string{"retry (missing " + who + " cannot serve retries)", "reassign (unverified — scopes or caps unproven)", "fail_closed (isolation plus escalation may still recover)"},
			0,
			"proposed: isolate affected tasks, verify a target, then recovery resume --new-agent"
	case ClassMergeConflict:
		return DecisionIsolate,
			"merge conflict is deterministic: retries and reassignment carry the same conflict along, so quarantine the scope for serialize/manual resolution, never auto-resolve",
			[]string{"retry (deterministic, retries cannot fix it)", "reassign (the conflict follows the work, not the worker)", "replan (the plan is sound, the integration is not)"},
			0,
			"proposed: isolate the conflicting scope, resolve manually, then resume"
	case ClassValidationFailed:
		if sig.ReplansUsed < p.MaxReplans {
			if err := CheckReplan(sig.Confirmed, sig.PlanTasks, sig.ModifiedPlan); err != nil {
				return DecisionPauseForApproval,
					"red gate with replan budget left, but the replan guard refused (" + err.Error() + "): confirmed results are never invalidated, a human owns the replan",
					[]string{"replan (guard refused: confirmed set unprotected)", "retry (the gate is red, not flaky)", "fail_closed (approval may still replan safely)"},
					0,
					"proposed: mission pause, human replans without touching confirmed tasks"
			}
			if sig.CompensationsPending > 0 && rollbackOwned(sig) {
				return DecisionRollback,
					"red gate with owned compensations pending: unwind the transactional unit explicitly (LIFO, mission-owned keys only) instead of layering a replan over applied effects",
					[]string{"replan (applied effects unwind first)", "retry (the gate is red, not flaky)", "isolate (the unit is compensable, unwind it)"},
					0,
					"proposed: recovery rollback (mission-owned compensations, LIFO)"
			}
			return DecisionReplan,
				fmt.Sprintf("red gate with replan budget left (%d of %d) and the confirmed set intact: replan dependents without repeating completed tasks or invalidating confirmed results",
					sig.ReplansUsed+1, p.MaxReplans),
				[]string{"retry (the gate is red, not flaky)", "reassign (the workers served, the change failed)", "rollback (no compensable unit pending)"},
				0,
				"proposed: mission replan preserving confirmed tasks"
		}
		if sig.CompensationsPending > 0 && rollbackOwned(sig) {
			return DecisionRollback,
				"red gate with the replan budget spent and owned compensations pending: unwind the transactional unit explicitly instead of stalling over applied effects",
				[]string{"replan (budget spent)", "retry (the gate is red, not flaky)", "pause_for_approval (rollback first, then escalate what remains)"},
				0,
				"proposed: recovery rollback (mission-owned compensations, LIFO)"
		}
		return DecisionPauseForApproval,
			fmt.Sprintf("red gate with the replan budget spent (%d/%d): park for approval with gate evidence attached, never widen the suite by guessing",
				sig.ReplansUsed, p.MaxReplans),
			[]string{"replan (budget spent)", "retry (the gate is red, not flaky)", "fail_closed (approval may still replan safely)"},
			0,
			"proposed: mission pause, human approves the next plan"
	case ClassBudgetExceeded:
		return DecisionPauseForApproval,
			"budget ceiling hit: ceilings never retry and never auto-extend, a human approves or raises the budget with the spend evidence attached",
			[]string{"retry (ceilings never retry)", "replan (no new spend without approval)", "rollback (unwind only on explicit human order)"},
			0,
			"proposed: mission pause, human approves or raises the budget"
	case ClassPermanent:
		if sig.CompensationsPending > 0 && rollbackOwned(sig) {
			return DecisionRollback,
				"permanent failure with owned compensations pending: the forward path is closed, unwind the transactional unit explicitly (LIFO, mission-owned keys only), never reverting work outside the mission",
				[]string{"retry (permanent never retries)", "replan (the defect is definitive, not plannable)", "pause_for_approval (rollback first, then stop)"},
				0,
				"proposed: recovery rollback (mission-owned compensations, LIFO)"
		}
		return DecisionFailClosed,
			"permanent failure with nothing compensable: stop safely and terminally, the evidence stays for the post-mortem",
			[]string{"retry (permanent never retries)", "replan (the defect is definitive)", "rollback (no compensations pending)"},
			0,
			"proposed: mission advance --to failed with the cause recorded"
	default: // ClassPermissionDenied, ClassCorruptedState, ClassUnknown
		why := map[string]string{
			ClassPermissionDenied: "a refusal is never retried and scopes only ever narrow: park, a human grants or denies explicitly",
			ClassCorruptedState:   "tampered or unreadable state is not evidence: park before anything reads intent from bytes nobody wrote",
			ClassUnknown:          "unclassified failure: park and ask, never guess a recovery path",
		}[class]
		return DecisionFailClosed,
			class + ": " + why,
			[]string{"retry (unproven path)", "reassign (unproven target)", "replan (unproven defect)", "rollback (unproven scope — run only on explicit human order)"},
			0,
			"proposed: mission advance --to blocked, human intervenes"
	}
}

// rollbackOwned reports whether a rollback's scope guard would
// pass, so selectDecision only names rollback when the undo steps
// are proven mission-owned.
func rollbackOwned(sig Signal) bool {
	return CheckRollbackScope(sig.CompensationKeys, sig.OwnedPrefixes) == nil
}

// suggestMissionEdge maps the decision onto the audited mission
// edge the operator runs next. Illegal edges (unknown or terminal
// sources) yield "": the record says so instead of suggesting a
// transition the state machine would refuse.
func suggestMissionEdge(from, decision, class string) string {
	if strings.TrimSpace(from) == "" || mission.IsTerminal(from) {
		return ""
	}
	want := map[string]string{
		DecisionRetry:            mission.StateRecovering,
		DecisionReassign:         mission.StateRecovering,
		DecisionReplan:           mission.StateRecovering,
		DecisionIsolate:          mission.StateBlocked,
		DecisionRollback:         mission.StateRecovering,
		DecisionPauseForApproval: mission.StateBlocked,
		DecisionFailClosed:       mission.StateBlocked,
	}[decision]
	if decision == DecisionFailClosed && class == ClassPermanent {
		want = mission.StateFailed
	}
	if want == "" || mission.CheckTransition(from, want) != nil {
		return ""
	}
	return want
}

// causeOf summarizes the intake for the audit: the raw error
// (trimmed), the gate verdict and the budget flag.
func causeOf(sig Signal) string {
	parts := []string{}
	if e := strings.TrimSpace(sig.ErrorText); e != "" {
		if len(e) > 512 {
			e = e[:512] + "…"
		}
		parts = append(parts, e)
	}
	if g := strings.ToLower(strings.TrimSpace(sig.GateVerdict)); g != "" {
		parts = append(parts, "gate="+g)
	}
	if sig.BudgetExhausted {
		parts = append(parts, "budget=exhausted")
	}
	if len(parts) == 0 {
		return "(no failure text: classified from mission state and gates)"
	}
	return strings.Join(parts, " | ")
}

// evidenceOf cites the intake refs plus the mission state the
// decision read, so every record traces back to receipts, traces
// and the confirmed mission snapshot.
func evidenceOf(sig Signal) []string {
	out := append([]string(nil), sig.EvidenceRefs...)
	if st := strings.TrimSpace(sig.MissionState); st != "" {
		out = append(out, "mission-state="+st)
	}
	if sig.EffectsApplied > 0 {
		out = append(out, fmt.Sprintf("effects-applied=%d", sig.EffectsApplied))
	}
	seen := map[string]bool{}
	uniq := out[:0]
	for _, e := range out {
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		uniq = append(uniq, e)
	}
	return uniq
}

// Explain renders why the record decided what it did: the class,
// the winning reason, the suggested edge and every discarded
// alternative. No new facts — only the sealed record, quoted.
func Explain(rec DecisionRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "decision %s on mission %s: %s (class %s)\n", rec.ID, rec.MissionID, rec.Decision, rec.Class)
	fmt.Fprintf(&b, "cause: %s\n", rec.Cause)
	fmt.Fprintf(&b, "reason: %s\n", rec.Reason)
	if rec.SuggestedMissionTo != "" {
		fmt.Fprintf(&b, "suggested mission edge: --to %s\n", rec.SuggestedMissionTo)
	} else {
		fmt.Fprintf(&b, "suggested mission edge: none (no legal edge from the recorded state)\n")
	}
	fmt.Fprintf(&b, "result: %s\n", rec.Result)
	if len(rec.Discarded) > 0 {
		fmt.Fprintf(&b, "discarded alternatives:\n")
		for _, d := range rec.Discarded {
			fmt.Fprintf(&b, "  - %s\n", d)
		}
	}
	return b.String()
}
