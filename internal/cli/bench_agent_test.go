// NOV-68: CLI paths. Unknown host degrades to NOT RUN (exit 3,
// never a false pass); the real host without usable credits lands
// INCONCLUSIVE (exit 1), with every outcome bucketed as infra.
package cli

import (
	"os"
	"path/filepath"
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
	c, out, _ := newCLI(nil)
	// Real opencode binary, no model credits: quota fails fast, every
	// outcome lands infra, suite reports INCONCLUSIVE, never a pass.
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
