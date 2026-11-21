// `bench diag` CLI paths (F11.4/NOV-90): usage errors, seeded
// slow-vs-baseline diagnosis, JSON shape, strict thresholds and the
// Lab evidence-dir path. The store is seeded through
// observability.RecordExec under a temp cwd, exactly like the
// timeline CLI tests.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
)

func diagSeedBase() time.Time {
	return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
}

func diagSeedAt(base time.Time, ms int64) time.Time {
	return base.Add(time.Duration(ms) * time.Millisecond)
}

func diagSeedEvent(t *testing.T, root, run, id, typ, status string, atMS, durMS int64, agent string, payload map[string]any) {
	t.Helper()
	_, _, err := observability.RecordExec(root, observability.ExecEvent{
		RunID: run, EventID: id, Type: typ, Status: status,
		AgentExecutionID: agent, At: diagSeedAt(diagSeedBase(), atMS),
		DurationMS: durMS, Payload: payload,
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// seedDiagSlowRun writes the controlled scenario: run_slow carries a
// 6-call read loop (2000ms each) plus a slow exec tool; run_fast is
// the clean baseline with one fast read.
func seedDiagSlowRun(t *testing.T, root string) {
	t.Helper()
	diagSeedEvent(t, root, "run_slow", "s01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", nil)
	diagSeedEvent(t, root, "run_slow", "s02", observability.ExecAgentStart, observability.ExecStarted, 100, 0, "A1", nil)
	for i := 0; i < 6; i++ {
		id := string(rune('a' + i))
		diagSeedEvent(t, root, "run_slow", "c"+id, observability.ExecToolCall, observability.ExecRunning, 200+int64(i)*2200, 0, "A1", map[string]any{"tool": "read"})
		diagSeedEvent(t, root, "run_slow", "r"+id, observability.ExecToolResult, observability.ExecRunning, 300+int64(i)*2200, 2000, "A1", map[string]any{"tool": "read"})
	}
	diagSeedEvent(t, root, "run_slow", "sx", observability.ExecToolCall, observability.ExecRunning, 14000, 0, "A1", map[string]any{"tool": "exec"})
	diagSeedEvent(t, root, "run_slow", "rx", observability.ExecToolResult, observability.ExecRunning, 14100, 3000, "A1", map[string]any{"tool": "exec"})
	diagSeedEvent(t, root, "run_slow", "sagent", observability.ExecAgentEnd, observability.ExecSucceeded, 20000, 19900, "A1", nil)
	diagSeedEvent(t, root, "run_slow", "send", observability.ExecRunEnd, observability.ExecSucceeded, 25000, 25000, "", nil)
	diagSeedEvent(t, root, "run_fast", "f01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", nil)
	diagSeedEvent(t, root, "run_fast", "f02", observability.ExecToolCall, observability.ExecRunning, 100, 0, "A9", map[string]any{"tool": "read"})
	diagSeedEvent(t, root, "run_fast", "f03", observability.ExecToolResult, observability.ExecRunning, 200, 100, "A9", map[string]any{"tool": "read"})
	diagSeedEvent(t, root, "run_fast", "f04", observability.ExecAgentEnd, observability.ExecSucceeded, 4600, 4500, "A9", nil)
	diagSeedEvent(t, root, "run_fast", "fend", observability.ExecRunEnd, observability.ExecSucceeded, 5000, 4000, "", nil)
}

func TestBenchDiagUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.benchDiagCmd(nil); code != 2 {
		t.Fatalf("no args exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus bench diag") {
		t.Fatalf("stderr must print the byte-identical usage, got %q", errb.String())
	}
	c, _, errb = newCLI(nil)
	if code := c.benchDiagCmd([]string{"--nope"}); code != 2 {
		t.Fatalf("unknown flag exit = %d, want 2", code)
	}
	c, _, errb = newCLI(nil)
	if code := c.benchDiagCmd([]string{"--run", "a", "extra"}); code != 2 {
		t.Fatalf("positional exit = %d, want 2", code)
	}
}

func TestBenchDiagSlowRunVsBaseline(t *testing.T) {
	t.Chdir(t.TempDir())
	seedDiagSlowRun(t, ".")
	c, out, _ := newCLI(nil)
	code := c.benchDiagCmd([]string{"--run", "run_slow,run_fast", "--baseline", "run_fast"})
	if code != 0 {
		t.Fatalf("diag exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{
		"perf-diagnosis perf-diag-v1",
		"dominant stage: tools",
		"dominant agent: A1",
		"wall 25000ms (trace-span) vs aggregate",
		"[loop] read",
		"loop_repeats=5",
		"[slow-tool] read",
		"slow_tool_ms=2000",
		"baseline run_fast vs run_slow: slower",
		"rules: slow_stage_ms=10000",
		"regulus trace timeline --run run_slow --rebuild",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("report misses %q:\n%s", want, got)
		}
	}
}

func TestBenchDiagJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	seedDiagSlowRun(t, ".")
	c, out, _ := newCLI(nil)
	if code := c.benchDiagCmd([]string{"--run", "run_slow", "--json"}); code != 0 {
		t.Fatalf("diag --json exit = %d, want 0", code)
	}
	var rep struct {
		Version    string `json:"version"`
		Thresholds struct {
			SlowToolMS int64 `json:"slow_tool_ms"`
		} `json:"thresholds"`
		Runs []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatalf("diag --json must parse: %v\n%s", err, out.String())
	}
	if rep.Version != bench.DiagVersion || len(rep.Runs) != 1 || rep.Runs[0].RunID != "run_slow" {
		t.Fatalf("json shape wrong: %+v", rep)
	}
}

func TestBenchDiagThresholdsStrict(t *testing.T) {
	t.Chdir(t.TempDir())
	seedDiagSlowRun(t, ".")
	th := filepath.Join(t.TempDir(), "th.json")
	if err := os.WriteFile(th, []byte(`{"slow_stage_ms": 999999999, "bogus": 1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.benchDiagCmd([]string{"--run", "run_slow", "--thresholds", th}); code != 1 {
		t.Fatalf("unknown threshold field exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench diag:") {
		t.Fatalf("refusal must name bench diag, got %q", errb.String())
	}
	if err := os.WriteFile(th, []byte(`{"slow_stage_ms": 999999999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.benchDiagCmd([]string{"--run", "run_slow", "--thresholds", th}); code != 0 {
		t.Fatalf("custom thresholds exit = %d, want 0", code)
	}
	if strings.Contains(out.String(), "[slow-stage]") {
		t.Fatalf("raised stage rule must silence slow-stage alerts:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "slow_stage_ms=999999999") {
		t.Fatalf("report must echo the configured rules:\n%s", out.String())
	}
}

func TestBenchDiagEvidenceDir(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(t.TempDir(), "ev")
	outcomes := []bench.AgentOutcome{
		{TaskID: "LAB1", Split: "exploration", Status: bench.AgentPass, LatencyMS: 4500},
	}
	m := bench.AgentRunManifest{SuiteVersion: bench.AgentSuiteVersion, Model: "m", Host: "opencode", Variant: "lab", Reps: 1, Split: "exploration", Seeds: []int64{11}, GitHead: "abc", StartedAt: "2026-11-16T00:00:00Z"}
	if _, err := bench.WriteAgentEvidence(dir, outcomes, m); err != nil {
		t.Fatal(err)
	}
	// Minimal Lab pair: config + reports name the run; the store has
	// no events for it, so the diagnosis runs on Lab data with a note.
	cfg := bench.LabConfig{LabVersion: bench.LabVersion, SuiteVersion: bench.AgentSuiteVersion, Mode: bench.LabModeSingle, Agent: "baseline", Task: "LAB1", Seed: 11}
	results := []bench.LabResult{
		{RunID: "run_lab1", AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 4500, Verdict: bench.LabPass, Outcome: outcomes[0]},
	}
	rawCfg, _ := json.MarshalIndent(cfg, "", "  ")
	rawRep, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, bench.LabConfigFile), append(rawCfg, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bench.LabReportsFile), append(rawRep, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.benchDiagCmd([]string{"--evidence-dir", dir}); code != 0 {
		t.Fatalf("diag --evidence-dir exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "run_lab1") || !strings.Contains(out.String(), "4500ms") {
		t.Fatalf("Lab durations must drive the diagnosis:\n%s", out.String())
	}
}

func TestBenchDiagNothingToDiagnose(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.benchDiagCmd([]string{"--run", "run_ghost"}); code != 1 {
		t.Fatalf("unknown run exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "nothing to diagnose") {
		t.Fatalf("refusal must say nothing to diagnose, got %q", errb.String())
	}
}
