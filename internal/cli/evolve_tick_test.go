package cli

// T4 (pro-harness): `evolve tick` fires the periodic proposer — every
// k traced sessions it mines repeated tool errors and stale lessons
// into library candidates (guardian still admits). Dry-run prints
// without writing; not-due prints the cadence position.
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recordTickCLI(t *testing.T, session, call, tool, status string) {
	t.Helper()
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", session, "--message", "m" + call,
		"--call", call, "--tool", tool, "--status", status}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record %s: exit = %d, stderr %q", call, code, errb.String())
	}
}

func TestEvolveTickUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evolve", "tick", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve tick") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvolveTickNotDue(t *testing.T) {
	t.Chdir(t.TempDir())
	recordTickCLI(t, "s1", "c1", "read", "completed")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "tick"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "no tick due") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestEvolveTickDryRunWritesNothing(t *testing.T) {
	t.Chdir(t.TempDir())
	recordTickCLI(t, "s1", "c1", "gate_run", "error")
	recordTickCLI(t, "s1", "c2", "gate_run", "error")
	recordTickCLI(t, "s2", "c3", "gate_run", "error")
	recordTickCLI(t, "s3", "c4", "read", "completed")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "tick", "--dry-run"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "repeated-errors") || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("dry run must print findings + mark itself: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(".regulus", "evolution", "tick.json")); !os.IsNotExist(err) {
		t.Fatal("dry run must not advance the tick state")
	}
	if _, err := os.Stat(filepath.Join(".regulus", "library")); !os.IsNotExist(err) {
		t.Fatal("dry run must not propose candidates")
	}
}

func TestEvolveTickApplies(t *testing.T) {
	t.Chdir(t.TempDir())
	recordTickCLI(t, "s1", "c1", "gate_run", "error")
	recordTickCLI(t, "s1", "c2", "gate_run", "error")
	recordTickCLI(t, "s2", "c3", "gate_run", "error")
	recordTickCLI(t, "s3", "c4", "read", "completed")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "tick"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "1 candidate(s)") {
		t.Fatalf("apply must report the proposal count: %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"evolve", "tick"}); code != 0 {
		t.Fatalf("second tick exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "no tick due") {
		t.Fatalf("second tick must not be due: %q", out2.String())
	}
}
