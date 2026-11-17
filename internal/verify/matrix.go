// Risk→gates matrix (F13.4/NOV-107): which gates a change must pass
// before it merges, integrates or advances the mission.
//
// The matrix is data, not prose: RequiredGates is the single function
// every plan, run and report consults, so the documented table and the
// enforced behavior can never drift apart. Escalation (EscalateRisk)
// is the "cuándo ampliar" rule: uncertainty only ever adds gates and
// levels, never removes them — a weak prediction may widen the suite,
// never narrow it.
package verify

import (
	"fmt"
	"sort"
	"strings"
)

// GateSpec is one matrix row: what runs, the exact command that
// reproduces it, and why the risk level requires it.
type GateSpec struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
}

// AllGates is the documented matrix: seven gates in pipeline order.
// Commands name the canonical reproduction; the run evidence cites
// the exact invocation per gate.
func AllGates() []GateSpec {
	return []GateSpec{
		{Kind: GateBuild, Name: "compilación",
			Command:     "go build ./...",
			Description: "el cambio compila en el toolchain registrado"},
		{Kind: GateStatic, Name: "análisis estático",
			Command:     "go vet ./...",
			Description: "vet limpio sobre el foco afectado"},
		{Kind: GateUnit, Name: "unit",
			Command:     "go test (foco verificado)",
			Description: "tests unitarios del foco: cada fallo bloquea"},
		{Kind: GateIntegration, Name: "integration",
			Command:     "go test -run Integration (foco)",
			Description: "integración del foco cuando el riesgo la exige"},
		{Kind: GateE2E, Name: "e2e",
			Command:     "e2e del flujo tocado",
			Description: "extremo a extremo del flujo que el diff toca"},
		{Kind: GateSecurity, Name: "security/adversarial",
			Command:     "security+adversarial review",
			Description: "revisión security/adversarial en superficie crítica o contratos"},
		{Kind: GateMutation, Name: "mutation/fuzz",
			Command:     "mutation/fuzz acotado",
			Description: "mutación/fuzz acotado ante riesgo crítico"},
	}
}

// gateByKind indexes the matrix rows.
func gateByKind() map[string]GateSpec {
	m := map[string]GateSpec{}
	for _, g := range AllGates() {
		m[g.Kind] = g
	}
	return m
}

// requiredKinds maps the effective risk onto gate kinds. Higher risk
// only adds gates: low ⊂ medium ⊂ high ⊂ critical.
func requiredKinds(risk string) []string {
	switch risk {
	case RiskCritical:
		return append([]string(nil), AllGateKinds...)
	case RiskHigh:
		return []string{GateBuild, GateStatic, GateUnit, GateIntegration, GateE2E, GateSecurity}
	case RiskMedium:
		return []string{GateBuild, GateStatic, GateUnit, GateIntegration}
	default:
		return []string{GateBuild, GateStatic, GateUnit}
	}
}

// EscalateRisk folds uncertainty onto the impact base level. Every
// term only raises: widened (unknown input → full suite) +1, F12.4
// severity ≥ 0.50 (degradación observada) +1, contratos tocados
// fijan high mínimo, breaking fija critical. Returns the effective
// level plus one reason line per applied term.
func EscalateRisk(base string, widened bool, severity float64, contractChanged, breaking bool) (string, []string) {
	rank := riskRank(base)
	if rank < 0 {
		rank = 0
	}
	var reasons []string
	if widened {
		if rank < riskRank(RiskCritical) {
			rank++
		}
		reasons = append(reasons, "ampliación conservadora (impacto widened): el riesgo sube un nivel, nunca baja por inferencia débil")
	}
	if severity >= 0.50 {
		if rank < riskRank(RiskCritical) {
			rank++
		}
		reasons = append(reasons, fmt.Sprintf("severidad F12.4 observada %.2f ≥ 0.50: la degradación medida suma, nunca resta", severity))
	}
	if contractChanged && rank < riskRank(RiskHigh) {
		rank = riskRank(RiskHigh)
		reasons = append(reasons, "contratos tocados: high mínimo (e2e+security obligatorios)")
	}
	if breaking {
		rank = riskRank(RiskCritical)
		reasons = append(reasons, "cambio de contrato breaking: critical (toda la matriz, mutation/fuzz incluidos)")
	}
	for _, r := range []string{RiskLow, RiskMedium, RiskHigh, RiskCritical} {
		if riskRank(r) == rank {
			return r, reasons
		}
	}
	return RiskLow, reasons
}

