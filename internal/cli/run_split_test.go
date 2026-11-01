package cli

// G4 SPLIT pins: the dispatch table extracted from Run routes every
// legacy subcommand with identical exit codes and messages.

import (
	"strings"
	"testing"

	"regulus-ai/internal/doctor"
)

func TestRunDispatchTableCompleteness(t *testing.T) {
	want := []string{
		"doctor", "spec", "status", "chart", "init", "surface",
		"evidence", "gate", "mcp", "memory", "budget", "library",
		"reflect", "evolve", "skills", "probe", "trace", "task",
		"decide", "yolo", "crap",
	}
	if len(cliDispatch) != len(want) {
		t.Fatalf("dispatch table has %d entries, want %d (%v)", len(cliDispatch), len(want), cliDispatch)
	}
	for _, name := range want {
		if _, ok := cliDispatch[name]; !ok {
			t.Errorf("dispatch table omits %q (was a Run case before the split)", name)
		}
	}
}

func TestRunBuiltinsUnchangedAfterSplit(t *testing.T) {
	c, out, errb := newCLI(nil)
	if code := c.Run(nil); code != 2 {
		t.Fatalf("no args exit = %d, want 2", code)
	}
	if code := c.Run([]string{"--version"}); code != 0 || out.String() != "regulus test\n" {
		t.Fatalf("--version exit/output changed: code=%d out=%q", code, out.String())
	}
	if code := c.Run([]string{"contract"}); code != 0 {
		t.Fatalf("contract exit = %d, want 0", code)
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"help"}); code != 0 || !strings.Contains(out2.String(), "Usage:") {
		t.Fatalf("help exit/output changed: code=%d", code)
	}
	if code := c.Run([]string{"frobnicate"}); code != 2 || !strings.Contains(errb.String(), `unknown command "frobnicate"`) {
		t.Fatalf("unknown command handling changed: code=%d stderr=%q", code, errb.String())
	}
}

func TestRunDispatchesThroughTable(t *testing.T) {
	// Every table entry must be reachable via Run with identical
	// behavior to a direct table call (same exit code, same output).
	probe := func() doctor.Result {
		return doctor.Result{Detected: "opencode test", Major: 2, Supported: true}
	}
	for name, fn := range cliDispatch {
		c1, out1, errb1 := newCLI(probe)
		want := fn(c1, []string{"--help"})
		c2, out2, errb2 := newCLI(probe)
		got := c2.Run([]string{name, "--help"})
		if got != want {
			t.Errorf("Run(%q --help) = %d, direct dispatch = %d", name, got, want)
		}
		if out1.String() != out2.String() || errb1.String() != errb2.String() {
			t.Errorf("Run(%q --help) output differs from direct dispatch", name)
		}
	}
}
