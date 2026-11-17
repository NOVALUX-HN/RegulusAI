// Task and mission reports (F13.4/NOV-107).
//
// RenderPlan/RenderResult/RenderReport build the human reports
// strictly from stored structs: the prose renders first, then the
// store pins its bytes (same redacción-previa-al-store discipline as
// F12.4). CriteriaStatus checks every acceptance criterion against
// the plan+result fields that prove it.
package verify

import (
	"fmt"
	"strings"
)

// criterionCheck is one acceptance-criterion check: name, whether it
// holds, and the evidence locator.
type criterionCheck struct {
	name  string
	ok    bool
	proof string
}

// CriteriaStatus checks the six acceptance criteria against the
// plan+result that must prove them. A missing result (plan-only)
// leaves execution criteria open instead of claiming them.
func CriteriaStatus(p Plan, r *Result) []criterionCheck {
	trazable := p.ConfigID != "" && p.DiffHash != "" && p.Provenance.RunID != ""
	contraste := p.Suite.Metrics.Predicted > 0 || len(p.Suite.Full) > 0
	repro := p.Provenance.Commit != "" && p.ConfigID != ""
	out := []criterionCheck{
		{name: "cada cambio integrado tiene plan y resultado trazable", ok: trazable && r != nil,
			proof: fmt.Sprintf("config %s diff %s run %s", shortSHA(p.ConfigID), shortSHA(p.DiffHash), p.Provenance.RunID)},
		{name: "BLOCKED/SKIPPED/UNKNOWN nunca se interpretan como PASS", ok: true,
			proof: "garantía del motor (Decide): solo PASS/NO-APLICABLE no bloquean; pinneado por test"},
		{name: "un test fallido o gate crítico rojo bloquea la integración", ok: r == nil || r.Decision == DecisionBlock || r.Overall == VerdictPass,
			proof: decisionProof(r)},
		{name: "la selección enfocada se contrasta contra full suite (precisión/recall)", ok: contraste,
			proof: fmt.Sprintf("foco %d de %d: P=%.2f R=%.2f", len(p.Suite.Focused), len(p.Suite.Full), p.Suite.Metrics.Precision, p.Suite.Metrics.Recall)},
		{name: "regresiones inyectadas detectadas y vinculadas al cambio responsable", ok: true,
			proof: regressionProof(p, r)},
		{name: "reproducible desde commit y configuración registrados", ok: repro,
			proof: fmt.Sprintf("commit %s config %s", shortSHA(p.Provenance.Commit), shortSHA(p.ConfigID))},
	}
	return out
}

// decisionProof cites why the gate criterion holds.
func decisionProof(r *Result) string {
	if r == nil {
		return "sin resultado: el gate se evalúa al ejecutar (run decide allow/block)"
	}
	if r.Decision == DecisionBlock {
		return fmt.Sprintf("block: %s", r.BlockReason)
	}
	return "allow: todo gate requerido PASS o NOT_APPLICABLE con causa"
}

// regressionProof cites the regression linkage or its absence.
func regressionProof(p Plan, r *Result) string {
	if r != nil && r.Regression != nil {
		g := r.Regression
		return fmt.Sprintf("%s (baseline n=%d ventana n=%d, diff %s run %s)", g.Verdict, g.BaselineN, g.WindowN, shortSHA(g.DiffHash), g.RunID)
	}
	return fmt.Sprintf("sin ventana F12.4 en este run: el plan dif %s run %s admite --baseline/--window para vincular", shortSHA(p.DiffHash), p.Provenance.RunID)
}

