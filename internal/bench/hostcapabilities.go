// Capability manifest per host (F11.7 / NOV-93): replace generic
// compatibility claims with executed journeys and verified
// capabilities.
//
// Reuse, not duplication: journey levels and outcomes come from
// NOV-69 (hostjourney.go: LevelDeclared/LevelContract/LevelHost,
// HostJourneyOutcome, ProbeHost, DefaultHostProbes); the three green
// journeys are NOV-76 evidence (2026-11-16, fixture install/doctor/
// context/spec/gate/close); traces reuse F11.1 (observability exec
// store, `trace timeline --run <id> --rebuild`); redaction reuses
// observability.RedactText/RedactMap (the same choke-point policy as
// the lab and the almanac: secrets never reach the store).
//
// The probe/journey distinction is structural: the execution class
// ("probe", "journey", "agent-journey", "strategy") travels with the
// run, never inferred from capability states, so a probe can never be
// reported as a journey. A probada capability always cites its
// evidence (ValidateCapabilitiesAttributable); undated claims fail
// the command instead of printing.
package bench

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/observability"
)

// Capability dimensions: the 12 columns every host is graded on. The
// first nine are the spec's (tools, hooks, subagents, MCP, sessions,
// tokens, filesystem, permissions, limits); install/gate/close are
// the journey legs that verify them end to end.
const (
	CapTools       = "tools"
	CapHooks       = "hooks"
	CapSubagents   = "subagents"
	CapMCP         = "mcp"
	CapSessions    = "sessions"
	CapTokens      = "tokens"
	CapFilesystem  = "filesystem"
	CapPermissions = "permissions"
	CapLimits      = "limits"
	CapInstall     = "install"
	CapGate        = "gate"
	CapClose       = "close"
)

// CapabilityIDs returns the 12 dimensions in documentation order.
func CapabilityIDs() []string {
	return []string{
		CapTools, CapHooks, CapSubagents, CapMCP, CapSessions,
		CapTokens, CapFilesystem, CapPermissions, CapLimits,
		CapInstall, CapGate, CapClose,
	}
}

// Capability states: the closed vocabulary the matrix may contain. It is
// closed on purpose — adding a word is a contract change.
//
// probada is the strongest and most expensive claim: it asserts that a run
// observed the dimension. soportada is what the host's contract and
// configuration support with no observation behind it; inferida is derived
// from a signal such as a version gate; parcial and degradada report observed
// behaviour that carries restrictions.
const (
	CapVerified    = "probada"
	CapSupported   = "soportada"
	CapInferred    = "inferida"
	CapPartial     = "parcial"
	CapDegraded    = "degradada"
	CapUnsupported = "no-soportada"
	CapNotExecuted = "no-ejecutada"
	CapUnknown     = "desconocida"
)

// Execution classes: what actually ran. "probe" is a version check
// with no model cost; "journey" is the fixture E2E (repeatable, no
// model cost); "agent-journey" is a real headless model call
// (credential-gated); "strategy" is a documented manual path for
// hosts with no non-interactive CLI (Devin).
const (
	ClassProbe        = "probe"
	ClassJourney      = "journey"
	ClassAgentJourney = "agent-journey"
	ClassStrategy     = "strategy"
)

// HostCapability is one graded dimension: its state, why, and the
// evidence behind it. A probada entry without Evidence is invalid.
type HostCapability struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence"`
}

// HostManifest is the per-host capability record: versions, the
// commands to reproduce each class, the 12 graded capabilities, the
// controlled-degradation policy and the F11.1 trace link.
type HostManifest struct {
	Host                string           `json:"host"`
	Version             string           `json:"version"`
	Class               string           `json:"class"`
	RequiresCredentials bool             `json:"requires_credentials"`
	RepeatableInCI      bool             `json:"repeatable_in_ci"`
	ProbeCommands       []string         `json:"probe_commands"`
	JourneyCommands     []string         `json:"journey_commands"`
	AgentCommands       []string         `json:"agent_commands"`
	Capabilities        []HostCapability `json:"capabilities"`
	Degradation         string           `json:"degradation"`
	JourneyEvidence     string           `json:"journey_evidence"`
	JourneyDate         string           `json:"journey_date"`
	ProbeNote           string           `json:"probe_note"`
	TraceHint           string           `json:"trace_hint"`
	RecordedAt          string           `json:"recorded_at"`
}

