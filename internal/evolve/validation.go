// Validation protocol (F11.6, NOV-92): reproducible proof that an
// Evolution change improves quality, latency or cost with independent
// evaluation and rollback. The hypothesis, reversible surface,
// baseline, metrics and budget are declared BEFORE any trial and frozen
// under a SHA-256 pin: later drift fails verification instead of
// silently comparing against moved content.
//
// This file holds the pre-registration layer: declaration, frozen
// storage, reload and integrity. Trial recording lives in
// validation_runs.go; comparison, decision and the reproducible report
// in validation_report.go.
//
// Dependencies, consumed without duplication:
//   - F11.1 (observability/exec.go): trials cite receipt and trace
//     (run/execution) ids; the report points at them, never copies them.
//   - F11.3 (bench tiers): tier names L0-L3 and the sealed-holdout rule
//     (holdout runs only under L3, only after the exploration verdict,
//     only behind an audit reference).
//   - F11.5 (trace host usage): token sources use the measured-host /
//     measured-local / estimated / proxy / unknown vocabulary;
//     unknown is counted, never summed.
//   - NOV-71 (docs/f7-cycle-real.md): first real cycle — 5/5 TIE,
//     sabotage negative control with a real KEEP_BASELINE rejection,
//     human adoption against the verdict with a recorded motive.
//   - NOV-80 (docs/f8-cycle-hypothesis.md): declared-not-executed
//     second-cycle hypothesis (TE_TOOLSET=minimal, n=132, MinDelta 0,
//     positive/negative controls, significance floor).
//
// The protocol never shells out: no auto commit, push or release.
// Promotion itself reuses the governed canary/rollback path
// (promote.go); this layer proves the decision, it does not apply it.
package evolve

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidationPlanSchema versions the validation-plan document.
const ValidationPlanSchema = 1

// Validation states. DECLARED freezes the pre-registration; RUNNING
// records exploration trials; COMPARED pins the exploration verdict;
// FINAL seals the decision with human approval and rejection proof.
// There is no backward edge: a new question needs a new plan.
const (
	ValidationDeclared = "DECLARED"
	ValidationRunning  = "RUNNING"
	ValidationCompared = "COMPARED"
	ValidationFinal    = "FINAL"
)

// Validation arms.
const (
	ValidationArmBaseline  = "baseline"
	ValidationArmCandidate = "candidate"
)

// Validation splits. Exploration decides; holdout only confirms behind
// an audited gate (F11.3: sealed below L3 and before the verdict).
const (
	ValidationSplitExploration = "exploration"
	ValidationSplitHoldout     = "holdout"
)

// Validation control kinds: a positive control must hold (A/A ties),
// a negative control must reject (sabotage keeps the baseline).
const (
	ValidationControlPositive = "positive"
	ValidationControlNegative = "negative"
)

// Inherited-change decisions: explicit at finalize, never defaulted.
const (
	ValidationInheritedAdopt   = "adopt"
	ValidationInheritedKeep    = "keep"
	ValidationInheritedRevisit = "revisit"
)

// validationTiers mirrors the F11.3 tier names (bench is the source of
// truth; evolve must not import bench — bench imports evolve).
var validationTiers = map[string]bool{"L0": true, "L1": true, "L2": true, "L3": true}

// validationTierIncludesHoldout mirrors bench TierIncludesHoldout:
// only the full L3 run may touch the holdout.
func validationTierIncludesHoldout(tier string) bool {
	return tier == "L3"
}

// ValidationSurface is the declared reversible surface: what the
// candidate touches, how it reverts, and whether it is critical.
// Reversibility is mandatory: an irreversible change cannot enter the
// protocol (rollback must be provable, F11.6 acceptance).
type ValidationSurface struct {
	Kind          string `json:"kind"`
	Description   string `json:"description"`
	Reversible    bool   `json:"reversible"`
	Critical      bool   `json:"critical"`
	RevertCommand string `json:"revert_command"`
}

// ValidationBudget bounds one validation before it starts. MaxRuns is
// the hard cap on recorded trials (it must cover the declared
// seed/repetition/model matrix); wall time and tokens are tracked and
// reported against the budget, never silently exceeded.
type ValidationBudget struct {
	MaxRuns       int   `json:"max_runs"`
	MaxWallTimeMS int64 `json:"max_wall_time_ms"`
	MaxTokens     int64 `json:"max_tokens"`
}

