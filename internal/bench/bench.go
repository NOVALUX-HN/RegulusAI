// Package bench owns the macizo self-benchmark (chart macizo-flow/F4):
// a fixed suite of 5-10 micro-tasks with hidden checks plus an offline
// runner. The worker sees only the prompt (Title + Prompt); the runner
// knows the answer (the check function). Every check is machine-checked
// (PASS/FAIL, no judgment) and runs offline: no network, no model, no
// tokens — wall time is the honest proxy cost (F1: the host exposes no
// token counts).
//
// The task list is fixed (v1, 7 tasks) so a baseline report and a
// candidate report stay comparable task by task: future ADOPT/REJECT
// decisions read the contrast table, not opinions.
package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/crap"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
	"regulus-ai/internal/trace"
)

// SuiteVersion pins the task list: reports compare only within one
// version. Adding a task bumps the version and re-baselines.
const SuiteVersion = "macizo-bench-v1"

// BaselinePath is the tracked baseline report (committed, not runtime
// state): the current tree's numbers future surfaces contrast against.
const BaselinePath = "bench/baseline.json"

// Task is one micro-task: the visible prompt plus the hidden check the
// runner — and never the worker — evaluates.
type Task struct {
	ID     string
	Title  string
	Kind   string
	Prompt string
	Check  func() (string, error)
}

// TaskResult is one executed check: PASS/FAIL plus the honest proxy
// cost (wall time; the host exposes no tokens).
type TaskResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Pass       bool   `json:"pass"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail"`
}

// Report is one suite run against one surface (baseline tree or
// candidate): comparable task by task.
type Report struct {
	Suite       string       `json:"suite"`
	Surface     string       `json:"surface"`
	GitHead     string       `json:"git_head"`
	StartedAt   time.Time    `json:"started_at"`
	Tasks       []TaskResult `json:"tasks"`
	Passed      int          `json:"passed"`
	Failed      int          `json:"failed"`
	TotalMS     int64        `json:"total_duration_ms"`
	OutputBytes int64        `json:"output_bytes"`
}

// Row is one contrast line: the same task on both surfaces.
type Row struct {
	ID      string
	Title   string
	Base    bool
	Cand    bool
	DeltaMS int64
}

// Comparison is the machine verdict over two reports: ADOPT only when
// the candidate regresses nothing and keeps the pass count.
type Comparison struct {
	Base         Report
	Candidate    Report
	Rows         []Row
	Regressions  int
	Improvements int
	Verdict      string
	Reason       string
}

// Suite returns the fixed v1 task list (7 tasks, within the 5-10 band):
// representative of repo work (COVER formula, SPLIT-bounded parse,
// prose spec, veracity round-trip, gate selection, security jail,
// honest-zero cost) with hidden machine checks.
func Suite() []Task {
	return []Task{
		{ID: "B1", Title: "cover-crap-formula", Kind: "cover",
			Prompt: "Recompute the CRAP score of a function from its complexity and coverage.",
			Check:  checkCrapFormula},
		{ID: "B2", Title: "split-bounded-parse", Kind: "split",
			Prompt: "Parse a two-row Tasks table into bounded delegation units.",
			Check:  checkSplitBounded},
		{ID: "B3", Title: "prose-spec-gwt", Kind: "prose",
			Prompt: "Judge whether an acceptance scenario states Given/When/Then.",
			Check:  checkSpecGWT},
		{ID: "B4", Title: "veracity-receipt-roundtrip", Kind: "veracity",
			Prompt: "Write one evidence receipt and read it back unchanged.",
			Check:  checkReceiptRoundTrip},
		{ID: "B5", Title: "gate-selection", Kind: "gates",
			Prompt: "Select which gates run under the quick profile.",
			Check:  checkGateSelection},
		{ID: "B6", Title: "security-evidence-jail", Kind: "security",
			Prompt: "Refuse a task id that escapes the evidence directory.",
			Check:  checkEvidenceJail},
		{ID: "B7", Title: "honest-zero-cost", Kind: "cost",
			Prompt: "Report trace cost honestly when the host exposes no tokens.",
			Check:  checkHonestZero},
	}
}

