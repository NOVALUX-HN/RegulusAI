package cli

// T16 NOV-42 (S16): governed self-improvement cycle over the existing
// evolve APIs (no API changes): baseline pins metrics, evaluate
// compares trials + cost + adversarial non-regression and reaches
// EVALUATED, adopt requires human approval + trust + snapshot,
// reject records its reason, rollback restores the snapshot file.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/evolve"
)

func writeCycleJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cycleBaseline() evolve.Metrics {
	return evolve.Metrics{Trials: 4, SuccessRate: 0.5, AcceptanceRate: 0.5, RegressionRate: 0.0, PolicyCompliance: 0.5, EvidenceCompleteness: 0.5, CostPerSuccess: 9, Tokens: 40, DurationMS: 20}
}

func cycleTrials() []evolve.Trial {
	return []evolve.Trial{
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 2, Tokens: 10, DurationMS: 5},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 2, Tokens: 10, DurationMS: 5},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 2, Tokens: 10, DurationMS: 5},
		{Pass: true, CriteriaMet: true, PolicyOK: true, EvidenceOK: true, Cost: 2, Tokens: 10, DurationMS: 5},
	}
}

func cycleHypothesis() evolve.Hypothesis {
	return evolve.Hypothesis{Problem: "p", RootCause: "r", Change: "c", Benefit: "b", PotentialRegressions: "rg", EvaluationCriteria: "ev"}
}

func cycleTrust() evolve.TrustCheck {
	return evolve.TrustCheck{Isolated: true, Provenance: "test receipts", Validations: true, Regressions: true, Policies: true, Reversible: true}
}

// mkCycleExp proposes an experiment and advances it to RUNNING with
// full trust attested, ready for `evolve evaluate`.
func mkCycleExp(t *testing.T, root, baseline, candidate string) evolve.Experiment {
	t.Helper()
	exp, err := evolve.ProposeExperiment(root, evolve.ExperimentSpec{
		Hypothesis: cycleHypothesis(), Baseline: baseline, Candidate: candidate,
		Dataset: "t-cycle", Domain: evolve.DomainINV, Budget: evolve.Budget{MaxRuns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Advance(root, evolve.StateRunning); err != nil {
		t.Fatal(err)
	}
	if err := exp.AttestTrust(root, cycleTrust()); err != nil {
		t.Fatal(err)
	}
	return exp
}

func TestEvolveBaselineSetShowRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	m := cycleBaseline()
	mfile := filepath.Join(dir, "metrics.json")
	writeCycleJSON(t, mfile, m)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "baseline", "--set", "--name", "vtest", "--file", mfile}); code != 0 {
		t.Fatalf("baseline set exit = %d, want 0", code)
	}
	if got := out.String(); !strings.Contains(got, "BASELINE_SET vtest") || !strings.Contains(got, "next:") {
		t.Fatalf("baseline set must print verdict + next, got %q", got)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "baseline", "--show", "--name", "vtest"}); code != 0 {
		t.Fatalf("baseline show exit = %d, want 0", code)
	}
	if got := out2.String(); !strings.Contains(got, "vtest") || !strings.Contains(got, "next:") {
		t.Fatalf("baseline show must print the pinned baseline + next, got %q", got)
	}
	c3, _, _ := newCLI(nil)
	if code := c3.Run([]string{"evolve", "baseline", "--show", "--name", "missing"}); code == 0 {
		t.Fatal("baseline show of a missing baseline must fail")
	}
}

func TestEvolveBaselineUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evolve", "baseline", "--set"}); code != 2 {
		t.Fatalf("baseline --set without name/file: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve baseline") {
		t.Fatalf("baseline misuse must print usage, got %q", errb.String())
	}
}

func TestEvolveCycleProposedToAdopted(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := evolve.SetBaseline(dir, "vtest", cycleBaseline()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "work.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	exp := mkCycleExp(t, dir, "vtest", "work.txt")
	trialsFile := filepath.Join(dir, "trials.json")
	writeCycleJSON(t, trialsFile, cycleTrials())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "evaluate", "--exp", exp.ID, "--baseline", trialsFile}); code != 0 {
		t.Fatalf("evaluate exit = %d, want 0 (stdout %q)", code, out.String())
	}
	if got := out.String(); !strings.Contains(got, "EVALUATED") || !strings.Contains(got, "ADOPTABLE") || !strings.Contains(got, "next:") {
		t.Fatalf("evaluate must print EVALUATED/ADOPTABLE + next, got %q", got)
	}
	stored, err := evolve.LoadExperiment(dir, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != evolve.StateEvaluated {
		t.Fatalf("after evaluate state = %s, want EVALUATED", stored.State)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "adopt", "--exp", exp.ID, "--approved-by", "ana: verified"}); code != 0 {
		t.Fatalf("adopt exit = %d, want 0 (stdout %q)", code, out2.String())
	}
	if got := out2.String(); !strings.Contains(got, "ADOPTED") || !strings.Contains(got, "next:") {
		t.Fatalf("adopt must print ADOPTED + next, got %q", got)
	}
	adopted, err := evolve.LoadExperiment(dir, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if adopted.State != evolve.StateAdopted {
		t.Fatalf("after adopt state = %s, want ADOPTED", adopted.State)
	}
	vers, err := evolve.ListVersions(dir)
	if err != nil || len(vers) != 1 || vers[0].Superseded {
		t.Fatalf("adopt must pin one active version, got %+v %v", vers, err)
	}
}

