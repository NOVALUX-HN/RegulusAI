package decisions

import "strings"

// T4 (S1,S3): punctual quantitative scoring over the four signals Assess
// already computes. Pure and advisory only: it never changes a state
// transition, it just numbers the Pipeline note.
type ScoreInput struct {
	EvidenceOK         bool
	Blocking           int
	AlternativesPriced int
	RisksTotal         int
	RisksMitigated     int
}

// Score folds the input into 0-100 plus a grade (A>=90, B>=75, C>=60,
// D>=40, F below): evidence 30, zero-blocking 30 (minus 15 per
// blocking finding), priced alternatives 20 (14 for one), mitigations
// 20 pro-rata (10 when no risks are stated: unknown exposure, half
// credit, matching the abstaining criterion).
func Score(in ScoreInput) (int, string) {
	if in.Blocking < 0 {
		in.Blocking = 0
	}
	total := 0
	if in.EvidenceOK {
		total += 30
	}
	if back := 30 - 15*in.Blocking; back > 0 {
		total += back
	}
	switch {
	case in.AlternativesPriced >= 2:
		total += 20
	case in.AlternativesPriced == 1:
		total += 14
	}
	switch {
	case in.RisksTotal <= 0:
		total += 10
	default:
		m := in.RisksMitigated
		if m < 0 {
			m = 0
		}
		if m > in.RisksTotal {
			m = in.RisksTotal
		}
		total += 20 * m / in.RisksTotal
	}
	switch {
	case total >= 90:
		return total, "A"
	case total >= 75:
		return total, "B"
	case total >= 60:
		return total, "C"
	case total >= 40:
		return total, "D"
	default:
		return total, "F"
	}
}

// pricedAlternatives counts alternatives naming their trade-off.
func pricedAlternatives(c Contract) int {
	n := 0
	for _, a := range c.Alternatives {
		if strings.TrimSpace(a.Tradeoff) != "" {
			n++
		}
	}
	return n
}

// mitigatedRisks splits risks into total vs mitigated (same
// mitigation proxy the risks criterion enforces).
func mitigatedRisks(c Contract) (total, mitigated int) {
	for _, r := range c.Risks {
		total++
		if mitigateRe.MatchString(r) {
			mitigated++
		}
	}
	return total, mitigated
}

// countBlocking counts findings that refuse ACCEPT.
func countBlocking(fs []Finding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == SeverityBlocking {
			n++
		}
	}
	return n
}
