// Thin deterministic tests for BenchLabCompareCmd: every early refusal
// plus stubbed comparison paths. Portable: t.TempDir + fakes only, no
// heavy lab, no shell, no network. The stubs replace the benchLab*
// seams so real hosts never run.
package cmdx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func stubLabCompareOK(t *testing.T, cmp bench.LabComparison, err error) {
	t.Helper()
	oldCmp, oldResolve, oldProbe := benchLabCompareFunc, benchLabResolveRunner, benchLabProbeFunc
	benchLabResolveRunner = func(host string) (bench.HeadlessAgent, string, int, bool) {
		return stubLabRunner{}, "", 0, true
	}
	benchLabCompareFunc = func(context.Context, []bench.LabContestant, bench.AgentTask, string, bench.LabConfig, string) (bench.LabComparison, error) {
		return cmp, err
	}
	benchLabProbeFunc = func() string { return "pv" }
	t.Cleanup(func() {
		benchLabCompareFunc, benchLabResolveRunner, benchLabProbeFunc = oldCmp, oldResolve, oldProbe
	})
}

func cmpResult(agent, verdict string) bench.LabResult {
	return bench.LabResult{
		AgentID: agent, TaskID: "AT1", Split: "exploration",
		RunID: "r-" + agent, AgentExecutionID: "a-" + agent, SessionID: "s",
		Verdict: verdict, DurationMS: 5, TraceHint: "hint-" + agent,
		Outcome: bench.AgentOutcome{Host: "opencode"},
	}
}

func TestBenchLabCompareParseFail(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, errb := labTestIO(dir)
	if code := io.BenchLabCompareCmd([]string{"--nope"}); code != 2 {
		t.Fatalf("bad flag exit = %d, want 2", code)
	}
	if code := io.BenchLabCompareCmd([]string{"--agents", "baseline,anchored", "--task", "AT1", "extra"}); code != 2 {
		t.Fatalf("positional exit = %d, want 2", code)
	}
	if errb.Len() == 0 {
		t.Fatal("parse failures must explain themselves on stderr")
	}
}

func TestBenchLabCompareSplitsFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(dir, "missing-splits.json"))
	io, _, errb := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("splits fail exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench agent-tasks:") {
		t.Fatalf("splits failure must keep the agent-tasks prefix, got %q", errb.String())
	}
}

func TestBenchLabCompareConfigFail(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, _ := labTestIO(dir)
	cfg := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--config", cfg, "--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 2 {
		t.Fatalf("config+selection exit = %d, want 2", code)
	}
	args = []string{"--config", filepath.Join(dir, "nope.json")}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("missing config exit = %d, want 1", code)
	}
}

func TestBenchLabCompareUnknownTaskAgent(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, _, errb := labTestIO(dir)
	if code := io.BenchLabCompareCmd([]string{"--agents", "baseline,anchored", "--task", "NOPE"}); code != 2 {
		t.Fatalf("unknown task exit = %d, want 2", code)
	}
	if code := io.BenchLabCompareCmd([]string{"--agents", "baseline,nope", "--task", "AT1"}); code != 2 {
		t.Fatalf("unknown agent exit = %d, want 2", code)
	}
	if code := io.BenchLabCompareCmd([]string{"--agents", "baseline", "--task", "AT1"}); code != 2 {
		t.Fatalf("single agent exit = %d, want 2 (compare needs two)", code)
	}
	if errb.Len() == 0 {
		t.Fatal("unknown task/agent must explain themselves on stderr")
	}
}

func TestBenchLabCompareNotRunHost(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	io, out, _ := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1", "--host", "no-such-host"}
	if code := io.BenchLabCompareCmd(args); code != 3 {
		t.Fatalf("unknown host exit = %d, want 3", code)
	}
	if !strings.Contains(out.String(), "lab NOT RUN") {
		t.Fatalf("NOT RUN must print the lab line, got %q", out.String())
	}
}

func TestBenchLabCompareLabErr(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	stubLabCompareOK(t, bench.LabComparison{}, errors.New("boom"))
	io, _, errb := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("compare err exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "boom") {
		t.Fatalf("compare error must surface, got %q", errb.String())
	}
}

func TestBenchLabCompareSuccess(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	cmp := bench.LabComparison{
		TaskID: "AT1", Split: "exploration",
		Results:   []bench.LabResult{cmpResult("baseline", bench.LabPass), cmpResult("anchored", bench.LabFail)},
		Evaluable: 2, Winner: "baseline", Reason: "best rank",
	}
	stubLabCompareOK(t, cmp, nil)
	io, out, _ := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 0 {
		t.Fatalf("success exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"lab compare AT1", "baseline", "anchored", "comparison: winner baseline", "trace baseline:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("success must print %q, got %q", want, got)
		}
	}
}

