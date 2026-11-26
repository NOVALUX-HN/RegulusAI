// Promotion evaluation with provenance (F19.4 / NOV-149).
//
// The F19.1 baseline found that the measured baseline per host and task does not
// exist: the host exposes no token counts, so most figures are estimated or proxy.
// The phase still needs a promotion rule, and the honest one is not "adopt if the
// proxy went down" - it is "adopt only when the saving is measured, and never at
// the cost of quality".
//
// Evaluate encodes that rule. It is deliberately the smallest thing that can hold
// the phase's honesty requirements: quality first, no saving invented from a
// proxy, and a reversal named so a promotion can be undone.
package context

// Provenance labels a metric's origin. The vocabulary mirrors
// internal/trace/hostusage.go; it is duplicated as literals because the context
// package must stay usable without the trace store, and a test pins the equality.
const (
	ProvenanceMeasuredHost  = "measured-host"
	ProvenanceMeasuredLocal = "measured-local"
	ProvenanceEstimated     = "estimated"
	ProvenanceProxy         = "proxy"
	ProvenanceUnknown       = "unknown"
)

// Metric is one measured figure with the provenance that makes it trustworthy.
type Metric struct {
	Value      int64  `json:"value"`
	Provenance string `json:"provenance"`
}

// Recommendation is one candidate optimization compared against its baseline.
type Recommendation struct {
	Name string `json:"name"`
	// Baseline and Candidate carry the same dimension (tokens) so the saving is
	// a subtraction, not a comparison of different units.
	Baseline  Metric `json:"baseline"`
	Candidate Metric `json:"candidate"`
	// EssentialsPreserved reports whether the critical fragments survived. Losing
	// them is never a saving.
	EssentialsPreserved bool `json:"essentials_preserved"`
	// QualityRegressed reports a measured quality loss, whatever the token delta.
	QualityRegressed bool `json:"quality_regressed"`
	// Reversal is the exact way back, e.g. TE_TOOLSET=full.
	Reversal string `json:"reversal"`
}

// Verdicts. The vocabulary is closed: a new verdict is a contract change.
const (
	VerdictAdopt        = "adopt"
	VerdictReject       = "reject"
	VerdictInconclusive = "inconclusive"
)

// Evaluate decides one recommendation.
//
// Order matters and it is the phase's rule written as code:
//
//  1. A measured quality regression rejects, whatever the token delta.
//  2. Losing essentials rejects: quality first, no savings claimed.
//  3. No saving rejects.
//  4. A saving whose figures are not measured is inconclusive, never adopted —
//     a proxy is a reason to keep measuring, not a reason to promote.
//  5. Only a measured saving on both sides adopts.
func Evaluate(r Recommendation) (verdict, reason string) {
	if r.QualityRegressed {
		return VerdictReject, "measured quality regression: tokens never buy back quality"
	}
	if !r.EssentialsPreserved {
		return VerdictReject, "essentials were lost: that is not a saving"
	}
	saving := r.Baseline.Value - r.Candidate.Value
	if saving <= 0 {
		return VerdictReject, "no saving: the candidate is not cheaper"
	}
	if !measured(r.Baseline.Provenance) || !measured(r.Candidate.Provenance) {
		return VerdictInconclusive, "saving exists but is not measured (" +
			labelOf(r.Baseline.Provenance) + " baseline, " + labelOf(r.Candidate.Provenance) +
			" candidate): keep measuring, do not promote on a proxy"
	}
	if r.Reversal == "" {
		return VerdictReject, "a promotion without a named reversal cannot be undone"
	}
	return VerdictAdopt, "measured saving with essentials preserved and a named reversal"
}

// measured reports whether a provenance is a host-side measurement. Only those two
// count: estimated and proxy are honest labels for something not measured.
func measured(p string) bool {
	return p == ProvenanceMeasuredHost || p == ProvenanceMeasuredLocal
}

// labelOf keeps an empty provenance readable instead of printing nothing.
func labelOf(p string) string {
	if p == "" {
		return ProvenanceUnknown
	}
	return p
}
