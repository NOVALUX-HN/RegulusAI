// Split paydown for benchAgentTasksCmd: focused coverage for the
// extracted parse/resolve/run/report helpers. Messages and exit codes
// stay byte-identical (3=NOT RUN, 1=INCONCLUSIVE, 0=pass); these tests
// pin the helper contracts so the orchestrator stays under the
// standard CRAP ceiling.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/bench"
)

func TestParseBenchAgentTasksOptionsDefaults(t *testing.T) {
	var errb bytes.Buffer
	opts, code, ok := parseBenchAgentTasksOptions(nil, &errb)
	if !ok || code != 0 {
		t.Fatalf("parse = (%d, %v), want (0, true)", code, ok)
	}
	if opts.reps != 1 || opts.split != "all" || opts.host != "opencode" {
		t.Fatalf("defaults = %+v, want reps=1 split=all host=opencode", opts)
	}
	if opts.dryRun || opts.maxWallS != 0 {
		t.Fatalf("defaults = %+v, want dryRun=false maxWallS=0", opts)
	}
}

func TestParseBenchAgentTasksOptionsRejects(t *testing.T) {
	var errb bytes.Buffer
	if _, code, ok := parseBenchAgentTasksOptions([]string{"--nope"}, &errb); ok || code != 2 {
		t.Fatalf("bad flag = (%d, %v), want (2, false)", code, ok)
	}
	var errb2 bytes.Buffer
	if _, code, ok := parseBenchAgentTasksOptions([]string{"extra"}, &errb2); ok || code != 2 {
		t.Fatalf("positional = (%d, %v), want (2, false)", code, ok)
	}
	if !strings.Contains(errb2.String(), "usage: regulus bench agent-tasks") {
		t.Fatalf("positional must print usage, got %q", errb2.String())
	}
	var errb3 bytes.Buffer
	if _, code, ok := parseBenchAgentTasksOptions([]string{"--reps", "0"}, &errb3); ok || code != 2 {
		t.Fatalf("reps=0 = (%d, %v), want (2, false)", code, ok)
	}
}

func TestApplyBenchAgentTaskFilterPassthrough(t *testing.T) {
	splits := bench.AgentSplits{Exploration: []string{"AT1"}, Holdout: []string{"AT5"}}
	var errb bytes.Buffer
	got, code, ok := applyBenchAgentTaskFilter(splits, nil, "", &errb)
	if !ok || code != 0 || len(got.Exploration) != 1 {
		t.Fatalf("passthrough = (%v, %d, %v), want splits intact", got, code, ok)
	}
}

func TestApplyBenchAgentTaskFilterUnknown(t *testing.T) {
	corpus := []bench.AgentTask{{ID: "AT1"}, {ID: "AT5"}}
	splits := bench.AgentSplits{Exploration: []string{"AT1"}, Holdout: []string{"AT5"}}
	var errb bytes.Buffer
	if _, code, ok := applyBenchAgentTaskFilter(splits, corpus, "AT1,NOPE", &errb); ok || code != 2 {
		t.Fatalf("unknown = (%d, %v), want (2, false)", code, ok)
	}
	if !strings.Contains(errb.String(), `unknown task "NOPE"`) {
		t.Fatalf("unknown must name the task, got %q", errb.String())
	}
}

func TestApplyBenchAgentTaskFilterSelects(t *testing.T) {
	corpus := []bench.AgentTask{{ID: "AT1"}, {ID: "AT2"}, {ID: "AT5"}}
	splits := bench.AgentSplits{Exploration: []string{"AT1", "AT2"}, Holdout: []string{"AT5"}}
	var errb bytes.Buffer
	got, code, ok := applyBenchAgentTaskFilter(splits, corpus, "AT2, AT5", &errb)
	if !ok || code != 0 {
		t.Fatalf("select = (%d, %v), want (0, true)", code, ok)
	}
	if len(got.Exploration) != 1 || got.Exploration[0] != "AT2" {
		t.Fatalf("exploration = %v, want [AT2]", got.Exploration)
	}
	if len(got.Holdout) != 1 || got.Holdout[0] != "AT5" {
		t.Fatalf("holdout = %v, want [AT5]", got.Holdout)
	}
}

func TestApplyBenchAgentTaskFilterMatchedNone(t *testing.T) {
	corpus := []bench.AgentTask{{ID: "AT1"}, {ID: "AT2"}}
	splits := bench.AgentSplits{Exploration: []string{"AT1"}, Holdout: nil}
	var errb bytes.Buffer
	if _, code, ok := applyBenchAgentTaskFilter(splits, corpus, "AT2", &errb); ok || code != 2 {
		t.Fatalf("matched-none = (%d, %v), want (2, false)", code, ok)
	}
	if !strings.Contains(errb.String(), "--task matched no tasks") {
		t.Fatalf("matched-none must say so, got %q", errb.String())
	}
}

