// Thin deterministic tests for BenchLabRunCmd: every early refusal plus
// stubbed success paths. Portable: t.TempDir + fakes only, no heavy lab,
// no shell, no network. The stubs replace the benchLab* seams so the
// real opencode host is never touched.
package cmdx

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/trace"
)

// stubLabRunner stands in for a headless host: the stubbed benchLabRunFunc
// never calls it, it only proves runner resolution succeeded.
type stubLabRunner struct{}

func (stubLabRunner) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	return "", trace.HostUsage{}, nil
}

// labFailRunner stands in for git: evidence falls back to "unknown"
// instead of spawning a shell.
type labFailRunner struct{}

func (labFailRunner) Run(ctx context.Context, command string) (string, string, int, error) {
	return "", "", 1, errors.New("no git in lab tests")
}

func labTestIO(dir string) (IO, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	return New(&out, &errb, dir, "test", labFailRunner{}), &out, &errb
}

func stubLabRunOK(t *testing.T, res bench.LabResult) {
	t.Helper()
	oldRun, oldResolve, oldProbe := benchLabRunFunc, benchLabResolveRunner, benchLabProbeFunc
	benchLabResolveRunner = func(host string) (bench.HeadlessAgent, string, int, bool) {
		return stubLabRunner{}, "", 0, true
	}
	benchLabRunFunc = func(context.Context, bench.HeadlessAgent, bench.LabAgent, bench.AgentTask, string, bench.LabConfig, string) bench.LabResult {
		return res
	}
	benchLabProbeFunc = func() string { return "pv" }
	t.Cleanup(func() {
		benchLabRunFunc, benchLabResolveRunner, benchLabProbeFunc = oldRun, oldResolve, oldProbe
	})
}

func passLabResult() bench.LabResult {
	return bench.LabResult{
		AgentID: "baseline", TaskID: "AT1", Split: "exploration",
		RunID: "r1", AgentExecutionID: "a1", SessionID: "s1",
		Verdict: bench.LabPass, DurationMS: 7,
		TraceHint: "trace-hint",
		Outcome:   bench.AgentOutcome{Host: "opencode"},
	}
}

func TestBenchLabRunParseFail(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, errb := labTestIO(dir)
	if code := io.BenchLabRunCmd(bench.LabModeSingle, []string{"--nope"}); code != 2 {
		t.Fatalf("bad flag exit = %d, want 2", code)
	}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, []string{"--agent", "baseline", "--task", "AT1", "extra"}); code != 2 {
		t.Fatalf("positional exit = %d, want 2", code)
	}
	if errb.Len() == 0 {
		t.Fatal("parse failures must explain themselves on stderr")
	}
}

func TestBenchLabRunSplitsFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(dir, "missing-splits.json"))
	io, _, errb := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 1 {
		t.Fatalf("splits fail exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench agent-tasks:") {
		t.Fatalf("splits failure must keep the agent-tasks prefix, got %q", errb.String())
	}
}

func TestBenchLabRunConfigFail(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, _ := labTestIO(dir)
	// Selection flags alongside --config refuse with exit 2.
	cfg := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config", cfg, "--agent", "baseline", "--task", "AT1"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 2 {
		t.Fatalf("config+selection exit = %d, want 2", code)
	}
	// Missing config file refuses with exit 1.
	args = []string{"--config", filepath.Join(dir, "nope.json")}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 1 {
		t.Fatalf("missing config exit = %d, want 1", code)
	}
}

func TestBenchLabRunUnknownTaskAgent(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, errb := labTestIO(dir)
	if code := io.BenchLabRunCmd(bench.LabModeSingle, []string{"--agent", "baseline", "--task", "NOPE"}); code != 2 {
		t.Fatalf("unknown task exit = %d, want 2", code)
	}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, []string{"--agent", "nope", "--task", "AT1"}); code != 2 {
		t.Fatalf("unknown agent exit = %d, want 2", code)
	}
	if errb.Len() == 0 {
		t.Fatal("unknown task/agent must explain themselves on stderr")
	}
}

func TestBenchLabRunNotRunHost(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, out, _ := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1", "--host", "no-such-host"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 3 {
		t.Fatalf("unknown host exit = %d, want 3", code)
	}
	if !strings.Contains(out.String(), "lab NOT RUN") {
		t.Fatalf("NOT RUN must print the lab line, got %q", out.String())
	}
}

