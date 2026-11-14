// NOV-68: CLI paths. Unknown host degrades to NOT RUN (exit 3,
// never a false pass); the real host without usable credits lands
// INCONCLUSIVE (exit 1), with every outcome bucketed as infra.
package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeAgentSplits(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sub := filepath.Join(dir, "bench")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sub, "agent-splits-v1.json")
	body := `{"version":"agent-splits-v1","seed":7,"exploration":["AT1","AT2","AT3","AT4"],"holdout":["AT5"]}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBenchAgentTasksUnknownHostNotRun(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-tasks", "--host", "bogus"}); code != 3 {
		t.Fatalf("exit = %d, want 3 NOT RUN", code)
	}
	if !strings.Contains(out.String(), "NOT RUN") {
		t.Fatalf("must say NOT RUN, got %q", out.String())
	}
}

func TestBenchAgentTasksNoUsableHostInconclusive(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
	// Fake headless host: fails like a quota wall so every outcome
	// lands infra and the suite reports INCONCLUSIVE, never a pass.
	fake := writeFakeOpenCode(t)
	t.Setenv("PATH", filepath.Dir(fake)+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, out, _ := newCLI(nil)
	// The host exists but cannot run, so the suite must refuse to conclude.
	code := c.Run([]string{"bench", "agent-tasks", "--reps", "1", "--split", "holdout", "--budget-s", "60"})
	if code != 1 {
		t.Fatalf("exit = %d, want 1 INCONCLUSIVE (got %q)", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "INCONCLUSIVE") {
		t.Fatalf("must say INCONCLUSIVE, got %q", got)
	}
	if strings.Contains(got, " 1 pass") {
		t.Fatalf("infra-only run must not report passes, got %q", got)
	}
}

func writeFakeOpenCode(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "opencode.cmd")
		body := "@echo off\r\necho {\"type\":\"error\",\"error\":{\"message\":\"Insufficient balance, 402 quota\"}}\r\nexit /b 1\r\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "opencode")
	body := "#!/usr/bin/env bash\necho '{\"type\":\"error\",\"error\":{\"message\":\"Insufficient balance, 402 quota\"}}'\nexit 1\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
