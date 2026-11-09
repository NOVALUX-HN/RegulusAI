package cli

// F1 `trace cost`: per-task proxy cost folded auditably from disk.

import (
	"strings"
	"testing"
)

func TestTraceRecordCostFlagsRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s1", "--message", "m1", "--call", "c1",
		"--tool", "read", "--status", "completed",
		"--task", "macizo-flow/F1", "--duration-ms", "120", "--output-bytes", "2048",
		"--input-tokens", "100", "--output-tokens", "50"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "trace recorded") {
		t.Fatalf("record output = %q", out.String())
	}
	c2, out2, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost"}); code != 0 {
		t.Fatalf("cost exit = %d, stderr %q", code, errb2.String())
	}
	got := out2.String()
	for _, want := range []string{"macizo-flow/F1", "1 call(s)", "120ms", "2048B", "100 in + 50 out", "1 session(s)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cost output missing %q:\n%s", want, got)
		}
	}
}

func TestTraceRecordNegativeCostRefused(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s", "--message", "m", "--call", "c",
		"--tool", "t", "--status", "completed", "--duration-ms", "-5"}
	if code := c.Run(args); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "must not be negative") {
		t.Fatalf("stderr must name the refusal, got %q", errb.String())
	}
}

func TestTraceCostGroupsTasksAndMarksUnknown(t *testing.T) {
	t.Chdir(t.TempDir())
	record := func(session, call, tool, status, task string, dur, bytes int64) {
		t.Helper()
		c, _, errb := newCLI(nil)
		args := []string{"trace", "record", "--session", session, "--message", "m" + call,
			"--call", call, "--tool", tool, "--status", status,
			"--duration-ms", itoa(dur), "--output-bytes", itoa(bytes)}
		if task != "" {
			args = append(args, "--task", task)
		}
		if code := c.Run(args); code != 0 {
			t.Fatalf("record %s: exit = %d, stderr %q", call, code, errb.String())
		}
	}
	record("s1", "c1", "read", "completed", "F1", 100, 1000)
	record("s1", "c2", "write", "error", "F1", 0, 0)
	record("s2", "c3", "bash", "completed", "", 0, 0)

	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "cost"}); code != 0 {
		t.Fatalf("cost exit = %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "task F1: 2 call(s), 1 error(s)") {
		t.Fatalf("F1 line wrong:\n%s", got)
	}
	if !strings.Contains(got, "100ms") || !strings.Contains(got, "1000B") {
		t.Fatalf("F1 proxy totals wrong:\n%s", got)
	}
	if !strings.Contains(got, "tokens unknown (host exposes none)") {
		t.Fatalf("tokenless report must read unknown:\n%s", got)
	}
	if !strings.Contains(got, "proxy tokens (chars/4, measured-local)") {
		t.Fatalf("report must carry the measured-local proxy alongside unknown host tokens:\n%s", got)
	}
	if !strings.Contains(got, "task (unattributed): 1 call(s)") {
		t.Fatalf("legacy bucket missing:\n%s", got)
	}
	if !strings.Contains(got, "unknown ms (unmeasured)") {
		t.Fatalf("unmeasured proxy must read unknown, never bare zero:\n%s", got)
	}

	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "--task", "F1"}); code != 0 {
		t.Fatalf("scoped cost exit = %d", code)
	}
	if strings.Contains(out2.String(), "(unattributed)") {
		t.Fatalf("--task F1 leaked other buckets:\n%s", out2.String())
	}
	if !strings.Contains(out2.String(), "task F1:") {
		t.Fatalf("--task F1 missing its bucket:\n%s", out2.String())
	}

	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"trace", "cost", "--task", "ghost"}); code != 0 {
		t.Fatalf("ghost cost exit = %d", code)
	}
	if !strings.Contains(out3.String(), "no trace cost for task ghost") {
		t.Fatalf("ghost output = %q", out3.String())
	}
}

func TestTraceCostEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"trace", "cost"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "(no trace costs: no sessions traced)") {
		t.Fatalf("output = %q", out.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "extra"}); code != 2 {
		t.Fatalf("stray positional: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "usage: regulus trace cost") {
		t.Fatalf("stderr = %q", errb2.String())
	}
}

func TestTraceUnknownCostSubcommand(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"trace", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("stderr = %q", errb.String())
	}
	if !strings.Contains(errb.String(), "trace cost") {
		t.Fatalf("trace usage must advertise cost, got %q", errb.String())
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
