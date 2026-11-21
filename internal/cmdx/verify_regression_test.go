// Regression helper cover (CRAP paydown sin cambiar comportamiento):
// verifyRegression directo con fixtures baseline/window en tmp
// (ambos vacíos->nil, uno vacío->error, ReadBaseline err,
// thresholds válida/inválida, LoadWindow err, DetectRegression con
// y sin regresión). Portable: t.TempDir, sin red.
package cmdx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/quality"
)

func regressionTestObs(prefix string, n int, success bool) []quality.Observation {
	out := make([]quality.Observation, 0, n)
	for i := 0; i < n; i++ {
		obs := quality.Observation{
			Agent: "atlas", Model: "m1", Host: "opencode",
			Task: "t-explore", Version: "v3",
			Source:   quality.SourceLab,
			RunID:    fmt.Sprintf("%s-%02d", prefix, i),
			Evidence: "lab-evidence " + prefix,
			At:       time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute),
		}
		if success {
			obs.Success = true
			obs.Retries = 0
			obs.LatencyMS = 100
			obs.LatencyKnown = true
			obs.TokensIn = 400
			obs.TokensOut = 600
			obs.TokensKnown = true
			obs.Errors = 0
			obs.CoveragePct = 82
			obs.CoverageKnown = true
		} else {
			obs.Success = false
			obs.Retries = 3
			obs.LatencyMS = 400
			obs.LatencyKnown = true
			obs.TokensIn = 900
			obs.TokensOut = 1400
			obs.TokensKnown = true
			obs.Errors = 1
			obs.CoveragePct = 70
			obs.CoverageKnown = true
		}
		out = append(out, obs)
	}
	return out
}

func regressionBaselineDir(t *testing.T, obs []quality.Observation) string {
	t.Helper()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	b, err := quality.BuildBaseline(quality.Segment{}, obs, quality.DefaultThresholds(), nil, now)
	if err != nil {
		t.Fatalf("BuildBaseline: %v", err)
	}
	dir := t.TempDir()
	if _, err := quality.WriteBaselineEvidence(dir, b, "base-head"); err != nil {
		t.Fatalf("WriteBaselineEvidence: %v", err)
	}
	return dir
}

func regressionWindowFile(t *testing.T, obs []quality.Observation) string {
	t.Helper()
	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal window: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "window.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func regressionThresholdsFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "thresholds.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyRegressionEmptyRefsReturnNil(t *testing.T) {
	rep, err := verifyRegression("", "", "", "abc", "run-1")
	if err != nil || rep != nil {
		t.Fatalf("empty refs must return nil,nil: %+v %v", rep, err)
	}
	rep, err = verifyRegression("  ", " \t ", "", "abc", "run-1")
	if err != nil || rep != nil {
		t.Fatalf("blank refs must return nil,nil: %+v %v", rep, err)
	}
}

func TestVerifyRegressionSingleRefTravelsTogether(t *testing.T) {
	cases := []struct {
		name     string
		baseline string
		window   string
	}{
		{name: "baseline sin window", baseline: "base", window: ""},
		{name: "window sin baseline", baseline: "", window: "win"},
		{name: "baseline sin window en blanco", baseline: "base", window: "  "},
		{name: "window sin baseline en blanco", baseline: "  ", window: "win"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifyRegression(tc.baseline, tc.window, "", "abc", "run-1"); err == nil {
				t.Fatal("half refs must fail")
			} else if !strings.Contains(err.Error(), "travel together") {
				t.Fatalf("half refs must mention travel together, got %v", err)
			}
		})
	}
}

func TestVerifyRegressionReadBaselineError(t *testing.T) {
	window := regressionWindowFile(t, regressionTestObs("win", 6, true))
	if _, err := verifyRegression(filepath.Join(t.TempDir(), "missing-baseline"), window, "", "abc", "run-1"); err == nil {
		t.Fatal("missing baseline dir must fail")
	}
}

