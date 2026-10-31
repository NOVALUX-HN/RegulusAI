package decisions

// C5 rare-branch cover for the Assess split: every helper is exercised
// through the orchestrator so findings order and verdicts stay identical
// to the pre-split pipeline.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/evidence"
)

func criterionVerdict(p Pipeline, name string) Criterion {
	for _, cr := range p.Criteria {
		if cr.Name == name {
			return cr
		}
	}
	return Criterion{}
}

func TestAssessTaskMissingBlocks(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Task = "feat/ghost"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !p.Policy.TaskMissing || p.Policy.TaskLinked {
		t.Fatalf("missing task must set TaskMissing only, got %+v", p.Policy)
	}
	if !hasFinding(p, "INSUFFICIENT_EVIDENCE", "blocking") {
		t.Fatalf("missing task must block on evidence, got %+v", p.Findings)
	}
	found := false
	for _, f := range p.Findings {
		if f.Code == "INSUFFICIENT_EVIDENCE" && f.Detail == "linked task feat/ghost has no receipts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("must name the missing task, got %+v", p.Findings)
	}
	if p.Recommendation != "INCONCLUSIVE" || !p.Abstained {
		t.Fatalf("missing task must abstain, got %s", p.Recommendation)
	}
	taskRefs := 0
	for _, r := range p.Evidence {
		if r.Kind == "task" && !r.Resolved {
			taskRefs++
		}
	}
	if taskRefs != 1 {
		t.Fatalf("one unresolved task ref expected, got %+v", p.Evidence)
	}
}

func TestAssessTaskLinkedVerdict(t *testing.T) {
	root := assessRoot(t)
	assessReceipt(t, root, "feat/T1")
	c := baseDecision()
	c.Task = "feat/T1"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !p.Policy.TaskLinked || p.Policy.TaskMissing {
		t.Fatalf("linked task must set TaskLinked only, got %+v", p.Policy)
	}
	if p.Policy.TaskVerdict != "PASS" {
		t.Fatalf("single passing receipt must consolidate to PASS, got %q", p.Policy.TaskVerdict)
	}
	if !p.Policy.EvidenceOK {
		t.Fatalf("linked task must count as evidence, got %+v", p.Policy)
	}
}

