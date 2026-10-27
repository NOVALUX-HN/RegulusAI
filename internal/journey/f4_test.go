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

// TestF4Evolution proves the F4 exit criterion: a lesson produced by
// real work is adopted with evidence (candidate → guardian → admitted,
// receipts cited), reaches a later task through retrieval, and the
// ledger shows the cap enforced (a third run refused and recorded).
func TestF4Evolution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// A real work fixture: a chart, tight gates and a tight evolution cap.
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(`{
  "gates": [{"name": "typecheck", "layer": 1, "command": "echo ok"}],
  "evolution": {"max_per_day": 2}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	chart := "# Fixture\n\n## Acceptance\n\n### Scenario: evolution works\n\n- Given evidence in receipts\n- When the guardian admits the lesson\n- Then the later task retrieves it and the cap is enforced\n"
	chartPath := filepath.Join(dir, "charts", "evolution-x.md")
	if err := os.WriteFile(chartPath, []byte(chart), 0o644); err != nil {
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
		return code, out.String() + "\n" + errb.String()
	}

	// Real work first: the gauntlet leaves receipts.
	code, out := run("gate", "run", "evo/T1")
	if code != 0 || !strings.Contains(out, "gauntlet: 1/1 gates passed") {
		t.Fatalf("gate run: exit=%d out=%q", code, out)
	}

	// Phase 1 — propose: nothing is admitted without a guardian.
	code, out = run("reflect", "--domain", "evo", "--task", "evo/T1",
		"--cue", "when a lesson is adopted", "--lesson", "the later task retrieves it from the library",
		chartPath)
	if code != 0 || !strings.Contains(out, "candidate proposed: les_") {
		t.Fatalf("reflect: exit=%d out=%q", code, out)
	}
	id := evolutionCandidateID(t, out)
	code, out = run("library", "list", "--domain", "evo")
	if code != 0 || strings.Contains(out, "when a lesson is adopted") {
		t.Fatalf("before admission the lesson must not be listed: %q", out)
	}

	// Phase 2 — the guardian validates; admission cites the evidence.
	code, out = run("library", "admit", "--domain", "evo", "--candidate", id,
		"--guardian", "regulus-gate-verifier")
	if code != 0 || !strings.Contains(out, "lesson admitted") {
		t.Fatalf("admit: exit=%d out=%q", code, out)
	}

	// The later task retrieves the lesson: memory search, the domain
	// INDEX, and the context block all surface it.
	code, out = run("memory", "search", "retrieves")
	if code != 0 || !strings.Contains(out, "when a lesson is adopted") {
		t.Fatalf("search: exit=%d out=%q", code, out)
	}
	code, out = run("library", "show", "--domain", "evo")
	if code != 0 || !strings.Contains(out, "when a lesson is adopted") {
		t.Fatalf("library show: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "context")
	if code != 0 || !strings.Contains(out, "evo: 1 lesson(s)") {
		t.Fatalf("context: exit=%d out=%q", code, out)
	}

	// A second proposal fits inside the cap; the third is refused and
	// the refusal is recorded in the ledger.
	code, out = run("gate", "run", "evo/T2")
	if code != 0 {
		t.Fatalf("gate run T2: exit=%d out=%q", code, out)
	}
	code, out = run("reflect", "--domain", "evo", "--task", "evo/T2",
		"--cue", "second within the cap", "--lesson", "still proposes", chartPath)
	if code != 0 || !strings.Contains(out, "candidate proposed: les_") {
		t.Fatalf("second reflect (within cap): exit=%d out=%q", code, out)
	}
	code, out = run("gate", "run", "evo/T3")
	if code != 0 {
		t.Fatalf("gate run T3: exit=%d out=%q", code, out)
	}
	code, out = run("reflect", "--domain", "evo", "--task", "evo/T3",
		"--cue", "third over the cap", "--lesson", "must be refused", chartPath)
	if code == 0 || !strings.Contains(out, "evolution cap reached") {
		t.Fatalf("third reflect must hit the cap: exit=%d out=%q", code, out)
	}

	code, out = run("budget", "show")
	if code != 0 ||
		!strings.Contains(out, "candidate-proposed") ||
		!strings.Contains(out, "admitted") ||
		!strings.Contains(out, "refused-cap") {
		t.Fatalf("the ledger must show the whole story: exit=%d\n%s", code, out)
	}
}

var evolutionCandidateIDPattern = regexp.MustCompile(`candidate proposed: (les_[0-9a-f]+)`)

func evolutionCandidateID(t *testing.T, out string) string {
	t.Helper()
	m := evolutionCandidateIDPattern.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no candidate id in %q", out)
	}
	return m[1]
}
