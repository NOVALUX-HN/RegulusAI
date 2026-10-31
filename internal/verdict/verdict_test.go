// Package verdict is the centralized verdict contract (Phase 1, trust
// foundation): the core is the sole verdict authority. Agents and guardians
// produce evaluations, which are recorded but can never override the core.
package verdict

import (
	"testing"
	"time"
)

func TestAdjudicatePass(t *testing.T) {
	v := Adjudicate(Facts{Executed: true, ExitCode: 0})
	if v.Status != StatusPass || v.Reason != ReasonOK {
		t.Fatalf("want PASS/ok, got %s/%s", v.Status, v.Reason)
	}
	if v.ExitCode() != 0 || v.Legacy() != "pass" {
		t.Fatalf("PASS must exit 0 and map to legacy pass, got %d/%q", v.ExitCode(), v.Legacy())
	}
}

func TestAdjudicateMatrix(t *testing.T) {
	cases := []struct {
		name       string
		facts      Facts
		wantStatus Status
		wantReason Reason
	}{
		{"exit non-zero", Facts{Executed: true, ExitCode: 2}, StatusFail, ReasonDeterministicFailure},
		{"metric violated", Facts{Executed: true, ExitCode: 0, MetricViolated: true}, StatusFail, ReasonDeterministicFailure},
		{"policy refusal", Facts{Refused: true}, StatusBlocked, ReasonPolicyViolation},
		{"spawn error", Facts{SpawnError: true}, StatusBlocked, ReasonInfrastructureFailure},
		{"timeout", Facts{Executed: true, TimedOut: true, ExitCode: -1}, StatusBlocked, ReasonInfrastructureFailure},
		{"resource exhausted", Facts{Executed: true, ResourceExhausted: true, ExitCode: -1}, StatusBlocked, ReasonInfrastructureFailure},
		{"never executed", Facts{}, StatusInconclusive, ReasonInsufficientEvidence},
		{"executed without evidence", Facts{Executed: true, ExitCode: 0, EvidenceMissing: true}, StatusInconclusive, ReasonInsufficientEvidence},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Adjudicate(c.facts)
			if v.Status != c.wantStatus || v.Reason != c.wantReason {
				t.Fatalf("want %s/%s, got %s/%s", c.wantStatus, c.wantReason, v.Status, v.Reason)
			}
			if v.Status != StatusPass && v.ExitCode() != 1 {
				t.Fatalf("non-pass verdict must exit 1, got %d", v.ExitCode())
			}
			if v.Status != StatusPass && v.Legacy() != "fail" {
				t.Fatalf("non-pass verdict must map to legacy fail, got %q", v.Legacy())
			}
		})
	}
}

func TestAdjudicatePrecedenceRefusalBeatsAll(t *testing.T) {
	// A refused command never runs: refusal wins even if other flags are set.
	v := Adjudicate(Facts{Refused: true, TimedOut: true, ExitCode: 1})
	if v.Status != StatusBlocked || v.Reason != ReasonPolicyViolation {
		t.Fatalf("refusal must take precedence, got %s/%s", v.Status, v.Reason)
	}
}

func TestResolveEvaluationsNeverOverride(t *testing.T) {
	core := Adjudicate(Facts{Executed: true, ExitCode: 0})
	evals := []Evaluation{
		{Evaluator: "guardian-a", Status: StatusFail, Note: "disagrees", At: time.Now().UTC()},
		{Evaluator: "guardian-b", Status: StatusFail, Note: "disagrees too", At: time.Now().UTC()},
	}
	r := Resolve(core, evals)
	if r.Verdict != core {
		t.Fatalf("evaluations must never override the core verdict: want %v, got %v", core, r.Verdict)
	}
	if !r.Disputed {
		t.Fatal("unanimous disagreement must be recorded as disputed")
	}
	if len(r.Evaluations) != 2 {
		t.Fatalf("evaluations must be preserved, got %d", len(r.Evaluations))
	}
}

func TestResolveAgreementNotDisputed(t *testing.T) {
	core := Adjudicate(Facts{Executed: true, ExitCode: 1})
	r := Resolve(core, []Evaluation{{Evaluator: "guardian-a", Status: StatusFail}})
	if r.Disputed {
		t.Fatal("agreeing evaluation must not mark disputed")
	}
	if r.Verdict.Status != StatusFail {
		t.Fatalf("core verdict must stand, got %s", r.Verdict.Status)
	}
}

func TestParseRoundTrip(t *testing.T) {
	for _, s := range []Status{StatusPass, StatusFail, StatusBlocked, StatusInconclusive} {
		got, err := ParseStatus(string(s))
		if err != nil || got != s {
			t.Fatalf("ParseStatus(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseStatus("MAYBE"); err == nil {
		t.Fatal("unknown status must be rejected")
	}
	for _, r := range []Reason{ReasonOK, ReasonDeterministicFailure, ReasonPolicyViolation, ReasonInsufficientEvidence, ReasonInfrastructureFailure, ReasonEvaluatorDisagreement} {
		got, err := ParseReason(string(r))
		if err != nil || got != r {
			t.Fatalf("ParseReason(%q) = %v, %v", r, got, err)
		}
	}
	if _, err := ParseReason("vibes"); err == nil {
		t.Fatal("unknown reason must be rejected")
	}
}

func TestReasonEvaluatorDisagreementIsRecordedNotAssigned(t *testing.T) {
	// The core never adjudicates evaluator_disagreement by itself: it only
	// appears when a resolution records a dispute. Adjudicate must never emit it.
	for _, f := range []Facts{
		{},
		{Executed: true, ExitCode: 0},
		{Executed: true, ExitCode: 1},
		{Refused: true},
		{SpawnError: true},
		{Executed: true, TimedOut: true},
		{Executed: true, ResourceExhausted: true},
	} {
		if v := Adjudicate(f); v.Reason == ReasonEvaluatorDisagreement {
			t.Fatalf("Adjudicate(%+v) must never emit evaluator_disagreement", f)
		}
	}
}
