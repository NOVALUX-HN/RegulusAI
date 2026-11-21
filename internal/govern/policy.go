// Deny-by-default policies and configurable autonomy profiles
// (F13.6/NOV-109). The matrix (govern.go) classifies; this file
// decides: every unknown (kind, surface, scope, profile) denies
// before any side effect, autonomous action lives only inside
// explicit scopes, and high/critical risk always needs a human —
// no profile overrides that floor.
//
// Scope membership mirrors the F11.8 closed world: unknown scope
// names are dropped from grants, never honored. The server-derived
// principal (mcp.PrincipalFor) is built by the edge (cmdx); this
// file only intersects, never invents identity.
package govern

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/verify"
)

// Profile is one autonomy posture: the highest risk that may run
// without a human, the scopes autonomous action may use, and the
// kinds that never run under this profile. High/critical risk
// always needs approval on top, whatever the profile allows.
//
// Scope vocabulary is the F11.8 MCP scope set (memory:read,
// gate:execute, ...): one closed permission language for mission
// autonomy and tool enforcement. Govern mirrors the names (it must
// not import internal/mcp for one list — see the reuse map) so the
// edge can feed the server-derived principal straight in; unknown
// scope names deny like unknown kinds.
type Profile struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	MaxAutonomousRisk string   `json:"max_autonomous_risk"`
	AllowScopes       []string `json:"allow_scopes"`
	DenyKinds         []string `json:"deny_kinds,omitempty"`
}

// BuiltinProfiles returns the three closed profiles. Projects pick
// one per environment (policy file); anything else is unknown and
// therefore denied — profiles are data, never code paths.
func BuiltinProfiles() []Profile {
	return []Profile{
		{
			Name:              "locked",
			Description:       "nada autónomo: toda acción con efecto necesita aprobación humana",
			MaxAutonomousRisk: verify.RiskLow,
			AllowScopes:       []string{},
			DenyKinds:         []string{KindWrite, KindExecute, KindDeploy, KindPolicy, KindPermission, KindEvaluator, KindSecurity, KindIrreversible},
		},
		{
			Name:              "standard",
			Description:       "rutina low autónoma en scopes explícitos; medium+ con aprobación",
			MaxAutonomousRisk: verify.RiskLow,
			AllowScopes:       []string{"memory:read", "spec:read", "doctor:read", "trace:read", "audit:read", "evolve:read", "skills:read"},
		},
		{
			Name:              "wide",
			Description:       "low y medium autónomos en scopes explícitos; high/critical siempre con aprobación",
			MaxAutonomousRisk: verify.RiskMedium,
			AllowScopes:       []string{"memory:read", "spec:read", "doctor:read", "trace:read", "audit:read", "evolve:read", "skills:read", "memory:write", "gate:execute", "evolve:write"},
		},
	}
}

