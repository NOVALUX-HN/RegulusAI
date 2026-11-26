package context

import (
	"strings"
	"testing"
)

// baseline is a recommendation that should adopt: measured on both sides, no
// quality loss, essentials kept, reversal named.
func baseline() Recommendation {
	return Recommendation{
		Name:                "minimal-profile",
		Baseline:            Metric{Value: 1000, Provenance: ProvenanceMeasuredHost},
		Candidate:           Metric{Value: 600, Provenance: ProvenanceMeasuredHost},
		EssentialsPreserved: true,
		Reversal:            "TE_TOOLSET=full",
	}
}

// TestEvaluateAdoptsOnlyAMeasuredSaving is the happy path, and it is the only path
// that may return adopt.
func TestEvaluateAdoptsOnlyAMeasuredSaving(t *testing.T) {
	verdict, reason := Evaluate(baseline())
	if verdict != VerdictAdopt {
		t.Fatalf("verdict = %q, want adopt (%s)", verdict, reason)
	}
}

// TestEvaluateRejectsQualityLossWhateverTheTokens: the phase's first rule.
func TestEvaluateRejectsQualityLossWhateverTheTokens(t *testing.T) {
	r := baseline()
	r.QualityRegressed = true
	if verdict, _ := Evaluate(r); verdict != VerdictReject {
		t.Fatalf("a quality regression must reject, got %q", verdict)
	}
}

// TestEvaluateRejectsLostEssentials: quality first, before any arithmetic.
func TestEvaluateRejectsLostEssentials(t *testing.T) {
	r := baseline()
	r.EssentialsPreserved = false
	if verdict, _ := Evaluate(r); verdict != VerdictReject {
		t.Fatalf("losing essentials must reject, got %q", verdict)
	}
}

// TestEvaluateRejectsNoSaving: a candidate that is not cheaper is not a candidate.
func TestEvaluateRejectsNoSaving(t *testing.T) {
	r := baseline()
	r.Candidate = Metric{Value: 1000, Provenance: ProvenanceMeasuredHost}
	if verdict, _ := Evaluate(r); verdict != VerdictReject {
		t.Fatalf("no saving must reject, got %q", verdict)
	}
}

// TestEvaluateRefusesToAdoptOnAProxy is the honesty rule the phase turns on: a
// proxy saving is a reason to keep measuring, never a reason to promote.
func TestEvaluateRefusesToAdoptOnAProxy(t *testing.T) {
	for _, provenance := range []string{ProvenanceProxy, ProvenanceEstimated, ProvenanceUnknown, ""} {
		r := baseline()
		r.Candidate = Metric{Value: 600, Provenance: provenance}
		verdict, reason := Evaluate(r)
		if verdict != VerdictInconclusive {
			t.Fatalf("provenance %q must be inconclusive, got %q", provenance, verdict)
		}
		if !strings.Contains(reason, "not measured") {
			t.Fatalf("the reason must say why: %q", reason)
		}
	}
}

// TestEvaluateRejectsWithoutAReversal: a promotion that cannot be undone is not a
// promotion, it is a bet.
func TestEvaluateRejectsWithoutAReversal(t *testing.T) {
	r := baseline()
	r.Reversal = ""
	if verdict, _ := Evaluate(r); verdict != VerdictReject {
		t.Fatalf("a promotion without a reversal must reject, got %q", verdict)
	}
}

// TestEvaluateOrderIsQualityFirst pins the precedence: a quality regression wins
// even when essentials are also lost, so the reason names the real cause.
func TestEvaluateOrderIsQualityFirst(t *testing.T) {
	r := baseline()
	r.QualityRegressed = true
	r.EssentialsPreserved = false
	verdict, reason := Evaluate(r)
	if verdict != VerdictReject || !strings.Contains(reason, "quality regression") {
		t.Fatalf("quality must be named first, got %q / %q", verdict, reason)
	}
}
