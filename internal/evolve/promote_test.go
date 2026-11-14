package evolve

import (
	"testing"
)

func promoteEvaluated(t *testing.T, root, candidate string) Experiment {
	t.Helper()
	exp, err := ProposeExperiment(root, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline:   "v0.9.0", Candidate: candidate, Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Advance(root, StateRunning); err != nil {
		t.Fatal(err)
	}
	full := TrustCheck{Isolated: true, Provenance: "T9 receipts", Validations: true, Regressions: true, Policies: true, Reversible: true}
	if err := exp.AttestTrust(root, full); err != nil {
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

func TestPromoteStatesPreserved(t *testing.T) {
	// Old edges still hold; new states only add.
	dir := t.TempDir()
	exp, err := ProposeExperiment(dir, ExperimentSpec{
		Hypothesis: Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"},
		Baseline: "b", Candidate: "c", Dataset: "d", Domain: DomainINV, Budget: Budget{MaxRuns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []string{StateRunning, StateEvaluated, StateRejected} {
		if err := exp.Advance(dir, to); err != nil {
			t.Fatalf("legacy edge to %s must hold: %v", to, err)
		}
	}
	if exp.State != StateRejected {
		t.Fatalf("want REJECTED, got %s", exp.State)
	}
}

func TestPromoteApprovalGatesCanary(t *testing.T) {
	dir := t.TempDir()
	exp := promoteEvaluated(t, dir, "improve prose renderer")
	scope := CanaryScope{Kind: "prompt", Reversible: true, Reason: "copy tweak, reversible"}
	if err := EnterCanary(dir, exp.ID, scope); err == nil {
		t.Fatal("canary without approval must be refused (bypass blocked)")
	}
	if err := ApproveForCanary(dir, exp.ID, CanaryApproval{}); err == nil {
		t.Fatal("empty approval must be refused")
	}
	if err := ApproveForCanary(dir, exp.ID, CanaryApproval{By: "ana", Reason: "low risk copy tweak"}); err != nil {
		t.Fatalf("human approval must hold: %v", err)
	}
	if err := EnterCanary(dir, exp.ID, scope); err != nil {
		t.Fatalf("approved canary entry must hold: %v", err)
	}
	stored, _ := LoadExperiment(dir, exp.ID)
	if stored.State != StateCanary {
		t.Fatalf("want CANARY, got %s", stored.State)
	}
	// Resume is idempotent: re-entry does not duplicate or fail.
	if err := EnterCanary(dir, exp.ID, scope); err != nil {
		t.Fatalf("canary re-entry must be idempotent for resume: %v", err)
	}
	if _, err := ResumeCanary(dir, exp.ID); err != nil {
		t.Fatalf("resume must hold on CANARY: %v", err)
	}
}

func TestPromoteScopeAndHighImpact(t *testing.T) {
	if err := ValidateCanaryScope(CanaryScope{Kind: "prompt", Reversible: true}); err != nil {
		t.Fatalf("prompt reversible must hold: %v", err)
	}
	if err := ValidateCanaryScope(CanaryScope{Kind: "db-migration", Reversible: true}); err == nil {
		t.Fatal("non-listed kind must be refused")
	}
	if err := ValidateCanaryScope(CanaryScope{Kind: "skill", Reversible: false}); err == nil {
		t.Fatal("irreversible scope must be refused")
	}
	if err := ValidateCanaryScope(CanaryScope{Kind: "workflow", Reversible: true, Critical: true}); err == nil {
		t.Fatal("critical scope must be refused for canary")
	}
	if !RequiresHumanApproval("rotate gate verdicts in production", CanaryScope{Kind: "prompt"}) {
		t.Fatal("gates/production text must force human approval")
	}
	if RequiresHumanApproval("improve prose renderer wording", CanaryScope{Kind: "prompt"}) {
		t.Fatal("benign copy must not force high-impact approval")
	}
	dir := t.TempDir()
	exp := promoteEvaluated(t, dir, "rotate gate verdicts in production")
	if err := ApproveForCanary(dir, exp.ID, CanaryApproval{By: "", Reason: ""}); err == nil {
		t.Fatal("high-impact without human must be refused")
	}
}

func TestPromoteAdoptAndPostAdoption(t *testing.T) {
	dir := t.TempDir()
	if err := SetBaseline(dir, "v0.9.0", goodMetrics()); err != nil {
		t.Fatal(err)
	}
	exp := promoteEvaluated(t, dir, "improve prose renderer")
	if err := ApproveForCanary(dir, exp.ID, CanaryApproval{By: "ana", Reason: "reversible prompt canary"}); err != nil {
		t.Fatal(err)
	}
	if err := EnterCanary(dir, exp.ID, CanaryScope{Kind: "prompt", Reversible: true, Reason: "copy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AdoptFromCanary(dir, exp.ID, goodMetrics(), "", "ana"); err == nil {
		t.Fatal("canary adoption without snapshot must be refused")
	}
	if _, err := AdoptFromCanary(dir, exp.ID, goodMetrics(), "snap-1", ""); err == nil {
		t.Fatal("canary adoption without human must be refused")
	}
	bad := goodMetrics()
	bad.SuccessRate = 0.1
	if _, err := AdoptFromCanary(dir, exp.ID, bad, "snap-1", "ana"); err == nil {
		t.Fatal("regressing candidate must be refused from canary")
	}
	v, err := AdoptFromCanary(dir, exp.ID, goodMetrics(), "snap-1", "ana: verified canary")
	if err != nil {
		t.Fatalf("canary adoption must hold: %v", err)
	}
	if v.N != 1 {
		t.Fatalf("want version 1, got %+v", v)
	}
	if err := PostAdoptionCheck(dir, exp.ID, goodMetrics()); err != nil {
		t.Fatalf("post-adoption vs baseline must hold: %v", err)
	}
	if err := PostAdoptionCheck(dir, exp.ID, bad); err == nil {
		t.Fatal("post-adoption regression must fail the check")
	}
}

func TestPromoteGovernedRollbackAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := SetBaseline(dir, "v0.9.0", goodMetrics()); err != nil {
		t.Fatal(err)
	}
	exp := promoteEvaluated(t, dir, "improve prose renderer")
	if err := ApproveForCanary(dir, exp.ID, CanaryApproval{By: "ana", Reason: "canary"}); err != nil {
		t.Fatal(err)
	}
	if err := EnterCanary(dir, exp.ID, CanaryScope{Kind: "skill", Reversible: true, Reason: "reversible"}); err != nil {
		t.Fatal(err)
	}
	v, err := AdoptFromCanary(dir, exp.ID, goodMetrics(), "snap-1", "ana")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := GovernedRollback(dir, exp.ID, v.N, "", "r", "m", "d", []string{"rec"}); err == nil {
		t.Fatal("rollback without operator must be refused")
	}
	if _, err := GovernedRollback(dir, exp.ID, v.N, "ana", "", "m", "d", []string{"rec"}); err == nil {
		t.Fatal("rollback without reason must be refused")
	}
	if _, err := GovernedRollback(dir, exp.ID, v.N, "ana", "r", "", "d", []string{"rec"}); err == nil {
		t.Fatal("rollback without manifest must be refused")
	}
	if _, err := GovernedRollback(dir, exp.ID, v.N, "ana", "r", "m", "d", nil); err == nil {
		t.Fatal("rollback without receipts must be refused")
	}
	rec, err := GovernedRollback(dir, exp.ID, v.N, "ana", "canary regression", "manifests/exp.json", "-old\n+new", []string{"rec_1"})
	if err != nil {
		t.Fatalf("governed rollback must hold: %v", err)
	}
	if rec.ManifestPath == "" || rec.Diff == "" || len(rec.ReceiptIDs) != 1 {
		t.Fatalf("rollback must cite manifest/diff/receipts: %+v", rec)
	}
	stored, _ := LoadExperiment(dir, exp.ID)
	if stored.State != StateRolledBack {
		t.Fatalf("want ROLLED_BACK, got %s", stored.State)
	}
	if _, err := ResumeCanary(dir, exp.ID); err != nil {
		t.Fatalf("resume must read ROLLED_BACK: %v", err)
	}
	versions, _ := ListVersions(dir)
	if len(versions) != 1 || !versions[0].Superseded {
		t.Fatalf("version must be superseded: %+v", versions)
	}
}
