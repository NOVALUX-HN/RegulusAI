// Failure classification (F13.5/NOV-108): map one mission-level
// signal onto the closed 10-class vocabulary. The F12.5 base
// (recovery.ClassifyError) runs first and is extended, never
// reimplemented: crash/network/timeout resume as transient,
// host_offline stays host_offline, permanent stays permanent,
// auth becomes permission_denied, partial resumes through the
// ledger (transient with the no-blind-retry note), cancelled and
// unknown stay unknown (fail closed, ask).
//
// Mission-specific evidence wins by precedence before the base
// mapping: corrupted_state (tampered evidence poisons everything),
// permission_denied (never retry a refusal), budget_exceeded
// (never retry an exhausted ceiling), merge_conflict
// (deterministic — retries cannot fix it), agent_offline (the
// worker is the failure, reassign it), then the F12.5 base, with
// gate-red resolving to validation_failed. Matching is
// case-insensitive substring on purpose: callers pass raw
// stderr/exit text, never a prettified label.
package failure

import (
	"fmt"
	"strings"

	"regulus-ai/internal/recovery"
)

// Classify maps one signal onto the closed class vocabulary. An
// explicit ClassOverride wins when it names a known class and
// refuses otherwise (never guess a label the operator misspelled).
// The returned detail names the winning evidence for the audit.
func Classify(s Signal) (class, detail string, err error) {
	if strings.TrimSpace(s.ClassOverride) != "" {
		c := strings.ToLower(strings.TrimSpace(s.ClassOverride))
		if !validClass(c) {
			return "", "", fmt.Errorf("failure: unknown class %q (want %s)", s.ClassOverride, strings.Join(AllClasses, "|"))
		}
		return c, "explicit operator override", nil
	}
	text := strings.ToLower(strings.TrimSpace(s.ErrorText))
	gate := strings.ToLower(strings.TrimSpace(s.GateVerdict))
	if corrupted(text) {
		return ClassCorruptedState, "corrupted-state marker in failure evidence", nil
	}
	if permissioned(text) {
		return ClassPermissionDenied, "permission-denied marker in failure evidence", nil
	}
	if s.BudgetExhausted || budgeted(text) {
		return ClassBudgetExceeded, "budget ceiling hit (flag or budget marker)", nil
	}
	if conflicting(text) {
		return ClassMergeConflict, "merge-conflict marker in failure evidence", nil
	}
	if agentGone(text) {
		return ClassAgentOffline, "agent-offline marker in failure evidence", nil
	}
	base := recovery.ClassifyError(s.ErrorText)
	switch base {
	case recovery.ClassHostOff:
		return ClassHostOffline, "F12.5 host_offline (same host refused, reassign elsewhere)", nil
	case recovery.ClassAuth:
		return ClassPermissionDenied, "F12.5 auth (delegation only restricts, never retry a refusal)", nil
	case recovery.ClassPermanent:
		return ClassPermanent, "F12.5 permanent (deterministic, retries cannot fix it)", nil
	case recovery.ClassPartial:
		return ClassTransient, "F12.5 partial: resume through the effect ledger, never blind-retry", nil
	case recovery.ClassCrash, recovery.ClassNetwork, recovery.ClassTimeout:
		return ClassTransient, "F12.5 " + base + " (bounded retry, same key replays stored results)", nil
	}
	if gate == "red" || validated(text) {
		return ClassValidationFailed, "red gate or validation marker in failure evidence", nil
	}
	if base == recovery.ClassCancelledE {
		return ClassUnknown, "cancelled signal is operator intent, not a classified failure (fail closed, ask)", nil
	}
	return ClassUnknown, "no marker matched (fail closed, ask)", nil
}

func corrupted(s string) bool {
	return containsAny(s, []string{
		"sha mismatch", "checksum", "corrupt", "tampered",
		"version drift", "id drift", "malformed store",
		"unsupported version", "manifest mismatch",
	})
}

func permissioned(s string) bool {
	return containsAny(s, []string{
		"unauthorized", "forbidden", "permission denied",
		"invalid scope", "auth failed", "not authorized",
		"policy denied", "mcp denied", "scope denied",
	})
}

func budgeted(s string) bool {
	return containsAny(s, []string{
		"budget exceeded", "quota exceeded", "out of tokens",
		"cost limit", "time limit exceeded", "presupuesto agotado",
		"budget exhausted", "token limit", "rate limited",
	})
}

func conflicting(s string) bool {
	return containsAny(s, []string{
		"merge conflict", "conflicting files", "conflict markers",
		"concurrent modification", "worktree conflict",
		"already exists", "divergent branches",
	})
}

func agentGone(s string) bool {
	return containsAny(s, []string{
		"worker lost", "agent missing", "no heartbeat",
		"worker disappeared", "agent offline", "worker gone",
		"process vanished", "agent unreachable",
	})
}

func validated(s string) bool {
	return containsAny(s, []string{
		"test failed", "gate red", "verification failed",
		"regression detected", "contract violated",
		"acceptance criteria unmet", "suite red",
	})
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// RetryableClass reports whether the class may ever spend a retry.
// Only transient (bounded by the shared F12.5 budget) retries;
// everything else resumes, replans, isolates, rolls back or stops
// through its own path.
func RetryableClass(class string) bool {
	return class == ClassTransient
}

// ConfidenceFor tags the F12.6 lesson self-assessment: medium when
// the pattern is operational (transient, validation, offline,
// conflict), low when the evidence is thin or the stop is
// definitive. Never high — the coordinator proposes hypotheses,
// it never certifies facts.
func ConfidenceFor(class string) string {
	switch class {
	case ClassTransient, ClassValidationFailed,
		ClassAgentOffline, ClassHostOffline, ClassMergeConflict:
		return "medium"
	default:
		return "low"
	}
}