// Capability returns one graded dimension by id.
func (m HostManifest) Capability(id string) (HostCapability, bool) {
	for _, c := range m.Capabilities {
		if c.ID == id {
			return c, true
		}
	}
	return HostCapability{}, false
}

// JourneyCoveredCaps are the dimensions the fixture journey actually
// exercises (install, context/sessions, spec/tools, gate, close plus
// the receipt/token, filesystem and MCP surfaces it touches). Hooks,
// subagents, permissions and limits are declared, never journey-met.
func JourneyCoveredCaps() []string {
	return []string{
		CapInstall, CapSessions, CapTools, CapGate,
		CapClose, CapTokens, CapFilesystem, CapMCP,
	}
}

// journeyGreen is the shared NOV-76 evidence citation: three hosts
// green on the same fixture, 2026-11-16, evidence pack linked.
const journeyGreen = "NOV-76 journey green 2026-11-16 " +
	"(.regulus/host-evidence-run/host-journeys-20261116T005418Z.json)"

func verified(id, detail, evidence string) HostCapability {
	return HostCapability{ID: id, Status: CapVerified, Detail: detail, Evidence: evidence}
}

// declared records a capability the host's contract and configuration support
// with no run observing it. It must never cite a journey fixture: claiming an
// observation that did not happen is exactly what ValidateEvidenceCoverage
// rejects.
func declared(id, detail, evidence string) HostCapability {
	return HostCapability{ID: id, Status: CapSupported, Detail: detail, Evidence: evidence}
}

// inferred records a capability derived from a signal — a version gate, a
// capability probe — rather than from configuring or observing the surface.
func inferred(id, detail, evidence string) HostCapability {
	return HostCapability{ID: id, Status: CapInferred, Detail: detail, Evidence: evidence}
}

