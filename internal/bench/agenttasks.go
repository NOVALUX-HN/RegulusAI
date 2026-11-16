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

	"regulus-ai/internal/executil"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/evolve"
	"regulus-ai/internal/trace"
)

// AgentSuiteVersion pins the task corpus: reports compare only within
// one version. Adding a task bumps the version and re-baselines.
// v3 expands the v2 set (36 tasks) with 8 hard tasks (NOV-77): 34
// exploration + 10 holdout, same splits validation and evidence pipeline.
const AgentSuiteVersion = "agent-tasks-v3"

// AgentSplitsPath is the tracked exploration/holdout split (committed,
// not runtime state): holdout stays >= 20% with no repo/pattern
// overlap with exploration.
const AgentSplitsPath = "bench/agent-splits-v3.json"

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

func splitOf(sp AgentSplits, id string) string { return SplitOf(sp, id) }

// AgentOutcome is one executed task attempt.
type AgentOutcome struct {
	TaskID      string `json:"task_id"`
	Split       string `json:"split"`
	Rep         int    `json:"rep"`
	Seed        int64  `json:"seed"`
	Variant     string `json:"variant"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	Evidence    string `json:"evidence"`
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
	// Progress, when non-nil, is called after each attempt completes.
	// Nil (default) preserves the old silent behavior.
	Progress func(AgentOutcome)
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
			select {
			case <-ctx.Done():
				return out
			default:
			}
			seed := splits.Seed + int64(rep)
			if rep < len(opts.Seeds) {
				seed = opts.Seeds[rep]
			}
			oc := runOne(ctx, agent, t, splitOf(splits, id), rep, seed, opts)
			if opts.Progress != nil {
				// Observable fast-loop: one callback per completed attempt.
				// Nil by default, so existing callers are unaffected.
				opts.Progress(oc)
			}
			out = append(out, oc)
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
	// Anchor the agent: some hosts resolve relative paths against
	// their own startup directory instead of the child cwd. Test
	// files are the spec and must not be modified (gaming voids the
	// run; the runner verifies their hashes below).
	anchored := fmt.Sprintf("Working directory: %s. Read and edit only files inside it. Do not modify test files.\n\n%s", fix, t.Prompt)
	testSnap := snapshotTestFiles(fix, t.Files)
	raw, usage, err := agent.Run(tctx, fix, anchored, opts.Model)
	oc.LatencyMS = time.Since(start).Milliseconds()
	if truncated := strings.TrimSpace(raw); len(truncated) > 4000 {
		oc.Evidence = truncated[:4000]
	} else {
		oc.Evidence = truncated
	}
	if err != nil {
		if ctxErr := tctx.Err(); ctxErr == context.DeadlineExceeded {
			oc.Status = AgentTimeout
		} else {
			oc.Status = AgentInfra
		}
		oc.Detail = err.Error()
		return oc
	}
	if tampered := changedTestFiles(fix, t.Files, testSnap); len(tampered) > 0 {
		oc.Status = AgentFail
		oc.Detail = fmt.Sprintf("gaming: test files must not be modified: %s", strings.Join(tampered, ", "))
		return oc
	}
	oc.TokensIn, oc.TokensOut = usage.InputTokens, usage.OutputTokens
	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		oc.TokenSource = trace.SourceMeasuredHost
	}
	cmd := exec.CommandContext(ctx, t.Check[0], t.Check[1:]...)
	cmd.Dir = fix
	executil.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	oc.Detail = strings.TrimSpace(string(out))
	if len(oc.Detail) > 2000 {
		oc.Detail = oc.Detail[:2000]
	}
	if err != nil {
		if appControlBlocked(oc.Detail) || appControlBlocked(err.Error()) {
			oc.Status = AgentInfra
			oc.Detail = "check could not execute (execution blocked by policy): " + oc.Detail
			return oc
		}
		oc.Status = AgentFail
		return oc
	}
	oc.Status = AgentPass
	return oc
}

// snapshotTestFiles records the content of shipped test files so the
// runner can tell a real fix from gaming: modifying a spec file voids
// the run with AgentFail instead of letting doctored tests pass.
func snapshotTestFiles(dir string, files map[string]string) map[string]string {
	snap := map[string]string{}
	for name := range files {
		if !isTestFile(name) {
			continue
		}
		if raw, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			snap[name] = string(raw)
		}
	}
	return snap
}

// changedTestFiles lists shipped test files whose content no longer
// matches the snapshot (modified or deleted).
func changedTestFiles(dir string, files map[string]string, snap map[string]string) []string {
	var out []string
	for name, want := range snap {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(raw) != want {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// isTestFile reports whether a fixture path is a test file: the spec
// the agent must satisfy, never rewrite.
func isTestFile(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	return strings.Contains(base, "test")
}

// appControlBlocked reports the Windows App Control execution block
// (NOV-43): the check never ran, so its failure is infrastructure,
// never a task fail. It matches the Spanish and English block
// wordings without executing anything.
func appControlBlocked(out string) bool {
	low := strings.ToLower(out)
	return strings.Contains(low, "control de aplicaciones") || strings.Contains(out, "blocked this file")
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
