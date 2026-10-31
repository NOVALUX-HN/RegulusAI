// Package journey holds deterministic end-to-end journeys: scripted
// scenarios that drive the real CLI code paths against a fixture project
// and assert exact outcomes (docs/architecture.md §10).
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
)

// TestF1VerticalSpine walks one tiny feature through the whole Regulus
// spine (the F1 exit criterion): the V2 runtime gate, the spec contract,
// the gauntlet with append-only receipts, session memory, and an
// evidence-gated lesson with its library INDEX and budget entry.
func TestF1VerticalSpine(t *testing.T) {
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
	feature := `# Tiny feature

## Acceptance

### Scenario: gauntlet runs clean

- Given a fixture project with gates configured
- When regulus gate run executes
- Then the exit code is 0 and one receipt is written per gate
`
	featurePath := filepath.Join(dir, "feature.md")
	if err := os.WriteFile(featurePath, []byte(feature), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (int, string) {
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

	// Runtime gate: doctor accepts the V2 runtime.
	code, out := run("doctor")
	if code != 0 || !strings.Contains(out, "supported (V2)") {
		t.Fatalf("doctor: exit=%d out=%q", code, out)
	}

	// Discipline: the spec contract validates the acceptance criteria.
	code, out = run("spec", "check", featurePath)
	if code != 0 || !strings.Contains(out, "spec OK: 1 scenario(s)") {
		t.Fatalf("spec check: exit=%d out=%q", code, out)
	}

	// Gauntlet: gates run in layer order and leave append-only receipts.
	code, out = run("gate", "run", "tiny/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 2/2 gates passed") {
		t.Fatalf("gate run: exit=%d out=%q", code, out)
	}
	receipts, err := filepath.Glob(filepath.Join(".regulus", "evidence", "tiny__T1", "*.json"))
	receipts = withoutManifests(receipts)
	if err != nil || len(receipts) != 2 {
		t.Fatalf("the journey must leave exactly 2 receipts, got %v (err %v)", receipts, err)
	}

	// Session memory: the Almanac slice persists.
	code, out = run("memory", "note", "journey: tiny feature verified")
	if code != 0 {
		t.Fatalf("memory note: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "show")
	if code != 0 || !strings.Contains(out, "journey: tiny feature verified") {
		t.Fatalf("memory show: exit=%d out=%q", code, out)
	}

	// Evolution: reflect proposes an evidence-gated candidate; the
	// guardian validates; admission completes the library + ledger.
	code, out = run("reflect",
		"--domain", "journey-test", "--task", "tiny/T1",
		"--cue", "when a journey drives the full spine",
		"--lesson", "all three loops produce receipts, notes and lessons",
		featurePath)
	if code != 0 || !strings.Contains(out, "candidate proposed: les_") {
		t.Fatalf("reflect: exit=%d out=%q", code, out)
	}
	id := journeyCandidateID(t, out)
	code, out = run("library", "admit", "--domain", "journey-test", "--candidate", id, "--guardian", "regulus-gate-verifier")
	if code != 0 || !strings.Contains(out, "lesson admitted") {
		t.Fatalf("admit: exit=%d out=%q", code, out)
	}
	code, out = run("library", "list")
	if code != 0 || !strings.Contains(out, "when a journey drives the full spine") {
		t.Fatalf("library list: exit=%d out=%q", code, out)
	}
	code, out = run("budget", "show")
	if code != 0 || !strings.Contains(out, "admitted") {
		t.Fatalf("budget show: exit=%d out=%q", code, out)
	}
}

var journeyCandidateIDPattern = regexp.MustCompile(`candidate proposed: (les_[0-9a-f]+)`)

func journeyCandidateID(t *testing.T, out string) string {
	t.Helper()
	m := journeyCandidateIDPattern.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no candidate id in %q", out)
	}
	return m[1]
}