// DefaultHostManifests returns the declared baseline per host: the
// NOV-69/NOV-76 verified state with dated evidence. OpenCode, Claude
// and Codex carry the green fixture journey; Devin carries the
// realistic strategy (desktop-only, no non-interactive CLI), with
// unsupported and unknown states where the host has no surface.
func DefaultHostManifests(now string) map[string]HostManifest {
	now = strings.TrimSpace(now)
	opencodeCaps := []HostCapability{
		verified(CapTools, "19 tools vía plugin TS (bun test ./lib/)", "contract tests + "+journeyGreen),
		declared(CapHooks, "hooks OpenCode V2", "contract tests"),
		declared(CapSubagents, "agents OpenCode V2", "contract tests"),
		verified(CapMCP, "MCP stdio regulus mcp", "contract tests + "+journeyGreen),
		verified(CapSessions, "context/memoria vía journey context", journeyGreen),
		verified(CapTokens, "accounting medido del host (scanOpenCodeUsage)", "agent_evidence tests + "+journeyGreen),
		verified(CapFilesystem, "fixture + backups hostcfg antes de escribir", journeyGreen),
		declared(CapPermissions, "respeta permisos, sin escalado silencioso", "contract tests"),
		declared(CapLimits, "presupuestos y cotas (30s probe, 5min/step, job 20min)", "contract tests"),
		verified(CapInstall, "install verde en fixture", journeyGreen),
		verified(CapGate, "gate --yes verde en fixture", journeyGreen),
		verified(CapClose, "close con receipts verde en fixture", journeyGreen),
	}
	codexCaps := []HostCapability{
		verified(CapTools, "CLI + skills .agents/skills/regulus", "contract tests + "+journeyGreen),
		inferred(CapHooks, "version-gated: inline degradado explícito si la versión no los soporta", "contract tests"),
		inferred(CapSubagents, "version-gated: inline si la versión no los soporta", "contract tests"),
		verified(CapMCP, "MCP stdio vía .codex/config.toml", "contract tests + "+journeyGreen),
		verified(CapSessions, "context/memoria vía journey context", journeyGreen),
		verified(CapTokens, "receipts con tokens cuando el host los expone, unknown si no", "agent_evidence tests + "+journeyGreen),
		verified(CapFilesystem, "fixture + preservación config ajena (hostcfg)", journeyGreen),
		declared(CapPermissions, "respeta permisos, sin escalado silencioso", "contract tests"),
		declared(CapLimits, "presupuestos y cotas como el resto de hosts", "contract tests"),
		verified(CapInstall, "install verde en fixture", journeyGreen),
		verified(CapGate, "gate --yes verde en fixture", journeyGreen),
		verified(CapClose, "close con receipts verde en fixture", journeyGreen),
	}
	claudeCaps := []HostCapability{
		verified(CapTools, "skills .claude/skills + CLI", "contract tests + "+journeyGreen),
		declared(CapHooks, "opt-in: solo cuando el proyecto los habilita", "contract tests"),
		declared(CapSubagents, "agents .claude/agents", "contract tests"),
		verified(CapMCP, "MCP vía .mcp.json", "contract tests + "+journeyGreen),
		verified(CapSessions, "context/memoria vía journey context", journeyGreen),
		verified(CapTokens, "receipts con tokens cuando el host los expone, unknown si no", "agent_evidence tests + "+journeyGreen),
		verified(CapFilesystem, "fixture + preservación config ajena (hostcfg)", journeyGreen),
		declared(CapPermissions, "sin mutar permisos sin consentimiento", "contract tests"),
		declared(CapLimits, "presupuestos y cotas como el resto de hosts", "contract tests"),
		verified(CapInstall, "install verde en fixture", journeyGreen),
		verified(CapGate, "gate --yes verde en fixture", journeyGreen),
		verified(CapClose, "close con receipts verde en fixture", journeyGreen),
	}
	devinCaps := []HostCapability{
		{ID: CapTools, Status: CapNotExecuted, Detail: "pack .devin/ declarado, sin CLI no-interactiva que lo ejerza", Evidence: "docs/devin.md + internal/bench/hostjourney.go (Devin probe)"},
		{ID: CapHooks, Status: CapUnsupported, Detail: "sin superficie CLI donde registrar hooks", Evidence: "Devin probe 2026-11-16: desktop-only, sin modo no interactivo"},
		{ID: CapSubagents, Status: CapNotExecuted, Detail: "estrategia manual vía sesión Desktop/Cloud", Evidence: "docs/devin.md"},
		{ID: CapMCP, Status: CapNotExecuted, Detail: "MCP vía pack, no verificable headless", Evidence: "docs/devin.md"},
		{ID: CapSessions, Status: CapNotExecuted, Detail: "sin CLI no-interactiva que abra sesión", Evidence: "Devin probe 2026-11-16"},
		{ID: CapTokens, Status: CapUnknown, Detail: "accounting Cloud no expuesto al runner", Evidence: "sin superficie medible (docs/devin.md)"},
		{ID: CapFilesystem, Status: CapUnsupported, Detail: "Cloud sin persistencia en disco; export/import explícito", Evidence: "docs/devin.md matriz CLI/Desktop/Cloud"},
		{ID: CapPermissions, Status: CapUnsupported, Detail: "modelo Cloud gestionado, sin mutación local de permisos", Evidence: "docs/devin.md"},
		{ID: CapLimits, Status: CapUnknown, Detail: "cuotas Cloud no visibles desde el runner", Evidence: "sin superficie medible (docs/devin.md)"},
		{ID: CapInstall, Status: CapNotExecuted, Detail: "scripts install-devin.* existen, no ejecutados en CI", Evidence: "scripts/install-devin.* + docs/devin.md"},
		{ID: CapGate, Status: CapNotExecuted, Detail: "sin CLI no-interactiva que corra el gate", Evidence: "Devin probe 2026-11-16"},
		{ID: CapClose, Status: CapNotExecuted, Detail: "cierre manual con evidencia importada, nunca inventada", Evidence: "docs/devin.md"},
	}
	out := map[string]HostManifest{
		"opencode": {
			Host: "opencode", Version: "v2.2.5", Class: ClassJourney,
			RequiresCredentials: false, RepeatableInCI: true,
			ProbeCommands:   []string{"opencode --version"},
			JourneyCommands: []string{"npm install -g opencode-ai@1.19.5", "opencode --version", "regulus bench host-journeys --run --host opencode --out host-evidence"},
			AgentCommands:   []string{"opencode run --model <model> \"<fixture prompt>\"  # credenciales: modelo con cuota (HOST_API_KEY)"},
			Capabilities:    opencodeCaps,
			Degradation:     "sin degradación: superficie completa; sin créditos de modelo el journey cae a no-ejecutado, nunca a pass",
			JourneyEvidence: journeyGreen, JourneyDate: "2026-11-16", RecordedAt: now,
		},
		"codex": {
			Host: "codex", Version: "codex-cli 0.169.0", Class: ClassJourney,
			RequiresCredentials: false, RepeatableInCI: true,
			ProbeCommands:   []string{"codex --version"},
			JourneyCommands: []string{"npm install -g @openai/codex@0.169.0", "codex --version", "regulus bench host-journeys --run --host codex --out host-evidence"},
			AgentCommands:   []string{"codex exec --model <model> \"<fixture prompt>\"  # credenciales: login con cuota (HOST_API_KEY)"},
			Capabilities:    codexCaps,
			Degradation:     "hooks/subagentes version-gated: inline degradado explícito cuando la versión no los soporta; el resto directo",
			JourneyEvidence: journeyGreen, JourneyDate: "2026-11-16", RecordedAt: now,
		},
		"claude": {
			Host: "claude", Version: "2.1.325 (Claude Code)", Class: ClassJourney,
			RequiresCredentials: true, RepeatableInCI: false,
			ProbeCommands:   []string{"claude --version"},
			JourneyCommands: []string{"npm install -g @anthropic-ai/claude-code", "claude --version", "regulus bench host-journeys --run --host claude --out host-evidence"},
			AgentCommands:   []string{"claude -p \"<fixture prompt>\" --model <model>  # credenciales: login (HOST_API_KEY); sin login el agent-leg es no-ejecutado"},
			Capabilities:    claudeCaps,
			Degradation:     "hooks opt-in; permisos: sin mutar sin consentimiento, el journey se detiene y reporta no-ejecutado",
			JourneyEvidence: journeyGreen, JourneyDate: "2026-11-16", RecordedAt: now,
		},
		"devin": {
			Host: "devin", Version: "desktop-only (sin CLI)", Class: ClassStrategy,
			RequiresCredentials: true, RepeatableInCI: false,
			ProbeCommands:   []string{"# sin binario CLI: la ausencia verificada ES el probe"},
			JourneyCommands: []string{"# sin journey ejecutable: ver estrategia"},
			AgentCommands: []string{
				"# estrategia realista Devin (manual, evidencia importada):",
				"# 1. exportar pack .devin/ del repo",
				"# 2. abrir sesión Desktop/Cloud y correr install/doctor/context/spec/gate/close",
				"# 3. importar la evidencia a host-evidence/ (nunca inventarla)",
			},
			Capabilities:    devinCaps,
			Degradation:     "estrategia manual vía pack .devin/ + sesión Desktop/Cloud; Cloud sin persistencia (export/import explícito); blueprint Windows runs-on:windows",
			JourneyEvidence: "estrategia documentada docs/devin.md (sin CLI no-interactiva conocida 2026-11-16)",
			JourneyDate:     "2026-11-16", RecordedAt: now,
		},
	}
	return out
}

