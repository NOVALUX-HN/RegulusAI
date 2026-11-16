// F11.3/NOV-89: tier selection, holdout seal, budgets, phases,
// parallelism and the L3 policy. Leakage and wrong-selection tests
// fail when the holdout leaks below L3, when hidden checks surface,
// or when a level drops coverage it must keep.
package bench

import (
	"os"
	"strings"
	"testing"
	"time"
)

func tierTestSplits(t *testing.T) (AgentSplits, []AgentTask) {
	t.Helper()
	corpus := AgentCorpus()
	sp, err := LoadAgentSplits(AgentSplitsPath, corpus)
	if err != nil {
		t.Skipf("committed splits unavailable: %v", err)
	}
	return sp, corpus
}

func tierIncludedSet(sel TierSelection) map[string]bool {
	m := map[string]bool{}
	for _, id := range sel.Included {
		m[id] = true
	}
	return m
}

// Small changes never trigger L3 by default: the recommender only
// emits L0/L1/L2, and L3 comes from an explicit request or policy.
func TestTierRecommendNeverL3ByDefault(t *testing.T) {
	cases := map[string][]string{
		"docs-only":   {"docs/eval-tiers.md", "README.md"},
		"single-go":   {"internal/guardians/guard.go"},
		"tiny-multi":  {"internal/cli/bench.go", "internal/cli/bench_agent.go"},
		"harness":     {"internal/bench/tiers.go"},
		"empty-scope": {},
		"seven-files": {"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go"},
	}
	for name, changed := range cases {
		if got := RecommendTier(changed); got == TierL3 {
			t.Fatalf("%s recommends L3 by default (small changes must not)", name)
		}
	}
	if got := RecommendTier([]string{"docs/x.md"}); got != TierL0 {
		t.Fatalf("docs-only recommends %s, want L0", got)
	}
	if got := RecommendTier([]string{"internal/guardians/guard.go"}); got != TierL0 {
		t.Fatalf("single-file change recommends %s, want L0 smoke", got)
	}
	if got := RecommendTier([]string{"internal/guardians/a.go", "internal/guardians/b.go", "internal/guardians/c.go"}); got != TierL1 {
		t.Fatalf("single-capability multi-file recommends %s, want L1", got)
	}
}

// Below L3 the holdout is sealed: no level except L3 may include a
// holdout task, and every holdout omission names the seal.
func TestTierHoldoutSealedBelowL3(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	for _, tier := range []string{TierL0, TierL1, TierL2} {
		sel, err := SelectTierTasks(tier, corpus, sp, []string{"internal/guardians/guard.go"}, "")
		if err != nil {
			t.Fatalf("%s select: %v", tier, err)
		}
		hold := map[string]bool{}
		for _, id := range sp.Holdout {
			hold[id] = true
		}
		for _, id := range sel.Included {
			if hold[id] {
				t.Fatalf("%s includes holdout %q (sealed below L3)", tier, id)
			}
		}
		sealed := 0
		for _, o := range sel.Omitted {
			if o.Split == "holdout" {
				sealed++
				if !strings.Contains(o.Reason, "sealed") {
					t.Fatalf("%s holdout omission %s lacks the seal reason: %q", tier, o.ID, o.Reason)
				}
			}
		}
		if sealed != len(sp.Holdout) {
			t.Fatalf("%s omits %d holdout, want %d sealed", tier, sealed, len(sp.Holdout))
		}
		if err := VerifyTierHoldoutSeal(sel, sp); err != nil {
			t.Fatalf("%s seal verify: %v", tier, err)
		}
	}
}

// L1 focuses by capability: a Go diff selects Go exploration tasks
// and omits node-only tasks with a capability reason.
func TestTierL1FocusedByCapability(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	sel, err := SelectTierTasks(TierL1, corpus, sp, []string{"internal/guardians/guard.go"}, "")
	if err != nil {
		t.Fatalf("L1 select: %v", err)
	}
	in := tierIncludedSet(sel)
	// Every included exploration task must carry the go capability.
	byID := map[string]AgentTask{}
	for _, task := range corpus {
		byID[task.ID] = task
	}
	for id := range in {
		if !taskHasCapability(byID[id], "go") {
			t.Fatalf("L1 go-focused includes %q without the go capability", id)
		}
	}
	// A node-only exploration task must be omitted with its reason.
	nodeOnly := ""
	for _, id := range sp.Exploration {
		task := byID[id]
		if task.Lang == "node" && !in[id] {
			nodeOnly = id
			break
		}
	}
	if nodeOnly == "" {
		t.Fatal("want at least one omitted node-only exploration task")
	}
	found := false
	for _, o := range sel.Omitted {
		if o.ID == nodeOnly && strings.Contains(o.Reason, "node") {
			found = true
		}
	}
	if !found {
		t.Fatalf("L1 omission of %s lacks a capability reason", nodeOnly)
	}
}

