// Coverage tests for the orchestration leftovers (F12.1/F12.2):
// descriptor/budget validation, history folding, registry grading,
// scoring factors, evaluation contrasts, DAG validation edges,
// worktree isolation/parsing/orphans and the scheduler seams. All
// in-process with memory fakes on t.TempDir-free inputs: no shells,
// no network, portable Linux+Windows.
package orchestrate

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/trace"
)

// covOps is a memory WorktreeOps with scripted failures. Distinct
// from parallel_test.go's fakeOps: same seam, independent knobs.
type covOps struct {
	mu        sync.Mutex
	branches  map[string]string
	conflicts map[string][]string
	mergeErr  map[string]error
	createErr error
	removeErr error
	deleteErr error
	listErr   error
	pruneErr  error
	removed   []string
	deleted   []string
	pruned    int
}

func newCovOps() *covOps { return &covOps{branches: map[string]string{}} }

func (f *covOps) Create(ctx context.Context, repoRoot, base, branch, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	if _, ok := f.branches[branch]; ok {
		return errors.New("branch " + branch + " already exists")
	}
	f.branches[branch] = path
	return nil
}

func (f *covOps) Remove(ctx context.Context, repoRoot, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, path)
	for b, p := range f.branches {
		if p == path {
			delete(f.branches, b)
		}
	}
	return nil
}

func (f *covOps) DeleteBranch(ctx context.Context, repoRoot, branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, branch)
	delete(f.branches, branch)
	return nil
}

func (f *covOps) List(ctx context.Context, repoRoot string) ([]WorktreeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []WorktreeEntry
	for b, p := range f.branches {
		out = append(out, WorktreeEntry{Path: p, Branch: b, Commit: "cov"})
	}
	return out, nil
}

func (f *covOps) MergeBranch(ctx context.Context, repoRoot, branch, base string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.mergeErr[branch]; ok {
		return nil, err
	}
	return append([]string(nil), f.conflicts[branch]...), nil
}

func (f *covOps) Prune(ctx context.Context, repoRoot string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pruneErr != nil {
		return f.pruneErr
	}
	f.pruned++
	return nil
}

func TestCovValidateDescriptorTable(t *testing.T) {
	base := testDescriptor()
	cases := []struct {
		name   string
		mutate func(*TaskDescriptor)
		want   string
	}{
		{"empty-domain", func(d *TaskDescriptor) { d.Domain = "" }, "domain"},
		{"empty-language", func(d *TaskDescriptor) { d.Language = " " }, "language"},
		{"unknown-complexity", func(d *TaskDescriptor) { d.Complexity = "extreme" }, "unknown complexity"},
		{"unknown-risk", func(d *TaskDescriptor) { d.Risk = "mild" }, "unknown risk"},
		{"long-domain", func(d *TaskDescriptor) { d.Domain = strings.Repeat("a", 65) }, "single short line"},
		{"multiline-domain", func(d *TaskDescriptor) { d.Domain = "a\nb" }, "single short line"},
		{"traversal-domain", func(d *TaskDescriptor) { d.Domain = "a/../b" }, "single short line"},
		{"too-many-constraints", func(d *TaskDescriptor) {
			d.Constraints = make([]string, 33)
			for i := range d.Constraints {
				d.Constraints[i] = "ok"
			}
		}, "at most 32"},
		{"multiline-constraint", func(d *TaskDescriptor) { d.Constraints = []string{"a\nb"} }, "single line"},
		{"long-constraint", func(d *TaskDescriptor) { d.Constraints = []string{strings.Repeat("x", 300)} }, "under 256"},
		{"unknown-capability", func(d *TaskDescriptor) { d.RequiredCapabilities = []string{"teleport"} }, "unknown capability"},
		{"duplicate-capability", func(d *TaskDescriptor) {
			d.RequiredCapabilities = []string{bench.CapTools, bench.CapTools}
		}, "duplicate required capability"},
	}
	for _, c := range cases {
		d := base
		c.mutate(&d)
		if err := ValidateTaskDescriptor(d); err == nil {
			t.Fatalf("%s: must refuse", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: must name %q, got %v", c.name, c.want, err)
		}
	}
	if err := ValidateTaskDescriptor(base); err != nil {
		t.Fatalf("valid descriptor refused: %v", err)
	}
}

func TestCovValidateBudgetsDelegationsWeights(t *testing.T) {
	if err := ValidateBudgets(Budgets{TimeMS: -1}); err == nil {
		t.Fatal("negative time budget must refuse")
	}
	if err := ValidateBudgets(Budgets{Tokens: -2}); err == nil {
		t.Fatal("negative token budget must refuse")
	}
	if err := ValidateBudgets(Budgets{Cost: -0.5}); err == nil {
		t.Fatal("negative cost budget must refuse")
	}
	if err := ValidateBudgets(Budgets{}); err != nil {
		t.Fatalf("zero budgets are unset, not hostile: %v", err)
	}
	if err := ValidateMaxDelegations(-1); err == nil {
		t.Fatal("negative delegations must refuse")
	}
	if err := ValidateMaxDelegations(5); err == nil {
		t.Fatal("delegations over 4 must refuse")
	}
	if err := ValidateMaxDelegations(4); err != nil {
		t.Fatalf("4 delegations are the ceiling, not hostile: %v", err)
	}
	w := DefaultScoreWeights()
	if err := w.validate(); err != nil {
		t.Fatalf("default weights must sum to 1: %v", err)
	}
	bad := w
	bad.Capability = 0.9
	if err := bad.validate(); err == nil {
		t.Fatal("weights over 1 must refuse")
	} else if !strings.Contains(err.Error(), "sum to 1") {
		t.Fatalf("must name the sum rule, got %v", err)
	}
	neg := w
	neg.Cost = -0.1
	neg.Latency = 0.25
	if err := neg.validate(); err == nil {
		t.Fatal("negative weights must refuse")
	}
}

func TestCovValidateParallelBudgetsTable(t *testing.T) {
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 0, MaxQueued: 1}, 1); err == nil {
		t.Fatal("concurrency 0 must refuse")
	}
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 9, MaxQueued: 64}, 1); err == nil {
		t.Fatal("concurrency 9 must refuse")
	}
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64, PerTaskTimeoutMS: -1}, 1); err == nil {
		t.Fatal("negative per-task timeout must refuse")
	}
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64, WallBudgetMS: -1}, 1); err == nil {
		t.Fatal("negative wall budget must refuse")
	}
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 2, MaxQueued: -1}, 1); err == nil {
		t.Fatal("negative queue bound must refuse")
	}
	if err := ValidateParallelBudgets(ParallelBudgets{MaxConcurrency: 2, MaxQueued: 1}, 2); err == nil {
		t.Fatal("over-queued plans must refuse")
	} else if !strings.Contains(err.Error(), "queue bound") {
		t.Fatalf("must name the queue bound, got %v", err)
	}
	if err := ValidateParallelBudgets(DefaultParallelBudgets(), 4); err != nil {
		t.Fatalf("default budgets must pass: %v", err)
	}
	def := DefaultParallelBudgets()
	if def.MaxConcurrency != 4 || def.MaxQueued != MaxParallelTasks {
		t.Fatalf("default policy drifted: %+v", def)
	}
}

