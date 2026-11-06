package decisions

import "testing"

// T4 (S1,S3): punctual quantitative scoring over the four signals Assess
// already computes (evidence, blocking findings, priced alternatives,
// mitigations). Golden pins: the formula must move only deliberately.
func TestScoreGoldenStrong(t *testing.T) {
	got, grade := Score(ScoreInput{EvidenceOK: true, Blocking: 0, AlternativesPriced: 1, RisksTotal: 1, RisksMitigated: 1})
	if got != 94 || grade != "A" {
		t.Fatalf("strong decision: got %d/%s, want 94/A", got, grade)
	}
}

func TestScoreGoldenEmpty(t *testing.T) {
	got, grade := Score(ScoreInput{EvidenceOK: false, Blocking: 3, AlternativesPriced: 0, RisksTotal: 0, RisksMitigated: 0})
	if got != 10 || grade != "F" {
		t.Fatalf("empty decision: got %d/%s, want 10/F", got, grade)
	}
}

func TestScoreGoldenMixed(t *testing.T) {
	got, grade := Score(ScoreInput{EvidenceOK: true, Blocking: 1, AlternativesPriced: 2, RisksTotal: 2, RisksMitigated: 1})
	if got != 75 || grade != "B" {
		t.Fatalf("mixed decision: got %d/%s, want 75/B", got, grade)
	}
}

// T4: Assess wires the score into the Pipeline without changing the
// advisory outcome: grounded still ACCEPTs at 94/A, ungrounded still
// abstains with a low score.
func TestScoreAssessWiring(t *testing.T) {
	root := assessRoot(t)
	p := Assess(root, Decision{Contract: baseDecision(), State: StateProposed}, nil)
	if p.Recommendation != "ACCEPT" || p.Abstained {
		t.Fatalf("grounded decision must still accept, got %s", p.Recommendation)
	}
	if p.Score != 94 || p.Grade != "A" {
		t.Fatalf("grounded pipeline: got score %d/%s, want 94/A", p.Score, p.Grade)
	}
	empty := baseDecision()
	empty.Evidence = nil
	q := Assess(root, Decision{Contract: empty, State: StateProposed}, nil)
	if q.Recommendation != "INCONCLUSIVE" || !q.Abstained {
		t.Fatalf("ungrounded decision must still abstain, got %s", q.Recommendation)
	}
	if q.Score >= 60 || q.Grade == "A" || q.Grade == "B" {
		t.Fatalf("ungrounded pipeline must score low, got %d/%s", q.Score, q.Grade)
	}
}
