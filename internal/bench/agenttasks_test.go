// NOV-68: agent-task evals. Splits validation, honest bucketing,
// degrade path, and controls the robust evaluator must reject/accept.
package bench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evolve"
	"regulus-ai/internal/trace"
)

func writeSplits(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "splits.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAgentSplitsValid(t *testing.T) {
	p := writeSplits(t, `{"version":"v","seed":1,"exploration":["AT1","AT2","AT3","AT4"],"holdout":["AT5"]}`)
	sp, err := LoadAgentSplits(p, AgentCorpus())
	if err != nil {
		t.Fatalf("valid splits rejected: %v", err)
	}
	if len(sp.Holdout) != 1 || sp.Exploration[0] != "AT1" {
		t.Fatalf("splits wrong: %+v", sp)
	}
}

func TestLoadAgentSplitsRejects(t *testing.T) {
	corpus := AgentCorpus()
	cases := map[string]string{
		"overlap":   `{"version":"v","seed":1,"exploration":["AT1","AT5"],"holdout":["AT5","AT2","AT3","AT4"]}`,
		"unknown":   `{"version":"v","seed":1,"exploration":["AT1","AT2","AT3","ATX"],"holdout":["AT5"]}`,
		"thin":      `{"version":"v","seed":1,"exploration":["AT1","AT2","AT3","AT4","AT5"],"holdout":[]}`,
		"partial":   `{"version":"v","seed":1,"exploration":["AT1"],"holdout":["AT5"]}`,
		"extra-key": `{"version":"v","seed":1,"exploration":["AT1","AT2","AT3","AT4"],"holdout":["AT5"],"x":1}`,
	}
	for name, doc := range cases {
		if _, err := LoadAgentSplits(writeSplits(t, doc), corpus); err == nil {
			t.Fatalf("%s must fail", name)
		}
	}
}

// stubAgent is a test-only stand-in for a headless host.
type stubAgent struct {
	fix   map[string]string // taskID -> fixed file body (nil = leave broken)
	sleep time.Duration
	fail  error
}

var stubFixes = map[string]struct{ file, body string }{
	"AT1": {"sum.go", "package calc\n\n// Sum returns the sum of xs.\nfunc Sum(xs []int) int {\n\ttotal := 0\n\tfor i := 0; i < len(xs); i++ {\n\t\ttotal += xs[i]\n\t}\n\treturn total\n}\n"},
	"AT2": {"rev.go", "package text\n\n// Reverse returns s reversed.\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r)\n}\n"},
	"AT3": {"median.js", "export function median(xs) {\n  const s = [...xs].sort((a, b) => a - b);\n  const m = Math.floor(s.length / 2);\n  if (s.length % 2 === 0) return (s[m - 1] + s[m]) / 2;\n  return s[m];\n}\n"},
	"AT4": {"sortnums.sh", "#!/usr/bin/env bash\nsort nums.txt\n"},
	"AT5": {"clamp.go", "package math2\n\n// Clamp restricts v to [lo, hi].\nfunc Clamp(v, lo, hi int) int {\n\tif v < lo {\n\t\treturn lo\n\t}\n\tif v > hi {\n\t\treturn hi\n\t}\n\treturn v\n}\n"},
}

func (s stubAgent) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	if s.fail != nil {
		return "", trace.HostUsage{}, s.fail
	}
	if s.sleep > 0 {
		select {
		case <-ctx.Done():
			return "", trace.HostUsage{}, ctx.Err()
		case <-time.After(s.sleep):
		}
	}
	if s.fix != nil {
		// Apply every known fix whose file exists in this fixture.
		for _, fx := range stubFixes {
			p := filepath.Join(workdir, fx.file)
			if _, err := os.Stat(p); err == nil {
				if err := os.WriteFile(p, []byte(fx.body), 0o644); err != nil {
					return "", trace.HostUsage{}, err
				}
			}
		}
		return "fixed", trace.HostUsage{InputTokens: 10, OutputTokens: 5, Source: trace.SourceMeasuredHost}, nil
	}
	return "no-op", trace.HostUsage{}, nil
}

func testSplits() AgentSplits {
	return AgentSplits{Version: "t", Seed: 7, Exploration: []string{"AT1", "AT2", "AT3", "AT4"}, Holdout: []string{"AT5"}}
}

func TestAgentSuitePositiveFixPasses(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{fix: map[string]string{}}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "candidate", Split: "exploration"})
	sum := SummarizeAgent("candidate", out)
	if sum.Evaluable != 4 || sum.Pass != 4 {
		t.Fatalf("fixing stub must pass 4/4 exploration, got %+v", sum)
	}
	for _, o := range out {
		if o.TokenSource != trace.SourceMeasuredHost {
			t.Fatalf("stub usage must label measured-host, got %q", o.TokenSource)
		}
	}
}

func TestAgentExplorationNeverTouchesHoldout(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{fix: map[string]string{}}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "exploration"})
	if len(out) != 4 {
		t.Fatalf("exploration must run 4 tasks, ran %d", len(out))
	}
	for _, o := range out {
		if o.TaskID == "AT5" {
			t.Fatal("holdout task AT5 must never run in exploration mode (anti-leakage)")
		}
		if o.Split != "exploration" {
			t.Fatalf("split label wrong: %+v", o)
		}
	}
	hold := RunAgentSuite(context.Background(), stubAgent{}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout"})
	if len(hold) != 1 || hold[0].TaskID != "AT5" {
		t.Fatalf("holdout must run only AT5, got %+v", hold)
	}
}