func TestCovEstimateParallelCost(t *testing.T) {
	wall, agg := EstimateParallelCost(nil, 0)
	if wall != 0 || agg != 0 {
		t.Fatalf("empty wave has no ceiling, got %d/%d", wall, agg)
	}
	tasks := []ParallelTask{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}
	if wall, agg := EstimateParallelCost(tasks, 0); wall != 0 || agg != 0 {
		t.Fatalf("all-zero estimates read unknown, got %d/%d", wall, agg)
	}
	est := []ParallelTask{
		{ID: "a", Title: "A", EstimateMS: 1000},
		{ID: "b", Title: "B", EstimateMS: 1000},
	}
	if wall, agg := EstimateParallelCost(est, 0); wall != 500 || agg != 2000 {
		t.Fatalf("zero parallelism defaults to 4: wall=%d agg=%d", wall, agg)
	}
	if wall, agg := EstimateParallelCost(est, 2); wall != 1000 || agg != 2000 {
		t.Fatalf("wall divides the aggregate: wall=%d agg=%d", wall, agg)
	}
}

func TestCovValidParallelIDTable(t *testing.T) {
	for _, id := range []string{"a", "A1", "a.b_c-d/e", "1a", strings.Repeat("x", 128)} {
		if !validParallelID(id) {
			t.Fatalf("valid id %q refused", id)
		}
	}
	for _, id := range []string{"", strings.Repeat("x", 129), "a..b", "a b", "a!", "-a", "/a", ".a"} {
		if validParallelID(id) {
			t.Fatalf("hostile id %q accepted", id)
		}
	}
	_ = MaxParallelTasks
}

func TestCovCheckParallelFileTable(t *testing.T) {
	if err := checkParallelFile("t", "pkg/a.go"); err != nil {
		t.Fatalf("clean file refused: %v", err)
	}
	cases := []struct {
		name string
		file string
		want string
	}{
		{"empty", "", "empty file"},
		{"long", strings.Repeat("a", 600), "exceeds 512"},
		{"nul", "a\x00b", "single line"},
		{"newline", "a\nb", "single line"},
		{"abs-posix", "/abs/x.go", "repo-relative"},
		{"abs-drive", `C:\x.go`, "repo-relative"},
		{"traversal", "a/../b", "must not contain '..'"},
		{"traversal-win", `a\..\b`, "must not contain '..'"},
	}
	for _, c := range cases {
		if err := checkParallelFile("t", c.file); err == nil {
			t.Fatalf("%s: must refuse", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: must name %q, got %v", c.name, c.want, err)
		}
	}
}

func TestCovNormalizeAndOverlap(t *testing.T) {
	if got := normalizeParallelFile(`a\b//c`); got != "a/b/c" {
		t.Fatalf("normalize = %q, want a/b/c", got)
	}
	if got := normalizeParallelFile("./a.go"); got != "a.go" {
		t.Fatalf("leading ./ must drop, got %q", got)
	}
	got := parallelFilesOf(ParallelTask{ID: "t", Title: "T", Files: []string{"b", "a", "a", "."}})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("files must dedupe and sort, got %v", got)
	}
	if !filesOverlap("a", "a") || !filesOverlap("a", "a/b") || !filesOverlap("a/b", "a") {
		t.Fatal("equal/prefix paths share state")
	}
	if filesOverlap("ab", "a") || filesOverlap("a", "ab") {
		t.Fatal("slash boundary required: ab and a do not collide")
	}
	a := ParallelTask{ID: "a", Title: "A", Files: []string{"pkg/x.go"}}
	b := ParallelTask{ID: "b", Title: "B", Files: []string{"other.go"}}
	if tasksCollide(a, b) {
		t.Fatal("disjoint tasks must not collide")
	}
	pairs := DetectParallelCollisions([]ParallelTask{b, a, {ID: "c", Title: "C", Files: []string{"pkg/x.go"}}})
	if len(pairs) != 1 || pairs[0] != [2]string{"a", "c"} {
		t.Fatalf("collision pairs must sort, got %v", pairs)
	}
}

