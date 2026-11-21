// CLI tests for `regulus quality ...` (F12.4/NOV-99): baseline,
// check and alerts round-trip with byte-identical usage and exits.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeQualityWindow seeds a JSON window file: n good runs plus n
// bad runs in the home segment.
func writeQualityWindow(t *testing.T, dir, name string, good, bad int) string {
	t.Helper()
	type obs struct {
		Agent         string  `json:"agent"`
		Model         string  `json:"model"`
		Host          string  `json:"host"`
		Task          string  `json:"task"`
		Version       string  `json:"version"`
		Success       bool    `json:"success"`
		Retries       int     `json:"retries"`
		LatencyMS     int64   `json:"latency_ms"`
		LatencyKnown  bool    `json:"latency_known"`
		TokensIn      int64   `json:"tokens_in"`
		TokensOut     int64   `json:"tokens_out"`
		TokensKnown   bool    `json:"tokens_known"`
		Errors        int     `json:"errors"`
		CoveragePct   float64 `json:"coverage_pct"`
		CoverageKnown bool    `json:"coverage_known"`
		Source        string  `json:"source"`
		RunID         string  `json:"run_id"`
		Evidence      string  `json:"evidence"`
	}
	var out []obs
	for i := 0; i < good; i++ {
		out = append(out, obs{Agent: "a", Model: "m", Host: "h", Task: "t", Version: "v",
			Success: true, LatencyMS: 100, LatencyKnown: true,
			TokensIn: 100, TokensOut: 100, TokensKnown: true,
			CoveragePct: 80, CoverageKnown: true,
			Source: "lab", RunID: "good-" + qitoa(i), Evidence: "ev"})
	}
	for i := 0; i < bad; i++ {
		out = append(out, obs{Agent: "a", Model: "m", Host: "h", Task: "t", Version: "v",
			Success: false, Retries: 4, LatencyMS: 900, LatencyKnown: true,
			TokensIn: 800, TokensOut: 800, TokensKnown: true, Errors: 2,
			CoveragePct: 60, CoverageKnown: true,
			Source: "lab", RunID: "bad-" + qitoa(i), Evidence: "ev"})
	}
	if out == nil {
		out = []obs{}
	}
	raw, _ := json.Marshal(out)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func qitoa(n int) string { return fmt.Sprintf("%d", n) }

// qualityBaselineFixture records a baseline over 10 good runs.
func qualityBaselineFixture(t *testing.T, dir string) string {
	t.Helper()
	w := writeQualityWindow(t, dir, "base-window.json", 10, 0)
	base := filepath.Join(dir, "baseline")
	c, out, _ := newCLI(nil)
	c.Dir = dir
	args := []string{"quality", "baseline", "--window", w,
		"--agent", "a", "--model", "m", "--host", "h", "--version", "v",
		"--out", base}
	if code := c.Run(args); code != 0 {
		t.Fatalf("baseline: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "quality: baseline agent=a|model=m|host=h|task=(any)|version=v n=10") {
		t.Fatalf("baseline must echo segment and N, got %q", out.String())
	}
	return base
}

// A clean window checks green; an injected regression alerts with
// baseline, window, N, magnitude and limitations — and stores the
// rendered diagnosis for reproduction.
func TestQualityCheckDetectsInjectedRegression(t *testing.T) {
	dir := t.TempDir()
	base := qualityBaselineFixture(t, dir)
	win := writeQualityWindow(t, dir, "win.json", 2, 8)
	c, out, _ := newCLI(nil)
	c.Dir = dir
	diagDir := filepath.Join(dir, "diag")
	if code := c.Run([]string{"quality", "check", "--baseline", base, "--window", win, "--out", diagDir}); code != 0 {
		t.Fatalf("check: exit = %d, want 0 (diagnosis is not a gate)", code)
	}
	text := out.String()
	for _, want := range []string{"ALERT success", "ALERT latency-p50", "limit:", "next:", "reproduce:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnosis must contain %q, got:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "window n=10 matched + 0 excluded") {
		t.Fatalf("diagnosis must show baseline scope and N, got:\n%s", text)
	}
	c2, out2, _ := newCLI(nil)
	c2.Dir = dir
	if code := c2.Run([]string{"quality", "alerts", "--dir", diagDir}); code != 0 {
		t.Fatalf("alerts: exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "ALERT success") || !strings.Contains(out2.String(), "reproduce:") {
		t.Fatalf("alerts must replay stored alerts with reproduction, got:\n%s", out2.String())
	}
}

// A clean window stays silent: no ALERT lines, verdict says so.
func TestQualityCheckCleanWindowStaysSilent(t *testing.T) {
	dir := t.TempDir()
	base := qualityBaselineFixture(t, dir)
	win := writeQualityWindow(t, dir, "clean.json", 10, 0)
	c, out, _ := newCLI(nil)
	c.Dir = dir
	if code := c.Run([]string{"quality", "check", "--baseline", base, "--window", win}); code != 0 {
		t.Fatalf("check: exit = %d, want 0", code)
	}
	if strings.Contains(out.String(), "ALERT") {
		t.Fatalf("clean window must not alert, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "no regressions") {
		t.Fatalf("clean verdict must say so, got:\n%s", out.String())
	}
}

// Missing flags and unknown subcommands keep the byte-identical
// usage with exit 2; unreadable evidence refuses with exit 1.
func TestQualityUsageAndRefusals(t *testing.T) {
	for _, argv := range [][]string{
		{"quality"},
		{"quality", "check"},
		{"quality", "check", "--baseline", "x"},
		{"quality", "baseline", "--window", "x"},
		{"quality", "alerts"},
		{"quality", "frobnicate"},
	} {
		c, _, errb := newCLI(nil)
		if code := c.Run(argv); code != 2 {
			t.Fatalf("%v: exit = %d, want 2", argv, code)
		}
		if !strings.Contains(errb.String(), "usage: regulus quality baseline") {
			t.Fatalf("%v: refusal must carry the byte-identical usage, got %q", argv, errb.String())
		}
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"quality", "check", "--baseline", "nope", "--window", "nope"}); code != 1 {
		t.Fatalf("unreadable evidence: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "quality check:") {
		t.Fatalf("refusal must name the command, got %q", errb.String())
	}
}
