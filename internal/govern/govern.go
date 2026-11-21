// Package govern is mission-level human governance (F13.6/NOV-109):
// uniform autonomy proportional to risk, with authorizations bound to
// exactly what was approved.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - risk vocabulary + risk→gates matrix: internal/verify (ParseRisk,
//     RequiredGates, MatrixDoc terms). Govern never redefines risk
//     levels and never invents gate sets: the matrix decides.
//   - scope enforcement: internal/mcp (F11.8 capability policy,
//     PrincipalFor/Delegate). Govern checks scope membership against
//     the server-derived principal; MCP owns containment.
//   - role separation + human-approval + hash binding: internal/evolve
//     governed.go / validation.go (proposer/evaluator/approver/
//     deployer distinct, human approver, commit/policy pins).
//     Govern lifts the pattern to every mission operation, it does
//     not reimplement the cycle.
//   - trace: observability.RecordExec (F11.1); the governance store
//     cites ids, never duplicates the exec log.
//
// This file holds the pure matrix: operation classification, risk
// derivation, approval need and autonomy. Profiles and enforcement
// live in policy.go; the approval lifecycle in approval.go; disk in
// store.go. Everything here is pure (no clock, no disk, no network).
package govern

import (
	"fmt"
	"strings"

	"regulus-ai/internal/verify"
)

// PolicyVersion versions the governance policy. Tightening or
// relaxing the matrix ships a new version, never a silent edit;
// approvals pin the version they were decided under.
const PolicyVersion = "regulus.govern-policy/v1"

// Operation kinds: the closed vocabulary. Unknown kinds deny by
// default (Authorize refuses them before any risk math).
const (
	KindRead         = "read"
	KindWrite        = "write"
	KindExecute      = "execute"
	KindDeploy       = "deploy"
	KindPolicy       = "policy"
	KindPermission   = "permission"
	KindEvaluator    = "evaluator"
	KindSecurity     = "security"
	KindIrreversible = "irreversible"
)

// AllKinds is the closed kind vocabulary in stable order.
var AllKinds = []string{
	KindRead, KindWrite, KindExecute, KindDeploy,
	KindPolicy, KindPermission, KindEvaluator, KindSecurity,
	KindIrreversible,
}

// ParseOpKind validates the closed kind vocabulary.
func ParseOpKind(s string) (string, error) {
	got := strings.ToLower(strings.TrimSpace(s))
	for _, k := range AllKinds {
		if got == k {
			return got, nil
		}
	}
	return "", fmt.Errorf("govern: unknown operation kind %q (want %s)", s, strings.Join(AllKinds, "|"))
}

// Surfaces: what the operation touches. Core, security, permissions,
// policy, evaluator and deploy surfaces are sensitive by definition.
const (
	SurfaceDocs        = "docs"
	SurfaceCode        = "code"
	SurfaceCore        = "core"
	SurfaceSecurity    = "security"
	SurfacePermissions = "permissions"
	SurfacePolicy      = "policy"
	SurfaceEvaluator   = "evaluator"
	SurfaceDeploy      = "deploy"
	SurfaceData        = "data"
)

// AllSurfaces is the closed surface vocabulary in stable order.
var AllSurfaces = []string{
	SurfaceDocs, SurfaceCode, SurfaceCore, SurfaceSecurity,
	SurfacePermissions, SurfacePolicy, SurfaceEvaluator,
	SurfaceDeploy, SurfaceData,
}

// ParseSurface validates the closed surface vocabulary.
func ParseSurface(s string) (string, error) {
	got := strings.ToLower(strings.TrimSpace(s))
	for _, k := range AllSurfaces {
		if got == k {
			return got, nil
		}
	}
	return "", fmt.Errorf("govern: unknown surface %q (want %s)", s, strings.Join(AllSurfaces, "|"))
}

// ParseRisk validates the closed risk vocabulary. The levels are
// verify's (single definition); govern never forks the terms.
func ParseRisk(s string) (string, error) {
	return verify.ParseRisk(s)
}

// Operation is one proposed mission action: what it does, what it
// touches, whether it rolls back, the scopes it needs and a
// single-line label for the audit trail.
type Operation struct {
	Kind       string   `json:"kind"`
	Surface    string   `json:"surface"`
	Reversible bool     `json:"reversible"`
	Scopes     []string `json:"scopes,omitempty"`
	Label      string   `json:"label,omitempty"`
}

func isSensitiveKind(kind string) bool {
	switch kind {
	case KindDeploy, KindPolicy, KindPermission, KindEvaluator, KindSecurity, KindIrreversible:
		return true
	}
	return false
}

func isSensitiveSurface(surface string) bool {
	switch surface {
	case SurfaceCore, SurfaceSecurity, SurfacePermissions, SurfacePolicy, SurfaceEvaluator, SurfaceDeploy:
		return true
	}
	return false
}