func TestCovPlanParallelExplicitOrderWins(t *testing.T) {
	tasks := []ParallelTask{
		{ID: "a", Title: "A", Files: []string{"shared.txt"}},
		{ID: "b", Title: "B", Files: []string{"shared.txt"}, DependsOn: []string{"a"}},
	}
	levels, serialized, err := PlanParallelLevels(tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(serialized) != 0 {
		t.Fatalf("explicit order already serializes: no implicit edge, got %v", serialized)
	}
	if len(levels) != 2 || levels[0][0] != "a" || levels[1][0] != "b" {
		t.Fatalf("explicit deps still level, got %v", levels)
	}
	if order := FlattenParallelLevels(levels); len(order) != 2 || order[0] != "a" || order[1] != "b" {
		t.Fatalf("flatten must concatenate levels, got %v", order)
	}
	if _, _, err := PlanParallelLevels([]ParallelTask{{ID: "a", Title: "A", DependsOn: []string{"ghost"}}}); err == nil {
		t.Fatal("unknown dependencies must refuse")
	} else if !strings.Contains(err.Error(), "unknown task") {
		t.Fatalf("must name the unknown task, got %v", err)
	}
}

func TestCovSummarizeHistoryBranches(t *testing.T) {
	if out := SummarizeHistory(nil); len(out) != 0 {
		t.Fatalf("no samples means no history, got %v", out)
	}
	hist := SummarizeHistory([]HistorySample{
		{AgentID: "x", Pass: true, Evaluable: true, LatencyMS: 100, Tokens: 10},
		{AgentID: "x", Evaluable: true, LatencyMS: 300, Tokens: 30},
		{AgentID: "y", LatencyMS: 10, Tokens: 1},
		{AgentID: "y", LatencyMS: 20, Tokens: 1},
		{AgentID: "y", LatencyMS: 30, Tokens: 1},
		{AgentID: "y", LatencyMS: 40, Tokens: 1},
		{AgentID: "y", LatencyMS: 50, Tokens: 1},
		{AgentID: "c", Tokens: 6, LatencyMS: -1, Cost: 4, CostKnown: true, Currency: "USD"},
	})
	byID := historyByID(hist)
	x := byID["x"]
	if !x.SuccessRateOK || x.SuccessRate != 0.5 || x.Evaluable != 2 || x.Pass != 1 {
		t.Fatalf("success rate over evaluable only: %+v", x)
	}
	if x.LatencyOK || x.MedianLatencyMS != 200 || !strings.Contains(x.LatencyNote, "mean over n=2") {
		t.Fatalf("small samples report the mean with a note: %+v", x)
	}
	if x.MeanTokens != 20 || x.CostKnown {
		t.Fatalf("tokens average, cost stays unknown: %+v", x)
	}
	y := byID["y"]
	if !y.LatencyOK || y.MedianLatencyMS != 30 {
		t.Fatalf("median over n>=5, got %+v", y)
	}
	c := byID["c"]
	if !c.CostKnown || c.MeanCost != 4 || c.Currency != "USD" {
		t.Fatalf("known money averages with currency: %+v", c)
	}
	if c.LatencyNote != "no latency samples" {
		t.Fatalf("missing latencies must say so: %+v", c)
	}
	if c.MeanTokens != 6 {
		t.Fatalf("mean tokens = %d, want 6", c.MeanTokens)
	}
}

func TestCovSamplesFromLabResultsMapping(t *testing.T) {
	samples := SamplesFromLabResults([]bench.LabResult{
		{AgentID: "p", Verdict: bench.LabPass, DurationMS: 50, TokensIn: 10, TokensOut: 5},
		{AgentID: "f", Verdict: bench.LabFail, DurationMS: 60, TokensIn: 1, TokensOut: 1},
		{AgentID: "n", Verdict: bench.LabNotRun, DurationMS: 70},
	}, nil)
	byAgent := map[string]HistorySample{}
	for _, s := range samples {
		byAgent[s.AgentID] = s
	}
	if !byAgent["p"].Evaluable || !byAgent["p"].Pass {
		t.Fatalf("pass must read evaluable+pass: %+v", byAgent["p"])
	}
	if !byAgent["f"].Evaluable || byAgent["f"].Pass {
		t.Fatalf("fail must read evaluable without pass: %+v", byAgent["f"])
	}
	if byAgent["n"].Evaluable {
		t.Fatalf("not-run must not read evaluable: %+v", byAgent["n"])
	}
	if byAgent["p"].Tokens != 15 || byAgent["p"].CostKnown {
		t.Fatalf("tokens sum, cost unknown without tariffs: %+v", byAgent["p"])
	}
	priced := SamplesFromLabResults([]bench.LabResult{{
		AgentID: "p", Verdict: bench.LabPass, DurationMS: 50,
		TokensIn: 1000, TokensOut: 500, TokenSource: trace.SourceMeasuredHost,
		Outcome: bench.AgentOutcome{Model: "m", Host: "h", HostVersion: "v"},
	}}, []trace.RateCard{{
		Model: "m", HostVersion: "v", EffectiveDate: "2026-01-01",
		Currency: "USD", InputPer1K: 1, OutputPer1K: 2,
	}})
	if !priced[0].CostKnown || priced[0].Cost != 2 || priced[0].Currency != "USD" {
		t.Fatalf("matching tariff must price: %+v", priced[0])
	}
}

func TestCovRegistryGrading(t *testing.T) {
	if reg := RegistryFromManifests(map[string]bench.HostManifest{}); len(reg) != 0 {
		t.Fatalf("no matrix means no routable agents, got %v", reg)
	}
	reg := DefaultRegistry("2026-11-16")
	if len(reg) != 3 {
		t.Fatalf("default registry routes the 3 lab agents, got %v", reg)
	}
	for i := 1; i < len(reg); i++ {
		if reg[i-1].ID >= reg[i].ID {
			t.Fatalf("registry must sort by id: %v", reg)
		}
	}
	for _, a := range reg {
		if len(a.Capabilities) == 0 {
			t.Fatalf("agent %s must declare verified capabilities", a.ID)
		}
		for _, c := range a.Capabilities {
			if strings.TrimSpace(c.Evidence) == "" {
				t.Fatalf("capability %s of %s lacks evidence", c.ID, a.ID)
			}
		}
	}
	if got, ok := FindRoutedAgent(reg, "baseline"); !ok || got.Host == "" {
		t.Fatal("baseline must resolve")
	}
	if _, ok := FindRoutedAgent(reg, "ghost"); ok {
		t.Fatal("unknown agents must not resolve")
	}
	if defaultMaxRisk("baseline") != RiskMedium || defaultMaxRisk("anchored") != RiskHigh || defaultMaxRisk("other") != RiskHigh {
		t.Fatal("risk ceilings drifted")
	}
	if len(defaultScopes("baseline")) != 3 || len(defaultScopes("anchored")) != 4 || len(defaultScopes("verify-first")) != 5 {
		t.Fatalf("scope sets drifted: %d/%d/%d", len(defaultScopes("baseline")), len(defaultScopes("anchored")), len(defaultScopes("verify-first")))
	}
	if got := len(capabilityVocabulary()); got != 12 {
		t.Fatalf("capability vocabulary = %d, want 12", got)
	}
}

func TestCovCheckVerifiedBranches(t *testing.T) {
	mk := func() (RoutedAgent, map[string]bench.HostManifest) {
		m := testManifests()
		a := RoutedAgent{
			ID: "w", Host: "opencode", Available: true,
			Capabilities: []RoutedCapability{{ID: bench.CapTools, Evidence: "ev", Limit: "x"}},
		}
		return a, m
	}
	a, m := mk()
	if reason, ok := checkVerified(a, []string{bench.CapTools}, m); !ok {
		t.Fatalf("verified capability must pass: %s", reason)
	}
	a.Host = "ghost"
	if _, ok := checkVerified(a, []string{bench.CapTools}, m); ok {
		t.Fatal("missing host must exclude")
	} else if reason, _ := checkVerified(a, []string{bench.CapTools}, m); !strings.Contains(reason, "fuera de la matriz") {
		t.Fatalf("must name the missing matrix, got %q", reason)
	}
	a, m = mk()
	if _, ok := checkVerified(a, []string{bench.CapHooks}, m); ok {
		t.Fatal("undeclared capability must exclude")
	} else if reason, _ := checkVerified(a, []string{bench.CapHooks}, m); !strings.Contains(reason, "sin evidencia declarada") {
		t.Fatalf("must name the missing declaration, got %q", reason)
	}
	a.Capabilities[0].Evidence = ""
	if _, ok := checkVerified(a, []string{bench.CapTools}, m); ok {
		t.Fatal("empty declaration evidence must exclude")
	}
	a, m = mk()
	for i, c := range m["opencode"].Capabilities {
		if c.ID == bench.CapTools {
			man := m["opencode"]
			man.Capabilities[i].Status = bench.CapNotExecuted
			m["opencode"] = man
		}
	}
	if _, ok := checkVerified(a, []string{bench.CapTools}, m); ok {
		t.Fatal("unverified host status must exclude")
	} else if reason, _ := checkVerified(a, []string{bench.CapTools}, m); !strings.Contains(reason, "se exige probada") {
		t.Fatalf("must demand probada, got %q", reason)
	}
	a, m = mk()
	for i, c := range m["opencode"].Capabilities {
		if c.ID == bench.CapTools {
			man := m["opencode"]
			man.Capabilities[i].Evidence = ""
			m["opencode"] = man
		}
	}
	if _, ok := checkVerified(a, []string{bench.CapTools}, m); ok {
		t.Fatal("probada without dated evidence must exclude")
	} else if reason, _ := checkVerified(a, []string{bench.CapTools}, m); !strings.Contains(reason, "sin evidencia fechada") {
		t.Fatalf("must name the missing dated evidence, got %q", reason)
	}
	a, m = mk()
	man := m["opencode"]
	kept := man.Capabilities[:0]
	for _, c := range man.Capabilities {
		if c.ID != bench.CapTools {
			kept = append(kept, c)
		}
	}
	man.Capabilities = kept
	m["opencode"] = man
	if _, ok := checkVerified(a, []string{bench.CapTools}, m); ok {
		t.Fatal("capability outside the host row must exclude")
	}
}

func TestCovDecideRefusalAndContrast(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	for i := range reg {
		reg[i].Available = false
	}
	dec, err := Decide(reg, m, nil, testDescriptor(), Budgets{}, 1, "", DefaultScoreWeights())
	if err != nil {
		t.Fatal(err)
	}
	if dec.Primary != "" {
		t.Fatalf("nothing eligible must refuse, routed %q", dec.Primary)
	}
	if len(dec.Excluded) != len(reg) {
		t.Fatalf("every agent must name its gate, got %v", dec.Excluded)
	}
	for _, e := range dec.Excluded {
		if !strings.Contains(e.Reason, "no disponible") {
			t.Fatalf("unavailable agents must say so: %q", e.Reason)
		}
	}
	if !strings.Contains(dec.FixedContrast, "sin decisión dinámica") {
		t.Fatalf("contrast without a decision must say so: %q", dec.FixedContrast)
	}

	live := decideOrFail(t, testRegistry(), m, nil, testDescriptor(), Budgets{}, 1, "baseline")
	if !strings.Contains(fixedContrastForTest(live, live.Primary), "coincide") {
		t.Fatal("fixed routing on the primary must coincide")
	}
	if !strings.Contains(fixedContrastForTest(live, "ghost-agent"), "la dinámica elige") {
		t.Fatalf("eligible non-primary fixed must contrast, got %q", live.FixedContrast)
	}
	excluded := decideOrFail(t, reg, m, nil, testDescriptor(), Budgets{}, 1, "anchored")
	if !strings.Contains(excluded.FixedContrast, "no elegible aquí") {
		t.Fatalf("excluded fixed agent must say so: %q", excluded.FixedContrast)
	}
}

// fixedContrastForTest re-renders the contrast for another fixed agent.
func fixedContrastForTest(dec Decision, fixed string) string {
	return fixedContrast(dec.Primary, fixed, testRegistry(), dec)
}

func TestCovDecideRiskCeilingsAndHistoryBudgets(t *testing.T) {
	m := testManifests()
	high := testDescriptor()
	high.Risk = RiskHigh
	dec := decideOrFail(t, testRegistry(), m, nil, high, Budgets{}, 1, "baseline")
	for _, e := range dec.Excluded {
		if e.AgentID == "baseline" && !strings.Contains(e.Reason, "sobre el techo") {
			t.Fatalf("medium ceiling must refuse high risk: %q", e.Reason)
		}
	}
	crit := testDescriptor()
	crit.Risk = RiskCritical
	decCrit := decideOrFail(t, testRegistry(), m, nil, crit, Budgets{}, 1, "baseline")
	for _, e := range decCrit.Excluded {
		if e.AgentID == "baseline" && !strings.Contains(e.Reason, "techo high") {
			t.Fatalf("critical must demand a high ceiling: %q", e.Reason)
		}
	}
	hist := SummarizeHistory([]HistorySample{
		{AgentID: "anchored", Pass: true, Evaluable: true, LatencyMS: 100, Tokens: 10},
	})
	refused := decideOrFail(t, testRegistry(), m, hist, testDescriptor(), Budgets{TimeMS: 50}, 1, "baseline")
	if refused.Primary != "" {
		t.Fatalf("50ms budget must refuse 100ms+ histories, routed %q", refused.Primary)
	}
	costHist := SummarizeHistory([]HistorySample{
		{AgentID: "anchored", Pass: true, Evaluable: true, LatencyMS: 100, Tokens: 10, Cost: 5, CostKnown: true, Currency: "USD"},
	})
	costed := decideOrFail(t, testRegistry(), m, costHist, testDescriptor(), Budgets{Cost: 1}, 1, "baseline")
	if costed.Primary == "" {
		t.Fatalf("unknown-cost agents stay eligible: %v", costed.Excluded)
	}
	for _, e := range costed.Excluded {
		if e.AgentID == "anchored" && !strings.Contains(e.Reason, "coste estimado") {
			t.Fatalf("known cost over budget must gate: %q", e.Reason)
		}
	}
}

func TestCovScoreFactorsAndNormalize(t *testing.T) {
	m := testManifests()
	d := testDescriptor()
	if f := capabilityFactor(RoutedAgent{Host: "ghost"}, m); f.value != 0 || !strings.Contains(f.why, "fuera de la matriz") {
		t.Fatalf("missing host scores zero with a reason: %+v", f)
	}
	if f := capabilityFactor(RoutedAgent{Host: "opencode"}, m); f.value != 1 || !strings.Contains(f.why, "12/12") {
		t.Fatalf("fully verified host scores 1: %+v", f)
	}
	if f := riskFactor(RoutedAgent{MaxRisk: RiskHigh}, d); f.value != 0.8 || !strings.Contains(f.why, "margen 2") {
		t.Fatalf("two-level margin scores 0.8: %+v", f)
	}
	if f := riskFactor(RoutedAgent{MaxRisk: RiskLow}, d); f.value != 1 {
		t.Fatalf("exact ceiling match scores 1: %+v", f)
	}
	if f := qualityFactor(RoutedAgent{ID: "x"}, nil); f.value != 0.5 {
		t.Fatalf("unknown quality stays neutral: %+v", f)
	}
	byHist := historyByID(SummarizeHistory([]HistorySample{
		{AgentID: "q", Pass: true, Evaluable: true, LatencyMS: 5, Tokens: 1},
	}))
	if f := qualityFactor(RoutedAgent{ID: "q"}, byHist); f.value != 1 {
		t.Fatalf("perfect history scores 1: %+v", f)
	}
	if f := latencyFactor(RoutedAgent{ID: "q"}, byHist); f.value >= 0 || !strings.Contains(f.why, "mediana histórica") {
		t.Fatalf("history latency is a negative raw for relative scaling: %+v", f)
	}
	if f := latencyFactor(RoutedAgent{EstimateTimeMS: 500}, nil); f.value != -500 {
		t.Fatalf("declared latency is a negative raw: %+v", f)
	}
	if f := latencyFactor(RoutedAgent{}, nil); f.value != 0 {
		t.Fatalf("unknown latency stays zero: %+v", f)
	}
	if f := costFactor(RoutedAgent{ID: "q", EstimateCost: 1.5}, nil); f.value != -1.5 {
		t.Fatalf("declared cost is a negative raw: %+v", f)
	}
	if f := costFactor(RoutedAgent{EstimateCost: -1}, nil); f.value != 0.5 {
		t.Fatalf("unknown cost stays neutral: %+v", f)
	}
	w := DefaultScoreWeights()
	ranked := []RankedAgent{
		scoreAgent(RoutedAgent{ID: "fast", Host: "opencode", MaxRisk: RiskHigh, Available: true, EstimateTimeMS: 100, EstimateTokens: 10, EstimateCost: 1}, d, Budgets{}, nil, m, w),
		scoreAgent(RoutedAgent{ID: "slow", Host: "opencode", MaxRisk: RiskHigh, Available: true, EstimateTimeMS: 200, EstimateTokens: 10, EstimateCost: 2}, d, Budgets{}, nil, m, w),
	}
	normalizeRelative(ranked)
	for _, r := range ranked {
		for _, f := range r.Factors {
			if f.Name == "latency" && r.AgentID == "fast" && f.Value != 1 {
				t.Fatalf("best latency normalizes to 1: %+v", f)
			}
			if f.Name == "latency" && r.AgentID == "slow" && f.Value != 0.5 {
				t.Fatalf("half-speed latency normalizes to 0.5: %+v", f)
			}
		}
		if r.Total <= 0 {
			t.Fatalf("totals recompute after normalizing: %+v", r)
		}
	}
	if topFactor(ranked[0]) == "" {
		t.Fatal("top factor must name the leading dimension")
	}
}

func TestCovAssignFallbackBranches(t *testing.T) {
	m := testManifests()
	zero := decideOrFail(t, testRegistry(), m, nil, testDescriptor(), Budgets{}, 0, "baseline")
	if zero.Fallback != "" || !strings.Contains(zero.NoDelegationReason, "límite de delegación 0") {
		t.Fatalf("max-delegations 0 names no fallback: %+v", zero)
	}
	reg := testRegistry()
	for i := range reg {
		if reg[i].ID != "anchored" {
			reg[i].Available = false
		}
	}
	lone := decideOrFail(t, reg, m, nil, testDescriptor(), Budgets{}, 2, "baseline")
	if lone.Fallback != "" || !strings.Contains(lone.NoDelegationReason, "sin otro agente elegible") {
		t.Fatalf("lone eligible runs solo with a reason: %+v", lone)
	}
}

func TestCovEvaluateCostRegression(t *testing.T) {
	rates := []trace.RateCard{{
		Model: "m", HostVersion: "v", EffectiveDate: "2026-01-01",
		Currency: "USD", InputPer1K: 1,
	}}
	mk := func(agent, verdict string, tokens int64, n int) []bench.LabResult {
		var out []bench.LabResult
		for i := 0; i < n; i++ {
			out = append(out, bench.LabResult{
				AgentID: agent, Verdict: verdict, DurationMS: 100,
				TokensIn: tokens, TokenSource: trace.SourceMeasuredHost,
				Outcome: bench.AgentOutcome{Model: "m", Host: "h", HostVersion: "v"},
			})
		}
		return out
	}
	pricey := Evaluate(mk("a", bench.LabPass, 10000, 6), mk("b", bench.LabFail, 1000, 6), rates, DefaultEvalThresholds())
	if pricey.Verdict != VerdictFixed {
		t.Fatalf("cost blowout must keep fixed, got %s", pricey.Verdict)
	} else if !strings.Contains(pricey.Reason, "coste") {
		t.Fatalf("must name the cost ceiling, got %q", pricey.Reason)
	}
	even := Evaluate(mk("a", bench.LabPass, 1000, 6), mk("b", bench.LabFail, 1000, 6), rates, DefaultEvalThresholds())
	if even.Verdict != VerdictDynamic {
		t.Fatalf("priced win must adopt dynamic, got %s (%s)", even.Verdict, even.Reason)
	} else if !strings.Contains(even.Reason, "la dinámica aporta") {
		t.Fatalf("must justify the win, got %q", even.Reason)
	}
	free := Evaluate(mk("a", bench.LabPass, 10, 6), mk("b", bench.LabFail, 10, 6), nil, DefaultEvalThresholds())
	if free.Verdict != VerdictDynamic || !strings.Contains(free.Reason, "no comparable") {
		t.Fatalf("unpriced win must say cost is not comparable: %+v", free)
	}
	if got := costWord(free); !strings.Contains(got, "no comparable") {
		t.Fatalf("costWord without tariffs must say so, got %q", got)
	}
}

func TestCovRenderDecisionBranches(t *testing.T) {
	m := testManifests()
	pair := testDescriptor()
	pair.Risk = RiskCritical
	decPair := decideOrFail(t, testRegistry(), m, nil, pair, Budgets{}, 1, "baseline")
	out := RenderDecision(decPair)
	for _, want := range []string{"modo pair", "revisor", "porque:", "fallback:", "fijo:", "capabilities exigidas"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pair render must quote %q:\n%s", want, out)
		}
	}
	reg := testRegistry()
	for i := range reg {
		reg[i].Available = false
	}
	refused := decideOrFail(t, reg, m, nil, testDescriptor(), Budgets{}, 1, "baseline")
	if !strings.Contains(RenderDecision(refused), "SIN AGENTE") {
		t.Fatal("refusal render must say so")
	}
	solo := decideOrFail(t, testRegistry(), m, nil, testDescriptor(), Budgets{}, 2, "baseline")
	if !strings.Contains(RenderDecision(solo), "scopes=") {
		t.Fatalf("solo render must name the fallback:\n%s", RenderDecision(solo))
	}
	dry := solo
	dry.DryRun = true
	if !strings.Contains(RenderDecision(dry), "dry-run") {
		t.Fatal("dry-run render must say so")
	}
	traced := solo
	traced.DryRun = false
	traced.TraceHint = "hint-1"
	if !strings.Contains(RenderDecision(traced), "traza:") {
		t.Fatal("trace hint prints as recording metadata")
	}
}