// ValidationPlanSpec is the pre-registration input: everything F11.6
// names (hypothesis, surface, baseline, metrics, budget) plus the
// execution matrix (seeds, repetitions, comparable models) and the
// independent evaluator. Outcomes, trials and decisions are recorded
// later; they are never part of the declaration.
type ValidationPlanSpec struct {
	ID               string            `json:"id,omitempty"`
	Hypothesis       Hypothesis        `json:"Hypothesis"`
	Surface          ValidationSurface `json:"surface"`
	Baseline         string            `json:"baseline"`
	Candidate        string            `json:"candidate"`
	Dataset          string            `json:"dataset"`
	Tier             string            `json:"tier"`
	Models           []string          `json:"models"`
	Seeds            []int64           `json:"seeds"`
	Repetitions      int               `json:"repetitions"`
	Primary          string            `json:"primary"`
	MinDelta         float64           `json:"min_delta"`
	Budget           ValidationBudget  `json:"budget"`
	Implementer      string            `json:"implementer"`
	Evaluator        string            `json:"evaluator"`
	EvaluatorVersion string            `json:"evaluator_version"`
	Commands         []string          `json:"commands"`
	Notes            string            `json:"notes,omitempty"`
}

// ValidationPlan is a frozen pre-registration with its recorded
// evidence. The frozen subset (everything the declaration fixed) is
// pinned by PlanSHA256; VerifyValidationPlan recomputes it from disk.
type ValidationPlan struct {
	ID                string              `json:"id"`
	Schema            int                 `json:"schema"`
	State             string              `json:"state"`
	DeclaredAt        string              `json:"declared_at"`
	PlanSHA256        string              `json:"plan_sha256"`
	Hypothesis        Hypothesis          `json:"hypothesis"`
	Surface           ValidationSurface   `json:"surface"`
	Baseline          string              `json:"baseline"`
	Candidate         string              `json:"candidate"`
	Dataset           string              `json:"dataset"`
	Tier              string              `json:"tier"`
	Models            []string            `json:"models"`
	Seeds             []int64             `json:"seeds"`
	Repetitions       int                 `json:"repetitions"`
	Primary           string              `json:"primary"`
	MinDelta          float64             `json:"min_delta"`
	Budget            ValidationBudget    `json:"budget"`
	Implementer       string              `json:"implementer"`
	Evaluator         string              `json:"evaluator"`
	EvaluatorVersion  string              `json:"evaluator_version"`
	Commands          []string            `json:"commands"`
	Notes             string              `json:"notes,omitempty"`
	Trials            []ValidationTrial   `json:"trials,omitempty"`
	Controls          []ValidationControl `json:"controls,omitempty"`
	Discards          []ValidationDiscard `json:"discards,omitempty"`
	LinkedExperiments []string            `json:"linked_experiments,omitempty"`
	HoldoutAudits     []string            `json:"holdout_audits,omitempty"`
	ComparedAt        string              `json:"compared_at,omitempty"`
	ComparedVerdict   string              `json:"compared_verdict,omitempty"`
	ComparedReason    string              `json:"compared_reason,omitempty"`
	ComparedTrials    int                 `json:"compared_trials,omitempty"`
	Decision          *ValidationDecision `json:"decision,omitempty"`
	Evidence          *ValidationEvidence `json:"evidence,omitempty"`
	History           []ExpHistoryEvent   `json:"history"`
}

// ValidationDir returns the validation-plan store under a project root.
func ValidationDir(root string) string {
	return filepath.Join(root, ".regulus", "evolution", "validations")
}

// mintValID mints a unique validation id: val_ + 8 hex chars.
func mintValID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("evolve: cannot mint validation id: %w", err)
	}
	return "val_" + hex.EncodeToString(b[:]), nil
}

// validationFrozen is the frozen declaration subset: exactly what the
// pre-registration fixed, in field order, for the SHA pin.
type validationFrozen struct {
	Schema           int
	Hypothesis       Hypothesis
	Surface          ValidationSurface
	Baseline         string
	Candidate        string
	Dataset          string
	Tier             string
	Models           []string
	Seeds            []int64
	Repetitions      int
	Primary          string
	MinDelta         float64
	Budget           ValidationBudget
	Implementer      string
	Evaluator        string
	EvaluatorVersion string
	Commands         []string
	Notes            string
}

// frozenOf projects a plan onto its frozen subset.
func frozenOf(p ValidationPlan) validationFrozen {
	return validationFrozen{
		Schema: p.Schema, Hypothesis: p.Hypothesis, Surface: p.Surface,
		Baseline: p.Baseline, Candidate: p.Candidate, Dataset: p.Dataset, Tier: p.Tier,
		Models: append([]string(nil), p.Models...), Seeds: append([]int64(nil), p.Seeds...),
		Repetitions: p.Repetitions, Primary: p.Primary, MinDelta: p.MinDelta,
		Budget: p.Budget, Implementer: p.Implementer, Evaluator: p.Evaluator,
		EvaluatorVersion: p.EvaluatorVersion, Commands: append([]string(nil), p.Commands...),
		Notes: p.Notes,
	}
}

