// Deterministic routing decision (F12.1/NOV-96): eligibility gates,
// explicable scoring, one or two agents, justified fallback or
// justified no-delegation.
//
// The decision is pure (Decide has no clock, no disk, no network):
// the same registry, matrix, history, descriptor, budgets and limits
// always produce the byte-identical decision. Recording (evidence
// dir, F11.1 trace) is the CLI's job, skipped on dry-run.
package orchestrate

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mcp"
)

// FactorScore is one scored dimension: name, 0..1 value, weight and
// the human-readable why behind the number.
type FactorScore struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Weight float64 `json:"weight"`
	Why    string  `json:"why"`
}

// RankedAgent is one eligible agent with its total and breakdown.
type RankedAgent struct {
	AgentID string        `json:"agent_id"`
	Host    string        `json:"host"`
	Total   float64       `json:"total"`
	Factors []FactorScore `json:"factors"`
}

// Exclusion is one refused agent with its naming reason: every
// exclusion says which gate fired and why, so the decision explains
// who was left out as well as who was picked.
type Exclusion struct {
	AgentID string `json:"agent_id"`
	Reason  string `json:"reason"`
}

// Decision is one routing verdict: the ranked eligible agents, the
// exclusions, the primary (and reviewer when the risk demands a
// pair), the fallback with its restricted scopes or the justified
// no-delegation, the budgets and weights it ran with, and the
// fixed-routing contrast.
type Decision struct {
	Version            string         `json:"version"`
	Task               TaskDescriptor `json:"task"`
	Weights            ScoreWeights   `json:"weights"`
	Ranking            []RankedAgent  `json:"ranking"`
	Excluded           []Exclusion    `json:"excluded"`
	Mode               string         `json:"mode"`
	Primary            string         `json:"primary"`
	Reviewer           string         `json:"reviewer,omitempty"`
	Fallback           string         `json:"fallback,omitempty"`
	FallbackScopes     []string       `json:"fallback_scopes,omitempty"`
	FallbackReason     string         `json:"fallback_reason"`
	NoDelegationReason string         `json:"no_delegation_reason,omitempty"`
	Budgets            Budgets        `json:"budgets"`
	MaxDelegations     int            `json:"max_delegations"`
	FixedAgent         string         `json:"fixed_agent"`
	FixedContrast      string         `json:"fixed_contrast"`
	Justification      []string       `json:"justification"`
	DryRun             bool           `json:"dry_run"`
	TraceHint          string         `json:"trace_hint,omitempty"`
}

// DecideModes are the closed mode vocabulary.
const (
	ModeSingle = "single"
	ModePair   = "pair"
)

