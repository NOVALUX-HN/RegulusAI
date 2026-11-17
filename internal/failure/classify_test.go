// Classification tests (F13.5/NOV-108): the 10-class vocabulary
// over the required failure shapes. The F12.5 base is extended,
// never reimplemented — host_offline, permanent and auth map
// through recovery's verdicts first.
package failure

import (
	"testing"
)

func classOf(t *testing.T, s Signal) string {
	t.Helper()
	c, _, err := Classify(s)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	return c
}

func TestClassifyCoordinatorCrashIsTransient(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "coordinator process died (exit 137)"}); got != ClassTransient {
		t.Fatalf("coordinator crash class = %q, want transient", got)
	}
}

func TestClassifyWorkerMissingIsAgentOffline(t *testing.T) {
	for _, text := range []string{"worker lost heartbeat on t3", "agent missing: no heartbeat for 5m", "worker disappeared mid-task"} {
		if got := classOf(t, Signal{ErrorText: text}); got != ClassAgentOffline {
			t.Fatalf("worker-missing %q class = %q, want agent_offline", text, got)
		}
	}
}

func TestClassifyHostOfflineReusesF125(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "host unreachable: node offline"}); got != ClassHostOffline {
		t.Fatalf("host down class = %q, want host_offline", got)
	}
}

func TestClassifyMergeConflict(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "merge conflict in internal/cmdx/mission.go"}); got != ClassMergeConflict {
		t.Fatalf("merge conflict class = %q, want merge_conflict", got)
	}
}

func TestClassifyGateRedIsValidationFailed(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "suite finished", GateVerdict: "red"}); got != ClassValidationFailed {
		t.Fatalf("gate red class = %q, want validation_failed", got)
	}
	if got := classOf(t, Signal{ErrorText: "regression detected in quality gate"}); got != ClassValidationFailed {
		t.Fatalf("regression class = %q, want validation_failed", got)
	}
}

func TestClassifyBudgetExhausted(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "tokens spent, looks slow", BudgetExhausted: true}); got != ClassBudgetExceeded {
		t.Fatalf("budget flag class = %q, want budget_exceeded", got)
	}
	if got := classOf(t, Signal{ErrorText: "budget exceeded: cost limit 50.00 hit"}); got != ClassBudgetExceeded {
		t.Fatalf("budget text class = %q, want budget_exceeded", got)
	}
}

func TestClassifyPermissionDenied(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "mcp denied: policy refused scope memory:write"}); got != ClassPermissionDenied {
		t.Fatalf("denial class = %q, want permission_denied", got)
	}
}

func TestClassifyCorruptStatePoisonsFirst(t *testing.T) {
	// Corruption wins over every other marker: tampered bytes
	// are never evidence for a "friendlier" class.
	if got := classOf(t, Signal{ErrorText: "sha mismatch but also connection reset"}); got != ClassCorruptedState {
		t.Fatalf("corrupt class = %q, want corrupted_state", got)
	}
}

func TestClassifyPermanentAndUnknown(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "refused: permanent plan invalid"}); got != ClassPermanent {
		t.Fatalf("permanent class = %q, want permanent", got)
	}
	if got := classOf(t, Signal{ErrorText: "something odd happened"}); got != ClassUnknown {
		t.Fatalf("odd class = %q, want unknown", got)
	}
	if got := classOf(t, Signal{}); got != ClassUnknown {
		t.Fatalf("empty class = %q, want unknown", got)
	}
}

func TestClassifyOverrideValidated(t *testing.T) {
	if got := classOf(t, Signal{ErrorText: "boom", ClassOverride: "merge_conflict"}); got != ClassMergeConflict {
		t.Fatalf("override class = %q, want merge_conflict", got)
	}
	if _, _, err := Classify(Signal{ClassOverride: "meltdown"}); err == nil {
		t.Fatal("unknown override must refuse, never guess")
	}
}

func TestRetryableOnlyTransient(t *testing.T) {
	if !RetryableClass(ClassTransient) {
		t.Fatal("transient must be retryable within budget")
	}
	for _, c := range []string{ClassPermanent, ClassPermissionDenied, ClassBudgetExceeded, ClassMergeConflict, ClassValidationFailed, ClassCorruptedState, ClassUnknown, ClassAgentOffline, ClassHostOffline} {
		if RetryableClass(c) {
			t.Fatalf("class %q must never blind-retry", c)
		}
	}
}
