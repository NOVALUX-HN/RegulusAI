package journey

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
)

func runSextantCLI(args ...string) (int, string) {
	var out, errb bytes.Buffer
	c := cli.CLI{
		Version: "test",
		Probe: func() doctor.Result {
			return doctor.Result{Detected: "opencode 2.1.4", Major: 2, Supported: true}
		},
		Stdout: &out,
		Stderr: &errb,
	}
	code := c.Run(args)
	return code, out.String() + "\n---stderr---\n" + errb.String()
}

// TestSextantGuidedFlow walks AUTHORIZE→CHART→BUILD→PROVE→LEARN at the
// CLI level: chart init, task check green, break chart → check red with
// next-step guidance, gate run blocked before BUILD, fix chart → gate
// run passes with actionable footer on failure, task close records CLOSED.
func TestSextantGuidedFlow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(`{
  "gates": [
    {"name": "typecheck", "layer": 1, "command": "echo ok"},
    {"name": "unit-tests", "layer": 2, "command": "echo ok"}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// CHART: scaffold a standard chart.
	if code, out := runSextantCLI("chart", "init", "demo"); code != 0 {
		t.Fatalf("chart init: exit=%d out=%q", code, out)
	}
	chart := filepath.Join("charts", "demo.md")

	// AUTHORIZE→CHART: a clean scaffold passes check.
	if code, out := runSextantCLI("task", "check", chart); code != 0 {
		t.Fatalf("task check clean: exit=%d out=%q", code, out)
	}

	// Break the chart: remove the acceptance scenario body so spec fails.
	raw, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(raw), "- Given a clean project", "- Given", 1)
	if broken == string(raw) {
		t.Fatal("fixture did not break as expected")
	}
	if err := os.WriteFile(chart, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	// CHART red: check must fail, explain why it matters, and name the next step.
	code, out := runSextantCLI("task", "check", chart)
	if code != 1 {
		t.Fatalf("task check broken: exit=%d want 1 out=%q", code, out)
	}
	if !strings.Contains(out, "violation:") {
		t.Fatalf("task check must list violations, got %q", out)
	}
	if !strings.Contains(out, "next:") {
		t.Fatalf("task check must name the next step (next:), got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "build") {
		t.Fatalf("task check must explain BUILD is blocked, got %q", out)
	}

	// BUILD blocked: gate run must refuse before layer 1 when the chart is invalid.
	code, out = runSextantCLI("gate", "run", "demo/T1")
	if code != 1 {
		t.Fatalf("gate run with broken chart: exit=%d want 1 out=%q", code, out)
	}
	if strings.Contains(out, "verdict:") || strings.Contains(out, "gauntlet:") {
		t.Fatalf("gate run must not execute layers when chart is invalid, got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "blocked") {
		t.Fatalf("gate run must say it is blocked by the chart, got %q", out)
	}
	if !strings.Contains(out, "next:") {
		t.Fatalf("gate run block must name the next step (next:), got %q", out)
	}

	// Fix the chart: restore and prove green again.
	if err := os.WriteFile(chart, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runSextantCLI("task", "check", chart); code != 0 {
		t.Fatalf("task check fixed: exit=%d out=%q", code, out)
	}

	// BUILD→PROVE: gauntlet runs and leaves receipts.
	if code, out := runSextantCLI("gate", "run", "demo/T1"); code != 0 {
		t.Fatalf("gate run fixed: exit=%d out=%q", code, out)
	}

	// PROVE→LEARN: close records CLOSED in ## Log (core-verified completion).
	if code, out := runSextantCLI("task", "close", chart, "T1"); code != 0 {
		t.Fatalf("task close: exit=%d out=%q", code, out)
	}
	closed, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(closed), "CLOSED T1") {
		t.Fatalf("close must append CLOSED T1 to ## Log, got:\n%s", closed)
	}
}