func TestEvolveAdoptRefusesWithoutApproval(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := evolve.SetBaseline(dir, "vtest", cycleBaseline()); err != nil {
		t.Fatal(err)
	}
	// Sensitive candidate: touches harness fundamentals (gates), so an
	// automated approver must never satisfy the human-approval gate.
	exp := mkCycleExp(t, dir, "vtest", "change gates execution policy for lint")
	trialsFile := filepath.Join(dir, "trials.json")
	writeCycleJSON(t, trialsFile, cycleTrials())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "evaluate", "--exp", exp.ID, "--baseline", trialsFile}); code != 0 {
		t.Fatal("sensitive experiment with passing trials must still reach EVALUATED")
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"evolve", "adopt", "--exp", exp.ID}); code == 0 {
		t.Fatal("adopt without --approved-by must fail")
	} else if !strings.Contains(errb2.String(), "approval") {
		t.Fatalf("missing approval must name the approval gate, got %q", errb2.String())
	}
	c3, _, errb3 := newCLI(nil)
	if code := c3.Run([]string{"evolve", "adopt", "--exp", exp.ID, "--approved-by", "auto"}); code == 0 {
		t.Fatal("adopt with an automated approver on a sensitive change must fail")
	} else if !strings.Contains(strings.ToLower(errb3.String()), "human") {
		t.Fatalf("sensitive refusal must demand a human, got %q", errb3.String())
	}
	stored, err := evolve.LoadExperiment(dir, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != evolve.StateEvaluated {
		t.Fatalf("refused adopt must keep EVALUATED, got %s", stored.State)
	}
}

func TestEvolveRollbackRestores(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := evolve.SetBaseline(dir, "vtest", cycleBaseline()); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "work.txt")
	if err := os.WriteFile(live, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	exp := mkCycleExp(t, dir, "vtest", "work.txt")
	trialsFile := filepath.Join(dir, "trials.json")
	writeCycleJSON(t, trialsFile, cycleTrials())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "evaluate", "--exp", exp.ID, "--baseline", trialsFile}); code != 0 {
		t.Fatal("evaluate must hold before rollback")
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "adopt", "--exp", exp.ID, "--approved-by", "ana: verified"}); code != 0 {
		t.Fatal("adopt must hold before rollback")
	}
	if err := os.WriteFile(live, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Evidence that rollback must never delete.
	keepDir := filepath.Join(dir, ".regulus", "evidence", "t-cycle")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(keepDir, "keep.json")
	if err := os.WriteFile(keep, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"evolve", "rollback", "--exp", exp.ID, "--reason", "drift"}); code != 0 {
		t.Fatalf("rollback exit = %d, want 0", code)
	}
	if got := out3.String(); !strings.Contains(got, "ROLLED_BACK") || !strings.Contains(strings.ToLower(got), "evidence") || !strings.Contains(got, "next:") {
		t.Fatalf("rollback must print verdict + evidence-kept + next, got %q", got)
	}
	if got, _ := os.ReadFile(live); string(got) != "original" {
		t.Fatalf("rollback must restore the snapshot over work.txt, got %q", got)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("rollback must not delete evidence: %v", err)
	}
	vers, err := evolve.ListVersions(dir)
	if err != nil || len(vers) != 1 || !vers[0].Superseded {
		t.Fatalf("rollback must supersede the version, got %+v %v", vers, err)
	}
}

func TestEvolveRejectListsReason(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := evolve.SetBaseline(dir, "vtest", cycleBaseline()); err != nil {
		t.Fatal(err)
	}
	exp := mkCycleExp(t, dir, "vtest", "candidate text")
	trialsFile := filepath.Join(dir, "trials.json")
	writeCycleJSON(t, trialsFile, cycleTrials())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "evaluate", "--exp", exp.ID, "--baseline", trialsFile}); code != 0 {
		t.Fatal("evaluate must hold before reject")
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "reject", "--exp", exp.ID, "--reason", "harmful: skips typecheck"}); code != 0 {
		t.Fatalf("reject exit = %d, want 0", code)
	}
	if got := out2.String(); !strings.Contains(got, "REJECTED") || !strings.Contains(got, "harmful: skips typecheck") {
		t.Fatalf("reject must list the reason, got %q", got)
	}
	stored, err := evolve.LoadExperiment(dir, exp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != evolve.StateRejected {
		t.Fatalf("after reject state = %s, want REJECTED", stored.State)
	}
	if !strings.Contains(stored.Decision, "harmful: skips typecheck") {
		t.Fatalf("reject reason must persist on the experiment, got %q", stored.Decision)
	}
}