// ApplyJourneyOutcome folds a fresh NOV-69 outcome into a manifest.
// Upgrades happen only on a green host-probado journey (covered caps
// become probada with the journey evidence). A failed journey records
// its reason without upgrading. Probe and not-run outcomes never
// touch capabilities or the class: they only refresh the version and
// leave a probe note, so dated journey evidence survives and a probe
// can never promote — or demote — a journey claim.
func ApplyJourneyOutcome(m HostManifest, o HostJourneyOutcome, now string) HostManifest {
	// Copy the capabilities slice before touching it. The signature returns a
	// new manifest, so callers may keep the one they passed in; without this
	// copy, promoting a dimension also mutated the caller's manifest through
	// the shared backing array, and a second fold started from the first
	// fold's result.
	m.Capabilities = append([]HostCapability(nil), m.Capabilities...)
	if strings.TrimSpace(o.Version) != "" && o.Host == m.Host {
		m.Version = o.Version
	}
	m.RecordedAt = now
	switch {
	case o.Level == LevelHost && o.Status == AgentPass:
		m.Class = ClassJourney
		m.JourneyEvidence = fmt.Sprintf("journey green %s (%s)", now, o.Reason)
		m.JourneyDate = now
		covered := map[string]bool{}
		for _, id := range JourneyCoveredCaps() {
			covered[id] = true
		}
		for i, c := range m.Capabilities {
			if !covered[c.ID] {
				continue
			}
			m.Capabilities[i].Status = CapVerified
			m.Capabilities[i].Evidence = m.JourneyEvidence
		}
		m.ProbeNote = ""
	case o.Level == LevelHost && o.Status == AgentFail:
		m.Class = ClassJourney
		m.JourneyEvidence = fmt.Sprintf("journey fallido %s (%s)", now, o.Reason)
		m.JourneyDate = now
		for i, c := range m.Capabilities {
			ev := fmt.Sprintf("journey fallido %s: %s; estado declarado preservado", now, o.Reason)
			if m.Capabilities[i].Evidence == "" {
				m.Capabilities[i].Evidence = ev
			} else {
				m.Capabilities[i].Evidence += " | " + ev
			}
			_ = c
		}
		m.ProbeNote = ""
	default:
		// Probe-only or host unavailable: no capability changes.
		// Dated journey evidence survives a missing binary (only a
		// probe note is added), but a manifest with no dated journey
		// at all becomes probe-class so the validator can pin it.
		// Either way a probe can never promote — or demote — a
		// journey claim.
		if o.Level == LevelNotRun && m.JourneyDate == "" && m.Class != ClassStrategy {
			m.Class = ClassProbe
		}
		if strings.TrimSpace(o.Reason) != "" {
			m.ProbeNote = fmt.Sprintf("probe %s: %s", now, o.Reason)
		} else {
			m.ProbeNote = fmt.Sprintf("probe %s: sin cambios (journey no ejecutado)", now)
		}
	}
	return m
}