// Wrong selection: an explicit node capability must not include
// go-only tasks.
func TestTierL1WrongCapabilityRejected(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	sel, err := SelectTierTasks(TierL1, corpus, sp, nil, "node")
	if err != nil {
		t.Fatalf("L1 node select: %v", err)
	}
	byID := map[string]AgentTask{}
	for _, task := range corpus {
		byID[task.ID] = task
	}
	for _, id := range sel.Included {
		if !taskHasCapability(byID[id], "node") {
			t.Fatalf("L1 node-focused includes %q without the node capability", id)
		}
	}
	if _, err := SelectTierTasks(TierL1, corpus, sp, nil, "cobol"); err == nil {
		t.Fatal("unknown capability must fail instead of selecting nothing silently")
	}
}

// L2 preserves coverage: every language stays represented, both
// gaming traps are kept, and the sample is smaller than exploration.
func TestTierL2PreservesCoverage(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	sel, err := SelectTierTasks(TierL2, corpus, sp, nil, "")
	if err != nil {
		t.Fatalf("L2 select: %v", err)
	}
	in := tierIncludedSet(sel)
	for _, trap := range []string{"AT28", "AT29"} {
		if !in[trap] {
			t.Fatalf("L2 drops gaming trap %s (anti-gaming coverage must survive)", trap)
		}
	}
	langs := map[string]bool{}
	for _, task := range corpus {
		if in[task.ID] {
			langs[task.Lang] = true
		}
	}
	for _, lang := range []string{"go", "node", "shell", "python"} {
		if !langs[lang] {
			t.Fatalf("L2 drops the whole %s language", lang)
		}
	}
	if len(sel.Included) >= len(sp.Exploration) {
		t.Fatalf("L2 includes %d, want fewer than exploration %d (no feedback gain)", len(sel.Included), len(sp.Exploration))
	}
	if len(sel.Omitted) == 0 {
		t.Fatal("L2 must omit with reasons, got none")
	}
	for _, o := range sel.Omitted {
		if strings.TrimSpace(o.Reason) == "" {
			t.Fatalf("L2 omission %s has no reason", o.ID)
		}
	}
}

// L3 runs the full corpus: every exploration and holdout task,
// nothing omitted, holdout share at or above the 20% floor.
func TestTierL3FullCorpus(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	sel, err := SelectTierTasks(TierL3, corpus, sp, nil, "")
	if err != nil {
		t.Fatalf("L3 select: %v", err)
	}
	if len(sel.Included) != len(corpus) {
		t.Fatalf("L3 includes %d tasks, want the full corpus %d (never recut)", len(sel.Included), len(corpus))
	}
	if len(sel.Omitted) != 0 {
		t.Fatalf("L3 omits %d tasks, want none", len(sel.Omitted))
	}
	if got := float64(len(sp.Holdout)) / float64(len(corpus)); got < 0.2 {
		t.Fatalf("holdout share = %.3f, want >= 0.20", got)
	}
	if fp := CorpusFingerprint(corpus); sel.CorpusSHA != fp {
		t.Fatal("L3 selection must pin the corpus fingerprint")
	}
}