func TestCovRenderEvalReportBranches(t *testing.T) {
	mk := func(ms int64, n int) []bench.LabResult {
		var out []bench.LabResult
		for i := 0; i < n; i++ {
			out = append(out, bench.LabResult{AgentID: "a", Verdict: bench.LabPass, DurationMS: ms, TokensIn: 10, TokensOut: 5})
		}
		return out
	}
	rep := Evaluate(mk(100, 6), mk(100, 6), nil, DefaultEvalThresholds())
	out := RenderEvalReport(rep)
	for _, want := range []string{"routing-eval", "reglas:", "mediana 100ms", "coste desconocido", "veredicto:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("eval render must quote %q:\n%s", want, out)
		}
	}
	thin := Evaluate(
		[]bench.LabResult{{AgentID: "a", Verdict: bench.LabNotRun, DurationMS: -1}},
		[]bench.LabResult{{AgentID: "b", Verdict: bench.LabNotRun, DurationMS: -1}},
		nil, DefaultEvalThresholds())
	thinOut := RenderEvalReport(thin)
	if !strings.Contains(thinOut, "coste desconocido") || !strings.Contains(thinOut, "no latency samples") {
		t.Fatalf("thin arms must say what is missing:\n%s", thinOut)
	}
}

func TestCovWorktreePure(t *testing.T) {
	if got := sanitizeParallelSegment("  a  b!!c  ", false); got != "a-b-c" {
		t.Fatalf("sanitize = %q, want a-b-c", got)
	}
	if got := sanitizeParallelSegment("!!!", false); got != "task" {
		t.Fatalf("empty folds to task, got %q", got)
	}
	if got := sanitizeParallelSegment("a/b", true); got != "a/b" {
		t.Fatalf("branch segments keep slashes, got %q", got)
	}
	if got := ParallelBranch("", "r", "t"); got != "parallel/r/t" {
		t.Fatalf("empty prefix means the default, got %q", got)
	}
	long := ParallelBranch("parallel", "r", strings.Repeat("a", 300))
	if len(long) > 255 {
		t.Fatalf("branches truncate to the git limit, got %d", len(long))
	}
	if got := ParallelWorktreePath("/wt", "a/b"); got != "/wt/a-b" {
		t.Fatalf("worktree path folds slashes, got %q", got)
	}
	longPath := ParallelWorktreePath("/wt", strings.Repeat("b", 200))
	if len(longPath) > len("/wt/")+128 {
		t.Fatalf("worktree segments truncate, got %d", len(longPath))
	}
	parent := testParent()
	iso := IsolateParallelWorker(parent, ParallelTask{ID: "t", Title: "T", Scopes: []string{"memory:read", "bogus-scope"}}, "br", "/p")
	if iso.Agent != "parallel-worker" {
		t.Fatalf("empty agent defaults, got %q", iso.Agent)
	}
	for _, s := range iso.Scopes {
		if s == "bogus-scope" {
			t.Fatalf("delegation only restricts, kept %q", s)
		}
	}
	if iso.Workspace != "/p" {
		t.Fatalf("workspace IS the worktree path, got %q", iso.Workspace)
	}
	named := IsolateParallelWorker(parent, ParallelTask{ID: "t", Title: "T", Agent: "w1"}, "br", "/p")
	if named.Agent != "w1" {
		t.Fatalf("named agent survives, got %q", named.Agent)
	}
	porcelain := "worktree /w/b\r\nHEAD abc\r\nbranch refs/heads/parallel/r/b\r\n\r\n" +
		"worktree /w/a\nHEAD def\nbranch refs/heads/main\n\n" +
		"worktree /w/d\nHEAD ghi\ndetached\n\n" +
		"worktree /w/e\nHEAD jkl\nbare\n"
	entries := ParseWorktreeList(porcelain)
	if len(entries) != 4 {
		t.Fatalf("must parse 4 entries, got %v", entries)
	}
	if entries[0].Path != "/w/a" || entries[0].Branch != "main" {
		t.Fatalf("entries sort by path with refs trimmed: %+v", entries[0])
	}
	if entries[2].Branch != "" || entries[3].Bare != true {
		t.Fatalf("detached/bare entries keep their shape: %+v", entries[2:])
	}
	if got := FindParallelOrphans([]WorktreeEntry{
		{Path: "/w/old", Branch: "parallel/r/a"},
		{Path: "/w/bare", Branch: "parallel/r/b", Bare: true},
		{Path: "/w/det", Branch: ""},
		{Path: "/w/user", Branch: "feature/x"},
		{Path: "/w/keep", Branch: "parallel/r/c"},
	}, map[string]bool{"parallel/r/c": true}, "parallel"); len(got) != 1 || got[0].Path != "/w/old" {
		t.Fatalf("only abandoned workers sweep, got %v", got)
	}
}

