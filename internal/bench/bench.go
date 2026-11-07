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
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/crap"
	"regulus-ai/internal/decisions"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
	"regulus-ai/internal/trace"
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

// Suite returns the fixed v3 task list (14 tasks: 8 behavior tasks
// within the 5-14 band, plus reported-never-judged cost tasks):
// representative of repo work (COVER formula, SPLIT-bounded parse,
// prose spec, veracity round-trip, gate selection, security jail,
// honest-zero cost) with hidden machine checks, plus the P1 golden
// additions: real retrieval over a golden corpus (B8), the real
// twelve-gate profile contract (B9), and golden T1 token-proxy values
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
			Prompt: "Select which gates run under the quick profile.",
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
			Prompt: "Select the full and quick gate profiles over a realistic twelve-gate gauntlet and refuse unknown profiles.",
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

// checkSpecGWT accepts a complete scenario and refuses a Then-less one,
// a one-word step (`Given a` states no criterion), and out-of-order
// phases — the P1 hardening, machine-checked.
func checkSpecGWT() (string, error) {
	good := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n- Then it passes\n"
	if n, errs := spec.Check(good); n != 1 || len(errs) != 0 {
		return "", fmt.Errorf("good scenario: got %d scenario(s), %d violation(s)", n, len(errs))
	}
	bad := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- When the check runs\n"
	if _, errs := spec.Check(bad); len(errs) == 0 {
		return "", fmt.Errorf("bad scenario accepted: missing Then not refused")
	}
	thin := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a\n- When the check runs\n- Then it passes\n"
	if _, errs := spec.Check(thin); len(errs) == 0 {
		return "", fmt.Errorf("thin scenario accepted: one-word Given not refused")
	}
	disordered := "# F\n\n## Acceptance\n\n### Scenario: s\n\n- Given a fixture\n- Then it passes\n- When the check runs\n"
	if _, errs := spec.Check(disordered); len(errs) == 0 {
		return "", fmt.Errorf("disordered scenario accepted: Given→When→Then order not enforced")
	}
	return "good accepted, Then-less/single-word/disordered refused", nil
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
// host tokens honestly unknown (never estimated to zero) with the
// measured-local proxy (chars/4) summed alongside.
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
	recorded := trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t",
		Status: trace.StatusCompleted, Task: "bench/B7",
	}
	if _, err := trace.Record(root, recorded, time.Now().UTC()); err != nil {
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
	wantLocal := trace.EstimateLocalTokens(recorded)
	if !got.LocalTokensKnown || got.TotalLocalTokens != wantLocal || got.TotalLocalTokens <= 0 {
		return "", fmt.Errorf("local proxy tokens not measured: %+v (want %d)", got, wantLocal)
	}
	return fmt.Sprintf("1 call counted, host tokens unknown + %d local proxy tokens (chars/4)", got.TotalLocalTokens), nil
}