// Decide routes one task descriptor over the registry: gates first
// (availability, verified capabilities, risk ceiling, budgets), then
// explicable scoring, then one agent — or a primary plus reviewer
// when the risk is critical — with a fallback or a justified
// no-delegation. An empty primary with a non-empty exclusion list is
// the refusal: nothing eligible, nothing invented.
func Decide(reg []RoutedAgent, manifests map[string]bench.HostManifest, hist []AgentHistory, d TaskDescriptor, b Budgets, maxDeleg int, fixedAgent string, w ScoreWeights) (Decision, error) {
	if err := ValidateTaskDescriptor(d); err != nil {
		return Decision{}, err
	}
	if err := ValidateBudgets(b); err != nil {
		return Decision{}, err
	}
	if err := ValidateMaxDelegations(maxDeleg); err != nil {
		return Decision{}, err
	}
	if err := w.validate(); err != nil {
		return Decision{}, err
	}
	dec := Decision{
		Version: RoutingVersion, Task: d, Weights: w,
		Budgets: b, MaxDelegations: maxDeleg, FixedAgent: fixedAgent,
	}
	byHist := historyByID(hist)
	var ranked []RankedAgent
	for _, a := range reg {
		if reason, ok := eligible(a, d, b, byHist); !ok {
			dec.Excluded = append(dec.Excluded, Exclusion{AgentID: a.ID, Reason: reason})
			continue
		}
		if reason, ok := checkVerified(a, d.RequiredCapabilities, manifests); !ok {
			dec.Excluded = append(dec.Excluded, Exclusion{AgentID: a.ID, Reason: reason})
			continue
		}
		ranked = append(ranked, scoreAgent(a, d, b, byHist, manifests, w))
	}
	normalizeRelative(ranked)
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Total != ranked[j].Total {
			return ranked[i].Total > ranked[j].Total
		}
		return ranked[i].AgentID < ranked[j].AgentID
	})
	dec.Ranking = ranked
	sort.Slice(dec.Excluded, func(i, j int) bool { return dec.Excluded[i].AgentID < dec.Excluded[j].AgentID })
	if len(ranked) == 0 {
		dec.Justification = []string{"sin agente elegible: ninguna capability sin evidencia fue asignada; amplía budgets, publica evidencia o baja el riesgo"}
		dec.FixedContrast = fixedContrast("", fixedAgent, reg, dec)
		return dec, nil
	}
	if d.Risk == RiskCritical {
		dec.Mode = ModePair
		dec.Primary = ranked[0].AgentID
		if len(ranked) < 2 {
			dec.Excluded = append(dec.Excluded, Exclusion{AgentID: "(revisor)", Reason: "tarea crítica exige revisión por un segundo agente elegible; solo hay uno"})
			dec.Justification = []string{
				fmt.Sprintf("tarea crítica: %s lidera pero falta revisor; no se ejecuta sin par", dec.Primary),
			}
			dec.FixedContrast = fixedContrast("", fixedAgent, reg, dec)
			dec.Primary = ""
			return dec, nil
		}
		dec.Reviewer = ranked[1].AgentID
		dec.Justification = []string{
			fmt.Sprintf("riesgo crítico: par %s (ejecuta) + %s (revisa); un solo agente no basta", dec.Primary, dec.Reviewer),
			fmt.Sprintf("%s lidera con %.3f: %s", dec.Primary, ranked[0].Total, topFactor(ranked[0])),
		}
	} else {
		dec.Mode = ModeSingle
		dec.Primary = ranked[0].AgentID
		dec.Justification = []string{
			fmt.Sprintf("%s lidera con %.3f: %s", dec.Primary, ranked[0].Total, topFactor(ranked[0])),
		}
		if d.Risk == RiskLow || d.Risk == RiskMedium {
			dec.Justification = append(dec.Justification,
				fmt.Sprintf("tarea %s de complejidad %s: un agente basta, sin revisor", d.Risk, d.Complexity))
		} else {
			dec.Justification = append(dec.Justification,
				fmt.Sprintf("tarea %s no crítica: un agente basta; el segundo mejor queda como fallback", d.Risk))
		}
	}
	assignFallback(&dec, ranked, reg, maxDeleg)
	dec.FixedContrast = fixedContrast(dec.Primary, fixedAgent, reg, dec)
	return dec, nil
}

