// NOV-71: agent-compare wiring. The robust verdict, not prose,
// decides adopt vs keep; ties keep the baseline.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func writeOutcomes(t *testing.T, dir, name string, outs []bench.AgentOutcome) string {
	t.Helper()
	raw, err := json.Marshal(outs)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func agentOutcome(id, status string) bench.AgentOutcome {
	return bench.AgentOutcome{TaskID: id, Split: "exploration", Rep: 0, Seed: 7, Variant: "v", Status: status}
}

func TestBenchAgentCompareAdoptsImprovement(t *testing.T) {
	dir := t.TempDir()
	base := writeOutcomes(t, dir, "base.json", []bench.AgentOutcome{
		agentOutcome("AT1", bench.AgentPass), agentOutcome("AT2", bench.AgentFail),
	})
	cand := writeOutcomes(t, dir, "cand.json", []bench.AgentOutcome{
		agentOutcome("AT1", bench.AgentPass), agentOutcome("AT2", bench.AgentPass),
	})
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-compare", "--base", base, "--candidate", cand}); code != 0 {
		t.Fatalf("improvement must adopt, exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "ADOPT_CANDIDATE") {
		t.Fatalf("verdict must be explicit, got %q", out.String())
	}
}

func TestBenchAgentCompareRejectsRegression(t *testing.T) {
	dir := t.TempDir()
	base := writeOutcomes(t, dir, "base.json", []bench.AgentOutcome{
		agentOutcome("AT1", bench.AgentPass), agentOutcome("AT2", bench.AgentPass),
	})
	cand := writeOutcomes(t, dir, "cand.json", []bench.AgentOutcome{
		agentOutcome("AT1", bench.AgentPass), agentOutcome("AT2", bench.AgentFail),
	})
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-compare", "--base", base, "--candidate", cand}); code == 0 {
		t.Fatal("regression must not adopt")
	}
	if !strings.Contains(out.String(), "KEEP_BASELINE") {
		t.Fatalf("verdict must name the keep, got %q", out.String())
	}
}

func TestBenchAgentCompareNeedsBothSides(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-compare", "--base", "x"}); code != 2 {
		t.Fatalf("missing candidate = usage error, got %d (%s)", code, errb.String())
	}
}