// RouteDecision classifies one capability for orchestration routing.
//
// The router used to accept probada and silently drop everything else, which
// made the vocabulary look binary and collapsed an agent the moment a
// capability was only partially available. A capability that the host can do
// with a stated restriction must still route, carrying that restriction; only
// the states that assert absence or ignorance are excluded.
type RouteDecision struct {
	Routable    bool
	Restriction string
}

// RouteFor maps a capability state to a routing decision.
//
// probada routes clean. degradada and parcial route with their restriction —
// the host can do the work, not without the limit. soportada and inferida also
// route, but always with an explicit "no observation" restriction, so a
// declared surface neither disappears nor masquerades as verified. Only
// no-soportada, no-ejecutada and desconocida are unroutable: nothing is known
// to be there.
func RouteFor(c HostCapability) RouteDecision {
	switch c.Status {
	case CapVerified:
		return RouteDecision{Routable: true}
	case CapDegraded:
		return RouteDecision{Routable: true, Restriction: restrictionOr(c.Detail, "degradación controlada declarada, sin observación")}
	case CapPartial:
		return RouteDecision{Routable: true, Restriction: restrictionOr(c.Detail, "observada con restricciones")}
	case CapSupported:
		return RouteDecision{Routable: true, Restriction: restrictionOr(c.Detail, "declarada por contrato, sin observación")}
	case CapInferred:
		return RouteDecision{Routable: true, Restriction: restrictionOr(c.Detail, "inferida de una señal, sin observación")}
	default:
		return RouteDecision{}
	}
}

// restrictionOr keeps the restriction human-readable when a manifest carries
// no detail for that dimension.
func restrictionOr(detail, fallback string) string {
	if trimmed := strings.TrimSpace(detail); trimmed != "" {
		return trimmed
	}
	return fallback
}

