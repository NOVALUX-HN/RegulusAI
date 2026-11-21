// Plan explanation and quality reports (F13.2/NOV-105): why the
// work split the way it did, why some tasks serialise, and how the
// plan grades against its quality gates. Rendering only — the data
// stays in Plan/PlanDiff, byte-identical under --json.
package planning

import (
	"fmt"
	"strings"
)

// RenderPlan prints one plan revision: status, analysis, the task
// DAG with budgets and autonomy, and the quality verdict.
func RenderPlan(p *Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "plan v%d (%s) mission %s: %s\n", p.PlanVersion, p.Kind, p.MissionID, p.Status)
	fmt.Fprintf(&b, "  objective: %s\n", p.Objective)
	if p.ChangeReason != "" {
		fmt.Fprintf(&b, "  change: %s\n", p.ChangeReason)
	}
	if p.Author != "" {
		fmt.Fprintf(&b, "  author: %s at %s\n", p.Author, p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if p.RepoHead != "" {
		fmt.Fprintf(&b, "  repo: %s (%s)\n", shortSHA(p.RepoHead), p.WorktreeStatus)
	}
	for _, h := range p.Hypotheses {
		fmt.Fprintf(&b, "  hypothesis: %s\n", h)
	}
	for _, a := range p.Assumptions {
		fmt.Fprintf(&b, "  assumption: %s\n", a)
	}
	for _, r := range p.Risks {
		fmt.Fprintf(&b, "  risk: %s\n", r)
	}
	for _, q := range p.OpenQuestions {
		fmt.Fprintf(&b, "  question: %s (porque: %s; bloquea: %s)\n", q.Question, q.Why, q.Blocks)
	}
	for _, d := range p.Deliverables {
		fmt.Fprintf(&b, "  deliverable: %s\n", d)
	}
	for _, c := range p.Criteria {
		mark := "verificable"
		if !c.Verifiable {
			mark = "NO verificable: " + c.UnverifiableWs
		}
		fmt.Fprintf(&b, "  criterion [%s]: %s\n", mark, c.Text)
	}
	for _, t := range p.Tasks {
		fmt.Fprintf(&b, "- %s %s [%s] agent=%s\n", t.ID, t.Title, t.Autonomy, orDash(t.Agent))
		if t.Reviewer != "" {
			fmt.Fprintf(&b, "    reviewer: %s (par crítico)\n", t.Reviewer)
		}
		fmt.Fprintf(&b, "    objective: %s\n", t.Objective)
		fmt.Fprintf(&b, "    deliverable: %s\n", t.Deliverable)
		if len(t.DependsOn) > 0 {
			fmt.Fprintf(&b, "    depends_on: %s\n", strings.Join(t.DependsOn, ", "))
		}
		if len(t.Excludes) > 0 {
			fmt.Fprintf(&b, "    excludes: %s\n", strings.Join(t.Excludes, ", "))
		}
		fmt.Fprintf(&b, "    scope: %s\n", strings.Join(t.Scope.Files, ", "))
		if len(t.Scope.Components) > 0 {
			fmt.Fprintf(&b, "    components: %s\n", strings.Join(t.Scope.Components, ", "))
		}
		fmt.Fprintf(&b, "    capabilities: %s\n", strings.Join(t.RequiredCapabilities, ", "))
		for _, ev := range t.CapabilityEvidence {
			fmt.Fprintf(&b, "      evidence: %s\n", ev)
		}
		fmt.Fprintf(&b, "    min_scopes: [%s]\n", strings.Join(t.MinScopes, ", "))
		if len(t.ContextRefs) > 0 {
			fmt.Fprintf(&b, "    context: %s\n", strings.Join(t.ContextRefs, ", "))
		}
		if len(t.Tests) > 0 {
			fmt.Fprintf(&b, "    tests: %s\n", strings.Join(t.Tests, ", "))
		}
		fmt.Fprintf(&b, "    verify: %s\n", t.Verification.Strategy)
		for _, c := range t.Verification.Criteria {
			fmt.Fprintf(&b, "      - %s\n", c)
		}
		fmt.Fprintf(&b, "    estimate: %s/%s time=%dms tokens=%d cost=%s (%s)\n",
			t.Estimate.Complexity, t.Estimate.Risk, t.Estimate.TimeMS, t.Estimate.Tokens,
			costStr(t.Estimate), t.Estimate.Note)
		if len(t.Blockers) > 0 {
			for _, bl := range t.Blockers {
				fmt.Fprintf(&b, "    blocker: %s\n", bl)
			}
		}
		if t.Rationale != "" {
			fmt.Fprintf(&b, "    why: %s\n", t.Rationale)
		}
	}
	fmt.Fprintf(&b, "  dag order: %s\n", strings.Join(p.DAG.Order, " -> "))
	for i, lv := range p.DAG.Levels {
		fmt.Fprintf(&b, "    level %d: %s (paralelizable)\n", i, strings.Join(lv, ", "))
	}
	for _, c := range p.DAG.Collisions {
		fmt.Fprintf(&b, "    serialized: %s antes que %s (colisión de ficheros)\n", c[0], c[1])
	}
	for _, e := range p.Exclusions {
		fmt.Fprintf(&b, "  excludes: %s\n", e)
	}
	for _, c := range p.Confirmed {
		fmt.Fprintf(&b, "  confirmed: %s = %q at %s\n", c.TaskID, c.Result, c.At.UTC().Format("2006-01-02T15:04:05Z"))
	}
	fmt.Fprintf(&b, "%s", RenderQuality(p.Quality))
	return b.String()
}

// RenderQuality prints the quality gates with their verdicts.
func RenderQuality(q Quality) string {
	var b strings.Builder
	pass := 0
	for _, c := range q.Checks {
		mark := "PASS"
		if !c.Pass {
			mark = "FAIL"
		} else {
			pass++
		}
		fmt.Fprintf(&b, "  quality [%s] %s: %s\n", mark, c.Name, c.Detail)
	}
	fmt.Fprintf(&b, "  quality: %d/%d gates en verde\n", pass, len(q.Checks))
	for _, w := range q.Warnings {
		fmt.Fprintf(&b, "  warning: %s\n", w)
	}
	return b.String()
}

// RenderExplain prints why the plan looks the way it does: the
// split rationale, the serialisation rationale, the autonomy table
// and the revision history with diffs against the original and the
// previous version.
func RenderExplain(current *Plan, history []*Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "explain plan v%d mission %s\n", current.PlanVersion, current.MissionID)
	fmt.Fprintf(&b, "why split: el trabajo se dividió por clúster de cambio — cada componente (%s) tiene distinta razón de cambio y distinto riesgo, así se revisa, revierte y verifica por separado. La tarea de relevo (t1) precede a todo cambio para confirmar el alcance contra el repo real antes de gastar presupuesto.\n",
		orUnknown(strings.Join(componentsOf(current), ", ")))
	if len(current.DAG.Collisions) == 0 {
		fmt.Fprintf(&b, "why serialize: sin colisiones de ficheros; el orden lo dictan solo las dependencias explícitas (niveles paralelizables arriba).\n")
	} else {
		fmt.Fprintf(&b, "why serialize: %d colisión(es) de ficheros obligan orden total donde hay estado compartido:\n", len(current.DAG.Collisions))
		for _, c := range current.DAG.Collisions {
			fmt.Fprintf(&b, "  - %s antes que %s: comparten ficheros, mismo nivel los rompería (la colisión manda sobre el paralelismo)\n", c[0], c[1])
		}
	}
	fmt.Fprintf(&b, "autonomy:\n")
	for _, t := range current.Tasks {
		fmt.Fprintf(&b, "  - %s: %s", t.ID, t.Autonomy)
		if len(t.Blockers) > 0 {
			fmt.Fprintf(&b, " (bloquea: %s)", strings.Join(t.Blockers, "; "))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s", RenderQuality(current.Quality))
	if len(history) > 0 {
		fmt.Fprintf(&b, "history: %d revision(es), ninguna borrada\n", len(history))
		for _, h := range history {
			fmt.Fprintf(&b, "  - v%d %s %s", h.PlanVersion, h.Status, h.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"))
			if h.ChangeReason != "" {
				fmt.Fprintf(&b, " (motivo: %s)", h.ChangeReason)
			}
			b.WriteString("\n")
		}
		original := history[0]
		if original.PlanVersion != current.PlanVersion {
			d := DiffPlans(original, current)
			fmt.Fprintf(&b, "diff vs original (v%d → v%d): %s\n", d.From, d.To, diffLine(d))
		}
		if len(history) >= 2 {
			prev := history[len(history)-1]
			if prev.PlanVersion == current.PlanVersion && len(history) >= 2 {
				prev = history[len(history)-2]
			}
			if prev.PlanVersion != current.PlanVersion {
				d := DiffPlans(prev, current)
				fmt.Fprintf(&b, "diff vs previous (v%d → v%d): %s\n", d.From, d.To, diffLine(d))
			}
		}
	}
	if len(current.Confirmed) > 0 {
		fmt.Fprintf(&b, "confirmed results travel across replans (%d):\n", len(current.Confirmed))
		for _, c := range current.Confirmed {
			fmt.Fprintf(&b, "  - %s = %q\n", c.TaskID, c.Result)
		}
	}
	return b.String()
}

// diffLine renders one diff in a single line.
func diffLine(d PlanDiff) string {
	parts := []string{}
	if len(d.Added) > 0 {
		parts = append(parts, "añadidas "+strings.Join(d.Added, ","))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, "eliminadas "+strings.Join(d.Removed, ","))
	}
	for _, c := range d.Changed {
		parts = append(parts, c.ID+" ("+strings.Join(c.Fields, ",")+")")
	}
	if d.StatusChange != "" {
		parts = append(parts, "estado "+d.StatusChange)
	}
	if len(parts) == 0 {
		return "sin cambios"
	}
	s := strings.Join(parts, "; ")
	if d.ReasonChange != "" {
		s += " — motivo: " + d.ReasonChange
	}
	return s
}

// RenderDiff prints a version-to-version comparison.
func RenderDiff(d PlanDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff v%d → v%d: %s\n", d.From, d.To, diffLine(d))
	return b.String()
}

// componentsOf collects the plan scope components.
func componentsOf(p *Plan) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range p.Tasks {
		for _, c := range t.Scope.Components {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// orDash prints "-" for empty agent slots (delimitation tasks run
// with the user, not with an agent).
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// orUnknown prints the fallback for empty lists.
func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(alcance por delimitar)"
	}
	return s
}

// costStr renders money honestly: unknown is never zero.
func costStr(e Estimate) string {
	if !e.CostKnown {
		return "desconocido (nunca cero inventado)"
	}
	if e.Currency != "" {
		return fmt.Sprintf("%v %s", e.Cost, e.Currency)
	}
	return fmt.Sprintf("%v", e.Cost)
}