// eligible applies the non-capability gates in fixed order:
// availability, solo risk ceiling, then budgets against history
// means (or declared estimates when the agent has no history).
// Cost gates only on known money (F11.5 honesty); unknown cost
// never refuses — it scores neutral with a note instead.
func eligible(a RoutedAgent, d TaskDescriptor, b Budgets, byHist map[string]AgentHistory) (string, bool) {
	if !a.Available {
		return "no disponible (host caído o agente pausado; se excluye, no se reintenta)", false
	}
	// Solo risk ceiling: an agent takes tasks up to its ceiling. A
	// critical task additionally needs a reviewer pair (decided
	// later), so high-ceiling agents stay eligible as pair
	// candidates while lower ceilings are refused here.
	if d.Risk == RiskCritical {
		if riskLevel(a.MaxRisk) < riskLevel(RiskHigh) {
			return fmt.Sprintf("riesgo critical exige techo high y par revisor; techo del agente %s", a.MaxRisk), false
		}
	} else if riskLevel(d.Risk) > riskLevel(a.MaxRisk) {
		return fmt.Sprintf("riesgo %s sobre el techo solo %s del agente", d.Risk, a.MaxRisk), false
	}
	h, hasHist := byHist[a.ID]
	estMS, estTok := a.EstimateTimeMS, a.EstimateTokens
	estCost, costKnown := a.EstimateCost, a.EstimateCost >= 0
	estNote := "estimación declarada"
	if hasHist && h.Samples > 0 {
		estMS = h.MedianLatencyMS
		estTok = h.MeanTokens
		estNote = fmt.Sprintf("media histórica (n=%d)", h.Samples)
		if h.CostKnown {
			estCost, costKnown = h.MeanCost, true
			estNote += " con coste conocido"
		} else {
			costKnown = false
		}
	}
	if b.TimeMS > 0 && estMS > b.TimeMS {
		return fmt.Sprintf("latencia estimada %dms sobre el budget %dms (%s)", estMS, b.TimeMS, estNote), false
	}
	if b.Tokens > 0 && estTok > b.Tokens {
		return fmt.Sprintf("tokens estimados %d sobre el budget %d (%s)", estTok, b.Tokens, estNote), false
	}
	if b.Cost > 0 && costKnown && estCost > b.Cost {
		return fmt.Sprintf("coste estimado %.4f sobre el budget %.4f (%s)", estCost, b.Cost, estNote), false
	}
	return "", true
}

// scoreAgent prices one eligible agent on the five weighted factors.
// Latency and cost are relative (best eligible estimate = 1.0);
// quality is absolute (success rate, neutral 0.5 without history);
// capability is the verified breadth of the host; risk is the fit
// margin between agent ceiling and task risk.
func scoreAgent(a RoutedAgent, d TaskDescriptor, b Budgets, byHist map[string]AgentHistory, manifests map[string]bench.HostManifest, w ScoreWeights) RankedAgent {
	r := RankedAgent{AgentID: a.ID, Host: a.Host}
	cap := capabilityFactor(a, manifests)
	risk := riskFactor(a, d)
	qual := qualityFactor(a, byHist)
	lat := latencyFactor(a, byHist)
	cost := costFactor(a, byHist)
	r.Factors = []FactorScore{
		{Name: "capability", Value: cap.value, Weight: w.Capability, Why: cap.why},
		{Name: "risk", Value: risk.value, Weight: w.Risk, Why: risk.why},
		{Name: "quality", Value: qual.value, Weight: w.Quality, Why: qual.why},
		{Name: "latency", Value: lat.value, Weight: w.Latency, Why: lat.why},
		{Name: "cost", Value: cost.value, Weight: w.Cost, Why: cost.why},
	}
	r.Total = cap.value*w.Capability + risk.value*w.Risk + qual.value*w.Quality + lat.value*w.Latency + cost.value*w.Cost
	return r
}

type factor struct {
	value float64
	why   string
}

// capabilityFactor scores verified breadth: the share of the 12
// F11.7 dimensions the host holds probada. Every eligible agent
// already covers every required capability; breadth breaks ties
// toward better-evidenced hosts.
func capabilityFactor(a RoutedAgent, manifests map[string]bench.HostManifest) factor {
	m, ok := manifests[a.Host]
	if !ok {
		return factor{0, fmt.Sprintf("host %s fuera de la matriz: 0/12", a.Host)}
	}
	got := 0
	for _, id := range bench.CapabilityIDs() {
		if c, found := m.Capability(id); found && c.Status == bench.CapVerified {
			got++
		}
	}
	return factor{
		value: float64(got) / float64(len(bench.CapabilityIDs())),
		why:   fmt.Sprintf("host %s verifica %d/12 capabilities probadas", a.Host, got),
	}
}

