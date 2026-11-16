// Tiered execution helpers (F11.3/NOV-89): phased timing, budgets,
// classification, independent-only parallelism, wall-vs-aggregate
// cost estimates and tamper-evident tier plans.
//
// The corpus, splits and outcomes stay the single owners (no parallel
// shapes here): this file only measures, budgets, schedules and
// reports selections made by tiers.go.
package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Tier phases: every tiered run splits its wall clock into these six
// buckets so prep/queue/inference/tools/eval/teardown bottlenecks
// stay attributable instead of hiding inside one latency number.
const (
	TierPhasePrep      = "prep"
	TierPhaseQueue     = "queue"
	TierPhaseInference = "inference"
	TierPhaseTools     = "tools"
	TierPhaseEval      = "eval"
	TierPhaseTeardown  = "teardown"
)

// TierPhases holds per-phase milliseconds for one attempt or one
// aggregated run.
type TierPhases struct {
	PrepMS      int64 `json:"prep_ms"`
	QueueMS     int64 `json:"queue_ms"`
	InferenceMS int64 `json:"inference_ms"`
	ToolsMS     int64 `json:"tools_ms"`
	EvalMS      int64 `json:"eval_ms"`
	TeardownMS  int64 `json:"teardown_ms"`
}

// Add records milliseconds on one phase; unknown phases fail closed.
func (p *TierPhases) Add(phase string, ms int64) error {
	switch phase {
	case TierPhasePrep:
		p.PrepMS += ms
	case TierPhaseQueue:
		p.QueueMS += ms
	case TierPhaseInference:
		p.InferenceMS += ms
	case TierPhaseTools:
		p.ToolsMS += ms
	case TierPhaseEval:
		p.EvalMS += ms
	case TierPhaseTeardown:
		p.TeardownMS += ms
	default:
		return fmt.Errorf("unknown tier phase %q (want prep|queue|inference|tools|eval|teardown)", phase)
	}
	return nil
}

// Total sums every phase: the six buckets always reconcile to the
// reported wall clock for the scope they cover.
func (p TierPhases) Total() int64 {
	return p.PrepMS + p.QueueMS + p.InferenceMS + p.ToolsMS + p.EvalMS + p.TeardownMS
}

// SumTierPhases folds per-attempt phases into run totals.
func SumTierPhases(all []TierPhases) TierPhases {
	var tot TierPhases
	for _, p := range all {
		tot.PrepMS += p.PrepMS
		tot.QueueMS += p.QueueMS
		tot.InferenceMS += p.InferenceMS
		tot.ToolsMS += p.ToolsMS
		tot.EvalMS += p.EvalMS
		tot.TeardownMS += p.TeardownMS
	}
	return tot
}

// TierBudgets carries the three budget scopes: per-task ceilings come
// from the corpus fixtures (BudgetS, never raised here), the agent
// scope bounds one agent's attempts, and the run scope bounds the
// whole tier run. Timeouts classify per scope; none are auto-raised.
type TierBudgets struct {
	TaskBudgetS   map[string]int `json:"task_budget_s"`
	AgentBudgetS  int            `json:"agent_budget_s"`
	RunBudgetS    int            `json:"run_budget_s"`
	RateLimitNote string         `json:"rate_limit_note"`
}

// TierBudgetsFor resolves the budgets for one level over the corpus:
// task ceilings stay the fixture pins, agent and run ceilings come
// from the tier spec (run) and twice the slowest task (agent).
func TierBudgetsFor(tier string, corpus []AgentTask) (TierBudgets, error) {
	spec, ok := FindTierSpec(tier)
	if !ok {
		return TierBudgets{}, fmt.Errorf("unknown tier %q (want L0|L1|L2|L3)", tier)
	}
	b := TierBudgets{
		TaskBudgetS:   map[string]int{},
		RunBudgetS:    spec.RunBudgetS,
		RateLimitNote: fmt.Sprintf("max %d concurrent isolated tasks, sequential starts (rate-limit friendly)", TierDefaultConcurrency()),
	}
	slowest := 0
	for _, t := range corpus {
		b.TaskBudgetS[t.ID] = t.BudgetS
		if t.BudgetS > slowest {
			slowest = t.BudgetS
		}
	}
	b.AgentBudgetS = 2 * slowest
	return b, nil
}

// ClassifyTierStatus maps a raw outcome status onto the closed tier
// vocabulary: only pass/fail are evaluable; timeout, infra and
// non-evaluable never pass; anything unknown degrades to
// non-evaluable, never to pass.
func ClassifyTierStatus(status string) string {
	switch status {
	case AgentPass:
		return AgentPass
	case AgentFail:
		return AgentFail
	case AgentTimeout:
		return AgentTimeout
	case AgentInfra:
		return AgentInfra
	case AgentNonEvaluable, AgentNotRun:
		return AgentNonEvaluable
	}
	return AgentNonEvaluable
}