func TestVerifyRegressionThresholdsOverrideAndError(t *testing.T) {
	baseline := regressionBaselineDir(t, regressionTestObs("base", 10, true))
	window := regressionWindowFile(t, regressionTestObs("win", 10, true))
	valid := regressionThresholdsFile(t, `{"success_drop_abs":0.5}`)
	rep, err := verifyRegression(baseline, window, valid, "abc123", "run-9")
	if err != nil {
		t.Fatalf("valid thresholds must load: %v", err)
	}
	if rep == nil {
		t.Fatal("valid thresholds must return a report")
	}
	if rep.BaselineRef != baseline || rep.WindowRef != window {
		t.Fatalf("report must cite evidence refs: %+v", rep)
	}
	if rep.DiffHash != "abc123" || rep.RunID != "run-9" {
		t.Fatalf("report must link diff and run: %+v", rep)
	}
	malformed := regressionThresholdsFile(t, `{no json`)
	if _, err := verifyRegression(baseline, window, malformed, "abc", "run-1"); err == nil {
		t.Fatal("malformed thresholds must fail")
	}
	unknown := regressionThresholdsFile(t, `{"bogus":1}`)
	if _, err := verifyRegression(baseline, window, unknown, "abc", "run-1"); err == nil {
		t.Fatal("unknown thresholds fields must fail (strict JSON)")
	}
	outOfRange := regressionThresholdsFile(t, `{"success_drop_abs":0}`)
	if _, err := verifyRegression(baseline, window, outOfRange, "abc", "run-1"); err == nil {
		t.Fatal("out-of-range thresholds must fail validation")
	}
}

func TestVerifyRegressionLoadWindowError(t *testing.T) {
	baseline := regressionBaselineDir(t, regressionTestObs("base", 10, true))
	if _, err := verifyRegression(baseline, filepath.Join(t.TempDir(), "missing-window.json"), "", "abc", "run-1"); err == nil {
		t.Fatal("missing window must fail")
	}
	badDir := t.TempDir()
	badPath := filepath.Join(badDir, "window.json")
	if err := os.WriteFile(badPath, []byte("[{oops}]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyRegression(baseline, badPath, "", "abc", "run-1"); err == nil {
		t.Fatal("malformed window must fail")
	}
}

func TestVerifyRegressionDetectsAndLinks(t *testing.T) {
	baseline := regressionBaselineDir(t, regressionTestObs("base", 10, true))
	// Ventana sana: sin regresión, veredicto calmado pero con N y vínculo.
	healthy := regressionWindowFile(t, regressionTestObs("win-ok", 10, true))
	rep, err := verifyRegression(baseline, healthy, "", "diff-ok", "run-ok")
	if err != nil {
		t.Fatalf("healthy window: %v", err)
	}
	if rep == nil {
		t.Fatal("healthy window must return a report")
	}
	if rep.Detected {
		t.Fatalf("healthy window must not detect: %+v", rep)
	}
	if rep.BaselineN != 10 || rep.WindowN != 10 {
		t.Fatalf("N must travel: baseline=%d window=%d", rep.BaselineN, rep.WindowN)
	}
	if !strings.Contains(rep.Verdict, "sin regresión") {
		t.Fatalf("healthy verdict must calm, got %q", rep.Verdict)
	}
	// Ventana degradada: regresión detectada y vinculada a diff/run.
	degraded := regressionWindowFile(t, regressionTestObs("win-bad", 10, false))
	rep, err = verifyRegression(baseline, degraded, "", "diff-bad", "run-bad")
	if err != nil {
		t.Fatalf("degraded window: %v", err)
	}
	if rep == nil || !rep.Detected {
		t.Fatalf("degraded window must detect: %+v", rep)
	}
	if rep.Alerts == 0 || len(rep.Magnitudes) == 0 {
		t.Fatalf("detection must carry magnitudes: %+v", rep)
	}
	if !strings.Contains(rep.Verdict, "diff-bad") || !strings.Contains(rep.Verdict, "run-bad") {
		t.Fatalf("verdict must link diff and run, got %q", rep.Verdict)
	}
}