func TestBenchLabCompareAllNotRun(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	cmp := bench.LabComparison{
		TaskID: "AT1", Split: "exploration",
		Results: []bench.LabResult{cmpResult("baseline", bench.LabNotRun), cmpResult("anchored", bench.LabNotRun)},
	}
	stubLabCompareOK(t, cmp, nil)
	io, out, _ := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 3 {
		t.Fatalf("all NOT RUN exit = %d, want 3", code)
	}
	if !strings.Contains(out.String(), "lab NOT RUN") {
		t.Fatalf("all NOT RUN must print the line, got %q", out.String())
	}
}

func TestBenchLabCompareEvaluableZero(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	cmp := bench.LabComparison{
		TaskID: "AT1", Split: "exploration",
		Results: []bench.LabResult{cmpResult("baseline", bench.LabTimeout), cmpResult("anchored", bench.LabInfraError)},
	}
	stubLabCompareOK(t, cmp, nil)
	io, out, _ := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1"}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("evaluable 0 exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "INCONCLUSIVE") {
		t.Fatalf("evaluable 0 must print INCONCLUSIVE, got %q", out.String())
	}
}

func TestBenchLabCompareExportErrAndOK(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	cmp := bench.LabComparison{
		TaskID: "AT1", Split: "exploration",
		Results:   []bench.LabResult{cmpResult("baseline", bench.LabPass), cmpResult("anchored", bench.LabFail)},
		Evaluable: 2, Winner: "baseline", Reason: "best rank",
	}
	stubLabCompareOK(t, cmp, nil)
	io, _, errb := labTestIO(dir)
	bad := filepath.Join(dir, "nope-dir", "cfg.json")
	args := []string{"--agents", "baseline,anchored", "--task", "AT1", "--export", bad}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("export err exit = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Fatal("export failure must explain itself on stderr")
	}
	good := filepath.Join(dir, "cfg.json")
	io2, out2, _ := labTestIO(dir)
	args = []string{"--agents", "baseline,anchored", "--task", "AT1", "--export", good}
	if code := io2.BenchLabCompareCmd(args); code != 0 {
		t.Fatalf("export ok exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "config exported to") {
		t.Fatalf("export must confirm, got %q", out2.String())
	}
	if _, err := os.Stat(good); err != nil {
		t.Fatalf("export must write the file: %v", err)
	}
}

func TestBenchLabCompareEvidenceErrAndOK(t *testing.T) {
	dir := t.TempDir()
	repoSplits(t)
	cmp := bench.LabComparison{
		TaskID: "AT1", Split: "exploration",
		Results:   []bench.LabResult{cmpResult("baseline", bench.LabPass), cmpResult("anchored", bench.LabFail)},
		Evaluable: 2, Winner: "baseline", Reason: "best rank",
	}
	stubLabCompareOK(t, cmp, nil)
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	io, _, errb := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1", "--evidence-dir", blocker}
	if code := io.BenchLabCompareCmd(args); code != 1 {
		t.Fatalf("evidence err exit = %d, want 1", code)
	}
	if errb.Len() == 0 {
		t.Fatal("evidence failure must explain itself on stderr")
	}
	ev := filepath.Join(dir, "ev")
	io2, out2, _ := labTestIO(dir)
	args = []string{"--agents", "baseline,anchored", "--task", "AT1", "--evidence-dir", ev}
	if code := io2.BenchLabCompareCmd(args); code != 0 {
		t.Fatalf("evidence ok exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "evidence recorded in") {
		t.Fatalf("evidence must confirm, got %q", out2.String())
	}
}

func TestBenchLabCompareHostVersionExplicit(t *testing.T) {
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
	oldCmp := benchLabCompareFunc
	benchLabCompareFunc = func(context.Context, []bench.LabContestant, bench.AgentTask, string, bench.LabConfig, string) (bench.LabComparison, error) {
		return bench.LabComparison{
			TaskID: "AT1", Split: "exploration",
			Results:   []bench.LabResult{cmpResult("baseline", bench.LabPass), cmpResult("anchored", bench.LabFail)},
			Evaluable: 2, Winner: "baseline", Reason: "best rank",
		}, nil
	}
	defer func() { benchLabCompareFunc = oldCmp }()
	io, _, _ := labTestIO(dir)
	args := []string{"--agents", "baseline,anchored", "--task", "AT1", "--host-version", "v9"}
	if code := io.BenchLabCompareCmd(args); code != 0 {
		t.Fatalf("explicit version exit = %d, want 0", code)
	}
}