// checkRetrievalGolden pins the real retrieval path on a golden corpus:
// three field notes in a fresh store, one golden query, exact hits.
// The golden query (AND over two terms) must surface exactly the
// rollback drill; the shared term must hit two notes; an unknown term
// must hit none.
func checkRetrievalGolden() (string, error) {
	root, err := os.MkdirTemp("", "bench-retrieval-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	now := time.Now().UTC()
	notes := []struct{ title, content string }{
		{"harbor rollback drill", "Drill the harbor rollback path until restoring service is routine. Rollback first, questions later."},
		{"harbor deploy checklist", "Checklist before every harbor deploy: freeze, snapshot, canary, watch."},
		{"ledger pruning routine", "Prune the evidence ledger on Fridays; retention never touches quarantine."},
	}
	for _, n := range notes {
		if _, err := almanac.Save(root, &almanac.Observation{
			Type: almanac.TypeDiscovery, Title: n.title, Content: n.content,
		}, now); err != nil {
			return "", err
		}
		now = now.Add(time.Second)
	}
	golden, err := almanac.Search(root, almanac.Query{Terms: []string{"harbor", "rollback"}})
	if err != nil {
		return "", err
	}
	if len(golden) != 1 || golden[0].Title != "harbor rollback drill" {
		return "", fmt.Errorf("golden query: got %d hit(s), want exactly the rollback drill", len(golden))
	}
	harbor, err := almanac.Search(root, almanac.Query{Terms: []string{"harbor"}})
	if err != nil {
		return "", err
	}
	if len(harbor) != 2 {
		return "", fmt.Errorf("shared term: got %d hit(s), want 2", len(harbor))
	}
	none, err := almanac.Search(root, almanac.Query{Terms: []string{"zebracorn"}})
	if err != nil {
		return "", err
	}
	if len(none) != 0 {
		return "", fmt.Errorf("unknown term: got %d hit(s), want 0", len(none))
	}
	return "golden query pins the rollback drill (1/2/0 hits)", nil
}

// realGateNames mirrors the shipped twelve-gate gauntlet in declaration
// order. The mirror is deliberate: the bench stays hermetic (no
// repo-root lookup, green in any temp dir) while pinning the real
// profile contract — full and quick select every required gate in
// order, unknown profiles refuse. Commands are shape-only; selection
// is what the check pins.
func realGateConfig() gates.Config {
	names := []string{"typecheck", "lint", "build", "unit-tests", "coverage-floor", "mutation", "crap", "vulncheck", "spec", "ts-typecheck", "ts-unit-tests", "gofmt"}
	cmds := []string{
		"go build ./...", "go vet ./...", "go build -o bin/regulus ./cmd/regulus",
		"go test ./...", "go tool cover -func .regulus/coverage.out",
		"gremlins unleash ./internal/receipts", "regulus crap --max 30",
		"govulncheck ./...", "regulus spec check charts/",
		"bunx tsc --noEmit", "bun test ./.opencode/lib/", "gofmt -l .",
	}
	cfg := gates.Config{}
	for i, name := range names {
		g := gates.Gate{
			Name: name, Layer: i + 1, Command: cmds[i],
		}
		if name == "mutation" || name == "vulncheck" {
			g.Required = boolPtr(false)
			g.Profiles = []string{"full"}
		}
		if name == "vulncheck" {
			g.Profiles = []string{"full", "security"}
		}
		cfg.Gates = append(cfg.Gates, g)
	}
	return cfg
}

// checkRealGateProfile pins the real gauntlet shape through profile
// selection: full keeps all twelve gates in declaration order with halt
// policy, quick keeps the 10 required gates (mutation + vulncheck are
// full-only), and an unknown profile refuses.
func checkRealGateProfile() (string, error) {
	cfg := realGateConfig()
	full, err := gates.SelectGates(cfg, "")
	if err != nil {
		return "", err
	}
	if len(full) != 12 {
		return "", fmt.Errorf("full profile: got %d gates, want 12", len(full))
	}
	for i, g := range full {
		if g.Name != cfg.Gates[i].Name {
			return "", fmt.Errorf("full profile order broke at %d: got %q, want %q", i, g.Name, cfg.Gates[i].Name)
		}
		if g.Command == "" || g.EffectiveOnFail() != gates.OnFailHalt {
			return "", fmt.Errorf("gate %q lost its command or halt policy", g.Name)
		}
	}
	quick, err := gates.SelectGates(cfg, "quick")
	if err != nil {
		return "", err
	}
	if len(quick) != 10 {
		return "", fmt.Errorf("quick profile: got %d gates, want 10 required", len(quick))
	}
	if _, err := gates.SelectGates(cfg, "nope"); err == nil {
		return "", fmt.Errorf("unknown profile accepted")
	}
	return "full=12, quick=10 required in order, unknown refused", nil
}

// checkTokenProxyGolden pins the T1 local-token proxy on golden values:
// exact chars/4 estimates over fixed entries plus the per-task fold
// (calls, errors, summed proxy) with host tokens honestly unknown.
func checkTokenProxyGolden() (string, error) {
	base := trace.Entry{Session: "s", Message: "hello", Call: "c1", Tool: "read", Task: "bench/B10"}
	// Tool(4) + Message(5) + Call(2) + Task(9) = 20 chars → (20+3)/4 = 5.
	if got := trace.EstimateLocalTokens(base); got != 5 {
		return "", fmt.Errorf("base estimate = %d, want 5", got)
	}
	withOut := base
	withOut.OutputBytes = 12
	// 20 + 12 = 32 chars → (32+3)/4 = 8.
	if got := trace.EstimateLocalTokens(withOut); got != 8 {
		return "", fmt.Errorf("output-bytes estimate = %d, want 8", got)
	}
	withFiles := base
	withFiles.Files = []string{"main.go"}
	// 20 + 7 = 27 chars → (27+3)/4 = 7.
	if got := trace.EstimateLocalTokens(withFiles); got != 7 {
		return "", fmt.Errorf("files estimate = %d, want 7", got)
	}
	if got := trace.EstimateLocalTokens(trace.Entry{}); got != 0 {
		return "", fmt.Errorf("empty entry estimate = %d, want 0", got)
	}
	root, err := os.MkdirTemp("", "bench-token-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	now := time.Now().UTC()
	okEntry := base
	okEntry.Status = trace.StatusCompleted
	if _, err := trace.Record(root, okEntry, now); err != nil {
		return "", err
	}
	errEntry := withOut
	errEntry.Message = "boom"
	errEntry.Call = "c2"
	errEntry.Status = trace.StatusError
	if _, err := trace.Record(root, errEntry, now.Add(time.Second)); err != nil {
		return "", err
	}
	costs, err := trace.CostByTask(root)
	if err != nil || len(costs) != 1 {
		return "", fmt.Errorf("two entries one task: got %d buckets (%v)", len(costs), err)
	}
	got := costs[0]
	// "boom"(4) replaces "hello"(5): 4+4+2+9 = 19 chars + 12 output bytes
	// = 31 → (31+3)/4 = 8. Fold: 5 + 8 = 13.
	if got.Calls != 2 || got.Errors != 1 || got.TotalLocalTokens != 13 {
		return "", fmt.Errorf("fold miscounted: %+v (want 2 calls, 1 error, 13 proxy tokens)", got)
	}
	if got.TokensKnown || got.TotalInputTokens != 0 || got.TotalOutputTokens != 0 {
		return "", fmt.Errorf("tokens not honestly unknown: %+v", got)
	}
	if !got.LocalTokensKnown {
		return "", fmt.Errorf("local proxy not marked measured: %+v", got)
	}
	return "proxy golden 5/8/7, fold 2 calls 1 error 13 tokens, host unknown", nil
}

// checkLessonContextCost pins the deterministic proxy-token price of
// one Almanac resume block over a fixed fixture store (2 notes +
// 1 evidence-gated lesson), assembled exactly like the compaction
// hook (--limit 20 --budget 20). Chars/4 rounded up, the same
// convention as trace.EstimateLocalTokens. The check reports, never
// judges: Fase 4 (pro-harness) diffs this number per admitted lesson
// to ask whether the evolution pays. Hermetic by construction:
// ContextScoped reads only the temp store, never charts or cwd, and
// every timestamp is fixed so the byte count is stable run to run.
func checkLessonContextCost() (string, error) {
	root, err := os.MkdirTemp("", "bench-lesson-cost-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	fixture := []almanac.Observation{
		{Type: almanac.TypeSessionNote, Title: "harbor deploy done", Content: "Deploy finished cleanly; canary watched for ten minutes."},
		{Type: almanac.TypeSessionNote, Title: "rollback drill done", Content: "Rollback drill finished; restoring service is routine now."},
		{Type: almanac.TypeLesson, Title: "harbor rollback first", Content: "When the harbor deploy fails, roll back before investigating.",
			Lesson: &almanac.Lesson{Cue: "harbor deploy fails", Lesson: "Roll back first, questions later.", Evidence: []string{"rec_fixture/1"}, Confidence: "high", Dimension: "dom"}},
	}
	for i, o := range fixture {
		if _, err := almanac.Save(root, &o, now.Add(time.Duration(i)*time.Second)); err != nil {
			return "", err
		}
	}
	ctx, err := almanac.ContextScoped(root, now.Add(3*time.Second), 20, almanac.ContextScope{Budget: 20})
	if err != nil {
		return "", err
	}
	// Lessons render as cue/lesson text (never the observation title):
	// assert on the cue, the most distinctive priced-in string.
	if !strings.Contains(ctx, "harbor deploy fails") {
		return "", fmt.Errorf("fixture lesson missing from the assembled block (%d chars)", len(ctx))
	}
	proxy := (int64(len(ctx)) + 3) / 4
	if proxy <= 0 {
		return "", fmt.Errorf("empty block prices nothing")
	}
	return fmt.Sprintf("fixture lessons priced: 3 observations (2 notes + 1 lesson) assemble %d chars = %d proxy tokens, host unknown", len(ctx), proxy), nil
}

// checkDecisionScoringGolden pins the T4 quantitative decision score on
// its golden values: the same formula decisions.Score computes, so
// score drift breaks the suite from the bench side too.
func checkDecisionScoringGolden() (string, error) {
	cases := []struct {
		in    decisions.ScoreInput
		want  int
		grade string
	}{
		{decisions.ScoreInput{EvidenceOK: true, Blocking: 0, AlternativesPriced: 1, RisksTotal: 1, RisksMitigated: 1}, 94, "A"},
		{decisions.ScoreInput{EvidenceOK: false, Blocking: 3, AlternativesPriced: 0, RisksTotal: 0, RisksMitigated: 0}, 10, "F"},
		{decisions.ScoreInput{EvidenceOK: true, Blocking: 1, AlternativesPriced: 2, RisksTotal: 2, RisksMitigated: 1}, 75, "B"},
	}
	for _, tc := range cases {
		if got, grade := decisions.Score(tc.in); got != tc.want || grade != tc.grade {
			return "", fmt.Errorf("Score(%+v) = %d/%s, want %d/%s", tc.in, got, grade, tc.want, tc.grade)
		}
	}
	return "scoring golden 94/A 10/F 75/B", nil
}

// chooseGateProfile is the cost/precision tradeoff: docs-only edits ride
// the lean quick set, anything else (or a forced full) rides the full
// gauntlet. Empty means full (unknown scope defaults to precision).
func chooseGateProfile(changed []string, forceFull bool) string {
	if forceFull || len(changed) == 0 {
		return ""
	}
	for _, p := range changed {
		if !strings.HasSuffix(p, ".md") {
			return ""
		}
	}
	return "quick"
}

// checkGateProfileDecision pins the tradeoff through real selection:
// docs-only chooses quick (10 required, no full-only gates), gate code
// or forced full chooses the 12-gate gauntlet.
func checkGateProfileDecision() (string, error) {
	cfg := realGateConfig()
	quick, err := gates.SelectGates(cfg, chooseGateProfile([]string{"charts/flow.md", "README.md"}, false))
	if err != nil || len(quick) != 10 {
		return "", fmt.Errorf("docs-only: got %d gates (%v), want 10 quick", len(quick), err)
	}
	for _, g := range quick {
		if g.Name == "mutation" || g.Name == "vulncheck" {
			return "", fmt.Errorf("docs-only quick kept full-only gate %q", g.Name)
		}
	}
	full, err := gates.SelectGates(cfg, chooseGateProfile([]string{"internal/gates/gates.go"}, false))
	if err != nil || len(full) != 12 {
		return "", fmt.Errorf("gate code: got %d gates (%v), want 12 full", len(full), err)
	}
	if got := chooseGateProfile([]string{"charts/flow.md"}, true); got != "" {
		return "", fmt.Errorf("forced full: chose %q, want full", got)
	}
	if got := chooseGateProfile([]string{"charts/flow.md", "internal/x.go"}, false); got != "" {
		return "", fmt.Errorf("mixed scope: chose %q, want full", got)
	}
	return "docs-only edits ride quick=10, gate code rides full=12, forced full honored", nil
}

// missDoc is one eval corpus entry for the miss-rate golden (B14).
type missDoc struct {
	key, title, content string
}

// missQuery is one dup-query: b-side wording plus its target key.
type missQuery struct {
	name, kind, bTitle, bContent, target string
}

// checkMissRateGolden pins the retrieval honesty contract on a fixed
// 5-doc corpus with 4 dup queries (2 paraphrase + 2 typo): the lexical
// AND channel must miss every dup (novel/typo word matches nothing)
// while hybrid top-3 recalls every one. A corpus-wide unknown term
// must return zero hits (honest miss, wired to RecordMiss by the CLI).
func checkMissRateGolden() (string, error) {
	corpus := []missDoc{
		{"pooling", "chose postgres connection pooling", "pooling keeps p95 latency flat under load; sized the pool to twice the worker count"},
		{"leak", "postgres pool leaks idle connections", "the harvester stalls under load and idle connections never return; restart recovers the pool"},
		{"cache", "cache the retrieval index in memory", "sizing the cache to the working set keeps p95 under fifty milliseconds"},
		{"runtime", "chose bun runtime for the plugin host", "cold start under fifty milliseconds keeps the picker responsive"},
		{"trace", "trace sessions prune to the call-order tail", "keeping the last two calls per second keeps the debug trail short"},
	}
	queries := []missQuery{
		{"paraphrase/postgres-pool", "paraphrase", "picked postgres connection pooling", "pooling keeps p95 latency flat under load; sized the pool to twice the worker count", "pooling"},
		{"paraphrase/pool-leak", "paraphrase", "postgres pool leaking idle connections", "the harvester stalls under load and idle connections never return; a restart recovers the pool", "leak"},
		{"typo/pooling", "typo", "chose postgres conenction pooling", "pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count", "pooling"},
		{"typo/cache-sizing", "typo", "cache the retrival index in memory", "sizing the cache to the working set keeps p95 under fifty miliseconds", "cache"},
	}
	root, err := os.MkdirTemp("", "bench-missrate-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	ids := map[string]string{}
	for i, d := range corpus {
		saved, err := almanac.Save(root, &almanac.Observation{Type: almanac.TypeDecision, Title: d.title, Content: d.content}, now.Add(time.Duration(i)*time.Second))
		if err != nil {
			return "", err
		}
		ids[d.key] = saved.ID
	}
	if _, _, err := almanac.Reindex(root); err != nil {
		return "", err
	}
	lexMiss, hybHit, paraHit, typoHit := 0, 0, 0, 0
	for _, mq := range queries {
		terms := benchMissTerms(mq.bTitle, mq.bContent)
		lex, err := almanac.Search(root, almanac.Query{Terms: terms})
		if err != nil {
			return "", err
		}
		found := false
		for _, o := range lex {
			if o.ID == ids[mq.target] {
				found = true
			}
		}
		if !found {
			lexMiss++
		}
		hyb, degraded, err := almanac.HybridSearchWith(root, almanac.Query{Terms: terms, Limit: 3}, "")
		if err != nil {
			return "", err
		}
		if degraded {
			return "", fmt.Errorf("query %q: fresh index must not degrade", mq.name)
		}
		hybFound := false
		for _, o := range hyb {
			if o.ID == ids[mq.target] {
				hybFound = true
			}
		}
		if hybFound {
			hybHit++
			if mq.kind == "paraphrase" {
				paraHit++
			} else {
				typoHit++
			}
		} else {
			return "", fmt.Errorf("query %q (%s): hybrid top-3 misses target (lexMiss=%v)", mq.name, mq.kind, !found)
		}
		if found {
			return "", fmt.Errorf("query %q (%s): lexical AND must miss the dup target", mq.name, mq.kind)
		}
	}
	none, err := almanac.Search(root, almanac.Query{Terms: []string{"zebracorn"}})
	if err != nil {
		return "", err
	}
	if len(none) != 0 {
		return "", fmt.Errorf("unknown term: got %d hits, want 0", len(none))
	}
	if lexMiss != 4 || hybHit != 4 {
		return "", fmt.Errorf("missrate: lexMiss=%d hybHit=%d, want 4/4", lexMiss, hybHit)
	}
	return fmt.Sprintf("missrate 4/4 lexical-miss, 4/4 hybrid top-3 (%d paraphrase + %d typo), unknown 0 hits", paraHit, typoHit), nil
}

// benchMissTerms tokenizes a query side like the eval: lowercase,
// punctuation stripped (matchTerms is substring-per-term, so a typo'd
// or novel word matches nothing — the lexical miss).
func benchMissTerms(title, content string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(title + " " + content)) {
		w := strings.TrimFunc(f, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}