func TestBenchLabRunPassAndFlow(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	stubLabRunOK(t, passLabResult())
	io, out, _ := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 0 {
		t.Fatalf("pass exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "lab baseline/AT1") {
		t.Fatalf("pass must print the result line, got %q", out.String())
	}
	out.Reset()
	if code := io.BenchLabRunCmd(bench.LabModeFlow, args); code != 0 {
		t.Fatalf("flow exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "trace timeline (F11.1)") {
		t.Fatalf("flow must print the timeline header, got %q", out.String())
	}
}

func TestBenchLabRunHostVersionExplicitSkipsProbe(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	oldProbe := benchLabProbeFunc
	benchLabProbeFunc = func() string { t.Fatal("probe must not run with explicit version"); return "" }
	defer func() { benchLabProbeFunc = oldProbe }()
	oldResolve := benchLabResolveRunner
	benchLabResolveRunner = func(host string) (bench.HeadlessAgent, string, int, bool) {
		return stubLabRunner{}, "", 0, true
	}
	defer func() { benchLabResolveRunner = oldResolve }()
	oldRun := benchLabRunFunc
	benchLabRunFunc = func(_ context.Context, _ bench.HeadlessAgent, _ bench.LabAgent, _ bench.AgentTask, _ string, _ bench.LabConfig, _ string) bench.LabResult {
		return passLabResult()
	}
	defer func() { benchLabRunFunc = oldRun }()
	io, _, _ := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1", "--host-version", "v9"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 0 {
		t.Fatalf("explicit version exit = %d, want 0", code)
	}
}

func TestBenchLabRunProbeDefaultAndHostOverride(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	probes := 0
	oldProbe := benchLabProbeFunc
	benchLabProbeFunc = func() string { probes++; return "pv" }
	defer func() { benchLabProbeFunc = oldProbe }()
	var seenHost string
	oldResolve := benchLabResolveRunner
	benchLabResolveRunner = func(host string) (bench.HeadlessAgent, string, int, bool) {
		seenHost = host
		return stubLabRunner{}, "", 0, true
	}
	defer func() { benchLabResolveRunner = oldResolve }()
	oldRun := benchLabRunFunc
	benchLabRunFunc = func(_ context.Context, _ bench.HeadlessAgent, _ bench.LabAgent, _ bench.AgentTask, _ string, cfg bench.LabConfig, _ string) bench.LabResult {
		if cfg.Host != "custom" {
			t.Fatalf("cfg.Host = %q, want custom override", cfg.Host)
		}
		return passLabResult()
	}
	defer func() { benchLabRunFunc = oldRun }()
	io, _, _ := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1", "--host", "custom"}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 0 {
		t.Fatalf("host override exit = %d, want 0", code)
	}
	if seenHost != "custom" {
		t.Fatalf("resolver host = %q, want custom", seenHost)
	}
	if probes != 0 {
		t.Fatalf("non-opencode host must not probe, got %d calls", probes)
	}
}

func TestBenchLabRunExportErrAndOK(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	stubLabRunOK(t, passLabResult())
	io, _, errb := labTestIO(dir)
	bad := filepath.Join(dir, "nope-dir", "cfg.json")
	args := []string{"--agent", "baseline", "--task", "AT1", "--export", bad}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 1 {
		t.Fatalf("export err exit = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Fatal("export failure must explain itself on stderr")
	}
	good := filepath.Join(dir, "cfg.json")
	io2, out2, _ := labTestIO(dir)
	args = []string{"--agent", "baseline", "--task", "AT1", "--export", good}
	if code := io2.BenchLabRunCmd(bench.LabModeSingle, args); code != 0 {
		t.Fatalf("export ok exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "config exported to") {
		t.Fatalf("export must confirm, got %q", out2.String())
	}
	if _, err := os.Stat(good); err != nil {
		t.Fatalf("export must write the file: %v", err)
	}
	// Replay the exported config exactly.
	io3, _, _ := labTestIO(dir)
	if code := io3.BenchLabRunCmd(bench.LabModeSingle, []string{"--config", good}); code != 0 {
		t.Fatalf("replay exit = %d, want 0", code)
	}
}

func TestBenchLabRunEvidenceErrAndOK(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	stubLabRunOK(t, passLabResult())
	// A file where the evidence dir belongs fails portably.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	io, _, errb := labTestIO(dir)
	args := []string{"--agent", "baseline", "--task", "AT1", "--evidence-dir", blocker}
	if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != 1 {
		t.Fatalf("evidence err exit = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Fatal("evidence failure must explain itself on stderr")
	}
	ev := filepath.Join(dir, "ev")
	io2, out2, _ := labTestIO(dir)
	args = []string{"--agent", "baseline", "--task", "AT1", "--evidence-dir", ev}
	if code := io2.BenchLabRunCmd(bench.LabModeSingle, args); code != 0 {
		t.Fatalf("evidence ok exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "evidence recorded in") {
		t.Fatalf("evidence must confirm, got %q", out2.String())
	}
}

func TestBenchLabRunVerdictExits(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	for _, tc := range []struct {
		name    string
		verdict string
		want    int
	}{
		{"fail", bench.LabFail, 0},
		{"not-run", bench.LabNotRun, 3},
		{"infra", bench.LabInfraError, 1},
		{"timeout", bench.LabTimeout, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := passLabResult()
			res.Verdict = tc.verdict
			stubLabRunOK(t, res)
			io, _, _ := labTestIO(dir)
			args := []string{"--agent", "baseline", "--task", "AT1"}
			if code := io.BenchLabRunCmd(bench.LabModeSingle, args); code != tc.want {
				t.Fatalf("verdict %q exit = %d, want %d", tc.verdict, code, tc.want)
			}
		})
	}
}

func TestBenchLabExitAndPrintHelpers(t *testing.T) {
	if benchLabExit(bench.LabPass) != 0 || benchLabExit(bench.LabFail) != 0 {
		t.Fatal("pass/fail must exit 0")
	}
	if benchLabExit(bench.LabNotRun) != 3 {
		t.Fatal("not-run must exit 3")
	}
	if benchLabExit(bench.LabInfraError) != 1 {
		t.Fatal("infra must exit 1")
	}
	var b bytes.Buffer
	res := passLabResult()
	res.Verdict = bench.LabNotRun
	printBenchLabResult(&b, res)
	if !strings.Contains(b.String(), "lab NOT RUN") {
		t.Fatalf("not-run print must note NOT RUN, got %q", b.String())
	}
	b.Reset()
	res.Verdict = bench.LabTimeout
	printBenchLabResult(&b, res)
	if !strings.Contains(b.String(), "INCONCLUSIVE") {
		t.Fatalf("timeout print must note INCONCLUSIVE, got %q", b.String())
	}
}