// riskFactor scores the fit margin: exact ceiling match = 1.0,
// overqualified by one level = 0.9, by two = 0.8. Underqualified
// never scores — eligibility already refused it.
func riskFactor(a RoutedAgent, d TaskDescriptor) factor {
	margin := riskLevel(a.MaxRisk) - riskLevel(d.Risk)
	if margin < 0 {
		margin = 0
	}
	v := 1.0 - 0.1*float64(margin)
	if v < 0.5 {
		v = 0.5
	}
	return factor{v, fmt.Sprintf("techo %s frente a riesgo %s (margen %d)", a.MaxRisk, d.Risk, margin)}
}

// qualityFactor scores proven success rate over evaluable attempts;
// without evaluable history it is neutral 0.5 with a note (unknown
// never punishes, never rewards).
func qualityFactor(a RoutedAgent, byHist map[string]AgentHistory) factor {
	if h, ok := byHist[a.ID]; ok && h.SuccessRateOK {
		return factor{h.SuccessRate, fmt.Sprintf("éxito %.2f en %d/%d evaluables", h.SuccessRate, h.Pass, h.Evaluable)}
	}
	return factor{0.5, "sin historial evaluable: neutral 0.5"}
}

// latencyFactor scores relative speed: the fastest eligible
// estimate is 1.0, the rest scale as best/estimate. It is finished
// in normalizeRelative once every candidate is scored.
func latencyFactor(a RoutedAgent, byHist map[string]AgentHistory) factor {
	if h, ok := byHist[a.ID]; ok && h.Samples > 0 {
		note := fmt.Sprintf("mediana histórica %dms (n=%d)", h.MedianLatencyMS, h.Samples)
		if !h.LatencyOK && h.LatencyNote != "" {
			note += "; " + h.LatencyNote
		}
		return factor{-float64(h.MedianLatencyMS), note}
	}
	if a.EstimateTimeMS > 0 {
		return factor{-float64(a.EstimateTimeMS), fmt.Sprintf("estimación declarada %dms (sin historial)", a.EstimateTimeMS)}
	}
	return factor{0, "sin latencia conocida: 0"}
}

// costFactor scores relative cheapness like latency; unknown money
// (no history cost, no declared estimate) is neutral 0.5 with a
// note instead of a fake zero.
func costFactor(a RoutedAgent, byHist map[string]AgentHistory) factor {
	if h, ok := byHist[a.ID]; ok && h.CostKnown {
		return factor{-h.MeanCost, fmt.Sprintf("coste medio histórico %.4f %s (n=%d)", h.MeanCost, h.Currency, h.Samples)}
	}
	if a.EstimateCost >= 0 {
		return factor{-a.EstimateCost, fmt.Sprintf("coste declarado %.4f (sin historial)", a.EstimateCost)}
	}
	return factor{0.5, "coste desconocido: neutral 0.5 (nunca cero inventado)"}
}

// normalizeRelative rescales the latency/cost factors relatively:
// the best (lowest) raw estimate becomes 1.0, the rest best/raw.
// Non-positive raws are placeholders (unknown cost): they keep
// their neutral value and carry the note.
func normalizeRelative(ranked []RankedAgent) {
	for _, name := range []string{"latency", "cost"} {
		best := 0.0
		first := true
		for _, r := range ranked {
			for _, f := range r.Factors {
				if f.Name != name || f.Value >= 0 {
					continue
				}
				raw := -f.Value
				if first || raw < best {
					best, first = raw, false
				}
			}
		}
		if first || best <= 0 {
			continue
		}
		for i := range ranked {
			for j := range ranked[i].Factors {
				f := &ranked[i].Factors[j]
				if f.Name != name || f.Value >= 0 {
					continue
				}
				raw := -f.Value
				f.Value = best / raw
				f.Why += fmt.Sprintf(" → relativo %.3f (mejor %s %.0f)", f.Value, name, best)
			}
		}
	}
	for i := range ranked {
		t := 0.0
		for _, f := range ranked[i].Factors {
			t += f.Value * f.Weight
		}
		ranked[i].Total = t
	}
}