func TestAgentSuiteNoOpFixFails(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "baseline", Split: "exploration"})
	sum := SummarizeAgent("baseline", out)
	if sum.Evaluable != 4 || sum.Fail != 4 || sum.Pass != 0 {
		t.Fatalf("no-op stub must fail 4/4, got %+v", sum)
	}
}

func TestAgentSuiteBucketsTimeoutInfra(t *testing.T) {
	slow := RunAgentSuite(context.Background(), stubAgent{sleep: 5 * time.Second}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout", BudgetS: 1})
	if len(slow) != 1 || slow[0].Status != AgentTimeout {
		t.Fatalf("slow stub must time out, got %+v", slow)
	}
	broken := RunAgentSuite(context.Background(), stubAgent{fail: errors.New("boom")}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout"})
	if len(broken) != 1 || broken[0].Status != AgentInfra {
		t.Fatalf("failing stub must be infra, got %+v", broken)
	}
	if broken[0].Detail == "" {
		t.Fatal("infra must carry the reason")
	}
}

func TestAgentControlsRobustRejectAccept(t *testing.T) {
	neg := []evolve.ExplorationOutcome{
		{ID: "neg/AT1", Status: evolve.ExploreFail},
		{ID: "neg/AT2", Status: evolve.ExploreFail},
	}
	pos := []evolve.ExplorationOutcome{
		{ID: "pos/AT1", Status: evolve.ExplorePass},
		{ID: "pos/AT2", Status: evolve.ExplorePass},
	}
	base := []evolve.ExplorationOutcome{
		{ID: "base/AT1", Status: evolve.ExplorePass},
		{ID: "base/AT2", Status: evolve.ExploreFail},
	}
	g := evolve.RobustGuardrails{Functional: true, Compliance: true, NoRegression: true, Adversarial: true, Maintainable: true}
	negD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("neg", neg), g, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if negD.Verdict == evolve.RobustAdoptCandidate {
		t.Fatalf("degraded control must not adopt, got %v", negD.Verdict)
	}
	posD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("pos", pos), g, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if posD.Verdict != evolve.RobustAdoptCandidate {
		t.Fatalf("known improvement must adopt, got %v (%s)", posD.Verdict, posD.Reason)
	}
	badG := g
	badG.Adversarial = false
	guardD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("pos", pos), badG, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if guardD.Verdict != evolve.RobustRejectGuardrail {
		t.Fatalf("guardrail failure must reject, got %v", guardD.Verdict)
	}
}

func TestAppControlBlockIsInfraNotFail(t *testing.T) {
	blocked := AgentTask{ID: "ATX", Title: "x", Lang: "shell",
		Prompt: "x", Files: map[string]string{"f.txt": "x\n"},
		Check:   []string{"bash", "-c", "echo 'fork/exec x: Una directiva de Control de aplicaciones bloqueó este archivo.' >&2; exit 1"},
		BudgetS: 60}
	out := RunAgentSuite(context.Background(), stubAgent{}, []AgentTask{blocked},
		AgentSplits{Version: "t", Seed: 1, Exploration: []string{"ATX"}},
		AgentOptions{Reps: 1, Variant: "v", Split: "exploration"})
	if len(out) != 1 || out[0].Status != AgentInfra {
		t.Fatalf("blocked check must be infra, got %+v", out)
	}
}

func TestAgentSummarySeparatesBuckets(t *testing.T) {
	out := []AgentOutcome{
		{TaskID: "AT1", Rep: 0, Status: AgentPass},
		{TaskID: "AT1", Rep: 1, Status: AgentFail},
		{TaskID: "AT2", Rep: 0, Status: AgentTimeout},
		{TaskID: "AT2", Rep: 1, Status: AgentInfra},
		{TaskID: "AT3", Rep: 0, Status: AgentNotRun},
	}
	sum := SummarizeAgent("v", out)
	if sum.Evaluable != 2 || sum.Pass != 1 || sum.Fail != 1 || sum.Timeout != 1 || sum.Infra != 1 || sum.NotRun != 1 {
		t.Fatalf("buckets wrong: %+v", sum)
	}
	if sum.PassRate != 0.5 {
		t.Fatalf("pass-rate must be pass/evaluable = 0.5, got %v", sum.PassRate)
	}
	if sum.RepMin != 0.5 || sum.RepMax != 0.5 {
		t.Fatalf("rep spread wrong: %+v", sum)
	}
}

func TestToExplorationOutcomesMapsStatuses(t *testing.T) {
	out := ToExplorationOutcomes("v", []AgentOutcome{
		{TaskID: "A", Rep: 0, Status: AgentPass},
		{TaskID: "B", Rep: 0, Status: AgentFail},
		{TaskID: "C", Rep: 0, Status: AgentTimeout},
		{TaskID: "D", Rep: 0, Status: AgentInfra},
		{TaskID: "E", Rep: 0, Status: AgentNotRun},
	})
	want := map[string]string{"A": "pass", "B": "fail", "C": "timeout", "D": "infra", "E": "non-evaluable"}
	if len(out) != len(want) {
		t.Fatalf("got %d outcomes, want %d", len(out), len(want))
	}
	for _, o := range out {
		id := strings.TrimPrefix(o.ID, "v/")
		if want[id] == "" || string(o.Status) != want[id] {
			t.Fatalf("task %s mapped to %q, want %q", id, o.Status, want[id])
		}
	}
}