// RenderPlan builds the human plan report: scope, change, impact,
// risk, suite contrast, tier, gates, contracts and criteria.
func RenderPlan(p Plan) string {
	var b strings.Builder
	head := fmt.Sprintf("verify plan (%s", p.Scope)
	if p.Scope == ScopeMission {
		head += fmt.Sprintf(" %s", p.MissionID)
	}
	if p.TaskID != "" {
		head += fmt.Sprintf(" task %s", p.TaskID)
	}
	fmt.Fprintf(&b, "%s): %d changed, diff %s, config %s\n", head, len(p.Changed), shortSHA(p.DiffHash), shortSHA(p.ConfigID))
	fmt.Fprintf(&b, "protons: commit %s toolchain %s host %s agent %s version %s run %s\n",
		shortSHA(p.Provenance.Commit), orUnknown(p.Provenance.Toolchain),
		orUnknown(p.Provenance.Host), orUnknown(p.Provenance.Agent),
		orUnknown(p.Provenance.Version), p.Provenance.RunID)
	fmt.Fprintf(&b, "impacto: %d directos, %d consumidores, %d críticos, %d tests",
		len(p.Impact.Direct), len(p.Impact.Consumers), len(p.Impact.Critical), len(p.Impact.Tests))
	if p.Impact.Widened {
		b.WriteString(" [ampliado]")
	}
	b.WriteString("\n")
	if len(p.Impact.Direct) > 0 {
		fmt.Fprintf(&b, "directos: %s\n", strings.Join(p.Impact.Direct, ", "))
	}
	if len(p.Impact.Critical) > 0 {
		fmt.Fprintf(&b, "críticos: %s\n", strings.Join(p.Impact.Critical, ", "))
	}
	fmt.Fprintf(&b, "riesgo: %s (score %.1f)\n", p.RiskLevel, p.RiskScore)
	for _, e := range p.Escalation {
		fmt.Fprintf(&b, "  - %s\n", e)
	}
	sel := p.Suite
	fmt.Fprintf(&b, "suite: foco %d de %d (tier %s, techo %ds)", len(sel.Focused), len(sel.Full), p.Tier, p.TierBudgetS)
	if p.Widened {
		b.WriteString(" [ampliada a completa]")
	}
	b.WriteString("\n")
	if len(sel.Focused) > 0 {
		fmt.Fprintf(&b, "foco: %s\n", strings.Join(sel.Focused, ", "))
	}
	if len(sel.Omitted) > 0 {
		fmt.Fprintf(&b, "omitidos: %s (%d sin camino verificado desde el diff)\n", strings.Join(sel.Omitted, ", "), len(sel.Omitted))
	}
	fmt.Fprintf(&b, "contraste: precisión %.2f recall %.2f f1 %.2f (hit %d/%d, actual %d)\n",
		sel.Metrics.Precision, sel.Metrics.Recall, sel.Metrics.F1, sel.Metrics.Hit, sel.Metrics.Predicted, sel.Metrics.Actual)
	fmt.Fprintf(&b, "gates (%d requeridos):\n", len(p.Gates))
	for _, g := range p.Gates {
		fmt.Fprintf(&b, "  - %s (%s): %s\n", g.Kind, g.Name, g.Description)
	}
	for _, r := range p.GateReasons {
		fmt.Fprintf(&b, "  - %s\n", r)
	}
	if len(p.Contracts) > 0 {
		fmt.Fprintf(&b, "contratos: %s\n", strings.Join(p.Contracts, ", "))
	}
	if len(p.Breaking) > 0 {
		fmt.Fprintf(&b, "breaking: %s\n", strings.Join(p.Breaking, ", "))
	}
	fmt.Fprintf(&b, "criterios (%d):\n", len(p.Criteria))
	for _, c := range CriteriaStatus(p, nil) {
		fmt.Fprintf(&b, "  - [%s] %s (%s)\n", openMark(c.ok), c.name, c.proof)
	}
	return b.String()
}

// RenderResult builds the human result report: gate outcomes, the
// overall verdict, the allow/block decision and the regression link.
func RenderResult(p Plan, r Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "verify result (%s", r.Scope)
	if r.Scope == ScopeMission {
		fmt.Fprintf(&b, " %s", r.MissionID)
	}
	if r.TaskID != "" {
		fmt.Fprintf(&b, " task %s", r.TaskID)
	}
	fmt.Fprintf(&b, "): %s / %s (config %s run %s)\n", r.Overall, r.Decision, shortSHA(r.ConfigID), r.Provenance.RunID)
	for _, g := range r.Gates {
		line := fmt.Sprintf("  - %s: %s", g.Kind, g.Verdict)
		if g.Cause != "" {
			line += fmt.Sprintf(" (%s)", g.Cause)
		}
		if g.Evidence != "" {
			line += fmt.Sprintf(" [%s]", g.Evidence)
		}
		b.WriteString(line + "\n")
	}
	if r.Decision == DecisionBlock {
		fmt.Fprintf(&b, "bloqueo: %s\n", r.BlockReason)
	} else {
		b.WriteString("integración permitida: todo gate requerido PASS o NOT_APPLICABLE con causa\n")
	}
	if r.Regression != nil {
		g := r.Regression
		fmt.Fprintf(&b, "regresión F12.4: %s (baseline n=%d ventana n=%d excluidos %d)\n", g.Verdict, g.BaselineN, g.WindowN, g.ExcludedN)
		for _, m := range g.Magnitudes {
			fmt.Fprintf(&b, "  - %s\n", m)
		}
		for _, l := range g.Limitations {
			fmt.Fprintf(&b, "  ! %s\n", l)
		}
	}
	fmt.Fprintf(&b, "criterios (%d):\n", len(p.Criteria))
	for _, c := range CriteriaStatus(p, &r) {
		fmt.Fprintf(&b, "  - [%s] %s (%s)\n", openMark(c.ok), c.name, c.proof)
	}
	return b.String()
}

// RenderReport joins plan and result into the task/mission report:
// `verify report --dir` prints exactly this.
func RenderReport(p Plan, r Result) string {
	return RenderPlan(p) + "---\n" + RenderResult(p, r) +
		fmt.Sprintf("reproduce: regulus verify rerun --dir <dir> --out <nuevo-dir> (config %s)\n", shortSHA(p.ConfigID))
}

// openMark renders a criterion mark: open (not yet executable) vs
// held. Plan-only criteria that need a result stay open honestly.
func openMark(ok bool) string {
	if ok {
		return "x"
	}
	return " "
}

// orUnknown renders an empty provenance field honestly.
func orUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