// ValidateCapabilitiesAttributable enforces the evidence rule: every
// probada capability cites its evidence, and only the closed state
// vocabulary is allowed. Undated claims fail instead of printing.
func ValidateCapabilitiesAttributable(manifests map[string]HostManifest) error {
	allowed := map[string]bool{
		CapVerified: true, CapSupported: true, CapInferred: true,
		CapPartial: true, CapDegraded: true,
		CapUnsupported: true, CapNotExecuted: true, CapUnknown: true,
	}
	hosts := make([]string, 0, len(manifests))
	for h := range manifests {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		m := manifests[h]
		for _, c := range m.Capabilities {
			if !allowed[c.Status] {
				return fmt.Errorf("host %s capacidad %s: estado %q fuera del vocabulario (probada/soportada/inferida/parcial/degradada/no-soportada/no-ejecutada/desconocida)", h, c.ID, c.Status)
			}
			if c.Status == CapVerified && strings.TrimSpace(c.Evidence) == "" {
				return fmt.Errorf("host %s capacidad %s: probada sin evidencia enlazada", h, c.ID)
			}
		}
	}
	return nil
}

// journeyCitation is the marker every journey-green evidence string carries.
// Matching on it is what lets this validator find the over-claim F16.1
// audited: a dimension reported as probada while its citation points at a
// run that never covered it.
const journeyCitation = "journey green"

// ValidateEvidenceCoverage rejects a citation that cannot support its claim.
//
// ValidateNoProbeAsJourney guards the probe class only, so the journey-class
// manifests — the ones that actually carry the twelve over-claims — were never
// checked. This validator closes that hole for every manifest: a dimension may
// be probada on the strength of the journey fixture only when
// JourneyCoveredCaps() says the journey covers it. Everything else is
// soportada or inferida, which is what the wider vocabulary is for.
func ValidateEvidenceCoverage(manifests map[string]HostManifest) error {
	covered := map[string]bool{}
	for _, id := range JourneyCoveredCaps() {
		covered[id] = true
	}
	hosts := make([]string, 0, len(manifests))
	for h := range manifests {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		for _, c := range manifests[h].Capabilities {
			if c.Status != CapVerified || !strings.Contains(c.Evidence, journeyCitation) {
				continue
			}
			if !covered[c.ID] {
				return fmt.Errorf("host %s capacidad %s: probada citando el journey, pero JourneyCoveredCaps no cubre esa dimensión (usar soportada o inferida)", h, c.ID)
			}
		}
	}
	return nil
}