func TestCovPruneParallelOrphans(t *testing.T) {
	ops := newCovOps()
	ops.branches["parallel/r/a"] = "/w/a"
	ops.branches["main"] = "/w/m"
	removed, err := PruneParallelOrphans(context.Background(), ops, "/repo", map[string]bool{}, "parallel")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "/w/a" || ops.pruned != 1 {
		t.Fatalf("orphan sweep must remove and prune, got %v pruned=%d", removed, ops.pruned)
	}
	if len(ops.deleted) != 1 || ops.deleted[0] != "parallel/r/a" {
		t.Fatalf("orphan branch must drop, got %v", ops.deleted)
	}
	// A branch-delete failure never resurrects the orphan as an error.
	ops2 := newCovOps()
	ops2.branches["parallel/r/a"] = "/w/a"
	ops2.deleteErr = errors.New("already gone")
	if _, err := PruneParallelOrphans(context.Background(), ops2, "/repo", map[string]bool{}, "parallel"); err != nil {
		t.Fatalf("delete failure after removal must not fail: %v", err)
	}
	badList := newCovOps()
	badList.listErr = errors.New("no git")
	if _, err := PruneParallelOrphans(context.Background(), badList, "/repo", map[string]bool{}, "parallel"); err == nil {
		t.Fatal("list failure must propagate")
	}
	badRemove := newCovOps()
	badRemove.branches["parallel/r/a"] = "/w/a"
	badRemove.removeErr = errors.New("locked")
	removed, err = PruneParallelOrphans(context.Background(), badRemove, "/repo", map[string]bool{}, "parallel")
	if err == nil || len(removed) != 0 {
		t.Fatalf("removal failure must stop the sweep, got %v %v", removed, err)
	}
}