// frozenSHA hashes the canonical frozen JSON.
func frozenSHA(f validationFrozen) (string, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// validateSurface enforces the reversible-surface rule: kind,
// description and revert command present, reversibility mandatory.
// Critical surface is admitted but flagged: the human approval at
// finalize owns that risk.
func validateSurface(s ValidationSurface) error {
	if strings.TrimSpace(s.Kind) == "" {
		return errors.New("evolve: validation surface needs a kind")
	}
	if strings.TrimSpace(s.Description) == "" {
		return errors.New("evolve: validation surface needs a description")
	}
	if !s.Reversible {
		return errors.New("evolve: validation surface must be reversible (rollback must be provable)")
	}
	if strings.TrimSpace(s.RevertCommand) == "" {
		return errors.New("evolve: validation surface needs a revert command")
	}
	return nil
}

// validateMatrix enforces a runnable execution matrix: tier in the
// F11.3 vocabulary, distinct non-empty models, distinct non-zero
// seeds, at least one repetition, non-negative bar, and a budget that
// covers the declared matrix before the first trial.
func validateMatrix(spec ValidationPlanSpec) error {
	if !validationTiers[spec.Tier] {
		return fmt.Errorf("evolve: validation tier %q must be L0, L1, L2 or L3", spec.Tier)
	}
	if len(spec.Models) == 0 {
		return errors.New("evolve: validation needs at least one comparable model")
	}
	seen := map[string]bool{}
	for _, m := range spec.Models {
		if strings.TrimSpace(m) == "" {
			return errors.New("evolve: validation models must be non-empty")
		}
		if seen[m] {
			return fmt.Errorf("evolve: validation lists model %q twice", m)
		}
		seen[m] = true
	}
	if len(spec.Seeds) == 0 {
		return errors.New("evolve: validation needs at least one non-zero seed")
	}
	sseen := map[int64]bool{}
	for _, s := range spec.Seeds {
		if s == 0 {
			return errors.New("evolve: validation needs at least one non-zero seed")
		}
		if sseen[s] {
			return fmt.Errorf("evolve: validation lists seed %d twice", s)
		}
		sseen[s] = true
	}
	if spec.Repetitions < 1 {
		return errors.New("evolve: validation needs at least one repetition")
	}
	if spec.MinDelta < 0 {
		return errors.New("evolve: validation MinDelta must not be negative")
	}
	need := len(spec.Seeds) * spec.Repetitions * len(spec.Models)
	if spec.Budget.MaxRuns < need {
		return fmt.Errorf("evolve: validation budget MaxRuns=%d below the declared matrix (%d seeds x %d reps x %d models = %d)",
			spec.Budget.MaxRuns, len(spec.Seeds), spec.Repetitions, len(spec.Models), need)
	}
	if spec.Budget.MaxWallTimeMS < 1 {
		return errors.New("evolve: validation budget needs MaxWallTimeMS >= 1")
	}
	if spec.Budget.MaxTokens < 0 {
		return errors.New("evolve: validation budget MaxTokens must not be negative")
	}
	return nil
}

// validValidationApprover reports whether s names a human owner.
// Mirrors the cli adopt gate (cli.isHumanApproval): automated
// approvers never own a validation decision.
func validValidationApprover(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 3 {
		return false
	}
	switch strings.ToLower(t) {
	case "auto", "bot", "system", "auto-approve", "self-approve", "ai":
		return false
	}
	return true
}

// DeclareValidationPlan freezes a pre-registration before any trial.
// Writes are exclusive: a reused id is refused. Free text (baseline
// label, candidate, notes, commands) is secret-redacted before the
// store, mirroring the manifest rule.
func DeclareValidationPlan(root string, spec ValidationPlanSpec) (ValidationPlan, error) {
	if err := spec.Hypothesis.Validate(); err != nil {
		return ValidationPlan{}, err
	}
	if err := validateSurface(spec.Surface); err != nil {
		return ValidationPlan{}, err
	}
	for _, field := range []struct {
		name, value string
	}{
		{"baseline", spec.Baseline}, {"candidate", spec.Candidate}, {"dataset", spec.Dataset},
		{"primary metric", spec.Primary}, {"implementer", spec.Implementer},
		{"evaluator", spec.Evaluator}, {"evaluator version", spec.EvaluatorVersion},
	} {
		if strings.TrimSpace(field.value) == "" {
			return ValidationPlan{}, fmt.Errorf("evolve: validation needs a %s declared before experimenting", field.name)
		}
	}
	if strings.EqualFold(strings.TrimSpace(spec.Evaluator), strings.TrimSpace(spec.Implementer)) {
		return ValidationPlan{}, errors.New("evolve: validation evaluator must be independent of the implementer (no self-evaluation)")
	}
	if len(spec.Commands) == 0 {
		return ValidationPlan{}, errors.New("evolve: validation needs at least one declared command")
	}
	if err := validateMatrix(spec); err != nil {
		return ValidationPlan{}, err
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		var err error
		if id, err = mintValID(); err != nil {
			return ValidationPlan{}, err
		}
	}
	commands := make([]string, 0, len(spec.Commands))
	for _, c := range spec.Commands {
		commands = append(commands, RedactFreeText(c))
	}
	p := ValidationPlan{
		ID: id, Schema: ValidationPlanSchema, State: ValidationDeclared,
		DeclaredAt: time.Now().UTC().Format(time.RFC3339),
		Hypothesis: spec.Hypothesis, Surface: spec.Surface,
		Baseline: RedactFreeText(spec.Baseline), Candidate: RedactFreeText(spec.Candidate),
		Dataset: spec.Dataset, Tier: spec.Tier,
		Models: append([]string(nil), spec.Models...), Seeds: append([]int64(nil), spec.Seeds...),
		Repetitions: spec.Repetitions, Primary: spec.Primary, MinDelta: spec.MinDelta,
		Budget:      spec.Budget,
		Implementer: strings.TrimSpace(spec.Implementer), Evaluator: strings.TrimSpace(spec.Evaluator),
		EvaluatorVersion: strings.TrimSpace(spec.EvaluatorVersion),
		Commands:         commands, Notes: RedactFreeText(spec.Notes),
		History: []ExpHistoryEvent{{
			At: time.Now().UTC().Format(time.RFC3339), Kind: "declared",
			From: "", To: ValidationDeclared, By: strings.TrimSpace(spec.Implementer),
			Note: "hypothesis, surface, baseline, metrics and budget frozen before experimenting",
		}},
	}
	sha, err := frozenSHA(frozenOf(p))
	if err != nil {
		return ValidationPlan{}, err
	}
	p.PlanSHA256 = sha
	if err := os.MkdirAll(ValidationDir(root), 0o755); err != nil {
		return ValidationPlan{}, err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return ValidationPlan{}, err
	}
	path := filepath.Join(ValidationDir(root), id+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return ValidationPlan{}, fmt.Errorf("evolve: validation %s already exists", id)
		}
		return ValidationPlan{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return ValidationPlan{}, err
	}
	return p, f.Close()
}

// LoadValidationPlan reads one frozen plan.
func LoadValidationPlan(root, id string) (ValidationPlan, error) {
	data, err := os.ReadFile(filepath.Join(ValidationDir(root), strings.TrimSpace(id)+".json"))
	if err != nil {
		return ValidationPlan{}, fmt.Errorf("evolve: %v", err)
	}
	var p ValidationPlan
	if err := json.Unmarshal(data, &p); err != nil {
		return ValidationPlan{}, fmt.Errorf("evolve: malformed validation %s: %v", id, err)
	}
	return p, nil
}

// saveValidationPlan writes the plan back (history and evidence grow;
// the frozen subset never changes, so the pin still verifies).
func (p ValidationPlan) saveValidationPlan(root string) error {
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ValidationDir(root), p.ID+".json"), append(raw, '\n'), 0o644)
}

