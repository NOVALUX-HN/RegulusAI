// Regression detection through F12.4 (F13.4/NOV-107): verify never
// reimplements detection. DetectRegression folds a versioned quality
// baseline and a window into the gate-grade report the decision
// needs: baseline, sample, magnitudes, linked cases and limitations.
// A quality diagnosis stays advisory on its own; linked to a diff
// hash and run id it becomes the change-culprit link the spec
// demands.
package verify

import (
	"fmt"
	"strings"

	"regulus-ai/internal/quality"
)

// RegressionReport is the gate-grade shape of a quality diagnosis:
// what was compared, what moved, which runs prove it, what bounds
// the claim, and which change it blames.
type RegressionReport struct {
	BaselineRef string   `json:"baseline_ref"`
	WindowRef   string   `json:"window_ref"`
	BaselineN   int      `json:"baseline_n"`
	WindowN     int      `json:"window_n"`
	ExcludedN   int      `json:"excluded_n"`
	Alerts      int      `json:"alerts"`
	Magnitudes  []string `json:"magnitudes,omitempty"`
	Cases       []string `json:"cases,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
	Detected    bool     `json:"detected"`
	Verdict     string   `json:"verdict"`
	DiffHash    string   `json:"diff_hash"`
	RunID       string   `json:"run_id"`
}

// DetectRegression runs the F12.4 check and shapes it for gating.
// baselineRef/windowRef are the evidence locators (dirs/files) so
// the report reproduces from the exact bytes; diffHash/runID link
// the finding to the responsible change.
func DetectRegression(b quality.Baseline, obs []quality.Observation, baselineRef, windowRef, diffHash, runID string) RegressionReport {
	d := quality.Check(b, obs, baselineRef, windowRef)
	rep := RegressionReport{
		BaselineRef: baselineRef, WindowRef: windowRef,
		BaselineN: d.BaselineN, WindowN: d.WindowN, ExcludedN: d.ExcludedN,
		Limitations: append([]string(nil), d.Limitations...),
		DiffHash:    diffHash, RunID: runID,
	}
	for _, a := range d.Alerts {
		rep.Magnitudes = append(rep.Magnitudes, a.Magnitude)
		for _, c := range a.Cases {
			rep.Cases = append(rep.Cases, fmt.Sprintf("%s: %s", a.Metric, c))
		}
	}
	rep.Cases = sortedUnique(rep.Cases)
	rep.Alerts = len(d.Alerts)
	rep.Detected = len(d.Alerts) > 0
	switch {
	case rep.Detected:
		rep.Verdict = fmt.Sprintf("regresión detectada: %d alerta(s) F12.4 en diff %s run %s", rep.Alerts, shortSHA(diffHash), runID)
	case d.WindowN == 0 || insufficientSample(d.Limitations):
		rep.Verdict = fmt.Sprintf("inconcluyente: ventana n=%d (mínimo %d): describe, no afirma", d.WindowN, quality.MinN)
	default:
		rep.Verdict = "sin regresión: baseline y ventana dentro de umbrales"
	}
	return rep
}

// insufficientSample reports whether the diagnosis limitations carry
// the small-N guard: below the floor nothing gates.
func insufficientSample(limitations []string) bool {
	for _, l := range limitations {
		if strings.Contains(l, "insufficient sample") {
			return true
		}
	}
	return false
}