// Sensitive reports whether the operation touches a governed
// surface: Core, security, permissions, policies, evaluators,
// deployments, or anything irreversible. Sensitive operations always
// need a human approval, whatever the profile says.
func Sensitive(op Operation) bool {
	return isSensitiveKind(op.Kind) || isSensitiveSurface(op.Surface) || !op.Reversible
}

// ClassifyRisk derives the risk level for one operation. Higher only:
// the kind sets the base, a sensitive surface floors high, and
// irreversibility floors high (critical when the kind or surface is
// already sensitive). Every applied term is returned, so the audit
// explains itself.
func ClassifyRisk(op Operation) (string, []string, error) {
	kind, err := ParseOpKind(op.Kind)
	if err != nil {
		return "", nil, err
	}
	surface, err := ParseSurface(op.Surface)
	if err != nil {
		return "", nil, err
	}
	level := verify.RiskLow
	var reasons []string
	switch kind {
	case KindRead:
		reasons = append(reasons, "kind read: base low (pure read, no side effect)")
	case KindWrite:
		level = verify.RiskMedium
		reasons = append(reasons, "kind write: base medium (mutates state)")
	case KindExecute:
		level = verify.RiskMedium
		reasons = append(reasons, "kind execute: base medium (spawns work with receipts)")
	case KindDeploy:
		level = verify.RiskHigh
		reasons = append(reasons, "kind deploy: base high (ships to a live surface)")
	case KindPolicy, KindPermission, KindEvaluator, KindSecurity:
		level = verify.RiskHigh
		reasons = append(reasons, fmt.Sprintf("kind %s: base high (governs what others may do)", kind))
	case KindIrreversible:
		level = verify.RiskCritical
		reasons = append(reasons, "kind irreversible: base critical (cannot be undone)")
	}
	if isSensitiveSurface(surface) && riskRank(level) < riskRank(verify.RiskHigh) {
		level = verify.RiskHigh
		reasons = append(reasons, fmt.Sprintf("surface %s: high floor (governed surface)", surface))
	}
	if !op.Reversible {
		if isSensitiveKind(kind) || isSensitiveSurface(surface) {
			level = verify.RiskCritical
			reasons = append(reasons, "irreversible on a governed kind/surface: critical (no undo, no second chance)")
		} else if riskRank(level) < riskRank(verify.RiskHigh) {
			level = verify.RiskHigh
			reasons = append(reasons, "irreversible: high floor (rollback must be provable, it is not)")
		}
	}
	return level, reasons, nil
}

// RequiresApproval reports whether the operation needs a human
// approval: every sensitive operation and every high/critical risk,
// no profile exception. Low/medium routine work inside explicit
// scopes stays autonomous so trivialities never page a human.
func RequiresApproval(op Operation, level string) bool {
	if Sensitive(op) {
		return true
	}
	return riskRank(level) >= riskRank(verify.RiskHigh)
}

// Groupable reports whether the operation is trivial enough to batch:
// low risk and not sensitive. Grouping never widens scope: a batch
// carries the union of member scopes only when every member stays
// autonomous on its own.
func Groupable(op Operation, level string) bool {
	return level == verify.RiskLow && !Sensitive(op)
}

// VerificationGates resolves the verify risk→gates matrix for the
// level: the same seven gates the mission engine enforces, cited
// here so the approval shows its tests before it is signed.
func VerificationGates(level string, escalation []string) ([]verify.GateSpec, []string) {
	specs, _, reasons := verify.RequiredGates(level, false, level == verify.RiskCritical, escalation)
	return specs, reasons
}

// MatrixDoc renders the risk→autonomy matrix: the documented table
// and the enforced behavior share ClassifyRisk, so they cannot drift.
func MatrixDoc() string {
	var b strings.Builder
	b.WriteString("matriz riesgo→autonomía (govern-v1; niveles de verify, nunca PASS por omisión):\n")
	b.WriteString("  low: read reversible en superficies no sensibles → autónomo en scopes explícitos (agrupable)\n")
	b.WriteString("  medium: write/execute reversible → autónomo solo en scopes explícitos del perfil (no agrupable)\n")
	b.WriteString("  high: deploy/policy/permission/evaluator/security, superficies core/security/permissions/policy/evaluator/deploy, o irreversible → aprobación humana\n")
	b.WriteString("  critical: irreversible gobernado o irreversible declarado → aprobación humana ligada a hashes + gates completos\n")
	b.WriteString("deny-by-default: kind/superficie/alcance/perfil desconocidos deniegan antes de actuar; high/critical nunca son autónomos.\n")
	return b.String()
}

// riskRank orders verify levels without redefining them.
func riskRank(r string) int {
	switch r {
	case verify.RiskLow:
		return 0
	case verify.RiskMedium:
		return 1
	case verify.RiskHigh:
		return 2
	case verify.RiskCritical:
		return 3
	}
	return -1
}
