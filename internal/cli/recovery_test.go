// CLI tests for F12.5/NOV-100: the recovery command family
// records, resumes, retries, reports, rolls back and cancels
// through the durable store, refusing usage errors with exit 2.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func recoveryCLI(t *testing.T) (CLI, *strings.Builder, *strings.Builder, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	var out, errb strings.Builder
	c := CLI{Version: "test", Stdout: &out, Stderr: &errb, Dir: dir}
	return c, &out, &errb, dir
}

func TestRecoveryHelp(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"recovery", "--help"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus recovery") {
		t.Fatalf("stdout = %q", out.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"recovery", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand exit = %d, want 2", code)
	}
}

func TestRecoveryCheckpointResumeStatusRoundTrip(t *testing.T) {
	c, out, errb, _ := recoveryCLI(t)
	argv := []string{"recovery", "checkpoint", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--state", "running", "--host", "h1",
		"--scope", "memory:read", "--require-cap", "code-review",
		"--input", "goal=ship", "--compensate", "e1=revert patch"}
	if code := c.Run(argv); code != 0 {
		t.Fatalf("checkpoint: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "key=r1/t1/a1#0") {
		t.Fatalf("checkpoint must print the idempotency key, got %q", out.String())
	}
	out.Reset()
	if code := c.Run([]string{"recovery", "checkpoint", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--state", "checkpointed"}); code != 0 {
		t.Fatalf("second checkpoint: exit = %d stderr = %q", code, errb.String())
	}
	out.Reset()
	if code := c.Run([]string{"recovery", "resume", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--effect", "e1,e2"}); code != 0 {
		t.Fatalf("resume: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "key=r1/t1/a1#0") {
		t.Fatalf("resume must keep the attempt key, got %q", out.String())
	}
	out.Reset()
	if code := c.Run([]string{"recovery", "status", "--run", "r1"}); code != 0 {
		t.Fatalf("status: exit = %d stderr = %q", code, errb.String())
	}
	for _, want := range []string{"running", "memory:read", "code-review"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status must show %q, got:\n%s", want, out.String())
		}
	}
}

func TestRecoveryScopeWideningRefused(t *testing.T) {
	// Unknown scope names are dropped, never granted (closed world):
	// recording admin:all must refuse as widening.
	c, _, errb, _ := recoveryCLI(t)
	code := c.Run([]string{"recovery", "checkpoint", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--state", "running", "--scope", "memory:read,admin:all"})
	if code != 1 {
		t.Fatalf("scope widening exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "widens") {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestRecoveryRetryCancelRollback(t *testing.T) {
	c, out, errb, _ := recoveryCLI(t)
	must := func(argv []string) {
		t.Helper()
		if code := c.Run(argv); code != 0 {
			t.Fatalf("%v: exit = %d stderr = %q", argv, code, errb.String())
		}
	}
	must([]string{"recovery", "checkpoint", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--state", "running"})
	out.Reset()
	if code := c.Run([]string{"recovery", "retry", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--error", "connection refused"}); code != 0 {
		t.Fatalf("retry: exit = %d stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "class=network") || !strings.Contains(out.String(), "backoff=") {
		t.Fatalf("retry must classify and quote backoff, got %q", out.String())
	}
	out.Reset()
	if code := c.Run([]string{"recovery", "retry", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--class", "permanent"}); code != 1 {
		t.Fatalf("permanent retry exit = %d, want 1", code)
	}
	out.Reset()
	must([]string{"recovery", "checkpoint", "--run", "r1", "--task", "t1",
		"--agent", "a1", "--state", "failed"})
	must([]string{"recovery", "rollback", "--run", "r1", "--task", "t1", "--agent", "a1"})
	must([]string{"recovery", "checkpoint", "--run", "r1", "--task", "t2",
		"--agent", "a1", "--state", "running"})
	must([]string{"recovery", "cancel", "--run", "r1", "--reason", "done here"})
	out.Reset()
	if code := c.Run([]string{"recovery", "resume", "--run", "r1", "--task", "t1",
		"--agent", "a1"}); code != 1 {
		t.Fatalf("resume after cancel exit = %d, want 1", code)
	}
	out.Reset()
	if code := c.Run([]string{"recovery", "status", "--run", "r1", "--json"}); code != 0 {
		t.Fatalf("status --json: exit = %d", code)
	}
	if !strings.Contains(out.String(), "rolled_back") || !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("final states must be audited, got:\n%s", out.String())
	}
}

func TestRecoveryUsageErrors(t *testing.T) {
	c, _, _, _ := recoveryCLI(t)
	for _, argv := range [][]string{
		{"recovery", "checkpoint", "--run", "r1"},
		{"recovery", "resume", "--run", "r1", "--task", "t1"},
		{"recovery", "retry", "--run", "r1"},
		{"recovery", "status"},
		{"recovery", "rollback", "--run", "r1", "--task", "t1"},
		{"recovery", "cancel", "--run", "r1"},
	} {
		if code := c.Run(argv); code != 2 {
			t.Fatalf("%v: exit = %d, want 2", argv, code)
		}
	}
	// --help must not create any store.
	if entries, err := os.ReadDir(filepath.Join(".regulus")); err == nil && len(entries) != 0 {
		t.Fatalf("--help created a store: %v", entries)
	}
}
