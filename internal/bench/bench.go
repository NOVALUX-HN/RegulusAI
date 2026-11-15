// Package bench owns the macizo self-benchmark (chart macizo-flow/F4):
// 8 behavior tasks with hidden checks plus reported-never-judged cost
// tasks and an offline runner. The worker sees only the prompt (Title + Prompt); the runner
// knows the answer (the check function). Every check is machine-checked
// (PASS/FAIL, no judgment) and runs offline: no network, no model —
// wall time is the proxy cost plus the measured-local token proxy
// (chars/4 per trace entry; the host exposes no token counts, so host
// totals stay honestly unknown).
//
// The task list is fixed (v2, 10 tasks) so a baseline report and a
// candidate report stay comparable task by task: future ADOPT/REJECT
// decisions read the contrast table, not opinions.
package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SuiteVersion pins the task list: reports compare only within one
// version. Adding a task bumps the version and re-baselines.

// BaselinePath is the tracked baseline report (committed, not runtime
// state): the current tree's numbers future surfaces contrast against.
const SuiteVersion = "macizo-bench-v3"
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
// cost (wall time plus the measured-local token proxy; host token
// counts stay unknown).
type TaskResult struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Kind        string `json:"kind"`
	Pass        bool   `json:"pass"`
	DurationMS  int64  `json:"duration_ms"`
	Detail      string `json:"detail"`
	ProxyTokens int64  `json:"proxy_tokens"`
	ProxyKnown  bool   `json:"proxy_known"`
}

// Report is one suite run against one surface (baseline tree or
// candidate): comparable task by task.
type Report struct {
	Suite            string       `json:"suite"`
	Surface          string       `json:"surface"`
	GitHead          string       `json:"git_head"`
	StartedAt        time.Time    `json:"started_at"`
	Tasks            []TaskResult `json:"tasks"`
	Passed           int          `json:"passed"`
	Failed           int          `json:"failed"`
	TotalMS          int64        `json:"total_duration_ms"`
	OutputBytes      int64        `json:"output_bytes"`
	TotalProxyTokens int64        `json:"total_proxy_tokens"`
	ProxyKnown       bool         `json:"proxy_known"`
	QualityPerKProxy float64      `json:"quality_per_kproxy"`
}

// Row is one contrast line: the same task on both surfaces.
type Row struct {
	ID         string
	Title      string
	Base       bool
	Cand       bool
	DeltaMS    int64
	DeltaProxy int64
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
	BaseProxy    int64
	CandProxy    int64
	DeltaProxy   int64
}

// Suite returns the fixed v3 task list (14 tasks: 8 behavior tasks
// within the 5-14 band, plus reported-never-judged cost tasks):
// representative of repo work (COVER formula, SPLIT-bounded parse,
// prose spec, veracity round-trip, gate selection, security jail,
// honest-zero cost) with hidden machine checks, plus the P1 golden
// additions: real retrieval over a golden corpus (B8), the real
// fifteen-gate profile contract (B9), and golden T1 token-proxy values
// (B10). B1–B7 keep their IDs, titles, kinds and prompts so the v1
// baseline rows stay comparable.
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
			Prompt: "Select the lean quick set and the full and quick profiles over the real fifteen-gate gauntlet.",
			Check:  checkGateSelection},
		{ID: "B6", Title: "security-evidence-jail", Kind: "security",
			Prompt: "Refuse a task id that escapes the evidence directory.",
			Check:  checkEvidenceJail},
		{ID: "B7", Title: "honest-zero-cost", Kind: "cost",
			Prompt: "Report trace cost honestly when the host exposes no tokens.",
			Check:  checkHonestZero},
		{ID: "B8", Title: "retrieval-golden-roundtrip", Kind: "retrieval",
			Prompt: "Persist three harbor field notes in a fresh local memory, then surface the note about restoring service after a failed deploy.",
			Check:  checkRetrievalGolden},
		{ID: "B9", Title: "gates-real-profile", Kind: "gates",
			Prompt: "Select the full and quick gate profiles over a realistic fifteen-gate gauntlet and refuse unknown profiles.",
			Check:  checkRealGateProfile},
		{ID: "B10", Title: "token-proxy-golden", Kind: "tokens",
			Prompt: "Measure golden token estimates over fixed trace entries and fold them into per-task totals.",
			Check:  checkTokenProxyGolden},
		{ID: "B11", Title: "lesson-context-cost", Kind: "tokens",
			Prompt: "Assemble one Almanac resume block over a fixed fixture store and report its token-proxy cost.",
			Check:  checkLessonContextCost},
		{ID: "B12", Title: "decision-scoring-golden", Kind: "decision",
			Prompt: "Rate a structured choice from its support, open objections, priced options, and safety nets.",
			Check:  checkDecisionScoringGolden},
		{ID: "B13", Title: "gate-profile-decision", Kind: "decision",
			Prompt: "Pick the lean gate set for a docs-only edit and the full set when gate code moves.",
			Check:  checkGateProfileDecision},
		{ID: "B14", Title: "missrate-golden", Kind: "retrieval",
			Prompt: "Recall the right field note when the wording shifts or a word is mistyped, and miss honestly when nothing matches.",
			Check:  checkMissRateGolden},
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
		res.ProxyTokens = int64(len(t.Prompt)+len(res.Detail)+3) / 4
		res.ProxyKnown = true
		rep.Tasks = append(rep.Tasks, res)
		rep.TotalMS += ms
		rep.OutputBytes += int64(len(res.Detail))
		rep.TotalProxyTokens += res.ProxyTokens
		if res.Pass {
			rep.Passed++
		} else {
			rep.Failed++
		}
	}
	rep.ProxyKnown = true
	if rep.TotalProxyTokens > 0 {
		rep.QualityPerKProxy = float64(rep.Passed) / (float64(rep.TotalProxyTokens) / 1000.0)
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
	c := Comparison{Base: base, Candidate: cand, BaseProxy: base.TotalProxyTokens, CandProxy: cand.TotalProxyTokens, DeltaProxy: cand.TotalProxyTokens - base.TotalProxyTokens}
	for _, b := range base.Tasks {
		r := Row{ID: b.ID, Title: b.Title, Base: b.Pass}
		if cr, ok := byID[b.ID]; ok {
			r.Cand = cr.Pass
			r.DeltaMS = cr.DurationMS - b.DurationMS
			r.DeltaProxy = cr.ProxyTokens - b.ProxyTokens
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