// TierIsEvaluable reports whether a classified status judges quality:
// only pass and fail do.
func TierIsEvaluable(classified string) bool {
	return classified == AgentPass || classified == AgentFail
}

// TierDefaultConcurrency bounds parallel tiered execution: isolated
// tasks only, limited concurrency, sequential starts so host rate
// limits are respected.
func TierDefaultConcurrency() int {
	return 4
}

// TierMaxConcurrency caps the --jobs override: unbounded parallelism
// would break isolation accounting and rate limits.
func TierMaxConcurrency() int {
	return 8
}

// PlanTierBatches schedules independent task ids into batches of at
// most maxConc: batch order is deterministic (sorted ids), every id
// runs exactly once, and the batches bound the wall clock while the
// aggregate stays the sum.
func PlanTierBatches(ids []string, maxConc int) [][]string {
	if maxConc <= 0 {
		maxConc = TierDefaultConcurrency()
	}
	if maxConc > TierMaxConcurrency() {
		maxConc = TierMaxConcurrency()
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	var batches [][]string
	for i := 0; i < len(sorted); i += maxConc {
		end := i + maxConc
		if end > len(sorted) {
			end = len(sorted)
		}
		batches = append(batches, append([]string(nil), sorted[i:end]...))
	}
	return batches
}

// ValidateTierParallel refuses schedules that are not independent and
// isolated: duplicate ids would share one fixture, and concurrency
// outside 1..TierMaxConcurrency breaks the isolation/rate-limit
// contract.
func ValidateTierParallel(ids []string, maxConc int) error {
	if maxConc < 1 || maxConc > TierMaxConcurrency() {
		return fmt.Errorf("tier parallel: concurrency %d outside 1..%d", maxConc, TierMaxConcurrency())
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("tier parallel: duplicate task %q shares one fixture (tasks must be independent)", id)
		}
		seen[id] = true
	}
	return nil
}

// EstimateTierCost compares feedback latency honestly as ceilings:
// the aggregate is the sum of fixture budgets, the wall divides it by
// the parallelism. Ceilings, never predictions: a smaller tier is
// faster because it runs fewer budgets, never because it skips
// quality (the corpus is unchanged).
func EstimateTierCost(tasks []AgentTask, parallelism int) (wallMS, aggregateMS int64) {
	if parallelism <= 0 {
		parallelism = TierDefaultConcurrency()
	}
	for _, t := range tasks {
		aggregateMS += int64(t.BudgetS) * 1000
	}
	if len(tasks) == 0 {
		return 0, 0
	}
	wallMS = (aggregateMS + int64(parallelism) - 1) / int64(parallelism)
	return wallMS, aggregateMS
}

// TierRunHint renders the exact selective-execution command for one
// selection: the existing agent-tasks runner with the --task filter
// so selection executes through the same fixtures, splits and hidden
// checks (no parallel runner, no bypass).
func TierRunHint(sel TierSelection) string {
	split := "exploration"
	if TierIncludesHoldout(sel.Tier) {
		split = "all"
	}
	return fmt.Sprintf("regulus bench agent-tasks --task %s --split %s", strings.Join(sel.Included, ","), split)
}

// TierPlanFilenames are stable inside a tier evidence dir: one plan,
// one manifest. A new plan uses a new directory.
const (
	TierPlanFile     = "tier-plan.json"
	TierPlanManifest = "tier-manifest.json"
)

// TierEvidenceManifest describes one stored tier plan: the config it
// was derived from plus the plan checksum.
type TierEvidenceManifest struct {
	TierVersion string `json:"tier_version"`
	Tier        string `json:"tier"`
	GitHead     string `json:"git_head"`
	StartedAt   string `json:"started_at"`
	PlanFile    string `json:"plan_file"`
	PlanSHA     string `json:"plan_sha256"`
}

// WriteTierEvidence stores one tier plan plus a manifest carrying its
// sha256: reports render strictly from these files, never
// hand-written.
func WriteTierEvidence(dir string, sel TierSelection, gitHead, startedAt string) (string, error) {
	sel.TierVersion = TierVersion
	raw, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, TierPlanFile), append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	m := TierEvidenceManifest{
		TierVersion: TierVersion, Tier: sel.Tier,
		GitHead: gitHead, StartedAt: startedAt,
		PlanFile: TierPlanFile, PlanSHA: hex.EncodeToString(sum[:]),
	}
	mraw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	manPath := filepath.Join(dir, TierPlanManifest)
	if err := os.WriteFile(manPath, append(mraw, '\n'), 0o644); err != nil {
		return "", err
	}
	return manPath, nil
}

