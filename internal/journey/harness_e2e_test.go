// Package journey — harness E2E (NOV-40, S7).
//
// The opencode plugin hooks (regulus.ts) are thin adapters over these same
// CLI commands; hook<->CLI behavior is covered at the CLI level here, and a
// simulated JS host is out of scope for this step.
package journey

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"regulus-ai/internal/cli"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/receipts"
)

// TestHarnessE2E walks one request through the whole harness as an
// integrated system: chart fixture, task check, two echo gates, readable
// receipts (evidence show), traces, memory, an evidence-gated lesson and
// the evolve evidence pack. Hermetic: only echo commands plus TempDir,
// Probe stubbed, no network, no model, no plugin host. It asserts
// observable effects (exits, contents, counts), never just no-panic.
func TestHarnessE2E(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	run := func(args ...string) (int, string) {
		t.Helper()
		var out, errb bytes.Buffer
		c := cli.CLI{
			Version: "test",
			Probe: func() doctor.Result {
				return doctor.Result{Detected: "opencode 2.2.0", Major: 2, Supported: true}
			},
			Stdout: &out,
			Stderr: &errb,
		}
		code := c.Run(args)
		return code, out.String() + "\n---stderr---\n" + errb.String()
	}

	// Chart fixture: a scaffolded chart passes the contract check.
	if code, out := run("chart", "init", "harness"); code != 0 {
		t.Fatalf("chart init: exit=%d out=%q", code, out)
	}
	chart := filepath.Join("charts", "harness.md")
	if code, out := run("task", "check", chart); code != 0 || !strings.Contains(out, "task check: OK") {
		t.Fatalf("task check: exit=%d out=%q", code, out)
	}

	// Gauntlet: two echo gates run (explicit --yes consent, S6) and pass.
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(`{
  "gates": [
    {"name": "typecheck", "layer": 1, "command": "echo ok"},
    {"name": "unit-tests", "layer": 2, "command": "echo ok"}
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := run("gate", "run", "--yes", "harness/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 2/2 gates passed") {
		t.Fatalf("gate run: exit=%d out=%q", code, out)
	}

	// Receipts: exactly two exist and each is legible via evidence show.
	paths, err := filepath.Glob(filepath.Join(".regulus", "evidence", receipts.Slug("harness/T1"), "*.json"))
	paths = withoutManifests(paths)
	if err != nil || len(paths) != 2 {
		t.Fatalf("want exactly 2 receipts, got %v (err %v)", paths, err)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		rec, err := receipts.ReadFile(p)
		if err != nil {
			t.Fatalf("receipt %s unreadable: %v", p, err)
		}
		if rec.Verdict != receipts.VerdictPass || rec.ExitCode != 0 {
			t.Fatalf("receipt %s must be pass/exit 0, got %+v", p, rec)
		}
		seen[rec.Gate] = true
		code, out := run("evidence", "show", p)
		if code != 0 || !strings.Contains(out, "verdict:     pass (exit 0)") || !strings.Contains(out, rec.Gate) {
			t.Fatalf("evidence show %s: exit=%d out=%q", p, code, out)
		}
	}
	if !seen["typecheck"] || !seen["unit-tests"] {
		t.Fatalf("receipts must cover both gates, got %v", seen)
	}
	code, out = run("evidence", "list", "--task", "harness/T1")
	if code != 0 || !strings.Contains(out, "typecheck") || !strings.Contains(out, "unit-tests") {
		t.Fatalf("evidence list: exit=%d out=%q", code, out)
	}

	// Traces: the tool executions land in the session log with file hints.
	for _, call := range []string{"harness-1", "harness-2"} {
		code, out = run("trace", "record", "--session", "harness", "--message", "m1", "--call", call,
			"--tool", "gate_run", "--status", "completed", "--task", "harness/T1",
			"--files", "internal/gates/gates.go")
		if code != 0 {
			t.Fatalf("trace record %s: exit=%d out=%q", call, code, out)
		}
	}
	code, out = run("trace", "show", "--session", "harness")
	if code != 0 || !strings.Contains(out, "1 gate_run completed") || !strings.Contains(out, "2 gate_run completed") {
		t.Fatalf("trace show: exit=%d out=%q", code, out)
	}
	code, out = run("trace", "files", "--session", "harness")
	if code != 0 || !strings.Contains(out, "internal/gates/gates.go") {
		t.Fatalf("trace files: exit=%d out=%q", code, out)
	}

	// Memory: the session note persists and reads back.
	code, out = run("memory", "note", "harness e2e: full loop green, receipts and traces linked")
	if code != 0 {
		t.Fatalf("memory note: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "show")
	if code != 0 || !strings.Contains(out, "harness e2e: full loop green") {
		t.Fatalf("memory show: exit=%d out=%q", code, out)
	}

	// Learning: reflect proposes the evidence-gated lesson, the guardian
	// admits it, and the domain list surfaces it.
	code, out = run("reflect", "--domain", "harness", "--task", "harness/T1",
		"--cue", "when the harness runs the full request-to-lesson loop",
		"--lesson", "drive every stage through the CLI so receipts, traces and lessons stay linked",
		chart)
	if code != 0 || !strings.Contains(out, "candidate proposed: les_") {
		t.Fatalf("reflect: exit=%d out=%q", code, out)
	}
	id := harnessCandidateID(t, out)
	code, out = run("library", "admit", "--domain", "harness", "--candidate", id,
		"--guardian", "regulus-gate-verifier")
	if code != 0 || !strings.Contains(out, "lesson admitted") {
		t.Fatalf("admit: exit=%d out=%q", code, out)
	}
	code, out = run("library", "list", "--domain", "harness")
	if code != 0 || !strings.Contains(out, "when the harness runs the full request-to-lesson loop") {
		t.Fatalf("library list: exit=%d out=%q", code, out)
	}

	// Evolve: the system assembles the evidence pack for the task.
	code, out = run("evolve", "propose", "--task", "harness/T1", "--domain", "harness")
	if code != 0 {
		t.Fatalf("evolve propose: exit=%d out=%q", code, out)
	}
	for _, want := range []string{
		"# Evidence pack — task harness/T1, domain harness",
		"### session harness",
		"## Task receipts",
		"## Domain INDEX",
		"harnesses/harness__T1/",
		"## Candidate template",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("evidence pack must contain %q, got:\n%s", want, out)
		}
	}
}

var harnessCandidateIDPattern = regexp.MustCompile(`candidate proposed: (les_[0-9a-f]+)`)

func harnessCandidateID(t *testing.T, out string) string {
	t.Helper()
	m := harnessCandidateIDPattern.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no candidate id in %q", out)
	}
	return m[1]
}