// ValidateNoProbeAsJourney enforces the CI rule: a manifest whose
// execution class is probe must not claim probada on journey-covered
// capabilities. Probe evidence is presence/version, never a journey.
func ValidateNoProbeAsJourney(manifests map[string]HostManifest) error {
	covered := map[string]bool{}
	for _, id := range JourneyCoveredCaps() {
		covered[id] = true
	}
	hosts := make([]string, 0, len(manifests))
	for h := range manifests {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	for _, h := range hosts {
		m := manifests[h]
		if m.Class != ClassProbe {
			continue
		}
		for _, c := range m.Capabilities {
			if covered[c.ID] && c.Status == CapVerified {
				return fmt.Errorf("host %s: clase probe con capacidad %s probada (un probe nunca es un journey)", h, c.ID)
			}
		}
	}
	return nil
}

// RenderCapabilityMatrix renders the host x capability matrix: one
// row per dimension, one column per host, cells holding only the four
// state words. Unknown hosts and unknown capabilities render
// desconocida, never pass.
func RenderCapabilityMatrix(hosts []string, manifests map[string]HostManifest) string {
	var b strings.Builder
	b.WriteString("| Capacidad |")
	for _, h := range hosts {
		fmt.Fprintf(&b, " %s |", h)
	}
	b.WriteString("\n|")
	for range hosts {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, id := range CapabilityIDs() {
		fmt.Fprintf(&b, "| %s |", id)
		for _, h := range hosts {
			m, ok := manifests[h]
			status := CapUnknown
			if ok {
				if c, found := m.Capability(id); found {
					status = c.Status
				}
			}
			fmt.Fprintf(&b, " %s |", status)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// RenderManifestSummary renders one host's record: version, class,
// credential needs, CI repeatability, reproduction commands,
// degradation policy and the F11.1 trace link.
func RenderManifestSummary(m HostManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "== %s ==\n", m.Host)
	fmt.Fprintf(&b, "versión: %s\n", m.Version)
	fmt.Fprintf(&b, "clase: %s\n", m.Class)
	fmt.Fprintf(&b, "credenciales: %v | repetible en CI: %v\n", m.RequiresCredentials, m.RepeatableInCI)
	if m.JourneyDate != "" {
		fmt.Fprintf(&b, "evidencia journey: %s (%s)\n", m.JourneyEvidence, m.JourneyDate)
	}
	if m.ProbeNote != "" {
		fmt.Fprintf(&b, "nota probe: %s\n", m.ProbeNote)
	}
	fmt.Fprintf(&b, "degradación: %s\n", m.Degradation)
	for _, cmd := range m.ProbeCommands {
		fmt.Fprintf(&b, "probe: %s\n", cmd)
	}
	for _, cmd := range m.JourneyCommands {
		fmt.Fprintf(&b, "journey: %s\n", cmd)
	}
	for _, cmd := range m.AgentCommands {
		fmt.Fprintf(&b, "agent: %s\n", cmd)
	}
	if m.TraceHint != "" {
		fmt.Fprintf(&b, "traza: %s\n", m.TraceHint)
	}
	return b.String()
}

// TraceHintFor links a capabilities run to its F11.1 timeline, with
// the same format the lab uses (no parallel store, no new vocabulary).
func TraceHintFor(runID string) string {
	return "regulus trace timeline --run " + runID + " --rebuild"
}

// HostEvidence is the redacted per-host evidence pack: versions,
// capabilities, reproduction commands, non-sensitive config and
// redacted artifacts, plus the trace link. Secrets never reach it:
// NewHostEvidence scrubs through observability before storing.
type HostEvidence struct {
	Host         string              `json:"host"`
	Version      string              `json:"version"`
	Class        string              `json:"class"`
	Capabilities []HostCapability    `json:"capabilities"`
	Commands     map[string][]string `json:"commands"`
	Config       map[string]any      `json:"config"`
	Artifacts    []string            `json:"artifacts"`
	TraceHint    string              `json:"trace_hint"`
	RecordedAt   string              `json:"recorded_at"`
}

// NewHostEvidence builds the redacted pack for one manifest: config
// passes through RedactMap (sensitive keys become [redacted]) and
// every artifact through RedactText (embedded bearer/key=value pairs
// scrubbed). Callers pass full data; this is the choke point.
func NewHostEvidence(m HostManifest, config map[string]any, artifacts []string) HostEvidence {
	scrubbed := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		scrubbed = append(scrubbed, observability.RedactText(a))
	}
	return HostEvidence{
		Host:         m.Host,
		Version:      m.Version,
		Class:        m.Class,
		Capabilities: m.Capabilities,
		Commands: map[string][]string{
			ClassProbe:        m.ProbeCommands,
			ClassJourney:      m.JourneyCommands,
			ClassAgentJourney: m.AgentCommands,
		},
		Config:     observability.RedactMap(config),
		Artifacts:  scrubbed,
		TraceHint:  m.TraceHint,
		RecordedAt: m.RecordedAt,
	}
}

// DegradationFor returns the controlled-degradation policy for one
// host and capability: what runs instead when the full surface is
// unavailable. One host's fallback never degrades the others.
func DegradationFor(host, capID string) string {
	key := host + "/" + capID
	table := map[string]string{
		"codex/hooks":        "inline degradado explícito cuando la versión no soporta hooks (se registra, no se finge)",
		"codex/subagents":    "inline degradado explícito cuando la versión no soporta subagentes",
		"claude/hooks":       "opt-in: solo cuando el proyecto los habilita; si no, capacidad no-ejecutada",
		"claude/permissions": "sin mutar permisos sin consentimiento: el journey se detiene y reporta no-ejecutado",
		"devin/filesystem":   "export/import explícito: Cloud sin persistencia en disco",
		"devin/install":      "scripts install-devin.* + pack .devin/ manual con evidencia importada",
	}
	if d, ok := table[key]; ok {
		return d
	}
	if host == "devin" {
		return "estrategia manual vía pack .devin/ + sesión Desktop/Cloud; evidencia importada, nunca inventada"
	}
	return "directo: sin degradación; si el host no está usable el journey marca no-ejecutado, nunca pass"
}

// EvidenceStamp returns the record timestamp for manifests.
func EvidenceStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