func TestCovParallelPlanDefaultsAndWetChecks(t *testing.T) {
	if _, err := planParallelRun(ParallelRequest{Budgets: DefaultParallelBudgets(), RunID: "x"}); err == nil {
		t.Fatal("empty tasks must refuse")
	}
	if _, err := planParallelRun(ParallelRequest{
		Tasks:   []ParallelTask{{ID: "a", Title: "A"}},
		Budgets: ParallelBudgets{MaxConcurrency: 99, MaxQueued: 64},
		RunID:   "x",
	}); err == nil {
		t.Fatal("wild concurrency must refuse")
	}
	if _, err := planParallelRun(ParallelRequest{
		Tasks: []ParallelTask{{ID: "a", Title: "A"}}, RunID: "a\nb",
	}); err == nil {
		t.Fatal("multiline run ids must refuse")
	}
	if _, err := planParallelRun(ParallelRequest{
		Tasks: []ParallelTask{{ID: "a", Title: "A"}}, RunID: strings.Repeat("r", 200),
	}); err == nil {
		t.Fatal("overlong run ids must refuse")
	}
	plan, err := planParallelRun(ParallelRequest{Tasks: []ParallelTask{{ID: "a", Title: "A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.runID, "run_") || plan.base != "HEAD" || plan.prefix != DefaultParallelBranchPrefix {
		t.Fatalf("plan defaults drifted: %+v", plan)
	}
	if plan.budgets.MaxConcurrency != 4 {
		t.Fatalf("zero budgets mean the default policy: %+v", plan.budgets)
	}
	if plan.branchOf["a"] != "parallel/"+plan.runID+"/a" {
		t.Fatalf("branch coordinates drifted: %v", plan.branchOf)
	}
	base := ParallelRequest{
		Tasks:   []ParallelTask{{ID: "a", Title: "A"}},
		Budgets: DefaultParallelBudgets(), RunID: "cov_wet",
		Ops:  newCovOps(),
		Work: func(ctx context.Context, task ParallelTask, dir string) error { return nil },
		Gate: func(ctx context.Context, root string) GateOutcome { return passGate() },
	}
	if _, err := RunParallel(context.Background(), base); err == nil || !strings.Contains(err.Error(), "worktree root") {
		t.Fatalf("wet runs need a worktree root, got %v", err)
	}
	base.WorktreeRoot = "/wt"
	if _, err := RunParallel(context.Background(), base); err == nil || !strings.Contains(err.Error(), "repo root") {
		t.Fatalf("wet runs need a repo root, got %v", err)
	}
	base.RepoRoot = "/repo"
	bare := base
	bare.Ops = nil
	if _, err := RunParallel(context.Background(), bare); err == nil || !strings.Contains(err.Error(), "no worktree backend") {
		t.Fatalf("wet runs need worktree ops, got %v", err)
	}
	noWork := base
	noWork.Work = nil
	if _, err := RunParallel(context.Background(), noWork); err == nil || !strings.Contains(err.Error(), "no worker function") {
		t.Fatalf("wet runs need a worker, got %v", err)
	}
	noGate := base
	noGate.Gate = nil
	if _, err := RunParallel(context.Background(), noGate); err == nil || !strings.Contains(err.Error(), "no gate check") {
		t.Fatalf("wet runs need a gate, got %v", err)
	}
	if got := taskOutcomeStatus(TaskPassed); got != observability.ExecSucceeded {
		t.Fatalf("passed maps to succeeded, got %q", got)
	}
	if got := taskOutcomeStatus(TaskTimedOut); got != observability.ExecTimedOut {
		t.Fatalf("timeouts map explicitly, got %q", got)
	}
	if got := taskOutcomeStatus(TaskCancelled); got != observability.ExecCancelled {
		t.Fatalf("cancelled maps explicitly, got %q", got)
	}
	if got := taskOutcomeStatus(TaskSkipped); got != observability.ExecCancelled {
		t.Fatalf("skipped maps to cancelled, got %q", got)
	}
	if got := taskOutcomeStatus("bogus"); got != observability.ExecFailed {
		t.Fatalf("unknown outcomes fail, got %q", got)
	}
	failed := ParallelReport{Failed: true}
	if statusForEnd(failed) != observability.ExecFailed {
		t.Fatal("failed reports close failed")
	}
	cancelled := ParallelReport{Failed: true, Cancelled: true}
	if statusForEnd(cancelled) != observability.ExecCancelled {
		t.Fatal("cancelled reports close cancelled")
	}
	rep := ParallelReport{}
	failParallelRun(&rep, makeParallelEmit(ParallelRequest{}, "run_x"), "run_x", "boom")
	if !rep.Failed || !strings.HasPrefix(rep.TraceHint, "regulus trace timeline") {
		t.Fatalf("failed reports flag and link: %+v", rep)
	}
}

func TestCovSkipReason(t *testing.T) {
	failed := &TaskResult{TaskID: "a", Outcome: TaskFailed}
	results := map[string]*TaskResult{"a": failed}
	if got := skipReason("b", map[string][]string{"b": {"a"}}, nil, results); !strings.Contains(got, "dependency a did not pass") {
		t.Fatalf("failed deps skip dependents, got %q", got)
	}
	if got := skipReason("c", nil, map[string][]string{"c": {"a"}}, results); !strings.Contains(got, "serialized predecessor a did not pass") {
		t.Fatalf("failed collision predecessors skip waiters, got %q", got)
	}
	passed := &TaskResult{TaskID: "a", Outcome: TaskPassed}
	if got := skipReason("b", map[string][]string{"b": {"a"}}, nil, map[string]*TaskResult{"a": passed}); got != "" {
		t.Fatalf("passed deps stay runnable, got %q", got)
	}
}

func TestCovRunOneWorkerBranches(t *testing.T) {
	task := ParallelTask{ID: "a", Title: "A"}
	iso := IsolateParallelWorker(testParent(), task, "br", "/p")
	mkReq := func(ops WorktreeOps, perTask int64, work func(ctx context.Context, task ParallelTask, dir string) error) ParallelRequest {
		return ParallelRequest{
			Tasks: []ParallelTask{task}, Budgets: ParallelBudgets{MaxConcurrency: 1, MaxQueued: 64, PerTaskTimeoutMS: perTask},
			Base: "HEAD", RunID: "cov_w", Parent: testParent(), Ops: ops, Work: work,
		}
	}
	ops := newCovOps()
	req := mkReq(ops, 0, func(ctx context.Context, task ParallelTask, dir string) error { return errors.New("boom") })
	if outcome, reason := runOneWorker(context.Background(), req, task, iso); outcome != TaskFailed || !strings.Contains(reason, "boom") {
		t.Fatalf("work errors fail the worker, got %q %q", outcome, reason)
	}
	broken := newCovOps()
	broken.createErr = errors.New("no space")
	reqBad := mkReq(broken, 0, func(ctx context.Context, task ParallelTask, dir string) error { return nil })
	if outcome, reason := runOneWorker(context.Background(), reqBad, task, iso); outcome != TaskFailed || !strings.Contains(reason, "worktree:") {
		t.Fatalf("worktree errors fail closed, got %q %q", outcome, reason)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if outcome, _ := runOneWorker(ctx, reqBad, task, iso); outcome != TaskCancelled {
		t.Fatalf("cancelled creation cancels, got %q", outcome)
	}
	slow := mkReq(newCovOps(), 30, func(ctx context.Context, task ParallelTask, dir string) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if outcome, reason := runOneWorker(context.Background(), slow, task, iso); outcome != TaskTimedOut || !strings.Contains(reason, "timeout after 30ms") {
		t.Fatalf("slow workers time out, got %q %q", outcome, reason)
	}
	cancelledWork := mkReq(newCovOps(), 0, func(ctx context.Context, task ParallelTask, dir string) error { return context.Canceled })
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	if outcome, _ := runOneWorker(cctx, cancelledWork, task, iso); outcome != TaskCancelled {
		t.Fatalf("cancelled work cancels, got %q", outcome)
	}
}

func TestCovMergeGateCleanupBranches(t *testing.T) {
	tasks := []ParallelTask{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}
	plan, err := planParallelRun(ParallelRequest{
		Tasks: tasks, Budgets: ParallelBudgets{MaxConcurrency: 2, MaxQueued: 64},
		RunID: "cov_m", Parent: testParent(),
	})
	if err != nil {
		t.Fatal(err)
	}
	emit := makeParallelEmit(ParallelRequest{}, plan.runID)
	ops := newCovOps()
	ops.mergeErr[plan.branchOf["a"]] = errors.New("disk gone")
	req := ParallelRequest{Tasks: tasks, Ops: ops}
	merges, conflicted := mergeParallelBranches(context.Background(), req, plan, emit)
	if !conflicted || len(merges) != 2 {
		t.Fatalf("merge errors conflict the line, got %v %v", merges, conflicted)
	}
	if merges[0].Error == "" || !strings.Contains(merges[1].Error, "blocked by conflict") {
		t.Fatalf("errors block later branches by name: %+v", merges)
	}
	ops2 := newCovOps()
	ops2.conflicts[plan.branchOf["b"]] = []string{"shared.txt"}
	merges2, conflicted2 := mergeParallelBranches(context.Background(), ParallelRequest{Tasks: tasks, Ops: ops2}, plan, emit)
	if !conflicted2 || !merges2[0].Merged {
		t.Fatalf("first branch merges clean: %+v", merges2)
	}
	got := merges2[1]
	if !got.Conflict || !got.NeedsReview || got.Reviewer != "integrator" || len(got.ConflictFiles) != 1 {
		t.Fatalf("conflicts need integrator review with files: %+v", got)
	}
	rep := ParallelReport{}
	reqGate := ParallelRequest{Gate: func(ctx context.Context, root string) GateOutcome {
		return GateOutcome{Ran: true, Passed: false, Detail: "red"}
	}}
	if gateParallelRun(context.Background(), reqGate, plan, emit, &rep) {
		t.Fatal("failed gates return false")
	}
	if !rep.Gate.Ran || rep.Gate.Passed {
		t.Fatalf("gate verdict lands on the report: %+v", rep.Gate)
	}
	rep2 := ParallelReport{}
	clean := newCovOps()
	reqClean := ParallelRequest{
		Tasks: tasks, Ops: clean, RepoRoot: "/repo",
		WorktreeRoot: "/wt", Parent: testParent(),
	}
	cleanupParallelWorktrees(context.Background(), reqClean, plan, emit, &rep2)
	if rep2.OrphansRemoved != 2 {
		t.Fatalf("cleanup removes every checkout, got %+v", rep2)
	}
	rep3 := ParallelReport{}
	dirty := newCovOps()
	dirty.removeErr = errors.New("locked")
	reqDirty := ParallelRequest{Tasks: tasks, Ops: dirty, RepoRoot: "/repo"}
	cleanupParallelWorktrees(context.Background(), reqDirty, plan, emit, &rep3)
	if len(rep3.CleanupErrors) != 2 {
		t.Fatalf("removal failures name their tasks: %+v", rep3)
	}
	rep4 := ParallelReport{}
	pruneBad := newCovOps()
	pruneBad.pruneErr = errors.New("stale")
	reqPrune := ParallelRequest{Tasks: tasks, Ops: pruneBad, RepoRoot: "/repo"}
	cleanupParallelWorktrees(context.Background(), reqPrune, plan, emit, &rep4)
	found := false
	for _, e := range rep4.CleanupErrors {
		if strings.Contains(e, "prune:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("prune failures surface: %+v", rep4)
	}
}

func TestCovRenderParallelReportBranches(t *testing.T) {
	rep := ParallelReport{
		Version: ParallelVersion, RunID: "r1", Base: "main",
		Levels: [][]string{{"a"}, {"b"}}, Order: []string{"a", "b"},
		Serialized: [][2]string{{"a", "b"}},
		Results: []TaskResult{
			{TaskID: "a", Outcome: TaskPassed, DurationMS: 10, Branch: "parallel/r1/a"},
			{TaskID: "b", Outcome: TaskFailed, DurationMS: 20, Branch: "parallel/r1/b", Reason: "boom"},
		},
		Merges: []MergeOutcome{
			{TaskID: "a", Branch: "parallel/r1/a", Merged: true},
			{TaskID: "b", Branch: "parallel/r1/b", Error: "blocked by conflict in a (integrator review first)"},
		},
		Gate:   GateOutcome{Ran: true, Passed: true, Detail: "clean"},
		WallMS: 100, AggregateMS: 300, Failed: true,
		CleanupErrors: []string{"a: locked"},
		TraceHint:     "regulus trace timeline --run r1 --rebuild",
	}
	out := RenderParallelReport(rep)
	for _, want := range []string{
		"serialized (shared files)", "merge a: ok", "merge b: skipped",
		"gate: pass", "speedup 3.00x", "cleanup: 1 note(s)", "result: FAILED", "traza:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("report must quote %q:\n%s", want, out)
		}
	}
	conflict := ParallelReport{
		Version: ParallelVersion, RunID: "r2", Base: "main",
		Levels: [][]string{{"a"}}, Order: []string{"a"},
		Results: []TaskResult{{TaskID: "a", Outcome: TaskFailed, Branch: "parallel/r2/a"}},
		Merges: []MergeOutcome{{
			TaskID: "a", Branch: "parallel/r2/a", Conflict: true,
			ConflictFiles: []string{"shared.txt"}, NeedsReview: true, Reviewer: "integrator",
		}},
		Cancelled: true,
	}
	confOut := RenderParallelReport(conflict)
	for _, want := range []string{"CONFLICT", "reviewer=integrator", "gate: not run", "cancelled:"} {
		if !strings.Contains(confOut, want) {
			t.Fatalf("conflict render must quote %q:\n%s", want, confOut)
		}
	}
	dryEmpty := ParallelReport{Version: ParallelVersion, RunID: "r3", Base: "HEAD", DryRun: true}
	if !strings.Contains(RenderParallelReport(dryEmpty), "sin estimaciones") {
		t.Fatal("dry-run without estimates must say so")
	}
	dry := ParallelReport{
		Version: ParallelVersion, RunID: "r4", Base: "HEAD", DryRun: true,
		Levels: [][]string{{"a", "b"}}, Order: []string{"a", "b"},
		WallMS: 1000, AggregateMS: 2000,
	}
	dryOut := RenderParallelReport(dry)
	if !strings.Contains(dryOut, "ceiling:") || !strings.Contains(dryOut, "dry-run:") {
		t.Fatalf("dry-run render must quote ceilings:\n%s", dryOut)
	}
	ok := ParallelReport{
		Version: ParallelVersion, RunID: "r5", Base: "HEAD",
		Levels: [][]string{{"a"}}, Order: []string{"a"},
		Results:        []TaskResult{{TaskID: "a", Outcome: TaskPassed, Branch: "parallel/r5/a"}},
		Gate:           GateOutcome{Ran: true, Passed: true, Detail: "clean"},
		OrphansRemoved: 1,
	}
	okOut := RenderParallelReport(ok)
	if !strings.Contains(okOut, "cleanup: 1 worktree(s) removed") || !strings.Contains(okOut, "result: ok") {
		t.Fatalf("success render must quote cleanup and result:\n%s", okOut)
	}
}