// Run executes the suite against one surface, timing each check. It
// never touches the network: every check is a local pure-or-tempdir
// predicate.
func Run(surface, gitHead string) Report {
	tasks := Suite()
	rep := Report{
		Suite:     SuiteVersion,
		Surface:   surface,
		GitHead:   gitHead,
		StartedAt: time.Now().UTC(),
	}
	for _, t := range tasks {
		start := time.Now()
		detail, err := t.Check()
		ms := time.Since(start).Milliseconds()
		res := TaskResult{
			ID: t.ID, Title: t.Title, Kind: t.Kind,
			Pass: err == nil, DurationMS: ms, Detail: detail,
		}
		if err != nil {
			res.Detail = err.Error()
		}
		rep.Tasks = append(rep.Tasks, res)
		rep.TotalMS += ms
		rep.OutputBytes += int64(len(res.Detail))
		if res.Pass {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	return rep
}

// Save writes one report as indented JSON (temp+rename: a torn
// baseline is worse than none).
func Save(path string, rep Report) error {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "report-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// Load reads one report written by Save.
func Load(path string) (Report, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	if err := json.Unmarshal(raw, &rep); err != nil {
		return Report{}, fmt.Errorf("%s: malformed bench report: %v", path, err)
	}
	return rep, nil
}

// Compare contrasts a candidate against the baseline task by task.
// ADOPT needs zero regressions and no pass-count drop; anything else
// is REJECT with the regressed ids named. Duration deltas are
// reported, never judged (wall-clock on one host, no normalization).
func Compare(base, cand Report) Comparison {
	byID := map[string]TaskResult{}
	for _, t := range cand.Tasks {
		byID[t.ID] = t
	}
	c := Comparison{Base: base, Candidate: cand}
	for _, b := range base.Tasks {
		r := Row{ID: b.ID, Title: b.Title, Base: b.Pass}
		if cr, ok := byID[b.ID]; ok {
			r.Cand = cr.Pass
			r.DeltaMS = cr.DurationMS - b.DurationMS
		}
		c.Rows = append(c.Rows, r)
		switch {
		case b.Pass && !r.Cand:
			c.Regressions++
		case !b.Pass && r.Cand:
			c.Improvements++
		}
	}
	switch {
	case c.Regressions > 0:
		c.Verdict = "REJECT"
		c.Reason = fmt.Sprintf("%d regressed task(s): candidate drops baseline PASS", c.Regressions)
	case cand.Passed < base.Passed:
		c.Verdict = "REJECT"
		c.Reason = fmt.Sprintf("pass count dropped %d -> %d", base.Passed, cand.Passed)
	default:
		c.Verdict = "ADOPT"
		c.Reason = fmt.Sprintf("no regressions, %d/%d pass (baseline %d/%d)", cand.Passed, len(cand.Tasks), base.Passed, len(base.Tasks))
	}
	return c
}

// checkCrapFormula pins the CRAP formula on three points (hidden: the
// worker prompt names no numbers).
func checkCrapFormula() (string, error) {
	cases := []struct {
		comp int
		cov  float64
		want float64
	}{
		{5, 1.0, 5},
		{1, 0.0, 2},
		{2, 0.5, 2.5},
	}
	for _, tc := range cases {
		if got := crap.CRAP(tc.comp, tc.cov); math.Abs(got-tc.want) > 1e-9 {
			return "", fmt.Errorf("CRAP(%d, %.1f) = %v, want %v", tc.comp, tc.cov, got, tc.want)
		}
	}
	return "formula holds on 3 points", nil
}

// splitFixture is the visible two-row table; the row count and routes
// below stay hidden from the worker prompt.
func splitFixture() string {
	return "## Tasks\n\n" +
		"| ID | Spec | Work | Route |\n" +
		"|----|------|------|-------|\n" +
		"| B-a | S1 | one unit | inline |\n" +
		"| B-b | S2 | other unit | @regulus-worker |\n"
}

// checkSplitBounded parses the fixture: exactly two units, no
// violations, second unit carrying its Spec context.
func checkSplitBounded() (string, error) {
	tasks, violations := sextant.ParseTasks(splitFixture())
	if len(violations) != 0 {
		return "", fmt.Errorf("unexpected violations: %s", strings.Join(violations, "; "))
	}
	if len(tasks) != 2 {
		return "", fmt.Errorf("parsed %d tasks, want 2", len(tasks))
	}
	if tasks[1].Route != "@regulus-worker" || tasks[1].Spec == "" {
		return "", fmt.Errorf("second unit lost its route/spec context")
	}
	return "2 bounded units, second carries spec", nil
}

// checkSpecGWT accepts a complete scenario and refuses one missing
// its Then step.
func checkSpecGWT() (string, error) {
	good := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n"
	if n, errs := spec.Check(good); n != 1 || len(errs) != 0 {
		return "", fmt.Errorf("good scenario: got %d scenario(s), %d violation(s)", n, len(errs))
	}
	bad := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n"
	if _, errs := spec.Check(bad); len(errs) == 0 {
		return "", fmt.Errorf("bad scenario accepted: missing Then not refused")
	}
	return "good accepted, Then-less refused", nil
}

// checkReceiptRoundTrip writes one receipt to a temp dir and reads it
// back: id assigned, full-output SHA pinned, stored-text SHA present.
func checkReceiptRoundTrip() (string, error) {
	dir, err := os.MkdirTemp("", "bench-receipt-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	body := "bench veracity body"
	text, sha := receipts.Capture(body)
	path, err := receipts.Write(dir, receipts.Receipt{
		Task: "bench/B4", Gate: "veracity", Layer: 1, Command: "bench",
		ExitCode: 0, Verdict: receipts.VerdictPass,
		StartedAt: time.Now().UTC(), DurationMS: 3,
		Stdout: text, StdoutSHA: sha,
	})
	if err != nil {
		return "", err
	}
	back, err := receipts.ReadFile(path)
	if err != nil {
		return "", err
	}
	if back.ID == "" || back.Stdout != text {
		return "", fmt.Errorf("round-trip lost id or body")
	}
	if back.StdoutSHA != sha || back.StdoutStoredSHA != receipts.StoredSHA(text) {
		return "", fmt.Errorf("SHA pins missing after round-trip")
	}
	return "receipt byte-identical with SHA pins", nil
}

// boolPtr marks one gate optional: the quick profile must skip it.
func boolPtr(b bool) *bool { return &b }

// checkGateSelection pins profile selection: full runs everything,
// quick keeps required gates only, unknown profiles error out.
func checkGateSelection() (string, error) {
	cfg := gates.Config{Gates: []gates.Gate{
		{Name: "typecheck", Layer: 1, Command: "echo hi"},
		{Name: "slow", Layer: 2, Command: "echo slow", Required: boolPtr(false), Profiles: []string{"slow"}},
	}}
	full, err := gates.SelectGates(cfg, "")
	if err != nil || len(full) != 2 {
		return "", fmt.Errorf("full profile: got %d gates (%v)", len(full), err)
	}
	quick, err := gates.SelectGates(cfg, "quick")
	if err != nil || len(quick) != 1 || quick[0].Name != "typecheck" {
		return "", fmt.Errorf("quick profile: got %+v (%v)", quick, err)
	}
	if _, err := gates.SelectGates(cfg, "nope"); err == nil {
		return "", fmt.Errorf("unknown profile accepted")
	}
	return "full=2, quick=1 required, unknown refused", nil
}

// checkEvidenceJail accepts a scoped task and refuses a traversal.
func checkEvidenceJail() (string, error) {
	if _, err := evidence.TaskDirSafe(".", "ok/T1"); err != nil {
		return "", fmt.Errorf("scoped task refused: %v", err)
	}
	if _, err := evidence.TaskDirSafe(".", "../evil"); err == nil {
		return "", fmt.Errorf("traversal accepted")
	}
	return "scoped accepted, traversal refused", nil
}

// checkHonestZero folds one unattributed-cost entry: calls counted,
// tokens honestly unknown (never estimated to zero).
func checkHonestZero() (string, error) {
	root, err := os.MkdirTemp("", "bench-cost-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	empty, err := trace.CostByTask(root)
	if err != nil || len(empty) != 0 {
		return "", fmt.Errorf("empty root: got %d buckets (%v)", len(empty), err)
	}
	if _, err := trace.Record(root, trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t",
		Status: trace.StatusCompleted, Task: "bench/B7",
	}, time.Now().UTC()); err != nil {
		return "", err
	}
	costs, err := trace.CostByTask(root)
	if err != nil || len(costs) != 1 {
		return "", fmt.Errorf("one entry: got %d buckets (%v)", len(costs), err)
	}
	got := costs[0]
	if got.Calls != 1 || got.Task != "bench/B7" {
		return "", fmt.Errorf("bucket miscounted: %+v", got)
	}
	if got.TokensKnown || got.TotalInputTokens != 0 || got.TotalOutputTokens != 0 {
		return "", fmt.Errorf("tokens not honestly unknown: %+v", got)
	}
	return "1 call counted, tokens unknown (host exposes none)", nil
}
