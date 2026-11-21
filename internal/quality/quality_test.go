// Tests for Continuous Quality Intelligence (F12.4/NOV-99):
// injected regression detected and linked, N gating, segmentation
// against false positives, and reproduction from versioned evidence.
package quality

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// qualitySeg is the shared test segment: one agent on one host.
func qualitySeg() Segment {
	return Segment{Agent: "atlas", Host: "opencode", Model: "m1", Version: "v3"}
}

// goodObs builds n passing observations in seg with ~100ms latency.
func goodObs(n int, seg Segment, runPrefix string, at time.Time) []Observation {
	out := make([]Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Observation{
			Agent: seg.Agent, Model: seg.Model, Host: seg.Host,
			Task: "t-explore", Version: seg.Version,
			Success: true, Retries: 0,
			LatencyMS: 100, LatencyKnown: true,
			TokensIn: 400, TokensOut: 600, TokensKnown: true,
			Errors:      0,
			CoveragePct: 82, CoverageKnown: true,
			Source: SourceLab, RunID: fmt.Sprintf("%s-%02d", runPrefix, i),
			Evidence: "lab-evidence " + runPrefix,
			At:       at.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

// badObs builds n failing observations in seg with ~400ms latency.
func badObs(n int, seg Segment, runPrefix string, at time.Time) []Observation {
	out := make([]Observation, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Observation{
			Agent: seg.Agent, Model: seg.Model, Host: seg.Host,
			Task: "t-explore", Version: seg.Version,
			Success: i%2 == 0, Retries: 3,
			LatencyMS: 400, LatencyKnown: true,
			TokensIn: 900, TokensOut: 1400, TokensKnown: true,
			Errors:      1,
			CoveragePct: 70, CoverageKnown: true,
			Source: SourceLab, RunID: fmt.Sprintf("%s-%02d", runPrefix, i),
			Evidence: "lab-evidence " + runPrefix,
			At:       at.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

func testBaseline(t *testing.T, obs []Observation) (Baseline, string) {
	t.Helper()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	b, err := BuildBaseline(qualitySeg(), obs, DefaultThresholds(), []EvidenceRef{
		{Kind: "window-file", Path: "w.json", SHA256: "abc123", GitHead: "base-head"},
	}, now)
	if err != nil {
		t.Fatalf("BuildBaseline: %v", err)
	}
	dir := t.TempDir()
	if _, err := WriteBaselineEvidence(dir, b, "base-head"); err != nil {
		t.Fatalf("WriteBaselineEvidence: %v", err)
	}
	_, rb, err := ReadBaselineEvidence(dir)
	if err != nil {
		t.Fatalf("ReadBaselineEvidence: %v", err)
	}
	return rb, dir
}

// An injected regression (success collapse + latency spike) must be
// detected and linked to runs and evidence.
func TestInjectedRegressionDetectedAndLinked(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	base, _ := testBaseline(t, goodObs(10, qualitySeg(), "base", now))
	window := badObs(10, qualitySeg(), "win", now.Add(24*time.Hour))
	d := Check(base, window, "base-dir", "win.json")
	if len(d.Alerts) == 0 {
		t.Fatalf("injected regression went undetected:\n%s", RenderDiagnosis(d))
	}
	metrics := map[string]bool{}
	for _, a := range d.Alerts {
		metrics[a.Metric] = true
		if len(a.Cases) == 0 {
			t.Errorf("alert %q names no cases", a.Metric)
		}
		if !strings.HasPrefix(a.Cases[0], "win-") {
			t.Errorf("alert %q cases %v are not linked to window runs", a.Metric, a.Cases)
		}
		if !strings.Contains(a.Magnitude, "->") {
			t.Errorf("alert %q magnitude %q hides the baseline", a.Metric, a.Magnitude)
		}
	}
	for _, want := range []string{"success", "latency-p50", "retries", "errors", "tokens", "coverage"} {
		if !metrics[want] {
			t.Errorf("expected alert on %q, got %v", want, metrics)
		}
	}
	if d.WindowN != 10 || d.BaselineN != 10 {
		t.Errorf("N must travel with the alert: baseline=%d window=%d", d.BaselineN, d.WindowN)
	}
	if d.Reproduce == "" || !strings.Contains(d.Reproduce, "win.json") {
		t.Errorf("diagnosis must carry the reproduction command, got %q", d.Reproduce)
	}
	text := RenderDiagnosis(d)
	for _, want := range []string{"ALERT success", "limit:", "next:", "reproduce:"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered diagnosis must contain %q", want)
		}
	}
	for _, s := range d.NextSteps {
		if strings.Contains(strings.ToLower(s), "rollback") || strings.Contains(strings.ToLower(s), "deploy") {
			t.Errorf("next steps must propose investigation, never action: %q", s)
		}
	}
}

// Below the N floor the diagnosis describes only: even a total
// collapse claims no regression.
func TestInsufficientNClaimsNothing(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	base, _ := testBaseline(t, goodObs(10, qualitySeg(), "base", now))
	window := badObs(3, qualitySeg(), "tiny", now.Add(24*time.Hour))
	d := Check(base, window, "base-dir", "tiny.json")
	if len(d.Alerts) != 0 {
		t.Fatalf("n=3 must claim nothing, got alerts:\n%s", RenderDiagnosis(d))
	}
	if len(d.Findings) != 0 {
		t.Fatalf("n=3 must run no metric rules, got %d findings", len(d.Findings))
	}
	joined := strings.Join(d.Limitations, "\n")
	if !strings.Contains(joined, "insufficient sample (n=3, need >=5)") {
		t.Fatalf("limitation must name N and the floor, got %v", d.Limitations)
	}
}

// Segmentation controls false positives: a bad foreign host mixed
// into the window must not alert the clean home segment, and the
// diagnosis must say the window was mixed.
func TestSegmentationControlsFalsePositives(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	base, _ := testBaseline(t, goodObs(10, qualitySeg(), "base", now))
	window := goodObs(6, qualitySeg(), "home", now.Add(24*time.Hour))
	foreign := qualitySeg()
	foreign.Host = "codex"
	window = append(window, badObs(6, foreign, "away", now.Add(25*time.Hour))...)
	d := Check(base, window, "base-dir", "mixed.json")
	if len(d.Alerts) != 0 {
		t.Fatalf("clean home segment must not alert on foreign failures:\n%s", RenderDiagnosis(d))
	}
	if d.WindowN != 6 || d.ExcludedN != 6 {
		t.Fatalf("segmentation must be counted: matched=%d excluded=%d", d.WindowN, d.ExcludedN)
	}
	joined := strings.Join(d.Limitations, "\n")
	if !strings.Contains(joined, "mixed window") {
		t.Fatalf("diagnosis must flag the mixed window, got %v", d.Limitations)
	}
	if !strings.Contains(joined, "segmentation reversed the naive read") {
		t.Fatalf("diagnosis must expose the confounded aggregate, got %v", d.Limitations)
	}
}

// p90 stays silent below its minimum even when the tail explodes.
func TestP90NeedsItsOwnN(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	seg := qualitySeg()
	baseObs := goodObs(12, seg, "base", now)
	base, _ := testBaseline(t, baseObs)
	window := goodObs(8, seg, "win", now.Add(24*time.Hour))
	for i := range window {
		window[i].LatencyMS = 100
	}
	window[7].LatencyMS = 50000
	d := Check(base, window, "base-dir", "tail.json")
	for _, f := range d.Findings {
		if f.Metric == "latency-p90" {
			t.Fatalf("p90 must not run at n=8:\n%s", RenderDiagnosis(d))
		}
	}
	found := false
	for _, f := range d.Findings {
		if f.Metric == "latency-p50" {
			found = true
		}
	}
	if !found {
		t.Fatalf("median must still run at n=8:\n%s", RenderDiagnosis(d))
	}
}

// Versioned evidence round-trips and fails closed on tampering, so
// reproduction replays the exact bytes on both sides.
func TestEvidenceRoundTripAndTamperFailsClosed(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	base, dir := testBaseline(t, goodObs(10, qualitySeg(), "base", now))
	window := badObs(10, qualitySeg(), "win", now.Add(24*time.Hour))
	d := Check(base, window, dir, "win.json")
	out := t.TempDir()
	if _, err := WriteDiagnosisEvidence(out, d, "base-sha", "win-sha"); err != nil {
		t.Fatalf("WriteDiagnosisEvidence: %v", err)
	}
	m, rd, err := ReadDiagnosisEvidence(out)
	if err != nil {
		t.Fatalf("ReadDiagnosisEvidence: %v", err)
	}
	if m.BaselineSHA != "base-sha" || m.WindowSHA != "win-sha" {
		t.Fatalf("manifest must pin both input shas: %+v", m)
	}
	if len(rd.Alerts) != len(d.Alerts) {
		t.Fatalf("stored diagnosis lost alerts: %d vs %d", len(rd.Alerts), len(d.Alerts))
	}
	text := RenderAlerts(rd)
	if !strings.Contains(text, "ALERT success") || !strings.Contains(text, "reproduce:") {
		t.Fatalf("alerts view must carry alerts and reproduction, got:\n%s", text)
	}
	tamper := filepath.Join(out, DiagnosisFile)
	raw, _ := os.ReadFile(tamper)
	raw = append(raw, '\n')
	if err := os.WriteFile(tamper, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadDiagnosisEvidence(out); err == nil {
		t.Fatal("tampered diagnosis must fail closed")
	}
	basePath := filepath.Join(dir, BaselineFile)
	raw, _ = os.ReadFile(basePath)
	raw = append(raw, ' ')
	if err := os.WriteFile(basePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadBaselineEvidence(dir); err == nil {
		t.Fatal("tampered baseline must fail closed")
	}
}

// Unknown tokens and coverage skip their rules instead of reading
// as zero: unknown is unknown.
func TestUnknownDimensionsSkipRules(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	seg := qualitySeg()
	baseObs := goodObs(10, seg, "base", now)
	for i := range baseObs {
		baseObs[i].TokensKnown = false
		baseObs[i].CoverageKnown = false
	}
	base, _ := testBaseline(t, baseObs)
	window := goodObs(10, seg, "win", now.Add(24*time.Hour))
	d := Check(base, window, "base-dir", "w.json")
	for _, f := range d.Findings {
		if f.Metric == "tokens" || f.Metric == "coverage" {
			t.Fatalf("unmeasured dimension must skip its rule, ran %q", f.Metric)
		}
	}
	joined := strings.Join(d.Limitations, "\n")
	if !strings.Contains(joined, "tokens unmeasured") || !strings.Contains(joined, "coverage unknown") {
		t.Fatalf("skipped rules must be declared, got %v", d.Limitations)
	}
}

// Strict inputs fail closed: unknown source, negative latency,
// out-of-range coverage and unknown window fields are refused.
func TestWindowValidationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mk := `[{"source":"lab","run_id":"r1","success":true,"retries":0,"latency_ms":10,"latency_known":true}]`
	if _, _, err := LoadWindow(write("ok.json", mk)); err != nil {
		t.Fatalf("valid window refused: %v", err)
	}
	for name, body := range map[string]string{
		"bad-source": `[{"source":"ouija","run_id":"r1"}]`,
		"neg-lat":    `[{"source":"lab","run_id":"r1","latency_ms":-1}]`,
		"bad-cov":    `[{"source":"lab","run_id":"r1","coverage_pct":101,"coverage_known":true}]`,
		"no-run":     `[{"source":"lab"}]`,
		"extra":      `[{"source":"lab","run_id":"r1","telepathy":true}]`,
	} {
		if _, _, err := LoadWindow(write(name+".json", body)); err == nil {
			t.Errorf("%s: malformed window must fail, passed", name)
		}
	}
	if _, err := LoadThresholds(write("th.json", `{"success_drop_abs": 2}`)); err == nil {
		t.Error("out-of-range threshold must fail")
	}
	if _, err := LoadThresholds(write("th2.json", `{"success_drop_abs": 0.1, "bogus": 1}`)); err == nil {
		t.Error("unknown threshold field must fail")
	}
}
