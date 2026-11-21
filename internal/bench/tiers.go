// Tiered evaluation L0–L3 with selective corpus execution (F11.3/NOV-89):
// reduce feedback latency without deleting tasks, lowering difficulty
// or contaminating the holdout.
//
// Levels: L0 smoke per change, L1 focused per agent/capability, L2
// representative regression, L3 full corpus + holdout. Selection maps
// a diff (changed paths) onto components/capabilities and explains
// every inclusion and omission; manifests, commit-pinned fixtures,
// hidden checks, splits and anti-leakage are preserved, never
// redefined here (the corpus, splits file and outcomes stay the
// single owners). Small changes never select L3 by default; L3 is
// required periodically and before releases, with the holdout sealed
// below L3.
package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// TierVersion pins the tier/selector document shape: plans compare
// only within one version.
const TierVersion = "eval-tiers-v1"

// Tier names: the closed level vocabulary.
const (
	TierL0 = "L0"
	TierL1 = "L1"
	TierL2 = "L2"
	TierL3 = "L3"
)

// TierL3StaleAfter bounds the periodic L3: a tier plan older than
// this without an L3 requires a fresh L3 (see TierL3Required).
const TierL3StaleAfter = 7 * 24 * time.Hour

// TierSpec describes one level: what it runs, whether the holdout is
// included, the canonical command and the run budget ceiling.
type TierSpec struct {
	Tier            string `json:"tier"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	IncludesHoldout bool   `json:"includes_holdout"`
	Command         string `json:"command"`
	RunBudgetS      int    `json:"run_budget_s"`
}

// TierSpecs is the documented level table: smoke per change, focused
// per agent/capability, representative regression, full corpus plus
// holdout. Run budgets are ceilings, never estimates.
func TierSpecs() []TierSpec {
	return []TierSpec{
		{Tier: TierL0, Name: "smoke",
			Description:     "Smoke per change: one exploration task per language (go/node/shell/python), hidden checks kept.",
			IncludesHoldout: false,
			Command:         "regulus bench tier-plan --tier L0",
			RunBudgetS:      300},
		{Tier: TierL1, Name: "focused",
			Description:     "Focused per agent/capability: every exploration task whose capability the diff touches.",
			IncludesHoldout: false,
			Command:         "regulus bench tier-plan --tier L1 --changed <paths>",
			RunBudgetS:      900},
		{Tier: TierL2, Name: "regression",
			Description:     "Representative regression: stratified exploration sample covering every language plus both gaming traps.",
			IncludesHoldout: false,
			Command:         "regulus bench tier-plan --tier L2",
			RunBudgetS:      1800},
		{Tier: TierL3, Name: "full+holdout",
			Description:     "Full corpus plus holdout: every exploration and holdout task, required periodically and pre-release.",
			IncludesHoldout: true,
			Command:         "regulus bench tier-plan --tier L3",
			RunBudgetS:      7200},
	}
}

// FindTierSpec returns the spec for one level.
func FindTierSpec(tier string) (TierSpec, bool) {
	for _, s := range TierSpecs() {
		if s.Tier == tier {
			return s, true
		}
	}
	return TierSpec{}, false
}

// tierByAlias maps every accepted tier spelling onto its canonical
// level: the closed vocabulary plus digits and long names.
var tierByAlias = map[string]string{
	TierL0: TierL0, "0": TierL0, "SMOKE": TierL0,
	TierL1: TierL1, "1": TierL1, "FOCUSED": TierL1,
	TierL2: TierL2, "2": TierL2, "REGRESSION": TierL2,
	TierL3: TierL3, "3": TierL3, "FULL": TierL3,
}

// ParseTier validates the closed level vocabulary.
func ParseTier(s string) (string, error) {
	if tier, ok := tierByAlias[strings.ToUpper(strings.TrimSpace(s))]; ok {
		return tier, nil
	}
	return "", fmt.Errorf("unknown tier %q (want L0|L1|L2|L3)", s)
}

// TierIncludesHoldout reports whether a level runs holdout tasks:
// only L3 does. Everything below L3 keeps the holdout sealed.
func TierIncludesHoldout(tier string) bool {
	return tier == TierL3
}

// TierSmokeIDs is the fixed L0 set: one exploration task per
// language, all with fast budgets. Fixed (not sampled) so smoke
// latency is stable run to run.
var TierSmokeIDs = []string{"AT1", "AT14", "AT19", "AT23"}

// TierCapabilities is the closed capability vocabulary for focused
// selection: one primary per task language plus the two
// cross-cutting traps the suite must never deselect silently.
var TierCapabilities = []string{"go", "node", "shell", "python", "gaming-trap", "concurrency"}

// isTierCapability reports the closed capability vocabulary.
func isTierCapability(c string) bool {
	for _, k := range TierCapabilities {
		if c == k {
			return true
		}
	}
	return false
}

// ParseTierCapability validates a --capability filter.
func ParseTierCapability(c string) (string, error) {
	got := strings.ToLower(strings.TrimSpace(c))
	if got == "" {
		return "", nil
	}
	if !isTierCapability(got) {
		return "", fmt.Errorf("unknown capability %q (want %s)", c, strings.Join(TierCapabilities, "|"))
	}
	return got, nil
}

// TaskCapabilities maps one corpus task onto capabilities: the
// primary language plus gaming-trap for AT28/AT29 and concurrency
// for the race/parallel tasks. Pure function of the task, never of
// the diff, so focused selection stays explainable.
func TaskCapabilities(t AgentTask) []string {
	caps := []string{t.Lang}
	switch t.ID {
	case "AT28", "AT29":
		caps = append(caps, "gaming-trap")
	case "AT31", "AT41":
		caps = append(caps, "concurrency")
	}
	return caps
}

// taskHasCapability reports whether a task carries a capability.
func taskHasCapability(t AgentTask, cap string) bool {
	for _, c := range TaskCapabilities(t) {
		if c == cap {
			return true
		}
	}
	return false
}

// AffectedCapabilities maps changed repo paths onto capabilities.
// Harness-wide paths (bench, gates, trace, observability, splits,
// gauntlet) affect every capability; docs-only paths affect none
// (smoke); language extensions map to their capability. Empty means
// unknown scope, which affects everything (never silently narrows).
func AffectedCapabilities(changed []string) []string {
	if len(changed) == 0 {
		return append([]string(nil), TierCapabilities...)
	}
	var caps []string
	seen := map[string]bool{}
	add := func(c string) {
		if !seen[c] {
			seen[c] = true
			caps = append(caps, c)
		}
	}
	docsOnly := true
	for _, p := range changed {
		n := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
		if isTierHarnessWide(n) {
			return append([]string(nil), TierCapabilities...)
		}
		if isTierDocsOnly(n) {
			continue
		}
		docsOnly = false
		add(tierCapabilityForPath(n))
	}
	if docsOnly {
		return []string{"docs"}
	}
	return caps
}

// isTierHarnessWide reports paths whose change invalidates focused
// narrowing: the bench, gates, trace, observability, splits and
// gauntlet surfaces every task depends on.
func isTierHarnessWide(n string) bool {
	for _, pre := range []string{"internal/bench/", "internal/gates/", "internal/trace/",
		"internal/observability/", "bench/", "regulus.json", "go.mod"} {
		if strings.HasPrefix(n, pre) || n == strings.TrimSuffix(pre, "/") {
			return true
		}
	}
	return false
}

// isTierDocsOnly reports paths that never touch code: markdown and
// the docs/charts trees.
func isTierDocsOnly(n string) bool {
	if strings.HasSuffix(n, ".md") {
		return true
	}
	for _, pre := range []string{"docs/", "charts/", "odd/"} {
		if strings.HasPrefix(n, pre) {
			return true
		}
	}
	return false
}

// tierCapabilityForPath maps one non-docs, non-harness path onto its
// capability by extension or tree.
func tierCapabilityForPath(n string) string {
	switch {
	case strings.HasSuffix(n, ".go"):
		return "go"
	case strings.HasSuffix(n, ".js") || strings.HasSuffix(n, ".mjs") ||
		strings.HasSuffix(n, ".ts") || strings.HasSuffix(n, ".tsx"):
		return "node"
	case strings.HasSuffix(n, ".sh") || strings.HasPrefix(n, "scripts/"):
		return "shell"
	case strings.HasSuffix(n, ".py"):
		return "python"
	}
	return "go"
}

// RecommendTier maps a diff onto the default level without ever
// selecting L3: docs-only or tiny diffs ride smoke, single-capability
// diffs ride focused, everything else (including unknown scope and
// harness-wide changes) rides regression. L3 comes only from an
// explicit request or the release/periodic policy.
func RecommendTier(changed []string) string {
	caps := AffectedCapabilities(changed)
	if len(caps) == 1 && caps[0] == "docs" {
		return TierL0
	}
	if len(changed) == 1 && !tierTouchesHarness(changed) {
		return TierL0
	}
	if len(caps) == 1 {
		return TierL1
	}
	return TierL2
}

// tierTouchesHarness reports whether any path is harness-wide.
func tierTouchesHarness(changed []string) bool {
	for _, p := range changed {
		n := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
		if isTierHarnessWide(n) {
			return true
		}
	}
	return false
}

// TierOmission is one deselected task plus its machine-readable
// reason: every omitted test names why it stays out.
type TierOmission struct {
	ID     string `json:"id"`
	Split  string `json:"split"`
	Reason string `json:"reason"`
}

// TierSelection is one reproducible selection: the level, the inputs
// it was derived from, the included ids and every omission with its
// reason. Manifest fields (suite, splits, corpus hash) travel with
// it so a report replays exactly what was selected.
type TierSelection struct {
	TierVersion   string         `json:"tier_version"`
	Tier          string         `json:"tier"`
	SuiteVersion  string         `json:"suite_version"`
	SplitsVersion string         `json:"splits_version"`
	SplitsSeed    int64          `json:"splits_seed"`
	CorpusSHA     string         `json:"corpus_sha256"`
	Changed       []string       `json:"changed"`
	Capability    string         `json:"capability"`
	Agent         string         `json:"agent"`
	Included      []string       `json:"included"`
	Omitted       []TierOmission `json:"omitted"`
	Reason        string         `json:"reason"`
}

// TierIncludeReason explains one inclusion for the report.
func TierIncludeReason(tier string, t AgentTask) string {
	switch tier {
	case TierL0:
		return fmt.Sprintf("L0 smoke covers %s (%s)", t.Lang, t.ID)
	case TierL1:
		return fmt.Sprintf("L1 focused: capability %s touched (%s)", t.Lang, t.ID)
	case TierL2:
		if taskHasCapability(t, "gaming-trap") {
			return fmt.Sprintf("L2 regression keeps gaming trap %s (anti-gaming coverage)", t.ID)
		}
		return fmt.Sprintf("L2 regression samples %s for %s coverage (%s)", t.ID, t.Lang, t.Title)
	default:
		return fmt.Sprintf("L3 full corpus includes %s", t.ID)
	}
}

// SelectTierTasks selects one level over the corpus against the
// committed splits: L0 the fixed smoke set, L1 the exploration tasks
// whose capabilities the diff (or --capability) touches, L2 the
// stratified exploration sample, L3 the full corpus. Below L3 the
// holdout is always omitted with its sealed reason; L3 omits
// nothing. Unknown task ids in any committed split fail closed.
func SelectTierTasks(tier string, corpus []AgentTask, splits AgentSplits, changed []string, capability string) (TierSelection, error) {
	sel := TierSelection{
		TierVersion: TierVersion, Tier: tier,
		SuiteVersion:  AgentSuiteVersion,
		SplitsVersion: splits.Version, SplitsSeed: splits.Seed,
		CorpusSHA:  CorpusFingerprint(corpus),
		Changed:    append([]string(nil), changed...),
		Capability: capability, Agent: "",
	}
	byID := map[string]AgentTask{}
	for _, t := range corpus {
		byID[t.ID] = t
	}
	for _, id := range append(append([]string{}, splits.Exploration...), splits.Holdout...) {
		if _, ok := byID[id]; !ok {
			return TierSelection{}, fmt.Errorf("tier select: splits reference unknown task %q", id)
		}
	}
	var included []AgentTask
	var err error
	switch tier {
	case TierL0:
		sel.Reason = "L0 smoke per change: one exploration task per language"
		included, err = tierSmokeSelect(byID, splits)
	case TierL1:
		included, err = tierFocusedSelect(byID, splits, changed, capability, &sel)
	case TierL2:
		sel.Reason = "L2 representative regression: stratified exploration sample, every language plus both gaming traps"
		included = tierRepresentativeSelect(byID, splits)
	case TierL3:
		sel.Reason = "L3 full corpus plus holdout: every task, nothing omitted"
		included = tierFullSelect(byID, splits)
	default:
		return TierSelection{}, fmt.Errorf("unknown tier %q (want L0|L1|L2|L3)", tier)
	}
	if err != nil {
		return TierSelection{}, err
	}
	includedIDs := map[string]bool{}
	for _, t := range included {
		if includedIDs[t.ID] {
			return TierSelection{}, fmt.Errorf("tier select: duplicate task %q", t.ID)
		}
		includedIDs[t.ID] = true
		sel.Included = append(sel.Included, t.ID)
	}
	for _, id := range splits.Exploration {
		if includedIDs[id] {
			continue
		}
		sel.Omitted = append(sel.Omitted, TierOmission{ID: id, Split: "exploration", Reason: tierOmitReason(tier, byID[id], capability, changed)})
	}
	for _, id := range splits.Holdout {
		if includedIDs[id] {
			continue
		}
		sel.Omitted = append(sel.Omitted, TierOmission{ID: id, Split: "holdout", Reason: "holdout sealed below L3: runs only at L3 (periodic or pre-release)"})
	}
	if tier != TierL3 {
		if err := VerifyTierHoldoutSeal(sel, splits); err != nil {
			return TierSelection{}, err
		}
	}
	return sel, nil
}

// tierSmokeSelect resolves the fixed smoke set against the corpus.
func tierSmokeSelect(byID map[string]AgentTask, splits AgentSplits) ([]AgentTask, error) {
	inExploration := map[string]bool{}
	for _, id := range splits.Exploration {
		inExploration[id] = true
	}
	var out []AgentTask
	for _, id := range TierSmokeIDs {
		t, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("tier select: smoke task %q not in corpus", id)
		}
		if !inExploration[id] {
			return nil, fmt.Errorf("tier select: smoke task %q is not exploration (holdout must stay sealed)", id)
		}
		out = append(out, t)
	}
	return out, nil
}

// tierFocusedSelect keeps the exploration tasks whose capabilities
// the diff touches (or the explicit --capability filter when set).
func tierFocusedSelect(byID map[string]AgentTask, splits AgentSplits, changed []string, capability string, sel *TierSelection) ([]AgentTask, error) {
	var want []string
	if capability != "" {
		if _, err := ParseTierCapability(capability); err != nil {
			return nil, err
		}
		want = []string{capability}
		sel.Reason = fmt.Sprintf("L1 focused per capability: filter %q over exploration", capability)
	} else {
		want = AffectedCapabilities(changed)
		sel.Reason = fmt.Sprintf("L1 focused per change: capabilities %s over exploration", strings.Join(want, ","))
	}
	matched := map[string]bool{}
	for _, c := range want {
		matched[c] = true
	}
	var out []AgentTask
	for _, id := range splits.Exploration {
		t := byID[id]
		for _, c := range TaskCapabilities(t) {
			if matched[c] {
				out = append(out, t)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tier select: L1 filter %q matches no exploration task (selection would run nothing)", strings.Join(want, ","))
	}
	return out, nil
}

// tierRepresentativeSelect samples exploration deterministically:
// every other task per language in corpus order, plus both gaming
// traps unconditionally (anti-gaming coverage is never sampled out).
func tierRepresentativeSelect(byID map[string]AgentTask, splits AgentSplits) []AgentTask {
	byLang := map[string][]AgentTask{}
	var langs []string
	for _, id := range splits.Exploration {
		t := byID[id]
		if _, ok := byLang[t.Lang]; !ok {
			langs = append(langs, t.Lang)
		}
		byLang[t.Lang] = append(byLang[t.Lang], t)
	}
	sort.Strings(langs)
	keep := map[string]bool{}
	for _, lang := range langs {
		seq := byLang[lang]
		for i, t := range seq {
			if i%2 == 0 {
				keep[t.ID] = true
			}
		}
	}
	for _, trap := range []string{"AT28", "AT29"} {
		if _, ok := byID[trap]; ok {
			keep[trap] = true
		}
	}
	var out []AgentTask
	for _, id := range splits.Exploration {
		if keep[id] {
			out = append(out, byID[id])
		}
	}
	return out
}

// tierFullSelect returns the full corpus in committed split order:
// exploration first, then holdout.
func tierFullSelect(byID map[string]AgentTask, splits AgentSplits) []AgentTask {
	var out []AgentTask
	for _, id := range append(append([]string{}, splits.Exploration...), splits.Holdout...) {
		out = append(out, byID[id])
	}
	return out
}

// tierOmitReason explains one exploration omission for the report.
func tierOmitReason(tier string, t AgentTask, capability string, changed []string) string {
	switch tier {
	case TierL0:
		return fmt.Sprintf("L0 smoke runs one %s task (%s); %s omitted (smoke set fixed)", t.Lang, smokeForLang(t.Lang), t.ID)
	case TierL1:
		if capability != "" {
			return fmt.Sprintf("L1 focused on %q: %s carries %s, not selected", capability, t.ID, strings.Join(TaskCapabilities(t), ","))
		}
		return fmt.Sprintf("L1 focused per change: %s carries %s, untouched by the diff", t.ID, strings.Join(TaskCapabilities(t), ","))
	case TierL2:
		return fmt.Sprintf("L2 representative sample: %s omitted by stratification, %s coverage kept by the sampled sibling", t.ID, t.Lang)
	default:
		return fmt.Sprintf("L3 includes everything: %s must not be omitted", t.ID)
	}
}

// smokeIDMatchesLang reports whether one smoke id covers a language.
func smokeIDMatchesLang(id, lang string) bool {
	for _, t := range AgentCorpus() {
		if t.ID == id && t.Lang == lang {
			return true
		}
	}
	return false
}

// smokeForLang names the smoke representative per language.
func smokeForLang(lang string) string {
	for _, id := range TierSmokeIDs {
		if smokeIDMatchesLang(id, lang) {
			return id
		}
	}
	return "smoke"
}

// VerifyTierHoldoutSeal fails when any included task is a holdout id
// below L3: the holdout stays sealed outside the full corpus run.
func VerifyTierHoldoutSeal(sel TierSelection, splits AgentSplits) error {
	if TierIncludesHoldout(sel.Tier) {
		return nil
	}
	hold := map[string]bool{}
	for _, id := range splits.Holdout {
		hold[id] = true
	}
	for _, id := range sel.Included {
		if hold[id] {
			return fmt.Errorf("tier select: holdout task %q selected below L3 (holdout sealed)", id)
		}
	}
	return nil
}

// CorpusFingerprint hashes the corpus shape: sorted id/lang/title/
// prompt/files/check/budget lines, sha256 hex. Fixture edits move
// the pin, so a selection manifest always names the corpus it ran.
func CorpusFingerprint(corpus []AgentTask) string {
	type line struct{ s string }
	var lines []string
	for _, t := range corpus {
		var files []string
		for name, body := range t.Files {
			sum := sha256.Sum256([]byte(body))
			files = append(files, name+"="+hex.EncodeToString(sum[:])[:12])
		}
		sort.Strings(files)
		lines = append(lines, strings.Join([]string{
			t.ID, t.Lang, t.Title, t.Prompt,
			strings.Join(files, ","),
			strings.Join(t.Check, " "),
			fmt.Sprint(t.BudgetS),
		}, "|"))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// TierL3Required enforces the periodic and pre-release L3: releases
// always require L3; otherwise an L3 older than TierL3StaleAfter (or
// never run) requires one. Fresh non-release plans do not.
func TierL3Required(hasLast bool, lastL3, now time.Time, isRelease bool) (bool, string) {
	if isRelease {
		return true, "pre-release L3 required: full corpus plus holdout before any release"
	}
	if !hasLast {
		return true, "periodic L3 required: no L3 recorded yet"
	}
	if now.Sub(lastL3) > TierL3StaleAfter {
		return true, fmt.Sprintf("periodic L3 required: last L3 %s is older than 7d", lastL3.UTC().Format(time.RFC3339))
	}
	return false, ""
}