// Leakage: gaming-trap hidden checks never ship in the visible
// fixture files, and selecting a trap never exposes its check.
func TestTierTrapsHideChecks(t *testing.T) {
	_, corpus := tierTestSplits(t)
	for _, task := range corpus {
		if !taskHasCapability(task, "gaming-trap") {
			continue
		}
		for name := range task.Files {
			if strings.HasPrefix(name, "zz_hidden") {
				t.Fatalf("trap %s exposes its hidden check in Files", task.ID)
			}
		}
		for _, line := range task.Check {
			if strings.Contains(line, "zz_hidden") && len(task.Files) == 0 {
				t.Fatalf("trap %s has no visible fixture", task.ID)
			}
		}
	}
	sp, corpus2 := tierTestSplits(t)
	sel, err := SelectTierTasks(TierL0, corpus2, sp, nil, "")
	if err != nil {
		t.Fatalf("L0 select: %v", err)
	}
	for _, o := range sel.Omitted {
		if (o.ID == "AT28" || o.ID == "AT29") && strings.Contains(o.Reason, "zz_hidden") {
			t.Fatalf("omission reason for %s leaks the hidden check name", o.ID)
		}
	}
}

// Leakage: overlapping splits fail closed, and tier selection with
// an unknown splits id refuses instead of running a partial corpus.
func TestTierSplitsOverlapFailsClosed(t *testing.T) {
	_, corpus := tierTestSplits(t)
	ids := []string{}
	for _, task := range corpus {
		ids = append(ids, task.ID)
	}
	doc := `{"version":"v","seed":1,"exploration":["` + strings.Join(ids, `","`) + `"],"holdout":["` + ids[0] + `"]}`
	if _, err := LoadAgentSplits(writeTierSplitsDoc(t, doc), corpus); err == nil {
		t.Fatal("overlapping splits must fail")
	}
	sp, _ := tierTestSplits(t)
	bad := sp
	bad.Exploration = append(append([]string{}, sp.Exploration...), "ATX")
	if _, err := SelectTierTasks(TierL2, corpus, bad, nil, ""); err == nil {
		t.Fatal("selection over unknown splits ids must fail closed")
	}
}

