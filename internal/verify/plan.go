// Verification plans (F13.4/NOV-107): one plan per task and per
// mission, derived from the diff, the F12.8 impact graph, contracts,
// F12.4 severity and the acceptance criteria.
//
// BuildPlan is pure: graph + changed paths + options in, plan out.
// No clock, no disk, no git, no toolchain probe — the edge (cmdx)
// resolves the HEAD and mints the run id. The plan pins everything
// a rerun needs: changed paths, diff hash, impact, risk, suite with
// its full-suite contrast, tier, gates, contracts, criteria and
// provenance.
package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/impact"
)

// Plan is one verification plan: what to run, why, and what it
// takes to reproduce. ConfigID hashes the derivation inputs so a
// rerun proves "misma configuración, mismo plan" without trusting
// filenames.
type Plan struct {
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	TaskID    string `json:"task_id,omitempty"`
	MissionID string `json:"mission_id,omitempty"`
	ConfigID  string `json:"config_id"`

	Changed  []string `json:"changed"`
	DiffHash string   `json:"diff_hash"`

	Impact      impact.Impact         `json:"impact"`
	RiskLevel   string                `json:"risk_level"`
	RiskScore   float64               `json:"risk_score"`
	Escalation  []string              `json:"escalation,omitempty"`
	Suite       impact.SuiteSelection `json:"suite"`
	Tier        string                `json:"tier"`
	TierBudgetS int                   `json:"tier_budget_s"`
	Widened     bool                  `json:"widened"`

	Gates       []GateSpec `json:"gates"`
	GateReasons []string   `json:"gate_reasons"`

	Contracts []string `json:"contracts,omitempty"`
	Breaking  []string `json:"breaking,omitempty"`
	Criteria  []string `json:"criteria,omitempty"`

	Provenance Provenance `json:"provenance"`
}

// PlanOptions carries the caller-supplied derivation inputs: scope,
// F12.4 severity, contracts, acceptance criteria, an optional tier
// override and the run provenance (run id mandatory — the edge
// mints it, the engine never invents identity).
type PlanOptions struct {
	Scope        string
	TaskID       string
	MissionID    string
	Severity     float64
	Contracts    []string
	Criteria     []string
	TierOverride string
	Provenance   Provenance
}

// DefaultCriteria is the spec's acceptance contract, stored verbatim
// in every plan that names no explicit criteria: each one maps to a
// checkable field (see CriteriaStatus in report.go).
var DefaultCriteria = []string{
	"cada cambio integrado tiene plan y resultado trazable",
	"BLOCKED/SKIPPED/UNKNOWN nunca se interpretan como PASS",
	"un test fallido o gate crítico rojo bloquea la integración",
	"la selección enfocada se contrasta contra full suite (precisión/recall)",
	"regresiones inyectadas detectadas y vinculadas al cambio responsable",
	"reproducible desde commit y configuración registrados",
}

// DiffHash hashes the normalized, sorted, deduplicated changed paths:
// the same diff always names the same bytes, whatever the flag order.
func DiffHash(changed []string) string {
	seen := map[string]bool{}
	var norm []string
	for _, c := range changed {
		n := strings.TrimSpace(strings.ReplaceAll(c, "\\", "/"))
		n = strings.TrimPrefix(n, "./")
		n = strings.TrimSuffix(n, "/")
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		norm = append(norm, n)
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:])
}

// isBreakingContract reports a contract entry that declares an
// incompatible change: explicit "breaking:" prefix or a "!" marker.
func isBreakingContract(c string) bool {
	n := strings.ToLower(strings.TrimSpace(c))
	return strings.HasPrefix(n, "breaking:") || strings.Contains(n, "!")
}

