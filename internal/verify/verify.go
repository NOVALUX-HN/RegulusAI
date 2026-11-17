// Package verify is Continuous Verification during the mission
// (F13.4/NOV-107): verify every change and every integration, not
// only at the end of the mission.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - impact (F12.8): the dependency graph, verified edges,
//     diff→consumers/tests (MapDiff), explicable risk (AssessRisk),
//     focused suite with measured precision/recall (SelectSuite,
//     MeasureSelection). Verify never rewalks the repo and never
//     redefines risk terms.
//   - quality (F12.4): versioned baselines and N-adequate change
//     detection (Check). Verify never reimplements detection —
//     regression evidence is a quality diagnosis folded into the
//     gate decision.
//   - bench tiers (F11.3): L0..L3 level names, tier recommendation
//     (RecommendTier) and run-budget ceilings (TierSpecs). Verify
//     never selects L3 by focus: the holdout stays sealed below L3.
//   - observability (F11.1): the CLI records run.start + task.end +
//     run.end around every verify command (best-effort); the engine
//     stays pure where it plans (no clock, no disk, no network
//     beyond its inputs). Execution timestamps live only in Result.
//
// Verdict vocabulary is the spec's closed six: PASS, FAIL, BLOCKED,
// SKIPPED, UNKNOWN, NOT_APPLICABLE. verdict.Status is NOT reused on
// purpose: it carries four different states (PASS/FAIL/BLOCKED/
// INCONCLUSIVE) and verify must distinguish SKIPPED from UNKNOWN
// from NOT_APPLICABLE. BLOCKED/SKIPPED/UNKNOWN are never PASS —
// the gate decision enforces it, and a test pins it.
package verify

import (
	"fmt"
	"strings"
)

// VerifyVersion pins the plan/result document shape: evidence dirs
// compare only within one version.
const VerifyVersion = "verify-v1"

// Verdict vocabulary: the only six outcomes. PASS executes clean;
// FAIL is a deterministic failure; BLOCKED never ran (policy,
// toolchain, infra); SKIPPED was deliberately not run with a cause;
// UNKNOWN has no evidence either way; NOT_APPLICABLE is explicitly
// out of scope for this change with a cause.
const (
	VerdictPass          = "PASS"
	VerdictFail          = "FAIL"
	VerdictBlocked       = "BLOCKED"
	VerdictSkipped       = "SKIPPED"
	VerdictUnknown       = "UNKNOWN"
	VerdictNotApplicable = "NOT_APPLICABLE"
)

// ParseVerdict validates the closed verdict vocabulary.
func ParseVerdict(s string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case VerdictPass:
		return VerdictPass, nil
	case VerdictFail:
		return VerdictFail, nil
	case VerdictBlocked:
		return VerdictBlocked, nil
	case VerdictSkipped:
		return VerdictSkipped, nil
	case VerdictUnknown:
		return VerdictUnknown, nil
	case VerdictNotApplicable:
		return VerdictNotApplicable, nil
	}
	return "", fmt.Errorf("verify: unknown verdict %q (want PASS|FAIL|BLOCKED|SKIPPED|UNKNOWN|NOT_APPLICABLE)", s)
}

// IsPass reports the only verdict that counts as passing. Everything
// else — including NOT_APPLICABLE — is not a pass; whether it blocks
// depends on BlocksRequired.
func IsPass(v string) bool { return v == VerdictPass }

// BlocksRequired reports whether a gate verdict blocks integration
// when the gate is required: FAIL always blocks; BLOCKED, SKIPPED
// and UNKNOWN block because they are not passes (the spec's
// load-bearing rule); NOT_APPLICABLE never blocks — it is an
// explicit, caused scope exclusion, not a silent pass.
func BlocksRequired(v string) bool {
	switch v {
	case VerdictPass, VerdictNotApplicable:
		return false
	}
	return true
}

// Risk levels: low/medium/high mirror impact.Risk.Level; critical is
// the verify escalation (breaking contracts, stacked uncertainty).
// impact owns the score; verify owns only the escalation step.
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// ParseRisk validates the closed risk vocabulary.
func ParseRisk(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case RiskLow:
		return RiskLow, nil
	case RiskMedium:
		return RiskMedium, nil
	case RiskHigh:
		return RiskHigh, nil
	case RiskCritical:
		return RiskCritical, nil
	}
	return "", fmt.Errorf("verify: unknown risk %q (want low|medium|high|critical)", s)
}

// riskRank orders levels for escalation and floor comparisons.
func riskRank(r string) int {
	switch r {
	case RiskLow:
		return 0
	case RiskMedium:
		return 1
	case RiskHigh:
		return 2
	case RiskCritical:
		return 3
	}
	return -1
}

// Gate kinds: the risk→gates matrix (see matrix.go). security covers
// security+adversarial, mutation covers mutation+fuzz — the spec's
// two combined rows stay combined so the matrix has seven columns.
const (
	GateBuild       = "build"
	GateStatic      = "static"
	GateUnit        = "unit"
	GateIntegration = "integration"
	GateE2E         = "e2e"
	GateSecurity    = "security"
	GateMutation    = "mutation"
)

// AllGateKinds is the fixed execution order: compile first, fuzz
// last, so a report always reads in pipeline order.
var AllGateKinds = []string{
	GateBuild, GateStatic, GateUnit,
	GateIntegration, GateE2E, GateSecurity, GateMutation,
}

// ParseGateKind validates the closed gate vocabulary.
func ParseGateKind(s string) (string, error) {
	got := strings.ToLower(strings.TrimSpace(s))
	for _, k := range AllGateKinds {
		if got == k {
			return got, nil
		}
	}
	return "", fmt.Errorf("verify: unknown gate %q (want %s)", s, strings.Join(AllGateKinds, "|"))
}

// Scopes: a plan verifies one task or one mission. A mission plan
// carries the mission id and optionally the member task; per-task
// results roll up to the mission report by run id, never by editing
// stored evidence.
const (
	ScopeTask    = "task"
	ScopeMission = "mission"
)

// ParseScope validates the closed scope vocabulary.
func ParseScope(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", ScopeTask:
		return ScopeTask, nil
	case ScopeMission:
		return ScopeMission, nil
	}
	return "", fmt.Errorf("verify: unknown scope %q (want task|mission)", s)
}

// Provenance binds one result to the exact change, toolchain, host,
// agent, version and run that produced it — the reproducibility
// contract. Every field is a single line; RunID is mandatory (the
// edge mints it, the engine never invents identity).
type Provenance struct {
	Commit    string `json:"commit"`
	DiffHash  string `json:"diff_hash"`
	Toolchain string `json:"toolchain"`
	Host      string `json:"host"`
	Agent     string `json:"agent"`
	Version   string `json:"version"`
	RunID     string `json:"run_id"`
}

// validate checks provenance shape: single lines, run id required.
func (p Provenance) validate() error {
	for name, v := range map[string]string{
		"commit": p.Commit, "diff_hash": p.DiffHash,
		"toolchain": p.Toolchain, "host": p.Host,
		"agent": p.Agent, "version": p.Version,
	} {
		if err := singleLine(name, v, false); err != nil {
			return err
		}
	}
	return singleLine("run_id", p.RunID, true)
}

// singleLine rejects empty-required and multiline values: evidence
// fields must stay greppable single lines.
func singleLine(name, v string, required bool) error {
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("verify: %s must not be empty", name)
		}
		return nil
	}
	if strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("verify: %s must be a single line", name)
	}
	return nil
}

// shortSHA renders a checksum prefix for reports.
func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	if s == "" {
		return "unknown"
	}
	return s
}
