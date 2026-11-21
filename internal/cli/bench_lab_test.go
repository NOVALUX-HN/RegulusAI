// NOV-88: Agent Lab CLI paths. Unknown agents/tasks and mixed
// --config selections are usage errors; an unusable host degrades to
// NOT RUN (never a verdict on work never done); a fake headless host
// pins the fail/tie/pass verdict paths hermetically.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

// writeFakeOpenCodeOK is a headless host that runs and exits 0 without
// touching the fixture: checks fail, the verdict is fail, exit 0.
func writeFakeOpenCodeOK(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "opencode.cmd")
		body := "@echo off\r\necho {}\r\nexit /b 0\r\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "opencode")
	body := "#!/usr/bin/env bash\necho '{}'\nexit 0\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeFakeOpenCodeFixAT4 fixes the AT4 shell fixture from the
// fixture cwd, so the anchored prompt path ends in pass.
func writeFakeOpenCodeFixAT4(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "opencode.cmd")
		body := "@echo off\r\necho sort nums.txt> sortnums.sh\r\necho {}\r\nexit /b 0\r\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := filepath.Join(dir, "opencode")
	body := "#!/usr/bin/env bash\nprintf '#!/usr/bin/env bash\\nsort nums.txt\\n' > sortnums.sh\necho '{}'\nexit 0\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func labTestEnv(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
}

func TestBenchLabAgentsListsRegistry(t *testing.T) {
	labTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "lab", "agents"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"baseline", "anchored", "verify-first", "opencode"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents must list %q:\n%s", want, got)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "lab", "agents", "--json"}); code != 0 {
		t.Fatalf("json exit = %d, want 0", code)
	}
	var reg []bench.LabAgent
	if err := json.Unmarshal([]byte(out2.String()), &reg); err != nil {
		t.Fatalf("agents --json must parse: %v\n%s", err, out2.String())
	}
	if len(reg) < 2 {
		t.Fatalf("registry holds %d agents, want at least 2", len(reg))
	}
}

func TestBenchLabRejectsUsage(t *testing.T) {
	labTestEnv(t)
	cases := [][]string{
		{"bench", "lab"},
		{"bench", "lab", "bogus"},
		{"bench", "lab", "run"},
		{"bench", "lab", "run", "--agent", "baseline"},
		{"bench", "lab", "run", "--agent", "ghost", "--task", "AT4"},
		{"bench", "lab", "run", "--agent", "baseline", "--task", "NOPE"},
		{"bench", "lab", "run", "--agent", "baseline", "--task", "AT4", "--retries", "9"},
		{"bench", "lab", "compare", "--agents", "baseline", "--task", "AT4"},
		{"bench", "lab", "compare", "--agents", "baseline,baseline", "--task", "AT4"},
		{"bench", "lab", "compare", "--agents", "baseline,ghost", "--task", "AT4"},
		{"bench", "lab", "report"},
		{"bench", "lab", "report", "--evidence-dir", "x", "extra"},
	}
	for _, args := range cases {
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("%v exit = %d, want 2", args, code)
		}
	}
}

func TestBenchLabConfigRefusesMixedFlags(t *testing.T) {
	labTestEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "lab.json")
	// Build a valid config through the bench API, then mix it.
	cfg := bench.LabConfig{Mode: bench.LabModeSingle, Agent: "anchored", Task: "AT4", Seed: 7}
	if err := bench.ExportLabConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "lab", "run", "--config", cfgPath, "--task", "AT4"}); code != 2 {
		t.Fatalf("mixed --config exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "--config replays exactly") {
		t.Fatalf("mixed flags must name the replay contract, got %q", errb.String())
	}
	c2, _, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "lab", "run", "--config", filepath.Join(dir, "missing.json")}); code != 1 {
		t.Fatalf("missing config exit = %d, want 1", code)
	}
}