// BuildPlan derives the verification plan over an already-built
// impact graph. head is the git HEAD (or "worktree" without git —
// the edge resolves it, the engine only records it).
func BuildPlan(g impact.Graph, changed []string, opts PlanOptions, head string) (Plan, error) {
	scope, err := ParseScope(opts.Scope)
	if err != nil {
		return Plan{}, err
	}
	if len(changed) == 0 {
		return Plan{}, fmt.Errorf("verify: --changed must name at least one path")
	}
	if opts.Severity < 0 || opts.Severity > 1 {
		return Plan{}, fmt.Errorf("verify: --severity must be 0..1 (observed F12.4 rate), got %v", opts.Severity)
	}
	for _, v := range []struct{ name, val string }{
		{"task", opts.TaskID}, {"mission", opts.MissionID},
	} {
		if err := singleLine(v.name, v.val, false); err != nil {
			return Plan{}, err
		}
	}
	if scope == ScopeMission && strings.TrimSpace(opts.MissionID) == "" {
		return Plan{}, fmt.Errorf("verify: mission scope requires --mission")
	}
	for i, c := range opts.Contracts {
		if err := singleLine(fmt.Sprintf("contracts[%d]", i), c, true); err != nil {
			return Plan{}, err
		}
	}
	for i, c := range opts.Criteria {
		if err := singleLine(fmt.Sprintf("criteria[%d]", i), c, true); err != nil {
			return Plan{}, err
		}
	}
	override := ""
	if strings.TrimSpace(opts.TierOverride) != "" {
		tier, err := bench.ParseTier(opts.TierOverride)
		if err != nil {
			return Plan{}, fmt.Errorf("verify: %v", err)
		}
		if tier == bench.TierL3 {
			return Plan{}, fmt.Errorf("verify: tier L3 nunca por foco (solo periódica/pre-release, holdout sellado)")
		}
		override = tier
	}
	if err := opts.Provenance.validate(); err != nil {
		return Plan{}, err
	}

	imp := impact.MapDiff(g, changed)
	risk := impact.AssessRisk(imp, opts.Severity)
	sel := impact.SelectSuite(g, imp, nil)
	sel.Metrics = impact.MeasureSelection(sel.Focused, nil, sel.Full)

	contracts := sortedUnique(opts.Contracts)
	contractChanged := len(contracts) > 0
	var breaking []string
	for _, c := range contracts {
		if isBreakingContract(c) {
			breaking = append(breaking, c)
		}
	}

	effective, escalation := EscalateRisk(risk.Level, imp.Widened, opts.Severity, contractChanged, len(breaking) > 0)
	escalation = append([]string{}, escalation...)
	floor := RiskTierFloor(effective)
	needed := higherTier(sel.Tier, bench.RecommendTier(changed), floor)
	tier := needed
	if override != "" {
		if riskTierRank(override) < riskTierRank(needed) && (imp.Widened || len(imp.Critical) > 0 || len(breaking) > 0) {
			return Plan{}, fmt.Errorf("verify: override %s bajo el necesario %s con incertidumbre (widened/críticos/breaking): el foco débil nunca omite pruebas críticas", override, needed)
		}
		tier = higherTier(override, needed)
		if override != needed {
			escalation = append(escalation, fmt.Sprintf("tier override %s: rige %s (máximo entre override, impacto %s y piso %s)", override, tier, sel.Tier, floor))
		}
	}
	widen, widenReasons := ShouldWiden(imp.Widened, len(imp.Critical) > 0, len(breaking) > 0, override, needed)
	focused, omitted := sel.Focused, sel.Omitted
	if widen && !imp.Widened {
		// Impact already widened to full when the input maps to no
		// node; this arm widens on criticals/breaking even when the
		// diff maps cleanly: run everything, keep the contrast.
		focused = append([]string(nil), sel.Full...)
		omitted = nil
	}
	spec, required, gateReasons := RequiredGates(effective, contractChanged, len(breaking) > 0, escalation)
	_ = required
	gateReasons = append(gateReasons, widenReasons...)
	gateReasons = append(gateReasons, sel.Reasons...)

	criteria := append([]string(nil), opts.Criteria...)
	if len(criteria) == 0 {
		criteria = append([]string(nil), DefaultCriteria...)
	}
	prov := opts.Provenance
	prov.DiffHash = DiffHash(changed)
	if strings.TrimSpace(head) == "" {
		head = "worktree"
	}
	prov.Commit = head

	p := Plan{
		Version: VerifyVersion, Scope: scope,
		TaskID: strings.TrimSpace(opts.TaskID), MissionID: strings.TrimSpace(opts.MissionID),
		Changed: sortedChanged(changed), DiffHash: prov.DiffHash,
		Impact: imp, RiskLevel: effective, RiskScore: risk.Score,
		Escalation: escalation,
		Suite: impact.SuiteSelection{
			Focused: focused, Full: sel.Full, Omitted: omitted,
			Widened: sel.Widened || widen, Tier: tier,
			Metrics: sel.Metrics, Reasons: sel.Reasons,
		},
		Tier: tier, TierBudgetS: tierBudget(tier),
		Widened: sel.Widened || widen,
		Gates:   spec, GateReasons: gateReasons,
		Contracts: contracts, Breaking: breaking, Criteria: criteria,
		Provenance: prov,
	}
	p.ConfigID = configID(p)
	return p, nil
}

// configID hashes the derivation inputs: same commit, diff,
// contracts, criteria and severity always rebuild the same plan.
func configID(p Plan) string {
	parts := []string{
		p.Scope, p.TaskID, p.MissionID,
		strings.Join(p.Changed, "\n"), p.DiffHash,
		strings.Join(p.Contracts, "\n"), strings.Join(p.Criteria, "\n"),
		p.RiskLevel, p.Tier,
		p.Provenance.Commit, p.Provenance.Toolchain,
		p.Provenance.Host, p.Provenance.Agent, p.Provenance.Version,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// higherTier returns the highest of the given tiers (L0<L1<L2<L3).
func higherTier(tiers ...string) string {
	best := ""
	for _, t := range tiers {
		if riskTierRank(t) > riskTierRank(best) {
			best = t
		}
	}
	return best
}

// tierBudget resolves the run-budget ceiling for the plan tier from
// the F11.3 spec table (ceilings, never estimates).
func tierBudget(tier string) int {
	if spec, ok := bench.FindTierSpec(tier); ok {
		return spec.RunBudgetS
	}
	return 0
}

// sortedChanged normalizes and sorts the changed paths for storage.
func sortedChanged(changed []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range changed {
		n := strings.TrimSpace(strings.ReplaceAll(c, "\\", "/"))
		n = strings.TrimPrefix(n, "./")
		n = strings.TrimSuffix(n, "/")
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// sortedUnique returns the sorted deduplicated copy.
func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
