package cli

// F11.1/NOV-87: `trace emit|timeline|prune-exec` — record versioned
// execution events, query the timeline by run/task/agent, and
// reconstruct after a crash.

import (
	"strings"
	"testing"
)

func TestTraceEmitRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "emit", "--run", "run-1", "--type", "run.start", "--task", "task-1"}); code != 0 {
		t.Fatalf("emit: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "exec recorded evt_") {
		t.Fatalf("emit stdout = %q, want exec recorded evt_...", out.String())
	}
	c, out, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-1"}); code != 0 {
		t.Fatalf("timeline: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "run.start started") || !strings.Contains(out.String(), "run=run-1") {
		t.Fatalf("timeline stdout = %q, want the recorded event", out.String())
	}
}

func TestTraceEmitDuplicate(t *testing.T) {
	t.Chdir(t.TempDir())
	args := []string{"trace", "emit", "--run", "run-1", "--type", "run.start", "--event-id", "evt_dup_cli"}
	c, _, _ := newCLI(nil)
	if code := c.Run(args); code != 0 {
		t.Fatalf("first emit: exit = %d, want 0", code)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run(args); code != 0 {
		t.Fatalf("duplicate emit: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "already recorded (duplicate ignored)") {
		t.Fatalf("duplicate emit stdout = %q", out.String())
	}
	c, out, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-1"}); code != 0 {
		t.Fatalf("timeline: exit = %d, want 0", code)
	}
	if strings.Count(out.String(), "run.start") != 1 {
		t.Fatalf("duplicate must store once, got:\n%s", out.String())
	}
}

func TestTraceTimelineFilters(t *testing.T) {
	t.Chdir(t.TempDir())
	emit := func(run, typ, task, agent string) {
		c, _, _ := newCLI(nil)
		args := []string{"trace", "emit", "--run", run, "--type", typ}
		if task != "" {
			args = append(args, "--task", task)
		}
		if agent != "" {
			args = append(args, "--agent", agent)
		}
		if code := c.Run(args); code != 0 {
			t.Fatalf("emit %v: exit = %d", args, code)
		}
	}
	emit("run-1", "run.start", "task-A", "agent-1")
	emit("run-1", "tool.call", "task-A", "agent-2")
	emit("run-2", "run.start", "task-B", "agent-1")
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--task", "task-A"}); code != 0 {
		t.Fatalf("timeline --task: exit = %d, want 0", code)
	}
	if strings.Count(out.String(), "run=run-1") != 2 || strings.Contains(out.String(), "run-2") {
		t.Fatalf("timeline --task must show only task-A events, got:\n%s", out.String())
	}
	c, out, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--agent", "agent-1", "--json"}); code != 0 {
		t.Fatalf("timeline --agent --json: exit = %d, want 0", code)
	}
	if strings.Count(out.String(), "agent-1") != 2 || strings.Contains(out.String(), "agent-2") {
		t.Fatalf("timeline --agent must show only agent-1 events, got:\n%s", out.String())
	}
}

func TestTraceTimelineRebuildAfterCrash(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"trace", "emit", "--run", "run-crash", "--type", "run.start", "--status", "started"},
		{"trace", "emit", "--run", "run-crash", "--type", "delegation.request", "--status", "running",
			"--agent", "agent_parent", "--delegation", "dlg-9", "--payload", `{"to":"agent_child"}`},
	} {
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 0 {
			t.Fatalf("emit %v: exit = %d", args, code)
		}
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-crash", "--rebuild"}); code != 0 {
		t.Fatalf("timeline --rebuild: exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"status crashed", "crashed: started without a terminal run.end", "delegation dlg-9: agent_parent -> agent_child (pending)"} {
		if !strings.Contains(got, want) {
			t.Errorf("timeline --rebuild must contain %q, got:\n%s", want, got)
		}
	}
	// Close the run: rebuild now reports the terminal verdict.
	c, _, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "emit", "--run", "run-crash", "--type", "run.end", "--status", "succeeded"}); code != 0 {
		t.Fatalf("emit run.end: exit = %d, want 0", code)
	}
	c, out, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-crash", "--rebuild"}); code != 0 {
		t.Fatalf("timeline --rebuild: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "status succeeded") || strings.Contains(out.String(), "crashed") {
		t.Fatalf("closed run must report succeeded, got:\n%s", out.String())
	}
}

func TestTraceTimelineEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-missing"}); code != 0 {
		t.Fatalf("timeline: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no execution events match)") {
		t.Fatalf("empty timeline stdout = %q", out.String())
	}
}

func TestTraceEmitValidation(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"trace", "emit", "--type", "run.start"},
		{"trace", "emit", "--run", "run-1"},
		{"trace", "emit", "--run", "run-1", "--type", "bogus"},
		{"trace", "emit", "--run", "run-1", "--type", "run.start", "--status", "bogus"},
		{"trace", "emit", "--run", "run-1", "--type", "run.start", "--payload", "{bad"},
	} {
		c, _, errb := newCLI(nil)
		if code := c.Run(args); code == 0 {
			t.Errorf("emit %v: exit = 0, want non-zero", args)
		} else if code != 1 && code != 2 {
			t.Errorf("emit %v: exit = %d, want 1 or 2", args, code)
		}
		if errb.String() == "" {
			t.Errorf("emit %v: must explain the refusal on stderr", args)
		}
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "timeline"}); code != 2 {
		t.Errorf("timeline without selector: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus trace timeline") {
		t.Errorf("timeline without selector stderr = %q", errb.String())
	}
}

func TestTracePruneExec(t *testing.T) {
	t.Chdir(t.TempDir())
	for i, typ := range []string{"run.start", "tool.call", "tool.result"} {
		c, _, _ := newCLI(nil)
		_ = i
		if code := c.Run([]string{"trace", "emit", "--run", "run-1", "--type", typ}); code != 0 {
			t.Fatalf("emit: exit = %d, want 0", code)
		}
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "prune-exec", "--keep", "1"}); code != 0 {
		t.Fatalf("prune-exec: exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "exec log: 1 event(s) kept") {
		t.Fatalf("prune-exec stdout = %q", out.String())
	}
	c, out, _ = newCLI(nil)
	if code := c.Run([]string{"trace", "timeline", "--run", "run-1"}); code != 0 {
		t.Fatalf("timeline: exit = %d, want 0", code)
	}
	if strings.Count(out.String(), "run=run-1") != 1 || !strings.Contains(out.String(), "tool.result") {
		t.Fatalf("prune must keep the last event, got:\n%s", out.String())
	}
}

func TestTraceTimelineHelp(t *testing.T) {
	for _, args := range [][]string{
		{"trace", "emit", "--help"},
		{"trace", "timeline", "--help"},
		{"trace", "prune-exec", "--help"},
	} {
		c, out, _ := newCLI(nil)
		if code := c.Run(args); code != 0 {
			t.Fatalf("%v: exit = %d, want 0", args, code)
		}
		if !strings.Contains(out.String(), "usage: regulus trace") {
			t.Fatalf("%v stdout = %q", args, out.String())
		}
	}
}