func TestBenchLabUnknownHostNotRun(t *testing.T) {
	labTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "lab", "run", "--agent", "baseline", "--task", "AT4", "--host", "bogus"}); code != 3 {
		t.Fatalf("exit = %d, want 3 NOT RUN", code)
	}
	if !strings.Contains(out.String(), "NOT RUN") {
		t.Fatalf("must say NOT RUN, got %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "lab", "compare", "--agents", "baseline,anchored", "--task", "AT4", "--host", "bogus"}); code != 3 {
		t.Fatalf("compare exit = %d, want 3 NOT RUN", code)
	}
	if !strings.Contains(out2.String(), "NOT RUN") {
		t.Fatalf("compare must say NOT RUN, got %q", out2.String())
	}
}

func TestBenchLabRunFailVerdict(t *testing.T) {
	labTestEnv(t)
	fake := writeFakeOpenCodeOK(t)
	t.Setenv("PATH", filepath.Dir(fake)+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	ev := filepath.Join(dir, "ev")
	exp := filepath.Join(dir, "lab.json")
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"bench", "lab", "run", "--agent", "baseline", "--task", "AT4",
		"--evidence-dir", ev, "--export", exp})
	if code != 0 {
		t.Fatalf("fail verdict exit = %d, want 0 (fail concludes)\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"lab baseline/AT4", "fail", "trace: regulus trace timeline --run run_", "evidence recorded"} {
		if !strings.Contains(got, want) {
			t.Fatalf("run must print %q:\n%s", want, got)
		}
	}
	for _, f := range []string{bench.EvidenceOutcomesFile, bench.EvidenceManifestFile, bench.LabConfigFile, bench.LabReportsFile} {
		if _, err := os.Stat(filepath.Join(ev, f)); err != nil {
			t.Fatalf("evidence must hold %s: %v", f, err)
		}
	}
	// Replay the exported config: same task, same verdict.
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "lab", "run", "--config", exp}); code != 0 {
		t.Fatalf("replay exit = %d, want 0\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "lab baseline/AT4") || !strings.Contains(out2.String(), "fail") {
		t.Fatalf("replay must repeat the verdict:\n%s", out2.String())
	}
}

func TestBenchLabCompareTiePass(t *testing.T) {
	labTestEnv(t)
	fake := writeFakeOpenCodeFixAT4(t)
	t.Setenv("PATH", filepath.Dir(fake)+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"bench", "lab", "compare", "--agents", "baseline,anchored", "--task", "AT4"})
	if code != 0 {
		t.Fatalf("tie exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"winner tie", "baseline: pass", "anchored: pass", "trace baseline:", "trace anchored:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("compare must print %q:\n%s", want, got)
		}
	}
}

func TestBenchLabFlowPrintsTimeline(t *testing.T) {
	labTestEnv(t)
	fake := writeFakeOpenCodeOK(t)
	t.Setenv("PATH", filepath.Dir(fake)+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"bench", "lab", "flow", "--agent", "anchored", "--task", "AT4"})
	if code != 0 {
		t.Fatalf("flow exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"lab anchored/AT4", "trace timeline", "status failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("flow must print %q:\n%s", want, got)
		}
	}
}

func TestBenchLabReportReadsEvidence(t *testing.T) {
	labTestEnv(t)
	dir := t.TempDir()
	if _, err := bench.WriteLabEvidence(dir,
		bench.LabConfig{Mode: bench.LabModeSingle, Agent: "baseline", Task: "AT4", Seed: 7},
		[]bench.LabResult{{
			AgentID: "baseline", Verdict: bench.LabFail, RunID: "run_x",
			SessionID: "ses_x", TraceHint: "trace hint x",
			Instructions: "Create it.",
			Outcome:      bench.AgentOutcome{TaskID: "AT4", Split: "exploration", Status: bench.AgentFail, Detail: "nope"},
		}}, "abc", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "lab", "report", "--evidence-dir", dir}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, want := range []string{"agent-lab", "baseline: fail", "trace hint x", "Create it."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report misses %q:\n%s", want, out.String())
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, bench.EvidenceOutcomesFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, bench.EvidenceOutcomesFile), append(raw, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	c2, _, errb := newCLI(nil)
	if code := c2.Run([]string{"bench", "lab", "report", "--evidence-dir", dir}); code != 1 {
		t.Fatalf("tampered exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "sha256") {
		t.Fatalf("tamper must name the sha256 mismatch, got %q", errb.String())
	}
}