// ParseProfile resolves one profile by name. Unknown names fail:
// an unknown policy denies, it never falls back to open.
func ParseProfile(name string) (Profile, error) {
	got := strings.ToLower(strings.TrimSpace(name))
	for _, p := range BuiltinProfiles() {
		if p.Name == got {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("govern: unknown profile %q (want locked|standard|wide): denying by default", name)
}

// Policy binds a profile to its version and scope vocabulary. The
// version pins which matrix decided: approvals cite it, and a
// policy change invalidates prior bindings (see approval Check).
type Policy struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Profile   string          `json:"profile"`
	UpdatedBy string          `json:"updated_by,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	resolved  Profile         `json:"-"`
	scopes    map[string]bool `json:"-"`
}

// ResolvePolicy validates a policy against the closed profile
// vocabulary and freezes its scope set. Unknown profiles fail
// closed; empty scope sets are the lockdown (nothing autonomous).
func ResolvePolicy(name, profile string) (Policy, error) {
	p, err := ParseProfile(profile)
	if err != nil {
		return Policy{}, err
	}
	scopes := map[string]bool{}
	for _, s := range p.AllowScopes {
		if t := strings.TrimSpace(s); t != "" {
			scopes[t] = true
		}
	}
	return Policy{Name: name, Version: PolicyVersion, Profile: p.Name, resolved: p, scopes: scopes}, nil
}

// DefaultPolicy is the deny-by-default posture a project gets
// before it configures anything: standard autonomy, closed scopes.
func DefaultPolicy() Policy {
	p, err := ResolvePolicy("default", "standard")
	if err != nil {
		panic("govern: builtin standard profile must resolve")
	}
	return p
}

// Decision is the outcome of Authorize: autonomous Allow, approval
// required (NeedsApproval with the risk that demands it), or deny
// with the naming reason. PolicyVersion pins which matrix decided.
type Decision struct {
	Allow         bool
	NeedsApproval bool
	Risk          string
	Reason        string
	PolicyVersion string
}

func allowDecision(risk, reason string) Decision {
	return Decision{Allow: true, Risk: risk, Reason: reason, PolicyVersion: PolicyVersion}
}

func approvalDecision(risk, reason string) Decision {
	return Decision{NeedsApproval: true, Risk: risk, Reason: reason, PolicyVersion: PolicyVersion}
}

func denyDecision(risk, reason string) Decision {
	return Decision{Risk: risk, Reason: reason, PolicyVersion: PolicyVersion}
}

// Authorize decides one operation against a resolved policy and the
// caller-granted scopes (the server-derived set the edge passes
// in — never client-claimed). Order: known profile → known
// operation → denied kind → scope containment → risk ceiling. The
// first failure denies or escalates; nothing autonomous survives
// a missing scope or an unknown name.
func Authorize(op Operation, granted []string, pol Policy) Decision {
	if pol.resolved.Name == "" {
		return denyDecision("", "unknown or unresolved policy: denying by default (resolve a locked|standard|wide profile first)")
	}
	level, reasons, err := ClassifyRisk(op)
	if err != nil {
		return denyDecision("", "unclassifiable operation: "+err.Error())
	}
	for _, d := range pol.resolved.DenyKinds {
		if strings.EqualFold(strings.TrimSpace(d), op.Kind) {
			return denyDecision(level, fmt.Sprintf("profile %s denies kind %s (%s)", pol.Profile, op.Kind, strings.Join(reasons, "; ")))
		}
	}
	have := map[string]bool{}
	for _, s := range granted {
		if t := strings.TrimSpace(s); t != "" {
			have[t] = true
		}
	}
	for _, need := range op.Scopes {
		need = strings.TrimSpace(need)
		if need == "" {
			continue
		}
		if !pol.scopes[need] {
			return denyDecision(level, fmt.Sprintf("scope %q outside profile %s autonomy (%s)", need, pol.Profile, strings.Join(reasons, "; ")))
		}
		if !have[need] {
			return denyDecision(level, fmt.Sprintf("missing granted scope %q for %s %s (%s)", need, op.Kind, op.Surface, strings.Join(reasons, "; ")))
		}
	}
	if RequiresApproval(op, level) {
		return approvalDecision(level, fmt.Sprintf("risk %s governed (%s): human approval required, autonomous path closed", level, strings.Join(reasons, "; ")))
	}
	if riskRank(level) > riskRank(pol.resolved.MaxAutonomousRisk) {
		return approvalDecision(level, fmt.Sprintf("risk %s above profile %s autonomy ceiling %s: human approval required", level, pol.Profile, pol.resolved.MaxAutonomousRisk))
	}
	return allowDecision(level, fmt.Sprintf("risk %s within profile %s autonomy in explicit scopes (%s)", level, pol.Profile, strings.Join(reasons, "; ")))
}

// GrantedNames returns the sorted granted scopes for audit payloads:
// what was held, never the values behind them.
func GrantedNames(granted []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range granted {
		if t := strings.TrimSpace(s); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}