// writeTierSplitsDoc stores one splits document for fail-closed tests.
func writeTierSplitsDoc(t *testing.T, doc string) string {
	t.Helper()
	p := t.TempDir() + "/splits.json"
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Tier budgets: per-task ceilings stay the fixture pins, run and
// agent ceilings come from the tier.
func TestTierBudgetsPreserveFixturePins(t *testing.T) {
	_, corpus := tierTestSplits(t)
	b, err := TierBudgetsFor(TierL2, corpus)
	if err != nil {
		t.Fatalf("budgets: %v", err)
	}
	for _, task := range corpus {
		if b.TaskBudgetS[task.ID] != task.BudgetS {
			t.Fatalf("task %s budget = %d, want fixture pin %d", task.ID, b.TaskBudgetS[task.ID], task.BudgetS)
		}
	}
	if b.RunBudgetS != 1800 {
		t.Fatalf("L2 run budget = %d, want 1800", b.RunBudgetS)
	}
	if b.AgentBudgetS <= 0 {
		t.Fatal("agent budget must be positive")
	}
	if _, err := TierBudgetsFor("L9", corpus); err == nil {
		t.Fatal("unknown tier budgets must fail")
	}
}

// Classification: only pass/fail are evaluable; timeout, infra and
// non-evaluable never pass; unknown degrades to non-evaluable.
func TestTierClassifyStatuses(t *testing.T) {
	if !TierIsEvaluable(ClassifyTierStatus(AgentPass)) || !TierIsEvaluable(ClassifyTierStatus(AgentFail)) {
		t.Fatal("pass/fail must be evaluable")
	}
	for _, s := range []string{AgentTimeout, AgentInfra, AgentNonEvaluable, AgentNotRun, "bogus"} {
		if got := ClassifyTierStatus(s); got == AgentPass {
			t.Fatalf("status %q classifies as pass", s)
		}
		if TierIsEvaluable(ClassifyTierStatus(s)) {
			t.Fatalf("status %q must not be evaluable", s)
		}
	}
}

// Phases: the six buckets reconcile to the total, and the fold sums
// attempts into run totals.
func TestTierPhasesReconcile(t *testing.T) {
	var p TierPhases
	steps := map[string]int64{"prep": 10, "queue": 20, "inference": 300, "tools": 40, "eval": 50, "teardown": 5}
	for phase, ms := range steps {
		if err := p.Add(phase, ms); err != nil {
			t.Fatalf("add %s: %v", phase, err)
		}
	}
	if got, want := p.Total(), int64(425); got != want {
		t.Fatalf("phases total = %d, want %d", got, want)
	}
	if err := p.Add("bogus", 1); err == nil {
		t.Fatal("unknown phase must fail closed")
	}
	tot := SumTierPhases([]TierPhases{p, p})
	if tot.Total() != 850 {
		t.Fatalf("folded total = %d, want 850", tot.Total())
	}
}

// Parallelism: independent tasks batch within the concurrency limit;
// duplicates (shared fixture) and out-of-range concurrency refuse.
func TestTierParallelBatchesIndependentOnly(t *testing.T) {
	batches := PlanTierBatches([]string{"AT3", "AT1", "AT2"}, 2)
	if len(batches) != 2 || len(batches[0]) != 2 || len(batches[1]) != 1 {
		t.Fatalf("batches wrong: %v", batches)
	}
	if batches[0][0] != "AT1" {
		t.Fatalf("batches must be deterministic (sorted), got %v", batches[0])
	}
	if err := ValidateTierParallel([]string{"AT1", "AT2"}, 4); err != nil {
		t.Fatalf("independent tasks must validate: %v", err)
	}
	if err := ValidateTierParallel([]string{"AT1", "AT1"}, 4); err == nil {
		t.Fatal("duplicate tasks share one fixture and must refuse")
	}
	if err := ValidateTierParallel([]string{"AT1"}, 0); err == nil {
		t.Fatal("zero concurrency must refuse")
	}
	if err := ValidateTierParallel([]string{"AT1"}, 99); err == nil {
		t.Fatal("unbounded concurrency must refuse (rate limits)")
	}
}

// Feedback latency: smaller tiers cost fewer budget ceilings than L3
// over the same unchanged corpus (fewer tasks, never cut tasks).
func TestTierFeedbackLatencyShrinks(t *testing.T) {
	sp, corpus := tierTestSplits(t)
	byID := map[string]AgentTask{}
	for _, task := range corpus {
		byID[task.ID] = task
	}
	tasks := func(sel TierSelection) []AgentTask {
		var out []AgentTask
		for _, id := range sel.Included {
			out = append(out, byID[id])
		}
		return out
	}
	l0, err := SelectTierTasks(TierL0, corpus, sp, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	l3, err := SelectTierTasks(TierL3, corpus, sp, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	w0, a0 := EstimateTierCost(tasks(l0), TierDefaultConcurrency())
	w3, a3 := EstimateTierCost(tasks(l3), TierDefaultConcurrency())
	if !(w0 < w3 && a0 < a3) {
		t.Fatalf("L0 wall/agg %d/%d must beat L3 %d/%d", w0, a0, w3, a3)
	}
	if len(corpus) < 44 {
		t.Fatalf("corpus shrank to %d tasks (never recut the corpus)", len(corpus))
	}
}

// L3 policy: releases always require L3; a stale or missing L3
// requires one; a fresh non-release plan does not.
func TestTierL3Policy(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	if ok, _ := TierL3Required(false, time.Time{}, now, true); !ok {
		t.Fatal("release must require L3")
	}
	if ok, _ := TierL3Required(false, time.Time{}, now, false); !ok {
		t.Fatal("missing L3 must require L3")
	}
	stale := now.Add(-8 * 24 * time.Hour)
	if ok, reason := TierL3Required(true, stale, now, false); !ok || !strings.Contains(reason, "7d") {
		t.Fatalf("stale L3 must require L3 with the window named, got %v %q", ok, reason)
	}
	fresh := now.Add(-24 * time.Hour)
	if ok, _ := TierL3Required(true, fresh, now, false); ok {
		t.Fatal("fresh non-release plan must not require L3")
	}
}

// Fingerprints: stable across calls, moved by fixture edits.
func TestTierCorpusFingerprintStable(t *testing.T) {
	_, corpus := tierTestSplits(t)
	if CorpusFingerprint(corpus) != CorpusFingerprint(corpus) {
		t.Fatal("fingerprint must be deterministic")
	}
	mut := append([]AgentTask(nil), corpus...)
	mut[0].Prompt += " drift"
	if CorpusFingerprint(mut) == CorpusFingerprint(corpus) {
		t.Fatal("fixture edits must move the fingerprint")
	}
}
