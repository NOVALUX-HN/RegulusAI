// Routing registry (F12.1/NOV-96): the inventory of routable agents
// with their capabilities, evidence and limits.
//
// No parallel agent shape: every entry references its bench.LabAgent
// (F11.2: responsibility, tools, host, inputs) by id, and every
// capability verdict reuses the F11.7 host matrix
// (bench.DefaultHostManifests): only estado probada counts as
// verified, and a probada claim without evidence is invalid — such
// an agent can never be assigned that capability.
package orchestrate

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mcp"
)

// capabilityVocabulary is the closed capability vocabulary: exactly
// the 12 F11.7 matrix dimensions. Required capabilities outside it
// fail validation instead of routing.
func capabilityVocabulary() []string {
	return bench.CapabilityIDs()
}

// RoutedCapability is one declared agent capability: its F11.7 id,
// the dated evidence behind it, and the operating limit (e.g. the
// controlled-degradation policy from bench.DegradationFor).
type RoutedCapability struct {
	ID       string `json:"id"`
	Evidence string `json:"evidence"`
	Limit    string `json:"limit"`
}

// RoutedAgent is one routable agent: its Lab identity (F11.2), its
// host, its declared capabilities with evidence and limits, the
// highest task risk it may take solo, its availability, its
// permission scopes (F11.8: a fallback may only restrict them) and
// its declared cost estimates (used only when no history covers the
// agent; EstimateCost < 0 means unknown, never zero).
type RoutedAgent struct {
	ID             string             `json:"id"`
	Responsibility string             `json:"responsibility"`
	Host           string             `json:"host"`
	Tools          []string           `json:"tools"`
	Capabilities   []RoutedCapability `json:"capabilities"`
	MaxRisk        string             `json:"max_risk"`
	Available      bool               `json:"available"`
	Scopes         []string           `json:"scopes"`
	EstimateTimeMS int64              `json:"estimate_time_ms"`
	EstimateTokens int64              `json:"estimate_tokens"`
	EstimateCost   float64            `json:"estimate_cost"`
	EstimateCurcy  string             `json:"estimate_currency,omitempty"`
}

// defaultMaxRisk is the solo risk ceiling per Lab agent: the control
// takes routine work, the disciplined strategies take high-risk work
// solo; critical tasks always need a reviewer pair (policy, not a
// per-agent exception).
func defaultMaxRisk(id string) string {
	switch id {
	case "baseline":
		return RiskMedium
	default:
		return RiskHigh
	}
}

// defaultScopes is the permission set per Lab agent (F11.8
// vocabulary): the control reads, anchored strategies write memory,
// verify-first additionally executes gates. A fallback keeps only
// the intersection with the primary (Delegate), so these sets are
// the ceiling a delegation can never widen.
func defaultScopes(id string) []string {
	base := []string{mcp.ScopeMemoryRead, mcp.ScopeSpecRead, mcp.ScopeTraceRead}
	switch id {
	case "baseline":
		return base
	case "anchored":
		return append(append([]string{}, base...), mcp.ScopeMemoryWrite)
	default: // verify-first and future strategies
		return append(append([]string{}, base...), mcp.ScopeMemoryWrite, mcp.ScopeGateExecute)
	}
}

// DefaultRegistry builds the routing inventory from the Lab registry
// (F11.2) graded by the F11.7 host matrix: an agent declares exactly
// the capabilities its host holds in estado probada, each citing the
// manifest evidence with the controlled-degradation limit. Hosts with
// no probada capabilities contribute no routable agent. now stamps
// the manifests (bench.EvidenceStamp format); callers that need a
// fixed matrix pass their own manifests to RegistryFromManifests.
func DefaultRegistry(now string) []RoutedAgent {
	return RegistryFromManifests(bench.DefaultHostManifests(now))
}

// RegistryFromManifests grades the Lab agents against an explicit
// host matrix (tests pin their own matrix here instead of the live
// default). Agents whose host is absent from the matrix, or holds
// nothing probada, are left out — never routed on unverified claims.
func RegistryFromManifests(manifests map[string]bench.HostManifest) []RoutedAgent {
	var out []RoutedAgent
	for _, lab := range bench.LabRegistry() {
		m, ok := manifests[lab.Host]
		if !ok {
			continue
		}
		var caps []RoutedCapability
		for _, id := range bench.CapabilityIDs() {
			c, found := m.Capability(id)
			if !found {
				continue
			}
			// Route by state, never by "is it probada": a capability the host
			// can do with a restriction must still reach the router, or the
			// core collapses as soon as one dimension is only partially
			// available.
			decision := bench.RouteFor(c)
			if !decision.Routable {
				continue
			}
			limit := bench.DegradationFor(lab.Host, c.ID)
			if decision.Restriction != "" {
				limit = decision.Restriction
			}
			caps = append(caps, RoutedCapability{
				ID:       c.ID,
				Evidence: c.Evidence,
				Limit:    limit,
			})
		}
		if len(caps) == 0 {
			continue
		}
		out = append(out, RoutedAgent{
			ID:             lab.ID,
			Responsibility: lab.Responsibility,
			Host:           lab.Host,
			Tools:          append([]string{}, lab.Tools...),
			Capabilities:   caps,
			MaxRisk:        defaultMaxRisk(lab.ID),
			Available:      true,
			Scopes:         defaultScopes(lab.ID),
			EstimateTimeMS: 60000,
			EstimateTokens: 4000,
			EstimateCost:   -1,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FindRoutedAgent returns the registry entry by id.
func FindRoutedAgent(reg []RoutedAgent, id string) (RoutedAgent, bool) {
	for _, a := range reg {
		if a.ID == id {
			return a, true
		}
	}
	return RoutedAgent{}, false
}

// declaredCapabilities indexes one agent's declared capabilities.
func (a RoutedAgent) declaredCapabilities() map[string]RoutedCapability {
	m := make(map[string]RoutedCapability, len(a.Capabilities))
	for _, c := range a.Capabilities {
		m[c.ID] = c
	}
	return m
}

// checkVerified enforces the F11.7 evidence rule at routing time:
// every required capability must be declared by the agent with
// non-empty evidence AND hold estado probada in the host matrix.
// Anything else excludes the agent with a naming reason — a
// capability without verified evidence is never assigned.
func checkVerified(a RoutedAgent, required []string, manifests map[string]bench.HostManifest) (string, bool) {
	declared := a.declaredCapabilities()
	m, ok := manifests[a.Host]
	if !ok {
		return fmt.Sprintf("host %q fuera de la matriz F11.7: sin capabilities verificables", a.Host), false
	}
	hostCaps := map[string]bench.HostCapability{}
	for _, c := range m.Capabilities {
		hostCaps[c.ID] = c
	}
	for _, want := range required {
		d, has := declared[want]
		if !has || strings.TrimSpace(d.Evidence) == "" {
			return fmt.Sprintf("capability %q sin evidencia declarada por el agente (nunca asignada sin evidencia)", want), false
		}
		h, has := hostCaps[want]
		if !has {
			return fmt.Sprintf("capability %q fuera de la matriz del host %s (nunca asignada sin evidencia)", want, a.Host), false
		}
		if h.Status != bench.CapVerified {
			return fmt.Sprintf("capability %q en estado %s en host %s (se exige probada; nunca asignada sin verificar)", want, h.Status, a.Host), false
		}
		if strings.TrimSpace(h.Evidence) == "" {
			return fmt.Sprintf("capability %q probada sin evidencia fechada en host %s (matriz inválida; nunca asignada)", want, a.Host), false
		}
	}
	return "", true
}
