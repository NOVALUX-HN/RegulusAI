// Gate execution and integration gating (F13.4/NOV-107).
//
// RunPlan executes the plan's gates through an injected Executor and
// folds the outcomes into one gate decision. The engine never runs
// tools itself: the edge supplies recorded outcomes (gate receipts,
// CI, --fail/--skip/--block/--na flags) and the toolchain guard. An
// unrecorded gate stays UNKNOWN — and UNKNOWN blocks when required.
// That is the whole "nunca PASS" guarantee, in one function
// (Decide) pinned by test.
package verify

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// GateResult is one executed gate: kind, closed verdict, cause and
// the evidence locator (receipt path, command output ref, run id).
// Cause is mandatory for every non-PASS verdict; without a cause the
// result is refused.
type GateResult struct {
	Kind     string `json:"kind"`
	Verdict  string `json:"verdict"`
	Cause    string `json:"cause,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// validate checks one gate result: known kind, known verdict, cause
// present unless PASS or NOT_APPLICABLE-with-cause.
func (r GateResult) validate() error {
	if _, err := ParseGateKind(r.Kind); err != nil {
		return err
	}
	if _, err := ParseVerdict(r.Verdict); err != nil {
		return err
	}
	if r.Verdict == VerdictPass {
		return nil
	}
	if strings.TrimSpace(r.Cause) == "" {
		return fmt.Errorf("verify: gate %s verdict %s requires a cause", r.Kind, r.Verdict)
	}
	return nil
}

// Executor runs one gate and returns its recorded outcome. The
// production edge maps recorded evidence (flags, results file,
// toolchain guard) onto outcomes; tests inject fakes. Returning a
// zero GateResult means "no evidence" → UNKNOWN.
type Executor func(spec GateSpec) GateResult

// Result is one verification run: the plan it ran, every gate
// outcome, the overall verdict and the gate decision. CreatedAt is
// the only clock read, taken at execution (plans stay pure).
type Result struct {
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	TaskID    string `json:"task_id,omitempty"`
	MissionID string `json:"mission_id,omitempty"`
	ConfigID  string `json:"config_id"`
	DiffHash  string `json:"diff_hash"`

	Provenance Provenance   `json:"provenance"`
	Gates      []GateResult `json:"gates"`

	Overall     string            `json:"overall"`
	Decision    string            `json:"decision"`
	BlockReason string            `json:"block_reason,omitempty"`
	FailedGates []string          `json:"failed_gates,omitempty"`
	Precision   float64           `json:"precision"`
	Recall      float64           `json:"recall"`
	Regression  *RegressionReport `json:"regression,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// Gate decisions: the only two integration answers.
const (
	DecisionAllow = "allow"
	DecisionBlock = "block"
)

// RunPlan executes every plan gate in pipeline order and decides.
// A nil outcome from the executor (zero GateResult) becomes UNKNOWN
// with cause "sin resultado registrado". Regression evidence with
// alerts fails the run even when every gate passes: a measured
// regression gates integration like a red gate does.
func RunPlan(plan Plan, exec Executor, regression *RegressionReport, now time.Time) (Result, error) {
	if plan.Version != VerifyVersion {
		return Result{}, fmt.Errorf("verify: plan version %q is not %q", plan.Version, VerifyVersion)
	}
	if exec == nil {
		return Result{}, fmt.Errorf("verify: executor is required (no silent passes)")
	}
	required := map[string]bool{}
	for _, g := range plan.Gates {
		required[g.Kind] = true
	}
	var gates []GateResult
	for _, spec := range plan.Gates {
		got := exec(spec)
		if strings.TrimSpace(got.Kind) == "" {
			got.Kind = spec.Kind
		}
		if strings.TrimSpace(got.Verdict) == "" {
			got.Verdict = VerdictUnknown
			got.Cause = "sin resultado registrado para el gate (UNKNOWN bloquea: aporte evidencia)"
		}
		if err := got.validate(); err != nil {
			return Result{}, err
		}
		gates = append(gates, got)
	}
	overall, decision, reason, failed := Decide(gates, required, regression)
	return Result{
		Version: VerifyVersion, Scope: plan.Scope,
		TaskID: plan.TaskID, MissionID: plan.MissionID,
		ConfigID: plan.ConfigID, DiffHash: plan.DiffHash,
		Provenance: plan.Provenance, Gates: gates,
		Overall: overall, Decision: decision, BlockReason: reason,
		FailedGates: failed,
		Precision:   plan.Suite.Metrics.Precision, Recall: plan.Suite.Metrics.Recall,
		Regression: regression, CreatedAt: now.UTC(),
	}, nil
}

// Decide folds gate outcomes into the integration answer. Precedence:
// FAIL > BLOCKED > UNKNOWN > SKIPPED; NOT_APPLICABLE and PASS never
// block. A FAIL/BLOCKED/SKIPPED/UNKNOWN on a required gate blocks
// merge, integration and mission transition; advisory gates (not in
// required) are recorded and never gate. Regression alerts fail the
// run with the alert magnitudes as the reason.
func Decide(gates []GateResult, required map[string]bool, regression *RegressionReport) (overall, decision, reason string, failed []string) {
	byKind := map[string]GateResult{}
	for _, g := range gates {
		byKind[strings.ToLower(g.Kind)] = g
	}
	// Deterministic order: pipeline order, not flag order.
	for _, kind := range AllGateKinds {
		g, ok := byKind[kind]
		if !ok || !required[kind] {
			continue
		}
		if BlocksRequired(g.Verdict) {
			failed = append(failed, kind)
		}
	}
	sort.Strings(failed)
	precedence := map[string]int{
		VerdictFail: 4, VerdictBlocked: 3,
		VerdictUnknown: 2, VerdictSkipped: 1,
	}
	worst, worstRank := VerdictPass, 0
	for _, kind := range failed {
		if r := precedence[byKind[kind].Verdict]; r > worstRank {
			worst, worstRank = byKind[kind].Verdict, r
		}
	}
	if len(failed) > 0 {
		overall = worst
		decision = DecisionBlock
		var parts []string
		for _, kind := range failed {
			g := byKind[kind]
			parts = append(parts, fmt.Sprintf("%s=%s (%s)", kind, g.Verdict, g.Cause))
		}
		reason = fmt.Sprintf("bloqueado por %d gate(s) requerido(s): %s", len(failed), strings.Join(parts, "; "))
		return overall, decision, reason, failed
	}
	if regression != nil && regression.Detected {
		return VerdictFail, DecisionBlock,
			fmt.Sprintf("regresión F12.4 detectada (%d alerta(s): %s) vinculada a diff %s run %s: la integración espera",
				len(regression.Magnitudes), strings.Join(regression.Magnitudes, " | "),
				shortSHA(regression.DiffHash), regression.RunID),
			nil
	}
	return VerdictPass, DecisionAllow, "", nil
}

// AllowsIntegration reports whether merge/integration/mission
// transition may proceed: only allow does.
func (r Result) AllowsIntegration() bool { return r.Decision == DecisionAllow }