// topFactor names the highest weighted contribution for the
// one-line justification.
func topFactor(r RankedAgent) string {
	best, contrib := "", -1.0
	for _, f := range r.Factors {
		if c := f.Value * f.Weight; c > contrib {
			best, contrib = f.Name, c
		}
	}
	return fmt.Sprintf("aporta más %s", best)
}

// assignFallback names the best non-selected eligible agent as the
// fallback with F11.8-restricted scopes (intersection with the
// primary via mcp.Principal.Delegate: delegating only restricts),
// or justifies why there is no delegation.
func assignFallback(dec *Decision, ranked []RankedAgent, reg []RoutedAgent, maxDeleg int) {
	byID := map[string]RoutedAgent{}
	for _, a := range reg {
		byID[a.ID] = a
	}
	picked := map[string]bool{dec.Primary: true, dec.Reviewer: true}
	var next *RankedAgent
	for i := range ranked {
		if !picked[ranked[i].AgentID] {
			next = &ranked[i]
			break
		}
	}
	primary, ok := byID[dec.Primary]
	if !ok {
		dec.FallbackReason = "sin primario: no hay fallback que asignar"
		return
	}
	if maxDeleg == 0 {
		dec.NoDelegationReason = fmt.Sprintf("límite de delegación 0: %s ejecuta sin delegar", dec.Primary)
		dec.FallbackReason = "delegación prohibida por --max-delegations 0 (no se nombra fallback)"
		return
	}
	if next == nil {
		dec.NoDelegationReason = fmt.Sprintf("sin otro agente elegible: %s ejecuta sin delegar", dec.Primary)
		dec.FallbackReason = "sin fallback: ningún otro agente elegible; no se delega a ciegas"
		return
	}
	fb, ok := byID[next.AgentID]
	if !ok {
		dec.FallbackReason = "sin fallback: candidato fuera del registry"
		return
	}
	parent := mcp.Principal{Scopes: scopesToSet(primary.Scopes)}
	child := parent.Delegate(fb.ID, fb.Scopes)
	dec.Fallback = fb.ID
	dec.FallbackScopes = child.GrantedScopes()
	dec.FallbackReason = fmt.Sprintf("fallback %s con scopes restringidos a %v (F11.8: delegar solo restringe; %s→%s)",
		fb.ID, dec.FallbackScopes, dec.Primary, fb.ID)
}

// scopesToSet folds a scope list into the F11.8 principal set.
func scopesToSet(scopes []string) map[string]bool {
	m := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		if strings.TrimSpace(s) != "" {
			m[s] = true
		}
	}
	return m
}

// fixedContrast contrasts the dynamic pick against fixed routing
// (always the named fixed agent, default the Lab control baseline):
// coincide, difiere con motivo, o el fijo ni siquiera era elegible.
func fixedContrast(primary, fixedAgent string, reg []RoutedAgent, dec Decision) string {
	if strings.TrimSpace(fixedAgent) == "" {
		fixedAgent = "baseline"
	}
	if primary == "" {
		return fmt.Sprintf("routing fijo (%s): sin decisión dinámica que contrastar (%d exclusiones)", fixedAgent, len(dec.Excluded))
	}
	if primary == fixedAgent {
		return fmt.Sprintf("routing fijo (%s): coincide — la dinámica no aporta cambio aquí", fixedAgent)
	}
	for _, e := range dec.Excluded {
		if e.AgentID == fixedAgent {
			return fmt.Sprintf("routing fijo (%s): no elegible aquí (%s); la dinámica lo habría excluido también", fixedAgent, e.Reason)
		}
	}
	return fmt.Sprintf("routing fijo (%s): la dinámica elige %s por %.3f frente al fijo", fixedAgent, primary, dec.Ranking[0].Total)
}
