package observability

// G4 SPLIT pins: the helpers extracted from Summarize preserve its exact
// folding behavior (same verdicts, same messages, zeros never NaN).

import (
	"math"
	"testing"

	"regulus-ai/internal/receipts"
)

func TestSummarizeReceiptGroupsMixed(t *testing.T) {
	m := newMetrics()
	metric := 0.5
	byTask := map[string][]receipts.Receipt{
		"task-a": {
			{Verdict: receipts.VerdictPass, DurationMS: 1000, CPUTimeMS: 100},
			{Verdict: receipts.VerdictPass, DurationMS: 2000, CPUTimeMS: 200},
		},
		"task-b": {
			{Verdict: receipts.VerdictPass, DurationMS: 1500, CPUTimeMS: 150},
			{Verdict: receipts.VerdictFail, DurationMS: 500, CPUTimeMS: 50, MetricValue: &metric},
		},
		"task-c": {
			{Verdict: receipts.VerdictFail, DurationMS: 100, CPUTimeMS: 10},
		},
	}
	summarizeReceiptGroups(&m, byTask)

	if m.Tasks != 3 {
		t.Errorf("tasks = %d, want 3", m.Tasks)
	}
	if m.ReceiptsTotal != 5 || m.ReceiptsPass != 3 || m.ReceiptsFail != 2 {
		t.Errorf("receipts total/pass/fail = %d/%d/%d, want 5/3/2",
			m.ReceiptsTotal, m.ReceiptsPass, m.ReceiptsFail)
	}
	if m.TaskCompletionRate != 1.0/3.0 {
		t.Errorf("completion rate = %v, want %v (task-a only)", m.TaskCompletionRate, 1.0/3.0)
	}
	if m.VerificationPassRate != 0.6 || m.VerificationFailureRate != 0.4 {
		t.Errorf("pass/fail rates = %v/%v, want 0.6/0.4", m.VerificationPassRate, m.VerificationFailureRate)
	}
	// Only the metric-gated failure regresses; the plain failure does not.
	if m.RegressionRate != 0.2 {
		t.Errorf("regression rate = %v, want 0.2 (one metric-gated failure of 5)", m.RegressionRate)
	}
	if m.TotalDurationMS != 5100 || m.TotalCPUTimeMS != 510 {
		t.Errorf("totals = %dms/%dms, want 5100ms/510ms", m.TotalDurationMS, m.TotalCPUTimeMS)
	}
	if m.MeanDurationMS != 1020 {
		t.Errorf("mean duration = %vms, want 1020ms", m.MeanDurationMS)
	}
}

func TestFinishReceiptRatesZeroTotal(t *testing.T) {
	m := newMetrics()
	finishReceiptRates(&m, 0, 0)
	for name, f := range map[string]float64{
		"task_completion_rate": m.TaskCompletionRate, "verification_pass_rate": m.VerificationPassRate,
		"verification_failure_rate": m.VerificationFailureRate, "regression_rate": m.RegressionRate,
		"mean_duration_ms": m.MeanDurationMS,
	} {
		if f != 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("zero totals: %s = %v, want clean 0", name, f)
		}
	}
}

func TestSummarizeGuardiansAbsentKeepsHonestZero(t *testing.T) {
	m := newMetrics()
	if err := summarizeGuardians(t.TempDir(), "", &m); err != nil {
		t.Fatalf("summarizeGuardians on empty root: %v", err)
	}
	if m.GuardianDisagreementRate != 0 {
		t.Errorf("absent log: rate = %v, want honest 0", m.GuardianDisagreementRate)
	}
	if note := m.Coverage["guardian_disagreement_rate"]; note == "" {
		t.Error("absent log: coverage note must name the missing store")
	}
	found := false
	for _, u := range m.Unmeasured {
		if u == unmeasuredGuardian {
			found = true
		}
	}
	if !found {
		t.Errorf("absent log: unmeasured must keep %q (have %v)", unmeasuredGuardian, m.Unmeasured)
	}
}
