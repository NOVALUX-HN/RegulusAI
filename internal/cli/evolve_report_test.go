package cli

// T5 (pro-harness): `evolve report` renders lesson economics —
// proxy-token price plus evidence state per admitted lesson, totals,
// and retire lines for rot. Read-only: it proposes nothing, advances
// nothing, judges nothing.
import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

func TestEvolveReportUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"evolve", "report", "extra"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve report") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestEvolveReportEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "report"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"admitted: 0", "proxy tokens: 0", "rot: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("empty report must show %q, got %q", want, got)
		}
	}
}

// T10 (speed-tokens): measured-local task costs print on stdout next to
// lesson price, so pruning reads measured cost without --save.
func TestEvolveReportPrintsMeasuredTaskCosts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for i, tool := range []string{"read", "bash"} {
		if _, err := trace.Record(dir, trace.Entry{Session: "s1", Message: "m", Call: "c", Tool: tool, Status: "completed", Files: []string{"x.go"}, Task: "t1"}, time.Now().Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"evolve", "report"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"t1", "2 calls", "proxy tokens (measured-local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("report must print measured cost %q, got:\n%s", want, got)
		}
	}
}