// VerifyValidationPlan revalidates a stored plan: schema, state
// vocabulary, frozen-subset hash agreement, and history continuity.
// Any drift in the pre-registration fails, never compares quietly.
func VerifyValidationPlan(p ValidationPlan) error {
	if p.Schema != ValidationPlanSchema {
		return fmt.Errorf("evolve: validation schema %d, want %d", p.Schema, ValidationPlanSchema)
	}
	switch p.State {
	case ValidationDeclared, ValidationRunning, ValidationCompared, ValidationFinal:
	default:
		return fmt.Errorf("evolve: validation %s has unknown state %q", p.ID, p.State)
	}
	sha, err := frozenSHA(frozenOf(p))
	if err != nil {
		return err
	}
	if sha != p.PlanSHA256 {
		return fmt.Errorf("evolve: validation %s drifted from its declaration (pin %s, disk %s)",
			p.ID, shortHash(p.PlanSHA256), shortHash(sha))
	}
	if len(p.History) == 0 {
		return fmt.Errorf("evolve: validation %s has empty history", p.ID)
	}
	if p.History[0].Kind != "declared" || p.History[0].To != ValidationDeclared {
		return fmt.Errorf("evolve: validation %s history must open with the declaration", p.ID)
	}
	prev := p.History[0].To
	for i, h := range p.History[1:] {
		if h.From != prev {
			return fmt.Errorf("evolve: validation %s broken history at event %d (%s -> %s)", p.ID, i+1, prev, h.From)
		}
		prev = h.To
	}
	if p.State != prev {
		return fmt.Errorf("evolve: validation %s state/history mismatch (state %s, history ends %s)", p.ID, p.State, prev)
	}
	return nil
}
