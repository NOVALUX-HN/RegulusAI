// Evaluation metrics (Phase 6, S6): per-trial raw outcomes aggregate into
// the metric set, and candidates compare against the stored baseline. The
// mandatory-guarantee gate (S7) refuses adoption on any deterioration of
// the guarantees below, however good the rest looks.
package evolve

import "errors"

// Trial is one raw evaluation outcome.
type Trial struct {
	Pass        bool
	CriteriaMet bool
	Regressed   bool
	DefectFound bool
	FalsePos    bool
	FalseNeg    bool
	Cost        float64
	Tokens      int64
	DurationMS  int64
	PolicyOK    bool
	EvidenceOK  bool
}

// Metrics is the aggregated S6 set.
type Metrics struct {
	Trials               int     `json:"trials"`
	SuccessRate          float64 `json:"success_rate"`
	AcceptanceRate       float64 `json:"acceptance_rate"`
	RegressionRate       float64 `json:"regression_rate"`
	DefectRate           float64 `json:"defect_rate"`
	FalsePositives       float64 `json:"false_positives"`
	FalseNegatives       float64 `json:"false_negatives"`
	CostPerSuccess       float64 `json:"cost_per_success"`
	Tokens               int64   `json:"tokens"`
	DurationMS           int64   `json:"duration_ms"`
	PolicyCompliance     float64 `json:"policy_compliance"`
	EvidenceCompleteness float64 `json:"evidence_completeness"`
}

// ErrMandatoryRegression refuses adoption of a deteriorating candidate.
var ErrMandatoryRegression = errors.New("evolve: candidate deteriorates mandatory guarantees, adoption refused")

// Summarize aggregates trials into Metrics. Empty input yields zero
// metrics, never NaN: no evidence means no claim.
func Summarize(trials []Trial) Metrics {
	var m Metrics
	n := float64(len(trials))
	m.Trials = len(trials)
	if n == 0 {
		return m
	}
	var succ, crit, reg, def, fp, fn, pok, eok float64
	var cost, succCost float64
	for _, tr := range trials {
		if tr.Pass {
			succ++
			succCost += tr.Cost
		}
		if tr.CriteriaMet {
			crit++
		}
		if tr.Regressed {
			reg++
		}
		if tr.DefectFound {
			def++
		}
		if tr.FalsePos {
			fp++
		}
		if tr.FalseNeg {
			fn++
		}
		if tr.PolicyOK {
			pok++
		}
		if tr.EvidenceOK {
			eok++
		}
		cost += tr.Cost
		m.Tokens += tr.Tokens
		m.DurationMS += tr.DurationMS
	}
	m.SuccessRate = succ / n
	m.AcceptanceRate = crit / n
	m.RegressionRate = reg / n
	m.DefectRate = def / n
	m.FalsePositives = fp / n
	m.FalseNegatives = fn / n
	m.PolicyCompliance = pok / n
	m.EvidenceCompleteness = eok / n
	if succ > 0 {
		m.CostPerSuccess = succCost / succ
	} else {
		m.CostPerSuccess = cost
	}
	return m
}

// Adoptable enforces the mandatory-guarantee gate: success, acceptance,
// policy and evidence must not drop below baseline, and the regression
// rate must not rise. Anything else (cost, tokens, duration) is advisory.
func Adoptable(candidate, baseline Metrics) error {
	if candidate.SuccessRate < baseline.SuccessRate ||
		candidate.AcceptanceRate < baseline.AcceptanceRate ||
		candidate.PolicyCompliance < baseline.PolicyCompliance ||
		candidate.EvidenceCompleteness < baseline.EvidenceCompleteness ||
		candidate.RegressionRate > baseline.RegressionRate {
		return ErrMandatoryRegression
	}
	return nil
}
