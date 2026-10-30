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

// TestV06Functional walks the v0.6 functional surface end-to-end, in
// process, on a fresh directory: init scaffolds a repo, the gauntlet
// writes receipts, traces capture tool executions, probe contrast
// produces behavioral evidence, reflect+admit complete the evolution
// flow, evolve propose assembles the evidence pack, and status reports
// per-task time. Deterministic: no model, no network, no plugin host.
func TestV06Functional(t *testing.T) {
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

	// init: a fresh repo becomes a Regulus project.
	code, out := run("init", "--stack", "smoke")
	if code != 0 || !strings.Contains(out, "initialized:") {
		t.Fatalf("init: exit=%d out=%q", code, out)
	}
	for _, p := range []string{"regulus.json", "charts", ".gitignore"} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("init must create %s: %v", p, err)
		}
	}

	// The smoke gate needs a README; reflect needs a valid spec contract.
	if err := os.WriteFile("README.md", []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	feature := `# V0.6 functional journey

## Acceptance

### Scenario: the functional loop completes

- Given a freshly initialized Regulus project
- When the loop runs gate, trace, contrast, reflect, admit, evolve and status
- Then every step exits 0 and leaves its state behind
`
	if err := os.WriteFile("feature.md", []byte(feature), 0o644); err != nil {
		t.Fatal(err)
	}

	// Gauntlet: the initialized gates run and leave receipts.
	code, out = run("gate", "run", "v06/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 1/1 gates passed") {
		t.Fatalf("gate run: exit=%d out=%q", code, out)
	}

	// Traces: tool executions land in the session log with file hints.
	for _, call := range []string{"c1", "c2"} {
		code, out = run("trace", "record", "--session", "s1", "--message", "m1", "--call", call,
			"--tool", "gate_run", "--status", "completed", "--files", "internal/gates/gates.go")
		if code != 0 {
			t.Fatalf("trace record %s: exit=%d out=%q", call, code, out)
		}
	}
	code, out = run("trace", "show", "--session", "s1")
	if code != 0 || !strings.Contains(out, "1 gate_run completed") || !strings.Contains(out, "2 gate_run completed") {
		t.Fatalf("trace show: exit=%d out=%q", code, out)
	}
	code, out = run("trace", "files", "--session", "s1")
	if code != 0 || !strings.Contains(out, "internal/gates/gates.go") {
		t.Fatalf("trace files: exit=%d out=%q", code, out)
	}

	// Probe contrast: two real gate runs, receipt diff as evidence.
	code, out = run("probe", "contrast", "--task", "v06/T1", "--gate", "presence",
		"--a", "echo alpha", "--b", "echo beta")
	if code != 0 {
		t.Fatalf("probe contrast: exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, "## Differences") && !strings.Contains(out, "+ B:") {
		t.Fatalf("contrast must show the behavioral diff, got %q", out)
	}

	// Evolution: reflect proposes, the guardian admits.
	code, out = run("reflect", "--domain", "journey", "--task", "v06/T1",
		"--dimension", "inv",
		"--cue", "when the functional journey runs the full loop",
		"--lesson", "every step leaves receipts, traces or lessons behind",
		"feature.md")
	if code != 0 || !strings.Contains(out, "candidate proposed: les_") {
		t.Fatalf("reflect: exit=%d out=%q", code, out)
	}
	id := journeyCandidateID(t, out)
	code, out = run("library", "admit", "--domain", "journey", "--candidate", id,
		"--guardian", "regulus-gate-verifier")
	if code != 0 || !strings.Contains(out, "lesson admitted") {
		t.Fatalf("admit: exit=%d out=%q", code, out)
	}

	// evolve propose: the system assembles the evidence pack.
	code, out = run("evolve", "propose", "--task", "v06/T1", "--domain", "journey")
	if code != 0 {
		t.Fatalf("evolve propose: exit=%d out=%q", code, out)
	}
	for _, want := range []string{
		"# Evidence pack — task v06/T1, domain journey",
		"### session s1",
		"## Task receipts",
		"## Domain INDEX",
		"harnesses/v06__T1/",
		"## Recent evolution decisions for this task",
		"## Candidate template",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("evidence pack must contain %q, got:\n%s", want, out)
		}
	}

	// status: one look at the project, time per task included.
	code, out = run("status")
	if code != 0 {
		t.Fatalf("status: exit=%d out=%q", code, out)
	}
	if !strings.Contains(out, "receipts:") || !strings.Contains(out, "time:") || !strings.Contains(out, "v06/T1") {
		t.Fatalf("status must report receipts and per-task time, got:\n%s", out)
	}

	// The library index exists on disk with the adopted snapshot.
	if _, err := os.Stat(filepath.Join(".regulus", "library", "journey", "INDEX.md")); err != nil {
		t.Fatalf("INDEX.md must exist after admission: %v", err)
	}
	exp, err := os.Stat(filepath.Join(".regulus", "library", "journey", "harnesses", "v06__T1", "EXPERIENCE.md"))
	if err != nil || exp.IsDir() {
		t.Fatalf("EXPERIENCE.md snapshot must exist after admission: %v", err)
	}
}
