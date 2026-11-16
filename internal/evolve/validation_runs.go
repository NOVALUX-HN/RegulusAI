// Validation trial recording (F11.6, NOV-92): candidates run with the
// declared seeds, repetitions and comparable models. Exploration trials
// decide; holdout trials only confirm behind an audited gate after the
// exploration verdict. Discards and positive/negative controls are
// registered alongside, never reconstructed after the fact.
package evolve

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/trace"
)

// ValidationTrial is one recorded attempt of one arm. Arm, seed,
// repetition and model must fall inside the declared matrix; the split
// selects exploration (decision) or holdout (confirmation-only).
// Latency is wall milliseconds of the attempt; Tokens carries the
// attempt cost with its TokenSource (F11.5 vocabulary: unknown trials
// are counted, never summed). ReceiptID and TraceID point at the F11.1
// exec log; the trial keeps the pointers, never the bodies.
type ValidationTrial struct {
	Arm         string `json:"arm"`
	TaskID      string `json:"task_id"`
	Seed        int64  `json:"seed"`
	Repetition  int    `json:"repetition"`
	Model       string `json:"model"`
	Split       string `json:"split"`
	Status      string `json:"status"`
	LatencyMS   int64  `json:"latency_ms"`
	Tokens      int64  `json:"tokens"`
	TokenSource string `json:"token_source"`
	ReceiptID   string `json:"receipt_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
}

// validTrialStatus reports whether s is an F11.3-classified outcome.
// Missing is not recordable: an unrun attempt is not evidence.
func validTrialStatus(s string) bool {
	switch s {
	case ManifestPass, ManifestFail, ManifestTimeout, ManifestInfra, ManifestNonEvaluable:
		return true
	}
	return false
}

// validTokenSource reports whether s is an F11.5 measurement label.
func validTokenSource(s string) bool {
	switch s {
	case trace.SourceMeasuredHost, trace.SourceMeasuredLocal, trace.SourceEstimated, trace.SourceProxy, trace.SourceUnknown:
		return true
	}
	return false
}

// inMatrix reports whether the trial coordinates fall inside the
// declared seed/repetition/model matrix.
func (p ValidationPlan) inMatrix(t ValidationTrial) error {
	switch t.Arm {
	case ValidationArmBaseline, ValidationArmCandidate:
	default:
		return fmt.Errorf("evolve: validation arm %q must be baseline or candidate", t.Arm)
	}
	seedOK := false
	for _, s := range p.Seeds {
		if s == t.Seed {
			seedOK = true
		}
	}
	if !seedOK {
		return fmt.Errorf("evolve: validation seed %d is outside the declared matrix", t.Seed)
	}
	modelOK := false
	for _, m := range p.Models {
		if m == t.Model {
			modelOK = true
		}
	}
	if !modelOK {
		return fmt.Errorf("evolve: validation model %q is outside the declared matrix", t.Model)
	}
	if t.Repetition < 0 || t.Repetition >= p.Repetitions {
		return fmt.Errorf("evolve: validation repetition %d is outside the declared 0..%d", t.Repetition, p.Repetitions-1)
	}
	return nil
}

// checkTrialFields validates one trial's shape without touching disk.
func checkTrialFields(t ValidationTrial) error {
	if strings.TrimSpace(t.TaskID) == "" {
		return errors.New("evolve: validation trial needs a task id")
	}
	if !validTrialStatus(t.Status) {
		return fmt.Errorf("evolve: validation status %q must be pass, fail, timeout, infra or non-evaluable", t.Status)
	}
	if t.LatencyMS < 0 {
		return errors.New("evolve: validation latency must not be negative")
	}
	if t.Tokens < 0 {
		return errors.New("evolve: validation tokens must not be negative")
	}
	if !validTokenSource(t.TokenSource) {
		return fmt.Errorf("evolve: validation token source %q must be measured-host, measured-local, estimated, proxy or unknown", t.TokenSource)
	}
	return nil
}

// RecordValidationTrial appends one exploration trial. The holdout
// stays sealed here: holdout evidence enters only through
// RecordHoldoutTrial after the exploration verdict with an audit
// reference. The budget hard-caps recorded trials; the first trial
// flips DECLARED to RUNNING.
func RecordValidationTrial(root, planID string, t ValidationTrial) (ValidationPlan, error) {
	if t.Split != ValidationSplitExploration {
		return ValidationPlan{}, fmt.Errorf("evolve: holdout is sealed until the exploration verdict (record via the audited holdout path), got split %q", t.Split)
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return ValidationPlan{}, err
	}
	if p.State != ValidationDeclared && p.State != ValidationRunning {
		return ValidationPlan{}, fmt.Errorf("evolve: validation %s records trials while DECLARED or RUNNING, not %s", p.ID, p.State)
	}
	if err := checkTrialFields(t); err != nil {
		return ValidationPlan{}, err
	}
	if err := p.inMatrix(t); err != nil {
		return ValidationPlan{}, err
	}
	if len(p.Trials)+1 > p.Budget.MaxRuns {
		return ValidationPlan{}, fmt.Errorf("evolve: validation %s budget exhausted (MaxRuns=%d)", p.ID, p.Budget.MaxRuns)
	}
	p.Trials = append(p.Trials, t)
	now := time.Now().UTC().Format(time.RFC3339)
	if p.State == ValidationDeclared {
		p.History = append(p.History, ExpHistoryEvent{
			At: now, Kind: "running", From: p.State, To: ValidationRunning,
			By: p.Implementer, Note: "first exploration trial recorded",
		})
		p.State = ValidationRunning
	} else {
		p.History = append(p.History, ExpHistoryEvent{
			At: now, Kind: "trial", From: p.State, To: p.State,
			By: p.Implementer, Note: fmt.Sprintf("exploration trial %s/%s seed=%d rep=%d", t.Arm, t.TaskID, t.Seed, t.Repetition),
		})
	}
	if err := p.saveValidationPlan(root); err != nil {
		return ValidationPlan{}, err
	}
	return p, nil
}

// RecordHoldoutTrial appends one holdout trial behind the governed
// gate: only under L3, only after the exploration verdict (COMPARED),
// only with a non-empty audit reference. The audit trail keeps every
// reference; the confirmation never re-decides.
func RecordHoldoutTrial(root, planID, auditRef string, t ValidationTrial) (ValidationPlan, error) {
	if strings.TrimSpace(auditRef) == "" {
		return ValidationPlan{}, errors.New("evolve: holdout is confirmation-only behind an audited gate (audit reference required)")
	}
	if t.Split != ValidationSplitHoldout {
		return ValidationPlan{}, fmt.Errorf("evolve: the audited holdout path records holdout trials, got split %q", t.Split)
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return ValidationPlan{}, err
	}
	if !validationTierIncludesHoldout(p.Tier) {
		return ValidationPlan{}, fmt.Errorf("evolve: holdout stays sealed below L3 (plan tier %s)", p.Tier)
	}
	if p.State != ValidationCompared {
		return ValidationPlan{}, fmt.Errorf("evolve: holdout opens after the exploration verdict (COMPARED), not %s", p.State)
	}
	if err := checkTrialFields(t); err != nil {
		return ValidationPlan{}, err
	}
	if err := p.inMatrix(t); err != nil {
		return ValidationPlan{}, err
	}
	if len(p.Trials)+1 > p.Budget.MaxRuns {
		return ValidationPlan{}, fmt.Errorf("evolve: validation %s budget exhausted (MaxRuns=%d)", p.ID, p.Budget.MaxRuns)
	}
	p.Trials = append(p.Trials, t)
	p.HoldoutAudits = append(p.HoldoutAudits, strings.TrimSpace(auditRef))
	p.History = append(p.History, ExpHistoryEvent{
		At: time.Now().UTC().Format(time.RFC3339), Kind: "holdout-recorded",
		From: p.State, To: p.State, By: p.Evaluator,
		Note: fmt.Sprintf("holdout trial %s/%s seed=%d rep=%d audited via %s", t.Arm, t.TaskID, t.Seed, t.Repetition, strings.TrimSpace(auditRef)),
	})
	if err := p.saveValidationPlan(root); err != nil {
		return ValidationPlan{}, err
	}
	return p, nil
}

// ValidationControl is one registered control: a positive control
// (A/A, trivially true change) must hold its expectation, a negative
// control (sabotage, gaming trap) must reject. Held=false is a signal
// the report carries, never a hidden rerun.
type ValidationControl struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Expectation string `json:"expectation"`
	Outcome     string `json:"outcome"`
	Held        bool   `json:"held"`
	Detail      string `json:"detail,omitempty"`
}

// RecordValidationControl registers one positive/negative control
// outcome. Controls are admitted before FINAL; a missing control is
// reported as missing, never invented.
func RecordValidationControl(root, planID string, c ValidationControl) error {
	switch c.Kind {
	case ValidationControlPositive, ValidationControlNegative:
	default:
		return fmt.Errorf("evolve: validation control kind %q must be positive or negative", c.Kind)
	}
	for _, field := range []struct {
		name, value string
	}{
		{"name", c.Name}, {"expectation", c.Expectation}, {"outcome", c.Outcome},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("evolve: validation control needs a %s", field.name)
		}
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return err
	}
	if p.State == ValidationFinal {
		return fmt.Errorf("evolve: validation %s is FINAL: controls are sealed", p.ID)
	}
	p.Controls = append(p.Controls, c)
	p.History = append(p.History, ExpHistoryEvent{
		At: time.Now().UTC().Format(time.RFC3339), Kind: "control",
		From: p.State, To: p.State, By: p.Evaluator,
		Note: fmt.Sprintf("%s control %s: expected %s, observed %s (held=%v)", c.Kind, c.Name, c.Expectation, c.Outcome, c.Held),
	})
	return p.saveValidationPlan(root)
}

// ValidationDiscard is one rejected candidate with its reason: the
// registry of what did NOT survive. F11.6 demands at least one real
// rejection in the report — a discard here or a linked
// REJECTED/ROLLED_BACK experiment at finalize.
type ValidationDiscard struct {
	CandidateID string `json:"candidate_id"`
	Reason      string `json:"reason"`
	By          string `json:"by"`
	At          string `json:"at"`
}

// DiscardValidationCandidate registers one rejected candidate. The
// reason is listed on the plan and persists; the baseline is kept.
func DiscardValidationCandidate(root, planID string, d ValidationDiscard) error {
	if strings.TrimSpace(d.CandidateID) == "" {
		return errors.New("evolve: discard needs a candidate id")
	}
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("evolve: discard needs a reason (silent drops are not evidence)")
	}
	if strings.TrimSpace(d.By) == "" {
		return errors.New("evolve: discard needs an owner")
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return err
	}
	if p.State == ValidationFinal {
		return fmt.Errorf("evolve: validation %s is FINAL: discards are sealed", p.ID)
	}
	d.By = strings.TrimSpace(d.By)
	d.At = time.Now().UTC().Format(time.RFC3339)
	p.Discards = append(p.Discards, d)
	p.History = append(p.History, ExpHistoryEvent{
		At: d.At, Kind: "discarded", From: p.State, To: p.State,
		By: d.By, Note: fmt.Sprintf("discarded %s: %s", d.CandidateID, strings.TrimSpace(d.Reason)),
	})
	return p.saveValidationPlan(root)
}

// LinkValidationExperiment attaches a stored experiment as rejection
// (or context) proof for the report. The link cites the experiment id;
// FinalizeValidation loads its live state, so a later adoption cannot
// masquerade as the required rejection.
func LinkValidationExperiment(root, planID, expID string) error {
	if strings.TrimSpace(expID) == "" {
		return errors.New("evolve: link needs an experiment id")
	}
	if _, err := LoadExperiment(root, strings.TrimSpace(expID)); err != nil {
		return err
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return err
	}
	if p.State == ValidationFinal {
		return fmt.Errorf("evolve: validation %s is FINAL: links are sealed", p.ID)
	}
	for _, linked := range p.LinkedExperiments {
		if linked == strings.TrimSpace(expID) {
			return nil
		}
	}
	p.LinkedExperiments = append(p.LinkedExperiments, strings.TrimSpace(expID))
	p.History = append(p.History, ExpHistoryEvent{
		At: time.Now().UTC().Format(time.RFC3339), Kind: "linked",
		From: p.State, To: p.State, By: p.Evaluator,
		Note: fmt.Sprintf("linked experiment %s as rejection/context proof", strings.TrimSpace(expID)),
	})
	return p.saveValidationPlan(root)
}
