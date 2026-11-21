// Error classification (F12.5/NOV-100): map a failure signal to
// the closed class vocabulary, deciding retryability and routing.
//
// Conservative by construction: only permanent and auth refuse
// every retry; host_offline additionally refuses the same host
// (reassign elsewhere); partial never blind-retries — it resumes
// through the effect ledger, skipping what is already applied.
// Unknown signals stay retryable but bounded: the budget, the
// timeout and the circuit breaker cap them anyway.
package recovery

import (
	"strings"
)

// ClassifyError maps a failure signal (error text, signal name) to
// its class. Matching is case-insensitive substring on purpose:
// callers pass raw stderr/exit text, never a prettified label.
func ClassifyError(signal string) string {
	s := strings.ToLower(strings.TrimSpace(signal))
	switch {
	case s == "", strings.Contains(s, "cancel"):
		return ClassCancelledE
	case containsAny(s, []string{"unauthorized", "forbidden", "permission denied", "invalid scope", "auth failed", "not authorized", "policy denied"}):
		return ClassAuth
	case containsAny(s, []string{"invalid", "malformed", "refused: permanent", "not supported", "assertion", "permanent"}):
		return ClassPermanent
	case containsAny(s, []string{"partial", "some effects applied", "half-applied", "incomplete apply"}):
		return ClassPartial
	case containsAny(s, []string{"no such host", "host offline", "host unreachable", "host down", "node offline"}):
		return ClassHostOff
	case containsAny(s, []string{"deadline exceeded", "timed out", "timedout", "timeout after", "context deadline"}):
		return ClassTimeout
	case containsAny(s, []string{"connection refused", "connection reset", "network unreachable", "dns", "tls handshake", "broken pipe", "502", "503", "504"}):
		return ClassNetwork
	case containsAny(s, []string{"panic", "fatal", "killed", "segfault", "sigkill", "sigsegv", "crash", "process died", "exit 137"}):
		return ClassCrash
	default:
		return ClassUnknown
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Retryable reports whether the class may ever be retried. permanent
// and auth never retry (retrying a refusal is not recovery); partial
// does not "retry" either — it resumes through the ledger, which the
// caller models as a resume, not a retry.
func Retryable(class string) bool {
	switch class {
	case ClassPermanent, ClassAuth, ClassCancelledE, ClassPartial:
		return false
	}
	return true
}

// SameHostAllowed reports whether the next attempt may stay on the
// same host. host_offline must move (the host is the failure);
// anything else may stay while the circuit is closed.
func SameHostAllowed(class string) bool {
	return class != ClassHostOff
}
