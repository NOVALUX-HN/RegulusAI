package evolve

import (
	"errors"
	"testing"
)

func goodMetrics() Metrics {
	return Metrics{Trials: 10, SuccessRate: 0.9, AcceptanceRate: 0.9, RegressionRate: 0.0,
		PolicyCompliance: 1.0, EvidenceCompleteness: 1.0, CostPerSuccess: 5, Tokens: 100, DurationMS: 50}
}

// Trial aggregates produce the S6 metric set.
func TestMetricsSummarize(t *testing.T) {
	trials := []Trial{
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 4, Tokens: 10, DurationMS: 5},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 6, Tokens: 10, DurationMS: 5},
		{Pass: false, Regressed: true, PolicyOK: true, EvidenceOK: false, Cost: 2, Tokens: 10, DurationMS: 5},
	}
	m := Summarize(trials)
	if m.Trials != 3 {
		t.Fatalf("want 3 trials, got %+v", m)
	}
	if m.SuccessRate != 2.0/3.0 || m.RegressionRate != 1.0/3.0 {
		t.Fatalf("bad rates: %+v", m)
	}
	if m.EvidenceCompleteness != 2.0/3.0 || m.CostPerSuccess != 5.0 {
		t.Fatalf("bad aggregates: %+v", m)
	}
	if Summarize(nil).Trials != 0 {
		t.Fatal("empty trials must summarize to zero metrics, never NaN")
	}
}

// A candidate that deteriorates mandatory guarantees is never adoptable.
func TestAdoptRefusesRegression(t *testing.T) {
	base := goodMetrics()
	worse := base
	worse.SuccessRate = 0.5
	if err := Adoptable(worse, base); !errors.Is(err, ErrMandatoryRegression) {
		t.Fatalf("lower success must refuse adoption, got %v", err)
	}
	reg := base
	reg.RegressionRate = 0.4
	if err := Adoptable(reg, base); !errors.Is(err, ErrMandatoryRegression) {
		t.Fatalf("higher regression rate must refuse adoption, got %v", err)
	}
	if err := Adoptable(base, base); err != nil {
		t.Fatalf("equal metrics must be adoptable: %v", err)
	}
}

func mkEvaluatedExp(t *testing.T, root string) Experiment {
	t.Helper()
	exp, err := ProposeExperiment(root, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Advance(root, StateRunning); err != nil {
		t.Fatal(err)
	}
	fullTrust := TrustCheck{Isolated: true, Provenance: "T9 receipts + gate manifests", Validations: true, Regressions: true, Policies: true, Reversible: true}
	if err := exp.AttestTrust(root, fullTrust); err != nil {
		t.Fatal(err)
	}
	if err := exp.SetOutcome("pass", "no regressions", "adopt"); err != nil {
		t.Fatal(err)
	}
	if err := exp.Advance(root, StateEvaluated); err != nil {
		t.Fatal(err)
	}
	return exp
}

// Adoption versions the change with human approval and an audit trail;
// rollback supersedes the version and hands back the snapshot.
func TestAdoptAndRollback(t *testing.T) {
	dir := t.TempDir()
	if err := SetBaseline(dir, "v0.9.0", goodMetrics()); err != nil {
		t.Fatal(err)
	}
	exp := mkEvaluatedExp(t, dir)
	if _, err := Adopt(dir, exp.ID, goodMetrics(), "snapshot-1", ""); err == nil {
		t.Fatal("adoption without human approval must be refused")
	}
	v, err := Adopt(dir, exp.ID, goodMetrics(), "snapshot-1", "ana: verified on T9")
	if err != nil {
		t.Fatalf("verified adoption must hold: %v", err)
	}
	if v.N != 1 || v.Superseded {
		t.Fatalf("first version must be N=1 active, got %+v", v)
	}
	stored, err := LoadExperiment(dir, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StateAdopted {
		t.Fatalf("adopted experiment must be ADOPTED, got %s", stored.State)
	}
	back, err := Rollback(dir, 1, "regression found in prod")
	if err != nil {
		t.Fatalf("rollback must hold: %v", err)
	}
	if back.Snapshot != "snapshot-1" || !back.Superseded {
		t.Fatalf("rollback must return the superseded snapshot, got %+v", back)
	}
	if _, err := Rollback(dir, 1, "again"); err == nil {
		t.Fatal("double rollback must be refused")
	}
	if _, err := Rollback(dir, 99, "ghost"); err == nil {
		t.Fatal("unknown version rollback must be refused")
	}
}

// Adoption without a stored baseline is refused: there is nothing to
// compare the candidate against.
func TestAdoptNeedsBaseline(t *testing.T) {
	dir := t.TempDir()
	exp := mkEvaluatedExp(t, dir)
	if _, err := Adopt(dir, exp.ID, goodMetrics(), "snapshot-1", "ana: ok"); err == nil {
		t.Fatal("adoption without a stored baseline must be refused")
	}
}