// RequiredGates resolves the effective-risk gate set over the
// matrix, in pipeline order. required[kind] is true for matrix
// members; reasons name the matrix row plus every escalation term
// so the report audits itself.
func RequiredGates(effectiveRisk string, contractChanged, breaking bool, escalation []string) ([]GateSpec, map[string]bool, []string) {
	byKind := gateByKind()
	kinds := requiredKinds(effectiveRisk)
	required := map[string]bool{}
	var specs []GateSpec
	for _, k := range kinds {
		specs = append(specs, byKind[k])
		required[k] = true
	}
	reasons := []string{
		fmt.Sprintf("matriz riesgo→gates: nivel %s exige %s", effectiveRisk, strings.Join(kinds, ", ")),
	}
	if contractChanged {
		reasons = append(reasons, "contratos en el diff: e2e+security verifican la superficie prometida")
	}
	if breaking {
		reasons = append(reasons, "breaking: mutation/fuzz acota el daño del cambio incompatible")
	}
	reasons = append(reasons, escalation...)
	return specs, required, reasons
}

// ShouldWiden reports whether the focused suite must widen to full:
// unknown input (impact widened), any critical node, breaking
// contracts, or a weak tier override. The caller keeps the focused
// precision/recall contrast either way — widening changes what runs,
// never what gets measured.
func ShouldWiden(widened, hasCritical, breaking bool, tierOverride, neededTier string) (bool, []string) {
	var reasons []string
	widen := false
	if widened {
		widen = true
		reasons = append(reasons, "impacto widened (entrada sin nodo verificado): la selección usa la suite completa")
	}
	if hasCritical {
		widen = true
		reasons = append(reasons, "nodo crítico en el camino: ningún test crítico se omite por inferencia débil")
	}
	if breaking {
		widen = true
		reasons = append(reasons, "breaking: el cambio incompatible se verifica contra todo, no contra el foco")
	}
	if tierOverride != "" && riskTierRank(tierOverride) < riskTierRank(neededTier) && (widened || hasCritical || breaking) {
		widen = true
		reasons = append(reasons, fmt.Sprintf("override %s bajo el necesario %s con incertidumbre: se rechaza el estrechamiento (foco débil no omite)", tierOverride, neededTier))
	}
	if !widen {
		reasons = append(reasons, "foco verificado suficiente: el diff mapea a nodos con camino, sin críticos ni breaking")
	}
	return widen, reasons
}

// riskTierRank orders tiers for the floor comparison (L0<L1<L2<L3).
func riskTierRank(t string) int {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "L0":
		return 0
	case "L1":
		return 1
	case "L2":
		return 2
	case "L3":
		return 3
	}
	return -1
}

// RiskTierFloor maps the effective risk onto the minimum F11.3 tier:
// low rides smoke/focus, medium focus, high and critical regression.
// L3 never comes from focus (periodic/release only, holdout sellado).
func RiskTierFloor(risk string) string {
	switch risk {
	case RiskCritical, RiskHigh:
		return "L2"
	case RiskMedium:
		return "L1"
	default:
		return "L0"
	}
}

// MatrixDoc renders the risk→gates table for `verify matrix` and the
// docs: generated from the same matrix the engine enforces.
func MatrixDoc() string {
	var b strings.Builder
	b.WriteString("matriz riesgo→gates (verify-v1, 7 gates en orden pipeline):\n")
	byKind := gateByKind()
	for _, r := range []string{RiskLow, RiskMedium, RiskHigh, RiskCritical} {
		kinds := requiredKinds(r)
		var names []string
		for _, k := range kinds {
			names = append(names, fmt.Sprintf("%s (%s)", k, byKind[k].Name))
		}
		sort.Strings(names)
		b.WriteString(fmt.Sprintf("  %s: %s [tier mínimo %s]\n", r, strings.Join(kinds, ", "), RiskTierFloor(r)))
	}
	b.WriteString("escalas que solo suben: widened +1, severidad F12.4 ≥0.50 +1, contratos→high mínimo, breaking→critical.\n")
	b.WriteString("BLOCKED/SKIPPED/UNKNOWN en un gate requerido bloquean (nunca PASS); NOT_APPLICABLE con causa no bloquea.\n")
	return b.String()
}
