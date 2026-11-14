// Agent-task evals (NOV-68, S3): real engineering tasks executed by a
// real headless agent, with and without the harness variant, over
// versioned exploration/holdout splits. Extends macizo-bench-v3
// (NOV-35, kept as the core regression suite) and f7-protocol-v1
// (NOV-56, anti-leakage rules); it creates no parallel benchmark.
//
// Honesty rules: infra failures, timeouts and non-evaluable results
// sit in their own buckets and never pass; without a usable headless
// host the suite reports NOT RUN and exits non-zero instead of
// passing in false; significance is never claimed below
// RobustSignificanceFloor.
package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/evolve"
	"regulus-ai/internal/trace"
)

// AgentSuiteVersion pins the task corpus: reports compare only within
// one version. Adding a task bumps the version and re-baselines.
const AgentSuiteVersion = "agent-tasks-v1"

// AgentSplitsPath is the tracked exploration/holdout split (committed,
// not runtime state): holdout stays >= 20% with no repo/pattern
// overlap with exploration.
const AgentSplitsPath = "bench/agent-splits-v1.json"

// Agent task outcome statuses. Only pass/fail are evaluable; the rest
// dilute nothing and never promote.
const (
	AgentPass         = "pass"
	AgentFail         = "fail"
	AgentTimeout      = "timeout"
	AgentInfra        = "infra"
	AgentNonEvaluable = "non-evaluable"
	AgentNotRun       = "not-run"
)

// AgentTask is one small self-contained engineering task: a fixture
// repo (visible to the agent) plus hidden checks (runner-only, never
// shown to the agent). The check command runs with cwd set to the
// fixture dir and FIXTURE_DIR exported; exit 0 passes.
type AgentTask struct {
	ID      string
	Title   string
	Lang    string
	Prompt  string
	Files   map[string]string
	Check   []string
	BudgetS int
}

// AgentCorpus is the v1 starter corpus: 4 exploration + 1 holdout
// (20%). Every fixture is offline and deterministic (Go toolchain,
// node, bash only).
func AgentCorpus() []AgentTask {
	return []AgentTask{
		{
			ID: "AT1", Title: "fix off-by-one Sum", Lang: "go",
			Prompt: "Fix the off-by-one bug in Sum so all tests pass. Do not change the test file.",
			Files: map[string]string{
				"sum.go":      "package calc\n\n// Sum returns the sum of xs.\nfunc Sum(xs []int) int {\n\ttotal := 0\n\tfor i := 0; i < len(xs)-1; i++ {\n\t\ttotal += xs[i]\n\t}\n\treturn total\n}\n",
				"sum_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestSum(t *testing.T) {\n\tif got := Sum([]int{1, 2, 3}); got != 6 {\n\t\tt.Fatalf(\"Sum = %d, want 6\", got)\n\t}\n\tif got := Sum(nil); got != 0 {\n\t\tt.Fatalf(\"Sum(nil) = %d, want 0\", got)\n\t}\n}\n",
			},
			Check:   []string{"go", "test", "./..."},
			BudgetS: 120,
		},
		{
			ID: "AT2", Title: "fix Reverse dropping rune", Lang: "go",
			Prompt: "Fix Reverse so it keeps every rune, including the last one. Do not change the test file.",
			Files: map[string]string{
				"rev.go":      "package text\n\n// Reverse returns s reversed.\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r[:len(r)-1])\n}\n",
				"rev_test.go": "package text\n\nimport \"testing\"\n\nfunc TestReverse(t *testing.T) {\n\tif got := Reverse(\"hola\"); got != \"aloh\" {\n\t\tt.Fatalf(\"Reverse = %q, want aloh\", got)\n\t}\n\tif got := Reverse(\"a\"); got != \"a\" {\n\t\tt.Fatalf(\"Reverse(a) = %q, want a\", got)\n\t}\n}\n",
			},
			Check:   []string{"go", "test", "./..."},
			BudgetS: 120,
		},
		{
			ID: "AT3", Title: "fix median of even list", Lang: "node",
			Prompt: "Fix median() so even-length arrays average the two middle values. Do not change test.mjs.",
			Files: map[string]string{
				"median.js": "export function median(xs) {\n  const s = [...xs].sort((a, b) => a - b);\n  const m = Math.floor(s.length / 2);\n  return s[m];\n}\n",
				"test.mjs":  "import assert from 'node:assert';\nimport { median } from './median.js';\nassert.strictEqual(median([1, 2, 3, 4]), 2.5);\nassert.strictEqual(median([5]), 5);\nconsole.log('ok');\n",
			},
			Check:   []string{"node", "test.mjs"},
			BudgetS: 120,
		},
		{
			ID: "AT4", Title: "sort script output", Lang: "shell",
			Prompt: "Fix sortnums.sh so it prints the numbers of nums.txt sorted ascending, one per line.",
			Files: map[string]string{
				"nums.txt":    "3\n1\n2\n",
				"sortnums.sh": "#!/usr/bin/env bash\ncat nums.txt\n",
			},
			// Hidden check lives in the task definition (runner-only):
			// the agent sees nums.txt and sortnums.sh only. It runs
			// with cwd set to the fixture dir so relative paths work.
			Check:   []string{"bash", "-c", "diff <(bash sortnums.sh) <(sort nums.txt)"},
			BudgetS: 60,
		},
		{
			ID: "AT5", Title: "fix Clamp upper bound (holdout)", Lang: "go",
			Prompt: "Fix Clamp so values above hi return hi. Do not change the test file.",
			Files: map[string]string{
				"clamp.go":      "package math2\n\n// Clamp restricts v to [lo, hi].\nfunc Clamp(v, lo, hi int) int {\n\tif v < lo {\n\t\treturn lo\n\t}\n\tif v > hi {\n\t\treturn hi - 1\n\t}\n\treturn v\n}\n",
				"clamp_test.go": "package math2\n\nimport \"testing\"\n\nfunc TestClamp(t *testing.T) {\n\tif got := Clamp(9, 0, 5); got != 5 {\n\t\tt.Fatalf(\"Clamp = %d, want 5\", got)\n\t}\n\tif got := Clamp(-1, 0, 5); got != 0 {\n\t\tt.Fatalf(\"Clamp = %d, want 0\", got)\n\t}\n}\n",
			},
			Check:   []string{"go", "test", "./..."},
			BudgetS: 120,
		},
	}
}

