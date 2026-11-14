// NOV-62: real host usage end to end — record with host flags,
// cost attributes it by task with a labeled source instead of
// "no entries attributed".
package cli

import (
	"strings"
	"testing"
)

func TestTraceRecordHostFlagsAttributed(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s9", "--message", "m9", "--call", "c9",
		"--tool", "read", "--status", "completed", "--task", "agent/T9",
		"--input-tokens", "200", "--output-tokens", "40", "--cache-read-tokens", "15",
		"--model", "test-model", "--host", "opencode", "--host-version", "9.9"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("record exit = %d, stderr %q", code, errb.String())
	}
	c2, out2, errb2 := newCLI(nil)
	if code := c2.Run([]string{"trace", "cost", "--task", "agent/T9"}); code != 0 {
		t.Fatalf("cost exit = %d, stderr %q", code, errb2.String())
	}
	got := out2.String()
	for _, want := range []string{"agent/T9", "200 in + 40 out (measured-host)", "cache 15 read", "1 session(s)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cost output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no entries attributed") {
		t.Fatalf("measured task must attribute entries:\n%s", got)
	}
}

func TestTraceRecordBadUsageSourceRefused(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	args := []string{"trace", "record", "--session", "s", "--message", "m", "--call", "c",
		"--tool", "t", "--status", "completed", "--usage-source", "trust-me"}
	if code := c.Run(args); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "usage_source") {
		t.Fatalf("stderr must name the refusal, got %q", errb.String())
	}
}
