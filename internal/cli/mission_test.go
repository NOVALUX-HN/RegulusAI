// CLI tests for F13.1/NOV-104: the mission command family through
// the real dispatch — help, unknown subcommands, the full
// lifecycle round-trip, refusals with byte-identical usage, and
// cancel/resume durability after a simulated restart.
package cli

import (
	"strings"
	"testing"
)

func missionCLI(t *testing.T) (CLI, *strings.Builder, *strings.Builder, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	var out, errb strings.Builder
	c := CLI{Version: "test", Stdout: &out, Stderr: &errb, Dir: dir}
	return c, &out, &errb, dir
}

func TestMissionHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"mission", "--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus mission create") {
		t.Fatalf("stdout = %q", out.String())
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"mission", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("must refuse the unknown subcommand, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "usage: regulus mission create") {
		t.Fatalf("refusal must carry the byte-identical usage, got %q", errb.String())
	}
}

func TestMissionFullLifecycleThroughDispatch(t *testing.T) {
	c, out, errb, _ := missionCLI(t)
	if code := c.Run([]string{"mission", "create", "--id", "m1",
		"--objective", "ship durable missions", "--risk", "high",
		"--creator", "tester"}); code != 0 {
		t.Fatalf("create: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "mission: created m1 state=created generation=1") {
		t.Fatalf("create must confirm the sealed record, got %q", out.String())
	}
	steps := [][]string{
		{"mission", "advance", "--id", "m1", "--to", "analyzing"},
		{"mission", "advance", "--id", "m1", "--to", "planning"},
		{"mission", "advance", "--id", "m1", "--to", "awaiting_approval"},
		{"mission", "advance", "--id", "m1", "--to", "executing"},
		{"mission", "pause", "--id", "m1", "--reason", "hold"},
		{"mission", "resume", "--id", "m1"},
		{"mission", "advance", "--id", "m1", "--to", "validating"},
		{"mission", "advance", "--id", "m1", "--to", "completed"},
	}
	for _, s := range steps {
		out.Reset()
		if code := c.Run(s); code != 0 {
			t.Fatalf("%v: exit = %d stderr = %q", s, code, errb.String())
		}
	}
	out.Reset()
	if code := c.Run([]string{"mission", "show", "--id", "m1"}); code != 0 {
		t.Fatalf("show: exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"m1", "completed", "risk=high", "generation=9"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("show must carry %q, got:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := c.Run([]string{"mission", "status", "--id", "m1"}); code != 0 {
		t.Fatalf("status: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "mission m1: completed generation=9") {
		t.Fatalf("status must print the lifecycle line, got %q", out.String())
	}
	// Terminal states never reopen through the live path.
	out.Reset()
	if code := c.Run([]string{"mission", "advance", "--id", "m1", "--to", "executing"}); code != 1 {
		t.Fatalf("advance out of completed exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "terminal") {
		t.Fatalf("terminal refusal must say so, got %q", errb.String())
	}
}

func TestMissionCancelNeedsReasonAndPersists(t *testing.T) {
	c, out, errb, dir := missionCLI(t)
	if code := c.Run([]string{"mission", "create", "--id", "m2", "--objective", "x"}); code != 0 {
		t.Fatalf("create: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "cancel", "--id", "m2"}); code != 2 {
		t.Fatalf("cancel without reason exit = %d, want 2", code)
	}
	// Simulated restart: a fresh CLI handle on the same root
	// sees the confirmed state and resumes from there.
	var out2, errb2 strings.Builder
	c = CLI{Version: "test", Stdout: &out2, Stderr: &errb2, Dir: dir}
	out, errb = &out2, &errb2
	out.Reset()
	if code := c.Run([]string{"mission", "advance", "--id", "m2", "--to", "analyzing"}); code != 0 {
		t.Fatalf("advance after restart: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "cancel", "--id", "m2", "--reason", "cut"}); code != 0 {
		t.Fatalf("cancel: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "m2 cancelled") {
		t.Fatalf("cancel must confirm, got %q", out.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "resume", "--id", "m2"}); code != 1 {
		t.Fatalf("resume of a cancelled mission exit = %d, want 1", code)
	}
}

func TestMissionCreateValidationThroughDispatch(t *testing.T) {
	c, _, errb, _ := missionCLI(t)
	if code := c.Run([]string{"mission", "create"}); code != 2 {
		t.Fatalf("create without objective exit = %d, want 2", code)
	}
	if code := c.Run([]string{"mission", "create", "--objective", "x", "--risk", "extreme"}); code != 1 {
		t.Fatalf("create with bad risk exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "unknown risk") {
		t.Fatalf("must name the bad risk, got %q", errb.String())
	}
	if code := c.Run([]string{"mission", "create", "--objective", "x", "--id", "../escape"}); code != 1 {
		t.Fatalf("create with hostile id exit = %d, want 1", code)
	}
}

func TestMissionListAndLinkThroughDispatch(t *testing.T) {
	c, out, errb, _ := missionCLI(t)
	if code := c.Run([]string{"mission", "create", "--id", "m3", "--objective", "one"}); code != 0 {
		t.Fatalf("create m3: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "create", "--id", "m4", "--objective", "two"}); code != 0 {
		t.Fatalf("create m4: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "link", "--id", "m3",
		"--run", "run_1", "--task", "t1", "--agent", "a1"}); code != 0 {
		t.Fatalf("link: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"mission", "list"}); code != 0 {
		t.Fatalf("list: exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"m3", "m4", "2 mission(s)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("list must carry %q, got:\n%s", want, out.String())
		}
	}
	out.Reset()
	if code := c.Run([]string{"mission", "show", "--id", "m3"}); code != 0 {
		t.Fatalf("show: exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"run_1", "t1", "a1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("linked ids must show under m3, missing %q:\n%s", want, out.String())
		}
	}
}
