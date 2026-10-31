package evolve

import (
	"strings"
	"testing"
)

// A DOM improvement that touches harness fundamentals is a domain breach:
// reclassify as INV or narrow the candidate.
func TestDOMCannotTouchFundamentals(t *testing.T) {
	dir := t.TempDir()
	mk := func(domain, candidate string) Experiment {
		exp, err := ProposeExperiment(dir, ExperimentSpec{
			Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
			Baseline:   "v0.9.0", Candidate: candidate, Dataset: "d", Domain: domain, Budget: Budget{MaxRuns: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		return exp
	}
	dom := mk(DomainDOM, "change gates execution policy for lint")
	if err := CheckDomain(dom); err == nil {
		t.Fatal("DOM candidate touching gates must be a domain breach")
	} else if !strings.Contains(err.Error(), "INV") {
		t.Fatalf("breach must point at INV reclassification, got %v", err)
	}
	inv := mk(DomainINV, "change gates execution policy for lint")
	if err := CheckDomain(inv); err != nil {
		t.Fatalf("INV candidate may touch fundamentals: %v", err)
	}
	doc := mk(DomainDOM, "reword the go domain INDEX examples")
	if err := CheckDomain(doc); err != nil {
		t.Fatalf("DOM candidate clear of fundamentals must hold: %v", err)
	}
}

// Adoption needs the full untrusted-proposal checklist: isolated,
// provenance, validations, regressions, policies, reversible. A single
// missing item refuses adoption naming the item.
func TestAdoptRequiresFullTrust(t *testing.T) {
	dir := t.TempDir()
	if err := SetBaseline(dir, "v0.9.0", goodMetrics()); err != nil {
		t.Fatal(err)
	}
	exp := mkEvaluatedExp(t, dir)
	exp2, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exp2.Advance(dir, StateRunning); err != nil {
		t.Fatal(err)
	}
	partial := TrustCheck{Isolated: true, Provenance: "exp: T9 receipts", Validations: true, Regressions: true, Policies: true, Reversible: false}
	if err := exp2.AttestTrust(dir, partial); err != nil {
		t.Fatal(err)
	}
	if err := exp2.SetOutcome("pass", "no regressions", "adopt"); err != nil {
		t.Fatal(err)
	}
	if err := exp2.Advance(dir, StateEvaluated); err != nil {
		t.Fatal(err)
	}
	if _, err := Adopt(dir, exp2.ID, goodMetrics(), "snap", "ana: ok"); err == nil {
		t.Fatal("adoption with a missing checklist item must be refused")
	} else if !strings.Contains(err.Error(), "reversible") {
		t.Fatalf("refusal must name the missing item, got %v", err)
	}
	if _, err := Adopt(dir, exp.ID, goodMetrics(), "snap", "ana: ok"); err != nil {
		t.Fatalf("fully trusted adoption must hold: %v", err)
	}
}

// Trust is attested while the experiment runs, never before.
func TestAttestTrustOnlyWhileRunning(t *testing.T) {
	dir := t.TempDir()
	exp, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: "c", Dataset: "d", Domain: DomainDOM, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	full := TrustCheck{Isolated: true, Provenance: "x", Validations: true, Regressions: true, Policies: true, Reversible: true}
	if err := exp.AttestTrust(dir, full); err == nil {
		t.Fatal("attestation on a PROPOSED experiment must be refused")
	}
}