func TestAssessTaskVerdictInconclusiveOnBadEvaluations(t *testing.T) {
	root := assessRoot(t)
	assessReceipt(t, root, "feat/T1")
	if err := os.WriteFile(filepath.Join(evidence.TaskDir(root, "feat/T1"), "evaluations.jsonl"), []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := baseDecision()
	c.Task = "feat/T1"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if !p.Policy.TaskLinked {
		t.Fatalf("task with receipts stays linked, got %+v", p.Policy)
	}
	if p.Policy.TaskVerdict != "INCONCLUSIVE" {
		t.Fatalf("broken evaluations log must force INCONCLUSIVE task verdict, got %q", p.Policy.TaskVerdict)
	}
	if p.Recommendation != "ACCEPT" {
		t.Fatalf("task verdict alone must not block, got %s (%+v)", p.Recommendation, p.Findings)
	}
}

func TestAssessMalformedConstraints(t *testing.T) {
	for _, bad := range []string{"no-colon-here", "keyonly:", ": no key", ""} {
		root := assessRoot(t)
		c := baseDecision()
		c.Constraints = []string{bad}
		p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
		cr := criterionVerdict(p, "constraints respected")
		if cr.Verdict != "fail" {
			t.Fatalf("constraint %q must fail, got %+v", bad, cr)
		}
		if !hasFinding(p, "CONTRADICTION", "blocking") {
			t.Fatalf("constraint %q must block, got %+v", bad, p.Findings)
		}
		if p.Recommendation != "REJECT" {
			t.Fatalf("constraint %q must reject, got %s", bad, p.Recommendation)
		}
	}
}

func TestAssessUnnamedCostBlocked(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Tradeoffs = []string{"faster no-op runs"}
	c.Alternatives = []Alternative{{Name: "keep all-always", Tradeoff: "easier to follow but repeats work"}}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	cr := criterionVerdict(p, "complexity justified")
	if cr.Verdict != "fail" {
		t.Fatalf("unnamed cost must fail, got %+v", cr)
	}
	if !hasFinding(p, "UNJUSTIFIED_COMPLEXITY", "blocking") {
		t.Fatalf("must flag unjustified complexity, got %+v", p.Findings)
	}
	if p.Recommendation != "REJECT" {
		t.Fatalf("unjustified complexity must reject, got %s", p.Recommendation)
	}
}

func TestAssessNoArtifactsWarns(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Artifacts = nil
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	cr := criterionVerdict(p, "impact stated")
	if cr.Verdict != "abstain" {
		t.Fatalf("missing artifacts must abstain impact, got %+v", cr)
	}
	if !hasFinding(p, "HIDDEN_DEPENDENCY", "warning") {
		t.Fatalf("must warn on hidden blast radius, got %+v", p.Findings)
	}
	if cr := criterionVerdict(p, "complexity justified"); cr.Verdict != "pass" || cr.Note != "no code touched" {
		t.Fatalf("no code touched must pass complexity, got %+v", cr)
	}
	if p.Recommendation != "ACCEPT" || p.Abstained {
		t.Fatalf("warnings alone must accept, got %s", p.Recommendation)
	}
	if !strings.HasSuffix(p.Rationale, " (with warnings noted)") {
		t.Fatalf("accept with warnings must say so, got %q", p.Rationale)
	}
}

func TestAssessMissingArtifactCriterion(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Artifacts = []string{"internal/does-not-exist.go"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	cr := criterionVerdict(p, "impact stated")
	if cr.Verdict != "fail" || cr.Note != "a listed artifact does not exist" {
		t.Fatalf("missing artifact must fail impact, got %+v", cr)
	}
}

func TestAssessUngroundedProblem(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Evidence = nil
	c.Task = ""
	c.Related = nil
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Perception != "problem stated but ungrounded: no evidence, task, or related decisions" {
		t.Fatalf("perception must name the missing grounding, got %q", p.Perception)
	}
	cr := criterionVerdict(p, "problem exists")
	if cr.Verdict != "fail" {
		t.Fatalf("ungrounded problem must fail, got %+v", cr)
	}
	if p.Recommendation != "INCONCLUSIVE" {
		t.Fatalf("ungrounded problem must abstain, got %s", p.Recommendation)
	}
}

func TestAssessProposalRestatement(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Proposal = c.Problem
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	cr := criterionVerdict(p, "problem-solution fit")
	if cr.Verdict != "fail" {
		t.Fatalf("restated proposal must fail fit, got %+v", cr)
	}
	if !hasFinding(p, "CONTRADICTION", "blocking") {
		t.Fatalf("must flag the disconnect, got %+v", p.Findings)
	}
}

func TestAssessNoEvidenceAtAll(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Evidence = nil
	c.Task = ""
	c.Related = []string{"dec:x"}
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	cr := criterionVerdict(p, "evidence available")
	if cr.Verdict != "fail" || cr.Note != "no evidence cited at all" {
		t.Fatalf("cited-nothing must use its own note, got %+v", cr)
	}
	found := false
	for _, f := range p.Findings {
		if f.Code == "INSUFFICIENT_EVIDENCE" && f.Detail == "decision cites no evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("must flag the uncited decision, got %+v", p.Findings)
	}
}

func TestAssessAbsoluteSuppressedByEvidence(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Consequences = "this will always be faster and can never regress"
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if hasFinding(p, "UNSUPPORTED_ASSUMPTION", "warning") {
		t.Fatalf("resolving evidence must suppress the absolute-claims warning, got %+v", p.Findings)
	}
}

func TestAssessObsoleteEdges(t *testing.T) {
	newDec := func(scope, id string, state string) Decision {
		c := baseDecision()
		c.Scope = scope
		d := Decision{Contract: c, State: state}
		d.ID = id
		return d
	}
	t.Run("empty scope never obsoletes", func(t *testing.T) {
		root := assessRoot(t)
		p := Assess(root, newDec("", "dec_new", StateProposed), []Decision{newDec("", "dec_old", StateAccepted)})
		if p.Policy.ObsoleteBy != "" || hasFinding(p, "OBSOLETE", "blocking") {
			t.Fatalf("empty scope must not obsolete, got %+v", p.Findings)
		}
	})
	t.Run("self is skipped", func(t *testing.T) {
		root := assessRoot(t)
		p := Assess(root, newDec("gate execution", "dec_same", StateProposed), []Decision{newDec("gate execution", "dec_same", StateAccepted)})
		if p.Policy.ObsoleteBy != "" || hasFinding(p, "OBSOLETE", "blocking") {
			t.Fatalf("self must not obsolete, got %+v", p.Findings)
		}
	})
	t.Run("non-accepted is skipped", func(t *testing.T) {
		root := assessRoot(t)
		p := Assess(root, newDec("gate execution", "dec_new", StateProposed), []Decision{newDec("gate execution", "dec_old", StateProposed)})
		if p.Policy.ObsoleteBy != "" || hasFinding(p, "OBSOLETE", "blocking") {
			t.Fatalf("proposed same-scope must not obsolete, got %+v", p.Findings)
		}
	})
	t.Run("scope matches case-insensitively", func(t *testing.T) {
		root := assessRoot(t)
		p := Assess(root, newDec("gate execution", "dec_new", StateProposed), []Decision{newDec("GATE EXECUTION", "dec_old", StateAccepted)})
		if p.Policy.ObsoleteBy != "dec_old" || !hasFinding(p, "OBSOLETE", "blocking") {
			t.Fatalf("scope match must obsolete, got %+v", p.Findings)
		}
	})
}

func TestAssessReversibilityVariants(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Reversibility = "hard"
	c.Rollback = "revert the flag and rerun the gate"
	if cr := criterionVerdict(Assess(root, Decision{Contract: c, State: StateProposed}, nil), "reversibility"); cr.Verdict != "pass" {
		t.Fatalf("hard with rollback must pass, got %+v", cr)
	}
	c.Reversibility = "moderate"
	c.Rollback = ""
	if cr := criterionVerdict(Assess(root, Decision{Contract: c, State: StateProposed}, nil), "reversibility"); cr.Verdict != "pass" {
		t.Fatalf("moderate without rollback must pass, got %+v", cr)
	}
}

func TestAssessAcceptWithWarningsNoted(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Risks = nil
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Recommendation != "ACCEPT" || p.Abstained {
		t.Fatalf("warnings alone must accept, got %s", p.Recommendation)
	}
	if p.Rationale != "justified, constrained, and evidenced (with warnings noted)" {
		t.Fatalf("must note the warnings, got %q", p.Rationale)
	}
	if p.Policy.NeedsHuman {
		t.Fatalf("non-critical must not need a human, got %+v", p.Policy)
	}
}

func TestAssessCriticalNeedsHuman(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Critical = true
	p := Assess(root, Decision{Contract: c, State: StateProposed}, nil)
	if p.Recommendation != "ACCEPT" {
		t.Fatalf("critical but clean must accept, got %s", p.Recommendation)
	}
	if !p.Policy.NeedsHuman {
		t.Fatalf("critical must need a human, got %+v", p.Policy)
	}
}

func TestAssessFindingsOrder(t *testing.T) {
	root := assessRoot(t)
	c := baseDecision()
	c.Task = "feat/ghost"
	d := Decision{Contract: c, State: StateProposed}
	d.ID = "dec_new"
	other := Decision{Contract: baseDecision(), State: StateAccepted}
	other.ID = "dec_old"
	p := Assess(root, d, []Decision{other})
	if len(p.Findings) < 2 {
		t.Fatalf("obsolete plus missing task need two findings, got %+v", p.Findings)
	}
	if p.Findings[0].Code != "OBSOLETE" {
		t.Fatalf("obsolete must come first, got %+v", p.Findings)
	}
	if p.Findings[1].Code != "INSUFFICIENT_EVIDENCE" || !strings.Contains(p.Findings[1].Detail, "feat/ghost") {
		t.Fatalf("missing task must come second, got %+v", p.Findings)
	}
	if len(p.Criteria) != 10 {
		t.Fatalf("ten criteria in order, got %d", len(p.Criteria))
	}
	want := []string{"problem exists", "problem-solution fit", "alternatives considered", "constraints respected", "risks identified", "evidence available", "architectural consistency", "complexity justified", "impact stated", "reversibility"}
	for i, n := range want {
		if p.Criteria[i].Name != n {
			t.Fatalf("criterion %d must be %q, got %q", i, n, p.Criteria[i].Name)
		}
	}
}

func TestPerceiveGroundingHelpers(t *testing.T) {
	if ok, _ := perceiveGrounding(baseDecision()); !ok {
		t.Fatalf("base decision carries evidence hooks")
	}
	c := baseDecision()
	c.Evidence = nil
	c.Task = "feat/T1"
	if ok, _ := perceiveGrounding(c); !ok {
		t.Fatalf("task alone must ground")
	}
	c.Task = ""
	c.Related = []string{"dec:x"}
	if ok, _ := perceiveGrounding(c); !ok {
		t.Fatalf("related alone must ground")
	}
	c.Related = nil
	if ok, msg := perceiveGrounding(c); ok || msg == "" {
		t.Fatalf("bare decision must be ungrounded, got %v/%q", ok, msg)
	}
	if n := countResolved([]EvidenceRef{{Resolved: true}, {Resolved: false}, {Resolved: true}}); n != 2 {
		t.Fatalf("countResolved must count 2, got %d", n)
	}
}

func TestRecommendDecisionPaths(t *testing.T) {
	c := baseDecision()
	p := Pipeline{Findings: []Finding{{Code: FindingContradiction, Severity: SeverityBlocking}}}
	recommendDecision(&p, c)
	if p.Recommendation != "REJECT" || !p.Policy.HasBlocking {
		t.Fatalf("blocking without evidence gaps must reject, got %s", p.Recommendation)
	}
	p = Pipeline{}
	recommendDecision(&p, c)
	if p.Recommendation != "ACCEPT" || p.Rationale != "justified, constrained, and evidenced" {
		t.Fatalf("clean pipeline must accept plainly, got %s/%q", p.Recommendation, p.Rationale)
	}
	p = Pipeline{Findings: []Finding{{Code: FindingUnsupportedAssumption, Severity: SeverityWarning}}}
	recommendDecision(&p, c)
	if p.Recommendation != "ACCEPT" || !strings.HasSuffix(p.Rationale, " (with warnings noted)") {
		t.Fatalf("warnings must ride along, got %s/%q", p.Recommendation, p.Rationale)
	}
}
