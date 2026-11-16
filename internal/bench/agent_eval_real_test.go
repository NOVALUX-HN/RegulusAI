// NOV-77 S4 real-eval harness: runs the CURRENT corpus against a real
// headless host and writes versioned evidence (manifest + outcomes +
// sha256) via the same WriteAgentEvidence path the CLI uses.
//
// Gated on REGULUS_REAL_EVAL=1 (default: skip) so the ordinary suite
// stays offline, deterministic and fast. Reproducible invocation:
//
//	REGULUS_REAL_EVAL=1 REGULUS_EVIDENCE_DIR=<dir> \
//	  go test ./internal/bench -run TestAgentRealEvalV3 -v
//
// Env knobs (all optional): REGULUS_EVAL_MODEL (default
// "opencode-go/beam-free"), REGULUS_EVAL_VARIANT (default "baseline"),
// REGULUS_EVIDENCE_DIR (default bench/evidence/v2-exploration-beamfree,
// with v3-<split>-beamfree subdirs). Per-rep identity is rep+seed on
// every outcome (bench agent-tasks has no --execution-id flag; only
// trace record does), plus one manifest per split with its own
// timestamps. Buckets stay separate: infra/timeout never pass, and a
// host with no quota yields INCONCLUSIVE evidence, never a false pass.
package bench

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

// loggingAgent wraps a HeadlessAgent with per-attempt progress so long
// eval runs stay observable; it never changes results.
type loggingAgent struct {
	inner HeadlessAgent
	t     *testing.T
	n     *int64
	total int64
}

func (l loggingAgent) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	i := atomic.AddInt64(l.n, 1)
	l.t.Logf("attempt %d/%d: host run started (model %q)", i, l.total, model)
	start := time.Now()
	raw, usage, err := l.inner.Run(ctx, workdir, prompt, model)
	l.t.Logf("attempt %d/%d: host run finished in %s err=%v in=%d out=%d",
		i, l.total, time.Since(start).Round(time.Second), err != nil, usage.InputTokens, usage.OutputTokens)
	return raw, usage, err
}

func TestAgentRealEvalV3(t *testing.T) {
	if os.Getenv("REGULUS_REAL_EVAL") != "1" {
		t.Skip("real host eval: set REGULUS_REAL_EVAL=1 to run (offline by default)")
	}
	runner, err := DefaultOpenCodeRunner()
	if err != nil {
		t.Skipf("real host eval NOT RUN: %v (suite skipped, nothing passed or failed)", err)
	}
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	splits, err := LoadAgentSplits(filepath.Join(root, AgentSplitsPath), AgentCorpus())
	if err != nil {
		t.Fatalf("committed splits must validate: %v", err)
	}
	model := os.Getenv("REGULUS_EVAL_MODEL")
	if model == "" {
		model = "opencode-go/beam-free"
	}
	variant := os.Getenv("REGULUS_EVAL_VARIANT")
	if variant == "" {
		variant = "baseline"
	}
	evidenceRoot := os.Getenv("REGULUS_EVIDENCE_DIR")
	if evidenceRoot == "" {
		evidenceRoot = filepath.Join(root, "bench", "evidence", "v2-exploration-beamfree")
	}
	splitEnv := os.Getenv("REGULUS_EVAL_SPLIT")
	if splitEnv == "" {
		splitEnv = "exploration,holdout"
	}
	hostVersion := probeOpenCodeVersionForEval()
	gitHead := gitHeadForEval()
	corpus := AgentCorpus()
	for _, split := range strings.Split(splitEnv, ",") {
		split = strings.TrimSpace(split)
		if split != "exploration" && split != "holdout" {
			t.Fatalf("REGULUS_EVAL_SPLIT must list exploration and/or holdout, got %q", splitEnv)
		}
		splitCount := len(splits.Exploration)
		if split == "holdout" {
			splitCount = len(splits.Holdout)
		}
		var n int64
		agent := loggingAgent{inner: runner, t: t, n: &n, total: int64(splitCount * 3)}
		splitStart := time.Now().UTC().Format(time.RFC3339)
		outcomes := RunAgentSuite(context.Background(), agent, corpus, splits, AgentOptions{
			Reps: 3, Variant: variant, Model: model,
			Host: "opencode", HostVersion: hostVersion, Split: split,
		})
		dir := filepath.Join(evidenceRoot, "v3-"+split+"-beamfree")
		seeds := []int64{splits.Seed, splits.Seed + 1, splits.Seed + 2}
		if _, err := WriteAgentEvidence(dir, outcomes, AgentRunManifest{
			SuiteVersion: AgentSuiteVersion, Model: model, Host: "opencode",
			HostVersion: hostVersion, Variant: variant, Reps: 3,
			Split: split, Seeds: seeds, GitHead: gitHead,
			StartedAt: splitStart,
		}); err != nil {
			t.Fatalf("writing %s evidence: %v", split, err)
		}
		sum := SummarizeAgent(variant, outcomes)
		t.Logf("%s: total=%d evaluable=%d pass=%d fail=%d timeout=%d infra=%d not-run=%d pass-rate=%.2f rep-spread=%.2f-%.2f tokens=%d+%d evidence=%s",
			split, sum.Total, sum.Evaluable, sum.Pass, sum.Fail, sum.Timeout, sum.Infra, sum.NotRun,
			sum.PassRate, sum.RepMin, sum.RepMax, sum.TokensIn, sum.TokensOut, dir)
		fmt.Fprintf(os.Stderr, "NOV77-EVAL %s evidence: %s\n", split, dir)
	}
}

// probeOpenCodeVersionForEval best-efforts `opencode --version`; empty
// on any failure, never an error.
func probeOpenCodeVersionForEval() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 2 && fields[0] == "opencode" {
		return fields[1]
	}
	return ""
}

// gitHeadForEval best-efforts `git rev-parse HEAD`; "unknown" when git
// is unavailable so evidence still records instead of failing.
func gitHeadForEval() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	if head := strings.TrimSpace(string(out)); head != "" {
		return head
	}
	return "unknown"
}