func TestPrintBenchAgentDryRunListsTasks(t *testing.T) {
	var out bytes.Buffer
	printBenchAgentDryRun(&out, bench.AgentSplits{Exploration: []string{"AT1"}, Holdout: []string{"AT5"}}, 2)
	got := out.String()
	if !strings.Contains(got, "dry-run: 2 task(s) x 2 rep(s) = 4 attempt(s), no model calls") {
		t.Fatalf("dry-run header wrong:\n%s", got)
	}
	for _, want := range []string{"- AT1 [exploration]", "- AT5 [holdout]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run missing %q:\n%s", want, got)
		}
	}
}

func TestSaveBenchAgentOutcomesNoop(t *testing.T) {
	var out, errb bytes.Buffer
	if code := saveBenchAgentOutcomes(&out, &errb, "", nil); code != 0 {
		t.Fatalf("empty save exit = %d, want 0", code)
	}
}

func TestSaveBenchAgentOutcomesRoundTrip(t *testing.T) {
	var out, errb bytes.Buffer
	path := filepath.Join(t.TempDir(), "out.json")
	outs := []bench.AgentOutcome{{TaskID: "AT1", Status: bench.AgentPass}}
	if code := saveBenchAgentOutcomes(&out, &errb, path, outs); code != 0 {
		t.Fatalf("save exit = %d, want 0 (%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), "outcome(s) saved to") {
		t.Fatalf("save must confirm, got %q", out.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "AT1") {
		t.Fatalf("saved file must contain AT1: %v %q", err, string(raw))
	}
}

func TestSaveBenchAgentOutcomesWriteError(t *testing.T) {
	var out, errb bytes.Buffer
	dir := t.TempDir()
	if code := saveBenchAgentOutcomes(&out, &errb, dir, []bench.AgentOutcome{{TaskID: "AT1"}}); code != 1 {
		t.Fatalf("save-to-dir exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench agent-tasks:") {
		t.Fatalf("save error must name the command, got %q", errb.String())
	}
}

func TestDecideBenchAgentExit(t *testing.T) {
	if code, msg := decideBenchAgentExit(bench.AgentSummary{Total: 2, NotRun: 2}, 2); code != 3 || !strings.Contains(msg, "NOT RUN") {
		t.Fatalf("all-not-run = (%d, %q), want (3, NOT RUN)", code, msg)
	}
	if code, msg := decideBenchAgentExit(bench.AgentSummary{Total: 1, Infra: 1}, 0); code != 1 || !strings.Contains(msg, "INCONCLUSIVE") {
		t.Fatalf("infra-only = (%d, %q), want (1, INCONCLUSIVE)", code, msg)
	}
	if code, msg := decideBenchAgentExit(bench.AgentSummary{Total: 2, Evaluable: 2, Pass: 1, Fail: 1}, 0); code != 0 || msg != "" {
		t.Fatalf("pass = (%d, %q), want (0, empty)", code, msg)
	}
}

func TestReportBenchAgentResultsVerdicts(t *testing.T) {
	var out bytes.Buffer
	if code := reportBenchAgentResults(&out, "v", []bench.AgentOutcome{
		{TaskID: "AT1", Split: "exploration", Status: bench.AgentNotRun},
	}); code != 3 || !strings.Contains(out.String(), "NOT RUN") {
		t.Fatalf("not-run report = (%d, %q), want 3 NOT RUN", code, out.String())
	}
	var out2 bytes.Buffer
	if code := reportBenchAgentResults(&out2, "v", []bench.AgentOutcome{
		{TaskID: "AT1", Split: "exploration", Status: bench.AgentInfra},
	}); code != 1 || !strings.Contains(out2.String(), "INCONCLUSIVE") {
		t.Fatalf("infra report = (%d, %q), want 1 INCONCLUSIVE", code, out2.String())
	}
	var out3 bytes.Buffer
	if code := reportBenchAgentResults(&out3, "v", []bench.AgentOutcome{
		{TaskID: "AT1", Split: "exploration", Rep: 1, Status: bench.AgentPass},
		{TaskID: "AT1", Split: "exploration", Rep: 2, Status: bench.AgentFail},
	}); code != 0 {
		t.Fatalf("pass report exit = %d, want 0", code)
	}
	if !strings.Contains(out3.String(), "1 pass, 1 fail") {
		t.Fatalf("pass report must summarize, got:\n%s", out3.String())
	}
}

func TestExecuteBenchAgentSuiteUnknownHost(t *testing.T) {
	c, out, _ := newCLI(nil)
	outs, _, code, ok := c.executeBenchAgentSuite(benchAgentTasksOptions{host: "bogus"}, nil, bench.AgentSplits{})
	if ok || code != 3 || outs != nil {
		t.Fatalf("unknown host = (%v, %d, %v), want (nil, 3, false)", outs, code, ok)
	}
	if !strings.Contains(out.String(), "NOT RUN") {
		t.Fatalf("unknown host must say NOT RUN, got %q", out.String())
	}
}

func TestWriteBenchAgentEvidenceNoop(t *testing.T) {
	c, _, _ := newCLI(nil)
	if code := c.writeBenchAgentEvidence(benchAgentTasksOptions{}, nil, bench.AgentSplits{}, "", time.Now().UTC()); code != 0 {
		t.Fatalf("empty dir exit = %d, want 0", code)
	}
}

func TestWriteBenchAgentEvidenceSuccess(t *testing.T) {
	c, out, _ := newCLI(nil)
	dir := filepath.Join(t.TempDir(), "ev")
	opts := benchAgentTasksOptions{evidenceDir: dir, reps: 1, variant: "v", split: "all", host: "opencode"}
	if code := c.writeBenchAgentEvidence(opts, []bench.AgentOutcome{{TaskID: "AT1"}}, bench.AgentSplits{Seed: 7}, "1.2.3", time.Now().UTC()); code != 0 {
		t.Fatalf("evidence exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "versioned run recorded in") {
		t.Fatalf("evidence must confirm, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, bench.EvidenceManifestFile)); err != nil {
		t.Fatalf("manifest must exist: %v", err)
	}
}

func TestWriteBenchAgentEvidenceError(t *testing.T) {
	c, _, errb := newCLI(nil)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := benchAgentTasksOptions{evidenceDir: filepath.Join(blocker, "ev"), reps: 1}
	if code := c.writeBenchAgentEvidence(opts, nil, bench.AgentSplits{}, "", time.Now().UTC()); code != 1 {
		t.Fatalf("evidence error exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "bench agent-tasks:") {
		t.Fatalf("evidence error must name the command, got %q", errb.String())
	}
}

func TestBenchAgentSplitsPathEnvWins(t *testing.T) {
	c, _, _ := newCLI(nil)
	t.Setenv("REGULUS_AGENT_SPLITS", "custom.json")
	if got := c.benchAgentSplitsPath(); got != "custom.json" {
		t.Fatalf("env path = %q, want custom.json", got)
	}
}

func TestLoadBenchAgentSplitsError(t *testing.T) {
	c, _, errb := newCLI(nil)
	t.Setenv("REGULUS_AGENT_SPLITS", filepath.Join(t.TempDir(), "missing.json"))
	if _, code, ok := c.loadBenchAgentSplits(bench.AgentCorpus()); ok || code != 1 {
		t.Fatalf("missing splits = (%d, %v), want (1, false)", code, ok)
	}
	if !strings.Contains(errb.String(), "bench agent-tasks:") {
		t.Fatalf("splits error must name the command, got %q", errb.String())
	}
}

func TestLoadBenchAgentSplitsSuccess(t *testing.T) {
	c, _, _ := newCLI(nil)
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
	if _, code, ok := c.loadBenchAgentSplits(bench.AgentCorpus()); !ok || code != 0 {
		t.Fatalf("valid splits = (%d, %v), want (0, true)", code, ok)
	}
}

func TestBenchAgentTasksDryRunCLI(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-tasks", "--dry-run", "--reps", "1"}); code != 0 {
		t.Fatalf("dry-run exit = %d, want 0 (%q)", code, out.String())
	}
	if !strings.Contains(out.String(), "dry-run") || !strings.Contains(out.String(), "no model calls") {
		t.Fatalf("dry-run must list without models, got:\n%s", out.String())
	}
}

func TestBenchAgentTasksTaskFilterCLI(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("REGULUS_AGENT_SPLITS", writeAgentSplits(t))
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"bench", "agent-tasks", "--host", "bogus", "--task", "NOPE"}); code != 2 {
		t.Fatalf("unknown task exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown task "NOPE"`) {
		t.Fatalf("unknown task must name it, got %q", errb.String())
	}
}

func TestShortOutcomeDetailEdges(t *testing.T) {
	if got := shortOutcomeDetail(""); got != "-" {
		t.Fatalf("empty = %q, want -", got)
	}
	if got := shortOutcomeDetail("first\nsecond"); got != "first" {
		t.Fatalf("multiline = %q, want first", got)
	}
	if got := shortOutcomeDetail(strings.Repeat("x", 200)); len(got) == 0 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long must truncate with ellipsis, got len %d", len(got))
	}
}