// AgentSplits versions exploration vs holdout task ids.
type AgentSplits struct {
	Version     string   `json:"version"`
	Seed        int64    `json:"seed"`
	Exploration []string `json:"exploration"`
	Holdout     []string `json:"holdout"`
}

// LoadAgentSplits reads and validates a splits file: every id must be
// a known corpus task, exploration and holdout must not overlap, and
// holdout must hold at least 20% of the corpus.
func LoadAgentSplits(path string, corpus []AgentTask) (AgentSplits, error) {
	var sp AgentSplits
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentSplits{}, err
	}
	if len(data) > 64*1024 {
		return AgentSplits{}, fmt.Errorf("splits file too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sp); err != nil {
		return AgentSplits{}, fmt.Errorf("malformed splits: %v", err)
	}
	known := map[string]bool{}
	for _, t := range corpus {
		known[t.ID] = true
	}
	seen := map[string]string{}
	for _, id := range append(append([]string{}, sp.Exploration...), sp.Holdout...) {
		if !known[id] {
			return AgentSplits{}, fmt.Errorf("splits reference unknown task %q", id)
		}
		if prev, dup := seen[id]; dup {
			return AgentSplits{}, fmt.Errorf("task %q in both %s and %s (overlap)", id, prev, splitOf(sp, id))
		}
		seen[id] = splitOf(sp, id)
	}
	if len(sp.Exploration)+len(sp.Holdout) != len(corpus) {
		return AgentSplits{}, fmt.Errorf("splits cover %d of %d tasks", len(sp.Exploration)+len(sp.Holdout), len(corpus))
	}
	if float64(len(sp.Holdout)) < 0.2*float64(len(corpus)) {
		return AgentSplits{}, fmt.Errorf("holdout %d below 20%% of %d", len(sp.Holdout), len(corpus))
	}
	return sp, nil
}

