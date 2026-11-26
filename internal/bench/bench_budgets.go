package bench

// token/proxy budgets and golden cost pins (NOV-67 split).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/decisions"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/trace"
)

// Budget gates (speed-tokens/T10): the suite pins token/context budgets
// as ceilings, not estimates — `bench run` FAILS when the measured
// bytes or proxy tokens exceed their goldens. Guidance/context bytes
// otherwise grow silently (every char rides every request). Narrow the
// text or widen a pin deliberately; never drift past it quietly.
//
// b10CollapsedProxyBudget caps the collapsed per-task proxy of the B10
// golden entries: the two-entry fold models per-call guidance
// descriptors (tool/message/call/task/files chars/4), and the fold is
// their collapsed per-task price.
const b10CollapsedProxyBudget = 13

// defaultContextBytesBudget and defaultContextProxyBudget cap the
// default bounded Almanac resume block (Context: latest 20 notes,
// budget 20 per long list) over the B11 fixture store: 3 observations
// (2 notes + 1 lesson) assembled deterministically (fixed timestamps,
// fixed temp dir name — the header embeds ProjectName(root) — hermetic
// temp store, never the repo). Measured 2026-11-08.
const defaultContextBytesBudget = 546
const defaultContextProxyBudget = 137

// checkTokenProxyBudget refuses a collapsed proxy total over budget.
// At-budget passes; one token over fails the suite with the pin named.
func checkTokenProxyBudget(total, budget int64) error {
	if total > budget {
		return fmt.Errorf("budget gate: collapsed proxy %d exceeds pinned golden %d — narrow the guidance or widen the pin deliberately", total, budget)
	}
	return nil
}

// checkContextBudget refuses a default bounded block over either
// golden (bytes and proxy price move together, but pin both: a
// rendering change could fatten one without the other).
func checkContextBudget(nBytes, proxy int64) error {
	if nBytes > defaultContextBytesBudget {
		return fmt.Errorf("budget gate: default bounded context %dB exceeds pinned golden %dB — narrow the block or widen the pin deliberately", nBytes, defaultContextBytesBudget)
	}
	if proxy > defaultContextProxyBudget {
		return fmt.Errorf("budget gate: default bounded context ~%d proxy tokens exceeds pinned golden %d — narrow the block or widen the pin deliberately", proxy, defaultContextProxyBudget)
	}
	return nil
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
	// The fold total is a budget ceiling, not an exact pin: the unit
	// estimates above already pin the composition, so only growth fails
	// here — shrinkage is a diet, not drift.
	if got.Calls != 2 || got.Errors != 1 {
		return "", fmt.Errorf("fold miscounted: %+v (want 2 calls, 1 error)", got)
	}
	if err := checkTokenProxyBudget(got.TotalLocalTokens, b10CollapsedProxyBudget); err != nil {
		return "", err
	}
	if got.TokensKnown || got.TotalInputTokens != 0 || got.TotalOutputTokens != 0 {
		return "", fmt.Errorf("tokens not honestly unknown: %+v", got)
	}
	if !got.LocalTokensKnown {
		return "", fmt.Errorf("local proxy not marked measured: %+v", got)
	}
	return fmt.Sprintf("proxy golden 5/8/7, fold 2 calls 1 error 13 tokens (budget %d), host unknown", b10CollapsedProxyBudget), nil
}

