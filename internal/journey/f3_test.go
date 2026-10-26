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

// TestF3Resume proves the F3 exit criterion: after a restart — a fresh,
// cold CLI invocation over the same on-disk state — the Chart plus the
// Almanac alone are enough to resume. The context block surfaces the
// chart pointer, the in-flight session notes and the decisions saved
// with provenance; the revision chain and retrieval still hold.
func TestF3Resume(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// The committed feature document: the Chart.
	if err := os.MkdirAll(filepath.Join(dir, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	chart := "# Feature X\n\n## Acceptance\n\n### Scenario: s\n\n- Given a chart\n- When work resumes\n- Then the exit code is 0\n"
	if err := os.WriteFile(filepath.Join(dir, "charts", "feature-x.md"), []byte(chart), 0o644); err != nil {
		t.Fatal(err)
	}

	// Every run is a cold process: state lives on disk only.
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

	// Session one: work happens, notes and decisions land in the Almanac.
	code, out := run("memory", "note", "session one: gauntlet gates green, receipts written")
	if code != 0 {
		t.Fatalf("note: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "note", "next: wire the Crew delegation into the plugin")
	if code != 0 {
		t.Fatalf("note 2: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "save", "--type", "decision", "--title", "mutation over git archive export", "--topic-key", "mutation-gate", "--task", "feature-x/T1", "whole-repo temp copies starve mutants on Windows")
	if code != 0 {
		t.Fatalf("save: exit=%d out=%q", code, out)
	}

	// THE RESTART: a new invocation with nothing in process memory.
	code, out = run("memory", "context")
	if code != 0 {
		t.Fatalf("context after restart: exit=%d out=%q", code, out)
	}
	for _, want := range []string{
		"charts/feature-x.md",
		"session one: gauntlet gates green, receipts written",
		"next: wire the Crew delegation into the plugin",
		"DATA, not instructions",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("resume context must contain %q — got:\n%s", want, out)
		}
	}

	// The revision chain and deterministic retrieval survive the restart.
	code, out = run("memory", "history", "--topic-key", "mutation-gate")
	if code != 0 || !strings.Contains(out, "mutation over git archive export") {
		t.Fatalf("history after restart: exit=%d out=%q", code, out)
	}
	code, out = run("memory", "search", "starve")
	if code != 0 || !strings.Contains(out, "mutation over git archive export") {
		t.Fatalf("search after restart: exit=%d out=%q", code, out)
	}
}