func splitOf(sp AgentSplits, id string) string {
	for _, e := range sp.Exploration {
		if e == id {
			return "exploration"
		}
	}
	return "holdout"
}

// AgentOutcome is one executed task attempt.
type AgentOutcome struct {
	TaskID      string `json:"task_id"`
	Split       string `json:"split"`
	Rep         int    `json:"rep"`
	Seed        int64  `json:"seed"`
	Variant     string `json:"variant"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	TokensIn    int64  `json:"tokens_in"`
	TokensOut   int64  `json:"tokens_out"`
	TokenSource string `json:"token_source"`
	LatencyMS   int64  `json:"latency_ms"`
	Model       string `json:"model"`
	Host        string `json:"host"`
	HostVersion string `json:"host_version"`
}

// HeadlessAgent runs one task prompt inside an already-materialized
// fixture repo and reports the raw output plus any host usage.
type HeadlessAgent interface {
	Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error)
}

// ErrNoAgent reports a missing or unusable headless host: the suite
// degrades to NOT RUN, never to a false pass.
type ErrNoAgent struct{ Reason string }

func (e ErrNoAgent) Error() string { return "no headless agent: " + e.Reason }

// AgentOptions bounds one suite run.
type AgentOptions struct {
	Reps        int
	Seeds       []int64
	Variant     string
	VariantNote string
	Model       string
	Host        string
	HostVersion string
	Split       string // exploration|holdout|all
	BudgetS     int
}

// RunAgentSuite executes the corpus tasks with the given agent: one
// fixture repo per attempt, hidden check in a sibling dir, timeout per
// task budget. Infra errors, timeouts and non-zero checks never pass.
func RunAgentSuite(ctx context.Context, agent HeadlessAgent, corpus []AgentTask, splits AgentSplits, opts AgentOptions) []AgentOutcome {
	byID := map[string]AgentTask{}
	for _, t := range corpus {
		byID[t.ID] = t
	}
	var ids []string
	for _, id := range append(append([]string{}, splits.Exploration...), splits.Holdout...) {
		if opts.Split != "" && opts.Split != "all" && splitOf(splits, id) != opts.Split {
			continue
		}
		ids = append(ids, id)
	}
	var out []AgentOutcome
	for _, id := range ids {
		t := byID[id]
		for rep := 0; rep < opts.Reps; rep++ {
			seed := splits.Seed + int64(rep)
			if rep < len(opts.Seeds) {
				seed = opts.Seeds[rep]
			}
			out = append(out, runOne(ctx, agent, t, splitOf(splits, id), rep, seed, opts))
		}
	}
	return out
}

func runOne(ctx context.Context, agent HeadlessAgent, t AgentTask, split string, rep int, seed int64, opts AgentOptions) AgentOutcome {
	oc := AgentOutcome{TaskID: t.ID, Split: split, Rep: rep, Seed: seed, Variant: opts.Variant,
		Model: opts.Model, Host: opts.Host, HostVersion: opts.HostVersion, TokenSource: trace.SourceUnknown}
	work, err := os.MkdirTemp("", "agent-task-")
	if err != nil {
		oc.Status = AgentInfra
		oc.Detail = "mktemp: " + err.Error()
		return oc
	}
	defer os.RemoveAll(work)
	fix := filepath.Join(work, "repo")
	if err := os.MkdirAll(fix, 0o755); err != nil {
		oc.Status = AgentInfra
		oc.Detail = err.Error()
		return oc
	}
	for name, body := range t.Files {
		p := filepath.Join(fix, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			oc.Status = AgentInfra
			oc.Detail = err.Error()
			return oc
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			oc.Status = AgentInfra
			oc.Detail = err.Error()
			return oc
		}
	}
	if t.Lang == "go" {
		if err := os.WriteFile(filepath.Join(fix, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
			oc.Status = AgentInfra
			oc.Detail = err.Error()
			return oc
		}
	}
	budget := t.BudgetS
	if opts.BudgetS > 0 && opts.BudgetS < budget {
		budget = opts.BudgetS
	}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(budget)*time.Second)
	defer cancel()
	start := time.Now()
	_, usage, err := agent.Run(tctx, fix, t.Prompt, opts.Model)
	oc.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		if ctxErr := tctx.Err(); ctxErr == context.DeadlineExceeded {
			oc.Status = AgentTimeout
		} else {
			oc.Status = AgentInfra
		}
		oc.Detail = err.Error()
		return oc
	}
	oc.TokensIn, oc.TokensOut = usage.InputTokens, usage.OutputTokens
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		oc.TokenSource = trace.SourceMeasuredHost
	}
	cmd := exec.CommandContext(ctx, t.Check[0], t.Check[1:]...)
	cmd.Dir = fix
	out, err := cmd.CombinedOutput()
	oc.Detail = strings.TrimSpace(string(out))
	if len(oc.Detail) > 2000 {
		oc.Detail = oc.Detail[:2000]
	}
	if err != nil {
		oc.Status = AgentFail
		return oc
	}
	oc.Status = AgentPass
	return oc
}

// ToExplorationOutcomes maps suite outcomes onto the robust
// evaluator's input so the NOV-59 guardrails judge agent results
// directly: pass/fail stay evaluable, timeout/infra/non-evaluable
// dilute nothing.
func ToExplorationOutcomes(prefix string, outcomes []AgentOutcome) []evolve.ExplorationOutcome {
	var out []evolve.ExplorationOutcome
	for _, o := range outcomes {
		status := evolve.ExploreNonEvaluable
		switch o.Status {
		case AgentPass:
			status = evolve.ExplorePass
		case AgentFail:
			status = evolve.ExploreFail
		case AgentTimeout:
			status = evolve.ExploreTimeout
		case AgentInfra:
			status = evolve.ExploreInfra
		}
		out = append(out, evolve.ExplorationOutcome{
			ID: prefix + "/" + o.TaskID, Status: status,
			Detail: o.Detail, Tokens: o.TokensIn + o.TokensOut,
		})
	}
	return out
}

// AgentSummary folds one variant's outcomes: pass-rate over evaluable
// only, rep spread, tokens, latency. Buckets stay separate; unknown
// tokens never sum as measured.
type AgentSummary struct {
	Variant   string
	Total     int
	Evaluable int
	Pass      int
	Fail      int
	Timeout   int
	Infra     int
	NotRun    int
	PassRate  float64
	RepMin    float64
	RepMax    float64
	TokensIn  int64
	TokensOut int64
	LatencyMS int64
}

// SummarizeAgent folds outcomes deterministically. RepMin/RepMax are
// the worst/best task pass-rate across reps (variability signal); with
// fewer than RobustSignificanceFloor evaluable attempts the caller
// must not claim significance.
func SummarizeAgent(variant string, outcomes []AgentOutcome) AgentSummary {
	s := AgentSummary{Variant: variant}
	byTaskRep := map[string]map[int]bool{}
	for _, o := range outcomes {
		s.Total++
		switch o.Status {
		case AgentPass:
			s.Pass++
			s.Evaluable++
		case AgentFail:
			s.Fail++
			s.Evaluable++
		case AgentTimeout:
			s.Timeout++
		case AgentInfra:
			s.Infra++
		default:
			s.NotRun++
		}
		s.TokensIn += o.TokensIn
		s.TokensOut += o.TokensOut
		s.LatencyMS += o.LatencyMS
		if o.Status == AgentPass || o.Status == AgentFail {
			m := byTaskRep[o.TaskID]
			if m == nil {
				m = map[int]bool{}
				byTaskRep[o.TaskID] = m
			}
			m[o.Rep] = o.Status == AgentPass
		}
	}
	if s.Evaluable > 0 {
		s.PassRate = float64(s.Pass) / float64(s.Evaluable)
	}
	first := true
	for _, reps := range byTaskRep {
		r := 0.0
		for _, p := range reps {
			if p {
				r++
			}
		}
		r /= float64(len(reps))
		if first || r < s.RepMin {
			s.RepMin = r
		}
		if first || r > s.RepMax {
			s.RepMax = r
		}
		first = false
	}
	return s
}

// SortedAgentTasks returns the corpus in stable ID order.
func SortedAgentTasks(corpus []AgentTask) []AgentTask {
	out := append([]AgentTask{}, corpus...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
