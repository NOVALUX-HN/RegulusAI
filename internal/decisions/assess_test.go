package decisions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

func assessRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "charts", "regulus-ai-v1.md"), []byte("# chart\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func assessReceipt(t *testing.T, root, task string) string {
	t.Helper()
	dir := evidence.TaskDir(root, task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rec := receipts.Receipt{
		Task: task, Gate: "build", Layer: 1, Command: "echo ok",
		ExitCode: 0, Verdict: receipts.VerdictPass, StartedAt: time.Unix(1700000000, 0).UTC(),
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture("ok")
	rec.Stderr, rec.StderrSHA = receipts.Capture("")
	path, err := receipts.Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := receipts.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return stored.ID
}

func baseDecision() Contract {
	c, err := ParseContract([]byte(`{
		"problem": "gate runs repeat unchanged work on every invocation",
		"scope": "gate execution",
		"context": "full profile runs all 12 gates even when nothing relevant changed",
		"proposal": "add when-conditions so optional gates skip when no relevant gate inputs changed",
		"alternatives": [{"name": "keep all-always", "tradeoff": "simpler to reason about but slower on no-op runs"}],
		"tradeoffs": ["skip logic costs one git status per run"],
		"constraints": ["mandatory: required gates always run"],
		"risks": ["stale skip output -> fail open with a receipt note"],
		"consequences": "faster no-op runs with recorded skip decisions",
		"evidence": ["chart:regulus-ai-v1"],
		"confidence": "medium",
		"reversibility": "easy",
		"rollback": "",
		"artifacts": ["internal/x.go"],
		"commits": [],
		"task": "",
		"related": [],
		"critical": false
	}`))
	if err != nil {
		panic(err)
	}
	return c
}

func TestAssessAcceptsGroundedDecision(t *testing.T) {
	root := assessRoot(t)
	id := assessReceipt(t, root, "feat/T1")
	c := baseDecision()
	c.Evidence = append(c.Evidence, "rec:"+id)
	c.Task = "feat/T1"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Recommendation != "ACCEPT" || p.Abstained {
		t.Fatalf("grounded decision must be accepted, got %v/%v", p.Recommendation, p.Rationale)
	}
	if len(p.Findings) != 0 {
		t.Fatalf("no findings expected, got %+v", p.Findings)
	}
	for _, cr := range p.Criteria {
		if len(p.Criteria) != 10 {
			t.Fatalf("want exactly 10 criteria, got %d", len(p.Criteria))
		}
		if cr.Verdict != "pass" {
			t.Fatalf("criterion %q must pass, got %+v", cr.Name, cr)
		}
	}
}

func TestAssessAbstainsWithoutEvidence(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Evidence = []string{"rec:rec_nope", "chart:missing-chart"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Recommendation != "INCONCLUSIVE" || !p.Abstained {
		t.Fatalf("missing evidence must force abstention, got %s", p.Recommendation)
	}
	if !hasFinding(p, "INSUFFICIENT_EVIDENCE", "blocking") {
		t.Fatalf("must flag insufficient evidence, got %+v", p.Findings)
	}
	unresolved := 0
	for _, r := range p.Evidence {
		if !r.Resolved {
			unresolved++
		}
	}
	if unresolved != 2 {
		t.Fatalf("both refs must be unresolved, got %+v", p.Evidence)
	}
}

func TestAssessDetectsContradiction(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Proposal = "add when-conditions via go get example.com/mod v1.0.0 for glob speed"
	c.Evidence = []string{"chart:regulus-ai-v1"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "CONTRADICTION", "blocking") {
		t.Fatalf("new Go dependency contradicts the zero-dep invariant, got %+v", p.Findings)
	}
	if p.Recommendation != "REJECT" {
		t.Fatalf("blocking contradiction must reject, got %s", p.Recommendation)
	}
}

func TestAssessMissingAlternatives(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Alternatives = nil
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "MISSING_ALTERNATIVES", "blocking") {
		t.Fatalf("must flag missing alternatives, got %+v", p.Findings)
	}
	c.Alternatives = []Alternative{{Name: "only idea"}}
	p = Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "MISSING_ALTERNATIVES", "blocking") {
		t.Fatalf("tradeoff-less alternative must fail, got %+v", p.Findings)
	}
}

func TestAssessUnmitigatedRisk(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Risks = []string{"everything might break"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "UNMITIGATED_RISK", "blocking") {
		t.Fatalf("must flag unmitigated risk, got %+v", p.Findings)
	}
}

func TestAssessHiddenDependency(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Artifacts = []string{"internal/does-not-exist.go"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "HIDDEN_DEPENDENCY", "blocking") {
		t.Fatalf("must flag missing artifact, got %+v", p.Findings)
	}
}

func TestAssessUnsupportedAssumption(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Evidence = nil
	c.Consequences = "this will always be faster and can never regress"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !hasFinding(p, "UNSUPPORTED_ASSUMPTION", "warning") {
		t.Fatalf("absolute claims without evidence must warn, got %+v", p.Findings)
	}
}

func TestAssessObsoleteScope(t *testing.T) {
	root := assessRoot(t)
	other := Decision{Contract: baseDecision(), State: StateAccepted}
	other.Contract.ID = "dec_old"
	c := baseDecision()
	p := Assess(root, Decision{Contract: c, State: StateProposed}, []Decision{other})
	if !hasFinding(p, "OBSOLETE", "blocking") {
		t.Fatalf("accepted same-scope decision must block, got %+v", p.Findings)
	}
}

func TestAssessHardWithoutRollback(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Reversibility = "hard"
	c.Rollback = ""
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Recommendation == "ACCEPT" {
		t.Fatalf("hard reversibility without rollback must not accept, got %+v", p.Recommendation)
	}
}

func TestResolveEvidenceKinds(t *testing.T) {
	root := assessRoot(t)
	id := assessReceipt(t, root, "feat/T1")
	refs := ResolveEvidence(root, []string{
		"rec:" + id, "chart:regulus-ai-v1", "path:internal/x.go",
		"task:feat/T1", "dec:" + "dec_x", "nope-nothing",
	})
	kinds := map[string]bool{}
	for _, r := range refs {
		kinds[r.Kind] = r.Resolved
	}
	for _, k := range []string{"receipt", "chart", "path", "task"} {
		if !kinds[k] {
			t.Fatalf("kind %s must resolve, got %+v", k, refs)
		}
	}
	if kinds["decision"] || kinds["unresolved"] {
		t.Fatalf("unknown refs must stay unresolved, got %+v", refs)
	}
}

func TestAssessEmptyRisksAbstainsCriterion(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Risks = nil
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	found := false
	for _, cr := range p.Criteria {
		if cr.Name == "risks identified" && cr.Verdict == "abstain" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unstated risks must abstain that criterion, got %+v", p.Criteria)
	}
}

func hasFinding(p Pipeline, code, severity string) bool {
	for _, f := range p.Findings {
		if f.Code == code && f.Severity == severity {
			return true
		}
	}
	return false
}
