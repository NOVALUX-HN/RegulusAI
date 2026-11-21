// CLI tests for `regulus orchestrate` (F12.1/NOV-96): byte-identical
// usage, deterministic dry-run decisions, evidence-gate refusals and
// the evaluation that may keep the fixed router.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func TestOrchestrateHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		c, out, _ := newCLI(nil)
		if code := c.Run([]string{"orchestrate", flag}); code != 0 {
			t.Fatalf("orchestrate %s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(out.String(), "usage: regulus orchestrate agents") {
			t.Fatalf("orchestrate %s: stdout = %q", flag, out.String())
		}
	}
}

func TestOrchestrateUnknownSubcommand(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("must refuse the unknown subcommand, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate agents") {
		t.Fatalf("refusal must carry the byte-identical usage, got %q", errb.String())
	}
}

func TestOrchestrateDecideNeedsDescriptor(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "decide", "--domain", "harness"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate agents") {
		t.Fatalf("missing descriptor must print the usage, got %q", errb.String())
	}
}

func TestOrchestrateDecideUnknownCapability(t *testing.T) {
	c, _, errb := newCLI(nil)
	args := []string{"orchestrate", "decide", "--domain", "harness", "--lang", "go",
		"--complexity", "low", "--risk", "low", "--cap", "teleport", "--dry-run"}
	if code := c.Run(args); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown capability "teleport"`) {
		t.Fatalf("unknown capability must be named, got %q", errb.String())
	}
}

// Dry-run decisions are byte-identical across invocations (the trace
// hint is recording metadata, absent on dry-run).
func TestOrchestrateDecideDryRunDeterministic(t *testing.T) {
	args := []string{"orchestrate", "decide", "--domain", "harness", "--lang", "go",
		"--complexity", "low", "--risk", "low", "--cap", "tools",
		"--dry-run", "--json"}
	c1, out1, _ := newCLI(nil)
	if code := c1.Run(args); code != 0 {
		t.Fatalf("first run: exit = %d, want 0", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run(args); code != 0 {
		t.Fatalf("second run: exit = %d, want 0", code)
	}
	if out1.String() != out2.String() {
		t.Fatalf("dry-run decisions diverged:\n%s\n%s", out1.String(), out2.String())
	}
	var dec struct {
		Primary string `json:"primary"`
		Mode    string `json:"mode"`
		DryRun  bool   `json:"dry_run"`
	}
	if err := json.Unmarshal([]byte(out1.String()), &dec); err != nil {
		t.Fatalf("decision must be JSON: %v", err)
	}
	if dec.Primary == "" || dec.Mode != "single" || !dec.DryRun {
		t.Fatalf("expected a single-mode dry-run primary, got %+v", dec)
	}
}

// Impossible requirements refuse with exit 1 and name the gate.
func TestOrchestrateDecideRefusesWithoutEligible(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	c, out, errb := newCLI(nil)
	args := []string{"orchestrate", "decide", "--domain", "harness", "--lang", "go",
		"--complexity", "low", "--risk", "low", "--cap", "tools",
		"--budget-time-ms", "1", "--dry-run"}
	if code := c.Run(args); code != 1 {
		t.Fatalf("exit = %d, want 1 (stdout %q stderr %q)", code, out.String(), errb.String())
	}
	if !strings.Contains(errb.String(), "sin agente elegible") {
		t.Fatalf("refusal must say so, got %q", errb.String())
	}
}

// Critical tasks route a pair with a named reviewer.
func TestOrchestrateDecideCriticalPair(t *testing.T) {
	c, out, _ := newCLI(nil)
	args := []string{"orchestrate", "decide", "--domain", "harness", "--lang", "go",
		"--complexity", "high", "--risk", "critical", "--cap", "tools",
		"--dry-run", "--json"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("exit = %d, want 0 (stdout %q)", code, out.String())
	}
	var dec struct {
		Mode     string `json:"mode"`
		Primary  string `json:"primary"`
		Reviewer string `json:"reviewer"`
	}
	if err := json.Unmarshal([]byte(out.String()), &dec); err != nil {
		t.Fatalf("decision must be JSON: %v", err)
	}
	if dec.Mode != "pair" || dec.Primary == "" || dec.Reviewer == "" {
		t.Fatalf("critical must route a pair, got %+v", dec)
	}
}

func writeArmFile(t *testing.T, dir, name string, results []bench.LabResult) string {
	t.Helper()
	raw, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Identical arms keep the fixed router — exit 0, a conclusion.
func TestOrchestrateEvaluateTieKeepsFixed(t *testing.T) {
	dir := t.TempDir()
	arm := []bench.LabResult{
		{AgentID: "baseline", Verdict: bench.LabPass, DurationMS: 100, TokensIn: 25, TokensOut: 25},
		{AgentID: "baseline", Verdict: bench.LabPass, DurationMS: 120, TokensIn: 25, TokensOut: 25},
	}
	dyn := writeArmFile(t, dir, "dyn.json", arm)
	fix := writeArmFile(t, dir, "fix.json", arm)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "evaluate", "--dynamic", dyn, "--fixed", fix}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "KEEP_FIXED") {
		t.Fatalf("tie must keep fixed, got %q", out.String())
	}
}

func TestOrchestrateEvaluateNeedsArms(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"orchestrate", "evaluate", "--dynamic", "a.json"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus orchestrate agents") {
		t.Fatalf("missing arm must print the usage, got %q", errb.String())
	}
}