// ReadTierEvidence loads one tier evidence dir, verifying the plan
// sha256 first: edited plans fail instead of producing a report from
// a lie.
func ReadTierEvidence(dir string) (TierEvidenceManifest, TierSelection, error) {
	var m TierEvidenceManifest
	var sel TierSelection
	raw, err := os.ReadFile(filepath.Join(dir, TierPlanManifest))
	if err != nil {
		return m, sel, fmt.Errorf("reading tier manifest: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, sel, fmt.Errorf("malformed tier manifest: %v", err)
	}
	if m.PlanFile == "" {
		m.PlanFile = TierPlanFile
	}
	praw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(m.PlanFile)))
	if err != nil {
		return m, sel, fmt.Errorf("reading tier plan: %v", err)
	}
	trimmed := praw
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	sum := sha256.Sum256(trimmed)
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(m.PlanSHA)) {
		return m, sel, fmt.Errorf("tier plan sha256 mismatch: evidence was modified after recording")
	}
	pdec := json.NewDecoder(strings.NewReader(string(praw)))
	pdec.DisallowUnknownFields()
	if err := pdec.Decode(&sel); err != nil {
		return m, sel, fmt.Errorf("malformed tier plan: %v", err)
	}
	return m, sel, nil
}

// RenderTierPlan builds the human selection report strictly from a
// selection: level and reason, executed ids with inclusion reasons,
// omitted ids with reasons, wall-vs-aggregate ceilings, budgets,
// policy and the exact rerun command.
func RenderTierPlan(m TierEvidenceManifest, sel TierSelection, corpus []AgentTask, splits AgentSplits, now time.Time) string {
	byID := map[string]AgentTask{}
	for _, t := range corpus {
		byID[t.ID] = t
	}
	var included []AgentTask
	for _, id := range sel.Included {
		if t, ok := byID[id]; ok {
			included = append(included, t)
		}
	}
	wall, agg := EstimateTierCost(included, TierDefaultConcurrency())
	var b strings.Builder
	fmt.Fprintf(&b, "tier %s (%s suite, splits %s seed %d, corpus %s)\n",
		sel.Tier, sel.SuiteVersion, sel.SplitsVersion, sel.SplitsSeed, shortTierSHA(sel.CorpusSHA))
	fmt.Fprintf(&b, "reason: %s\n", sel.Reason)
	if sel.Capability != "" {
		fmt.Fprintf(&b, "capability filter: %s\n", sel.Capability)
	}
	if len(sel.Changed) > 0 {
		fmt.Fprintf(&b, "changed: %s\n", strings.Join(sel.Changed, ", "))
	}
	fmt.Fprintf(&b, "evidence: %s sha256:%s git %s %s\n",
		m.PlanFile, shortTierSHA(m.PlanSHA), tierHeadOrUnknown(m.GitHead), m.StartedAt)
	fmt.Fprintf(&b, "executed %d:\n", len(sel.Included))
	for _, id := range sel.Included {
		t := byID[id]
		fmt.Fprintf(&b, "  - %s [%s] %s\n", id, SplitOf(splits, id), TierIncludeReason(sel.Tier, t))
	}
	fmt.Fprintf(&b, "omitted %d:\n", len(sel.Omitted))
	for _, o := range sel.Omitted {
		fmt.Fprintf(&b, "  - %s [%s] %s\n", o.ID, o.Split, o.Reason)
	}
	fmt.Fprintf(&b, "feedback ceilings (budgets, not predictions): wall ~%ds at %dx parallel, aggregate ~%ds over %d task(s)\n",
		wall/1000, TierDefaultConcurrency(), agg/1000, len(sel.Included))
	fmt.Fprintf(&b, "phases measured separately at run time: prep/queue/inference/tools/eval/teardown (see docs/eval-tiers.md)\n")
	fmt.Fprintf(&b, "budgets: run <= %ds, agent <= %ds, per-task fixture pins preserved\n",
		tierRunBudget(sel.Tier, corpus), tierAgentBudget(corpus))
	if TierIncludesHoldout(sel.Tier) {
		b.WriteString("holdout: included (L3 full corpus plus holdout)\n")
	} else {
		b.WriteString("holdout: sealed (runs only at L3, periodic or pre-release)\n")
	}
	fmt.Fprintf(&b, "run: %s\n", TierRunHint(sel))
	_ = now
	return b.String()
}

// tierRunBudget resolves the run ceiling for the report.
func tierRunBudget(tier string, corpus []AgentTask) int {
	b, err := TierBudgetsFor(tier, corpus)
	if err != nil {
		return 0
	}
	return b.RunBudgetS
}

// tierAgentBudget resolves the agent ceiling for the report.
func tierAgentBudget(corpus []AgentTask) int {
	b, err := TierBudgetsFor(TierL3, corpus)
	if err != nil {
		return 0
	}
	return b.AgentBudgetS
}

// shortTierSHA renders a checksum prefix for the report.
func shortTierSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// tierHeadOrUnknown renders the git head for the report.
func tierHeadOrUnknown(head string) string {
	if strings.TrimSpace(head) == "" {
		return "unknown"
	}
	return head
}