// checkLessonContextCost pins the deterministic proxy-token price of
// one Almanac resume block over a fixed fixture store (2 notes +
// 1 evidence-gated lesson), assembled through the real default path
// (Context: the default bounded block, --limit 20 --budget 20 — what
// every restart injects). Chars/4 rounded up, the same convention as
// trace.EstimateLocalTokens. The check gates, never merely reports:
// block bytes and proxy tokens must stay within their pinned goldens
// (T10 budget gate) — Fase 4 (pro-harness) diffs this number per
// admitted lesson to ask whether the evolution pays. Hermetic by
// construction: Context reads only the temp store, never charts or
// cwd, and every timestamp is fixed so the byte count is stable run
// to run.
func checkLessonContextCost() (string, error) {
	// Deterministic dir name — not MkdirTemp's random decimal suffix:
	// the block header embeds ProjectName(root), so a random suffix
	// would wobble the byte count run to run and no golden could pin
	// it exactly. Cleaned before and after; a crashed run leaves
	// nothing the next run cannot clear.
	root := filepath.Join(os.TempDir(), "bench-lesson-cost-golden")
	if err := os.RemoveAll(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
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
	ctx, err := almanac.Context(root, now.Add(3*time.Second))
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
	if err := checkContextBudget(int64(len(ctx)), proxy); err != nil {
		return "", err
	}
	return fmt.Sprintf("fixture lessons priced: 3 observations (2 notes + 1 lesson) assemble %d chars = %d proxy tokens (budget %dB/%dt), host unknown", len(ctx), proxy, defaultContextBytesBudget, defaultContextProxyBudget), nil
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

// checkDocsOnlyQuick selects the lean set for docs-only edits: 12
// required gates. Pure selection split from checkGateProfileDecision.
func checkDocsOnlyQuick(cfg gates.Config) ([]gates.Gate, error) {
	quick, err := gates.SelectGates(cfg, chooseGateProfile([]string{"charts/flow.md", "README.md"}, false))
	if err != nil || len(quick) != 12 {
		return nil, fmt.Errorf("docs-only: got %d gates (%v), want 12 quick", len(quick), err)
	}
	return quick, nil
}

// isFullOnlyGate reports the gates the quick profile must never keep:
// the mutation, vulncheck and race jobs that only run in full. One
// case clause keeps the helper itself under the CRAP ceiling.
func isFullOnlyGate(name string) bool {
	switch name {
	case "mutation-receipts", "mutation-gates", "vulncheck", "race-almanac", "race-evolve", "race-gates", "race-runtime", "race-httpapi", "race-mcp", "race-trace", "race-guardians":
		return true
	}
	return false
}

// checkQuickHasNoFullOnly refuses a docs-only quick set that kept a
// full-only gate. Pure scan split from checkGateProfileDecision.
func checkQuickHasNoFullOnly(quick []gates.Gate) error {
	for _, g := range quick {
		if isFullOnlyGate(g.Name) {
			return fmt.Errorf("docs-only quick kept full-only gate %q", g.Name)
		}
	}
	return nil
}

// checkFullAndForcedProfiles pins the precision side: gate code rides
// the 25-gate gauntlet and a forced full always wins over quick.
func checkFullAndForcedProfiles(cfg gates.Config) error {
	full, err := gates.SelectGates(cfg, chooseGateProfile([]string{"internal/gates/gates.go"}, false))
	if err != nil || len(full) != 25 {
		return fmt.Errorf("gate code: got %d gates (%v), want 25 full", len(full), err)
	}
	if got := chooseGateProfile([]string{"charts/flow.md"}, true); got != "" {
		return fmt.Errorf("forced full: chose %q, want full", got)
	}
	if got := chooseGateProfile([]string{"charts/flow.md", "internal/x.go"}, false); got != "" {
		return fmt.Errorf("mixed scope: chose %q, want full", got)
	}
	return nil
}

// checkGateProfileDecision pins the tradeoff through real selection:
// docs-only chooses quick (12 required, no full-only gates), gate code
// or forced full chooses the 25-gate gauntlet.
func checkGateProfileDecision() (string, error) {
	cfg := realGateConfig()
	quick, err := checkDocsOnlyQuick(cfg)
	if err != nil {
		return "", err
	}
	if err := checkQuickHasNoFullOnly(quick); err != nil {
		return "", err
	}
	if err := checkFullAndForcedProfiles(cfg); err != nil {
		return "", err
	}
	return "docs-only edits ride quick=12, gate code rides full=25, forced full honored", nil
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
