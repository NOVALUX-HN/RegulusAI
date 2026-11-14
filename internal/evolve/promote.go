// Governed promotion with canary and atomic rollback (F7, NOV-60,
// f7-protocol-v1): EVALUATED -> CANARY -> ADOPTED with an explicit human
// approval artifact ("approved") gating canary entry, and ROLLED_BACK as
// the terminal recovery state. Existing states and edges are preserved;
// only new edges are added. Canary scope is limited to reversible,
// non-critical prompts/skills/workflows; permissions, gates, security,
// limits and other high-impact surface always need human approval.
// Rollback is atomic and cites manifest/diff/receipts/reason.
// Post-adoption re-checks the candidate against the baseline. Promotion
// never shells out: no auto commit, push or release.
//
// This file reuses CheckDomain, TrustCheck.Verify and Adoptable from the
// NOV-42 cycle instead of redefining them.
package evolve

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// New governed states. APPROVED is not a state: it is the approval
// artifact recorded in history that gates EVALUATED -> CANARY, matching
// the "evaluated -> approved -> canary -> adopted" flow without breaking
// the PROPOSED/RUNNING/EVALUATED/ADOPTED/REJECTED vocabulary.
const (
	StateCanary     = "CANARY"
	StateRolledBack = "ROLLED_BACK"
)

func init() {
	// Extend, never replace: keep every pre-existing edge and add the
	// governed canary path plus recovery to ROLLED_BACK.
	has := func(list []string, s string) bool {
		for _, v := range list {
			if v == s {
				return true
			}
		}
		return false
	}
	if !has(expAllowed[StateEvaluated], StateCanary) {
		expAllowed[StateEvaluated] = append(expAllowed[StateEvaluated], StateCanary)
	}
	if _, ok := expAllowed[StateCanary]; !ok {
		expAllowed[StateCanary] = []string{StateAdopted, StateRolledBack}
	}
	if !has(expAllowed[StateAdopted], StateRolledBack) {
		expAllowed[StateAdopted] = append(expAllowed[StateAdopted], StateRolledBack)
	}
	if _, ok := expAllowed[StateRolledBack]; !ok {
		expAllowed[StateRolledBack] = []string{}
	}
}

// CanaryScope bounds a canary to reversible, non-critical surface.
type CanaryScope struct {
	Kind       string
	Reversible bool
	Critical   bool
	Reason     string
}

// CanaryApproval is the mandatory human approval artifact. By is the
// human approver (never empty, never the candidate), Reason explains the
// risk accepted, At timestamps the decision.
type CanaryApproval struct {
	By     string
	Reason string
	At     string
}

// GovernedRollbackRecord is the atomic rollback artifact: version plus
// manifest/diff/receipts/reason, written temp+rename.
type GovernedRollbackRecord struct {
	ExperimentID string   `json:"experiment_id"`
	VersionN     int      `json:"version_n"`
	By           string   `json:"by"`
	Reason       string   `json:"reason"`
	ManifestPath string   `json:"manifest_path"`
	Diff         string   `json:"diff"`
	ReceiptIDs   []string `json:"receipt_ids"`
	At           string   `json:"at"`
}

// canaryKinds are the only canary-eligible surface.
var canaryKinds = map[string]bool{"prompt": true, "skill": true, "workflow": true}

// highImpactMarkers force human approval. They extend
// FundamentalComponents with rollout-adjacent surface.
var highImpactMarkers = []string{
	"permission", "gate", "security", "limit", "quota", "auth",
	"production", "rollout", "budget", "manifest", "verdict",
	"receipt", "trace", "fundamental",
}

// ValidateCanaryScope enforces the limited canary surface: kind in
// {prompt, skill, workflow}, reversible, non-critical.
func ValidateCanaryScope(scope CanaryScope) error {
	if !canaryKinds[strings.ToLower(strings.TrimSpace(scope.Kind))] {
		return fmt.Errorf("evolve: canary scope kind %q must be prompt, skill or workflow", scope.Kind)
	}
	if !scope.Reversible {
		return errors.New("evolve: canary scope must be reversible")
	}
	if scope.Critical {
		return errors.New("evolve: canary scope must not be critical (high-impact needs full governance, not a canary)")
	}
	return nil
}

// RequiresHumanApproval reports whether candidate text or scope forces
// human approval: harness fundamentals, high-impact markers, or a
// critical flag. Pure and deterministic.
func RequiresHumanApproval(candidateText string, scope CanaryScope) bool {
	if scope.Critical {
		return true
	}
	if len(TouchesFundamentals(candidateText)) > 0 {
		return true
	}
	lower := strings.ToLower(candidateText)
	for _, m := range highImpactMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// hasApprovalEvent reports whether history already carries a human
// approval artifact.
func hasApprovalEvent(exp Experiment) bool {
	for _, h := range exp.History {
		if h.Kind == "approval" && strings.TrimSpace(h.By) != "" {
			return true
		}
	}
	return false
}

// ApproveForCanary records the human approval artifact on an EVALUATED
// experiment. High-impact candidates are refused without a named human
// and a reason. Idempotent: repeating the same approval appends one
// event per call but never changes state.
func ApproveForCanary(root, expID string, approval CanaryApproval) error {
	if strings.TrimSpace(approval.By) == "" {
		return errors.New("evolve: canary approval needs a human approver (no self-approval)")
	}
	if strings.TrimSpace(approval.Reason) == "" {
		return errors.New("evolve: canary approval needs a reason")
	}
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return err
	}
	if exp.State != StateEvaluated {
		return fmt.Errorf("evolve: canary approval applies to EVALUATED experiments, not %s", exp.State)
	}
	if RequiresHumanApproval(exp.Candidate, CanaryScope{}) && strings.TrimSpace(approval.By) == "" {
		return errors.New("evolve: high-impact promotion needs human approval")
	}
	at := strings.TrimSpace(approval.At)
	if at == "" {
		at = time.Now().UTC().Format(time.RFC3339)
	}
	exp.History = append(exp.History, ExpHistoryEvent{
		At: at, Kind: "approval", From: exp.State, To: exp.State,
		By: approval.By, Note: "approved for canary: " + approval.Reason,
	})
	return exp.save(root)
}

// EnterCanary advances EVALUATED -> CANARY behind a recorded approval and
// a valid limited scope. Idempotent for resume: an experiment already in
// CANARY returns nil without duplicating history.
func EnterCanary(root, expID string, scope CanaryScope) error {
	if err := ValidateCanaryScope(scope); err != nil {
		return err
	}
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return err
	}
	if exp.State == StateCanary {
		return nil
	}
	if exp.State != StateEvaluated {
		return fmt.Errorf("evolve: canary entry applies to EVALUATED experiments, not %s", exp.State)
	}
	if !hasApprovalEvent(exp) {
		return errors.New("evolve: canary entry needs a recorded human approval (approval bypass refused)")
	}
	kind := strings.ToLower(strings.TrimSpace(scope.Kind))
	exp.History = append(exp.History, ExpHistoryEvent{
		At: time.Now().UTC().Format(time.RFC3339), Kind: "canary-enter",
		From: exp.State, To: StateCanary, By: "governance",
		Note: fmt.Sprintf("canary scope kind=%s reversible=%v: %s", kind, scope.Reversible, scope.Reason),
	})
	exp.State = StateCanary
	return exp.save(root)
}

// AdoptFromCanary promotes CANARY -> ADOPTED with the same guarantees as
// Adopt (domain, trust, baseline, Adoptable) plus a named human approver.
// It never shells out: no commit, push or release happens here.
func AdoptFromCanary(root, expID string, candidate Metrics, snapshot, approvedBy string) (Version, error) {
	if strings.TrimSpace(snapshot) == "" {
		return Version{}, errors.New("evolve: canary adoption needs a snapshot to roll back to")
	}
	if strings.TrimSpace(approvedBy) == "" {
		return Version{}, errors.New("evolve: canary adoption needs human approval (no self-approval)")
	}
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return Version{}, err
	}
	if exp.State != StateCanary {
		return Version{}, fmt.Errorf("evolve: only CANARY experiments adopt via canary, not %s", exp.State)
	}
	if strings.TrimSpace(exp.Results) == "" {
		return Version{}, errors.New("evolve: canary adoption needs recorded evaluation outcomes")
	}
	if err := CheckDomain(exp); err != nil {
		return Version{}, err
	}
	if err := exp.Trust.Verify(); err != nil {
		return Version{}, err
	}
	base, err := GetBaseline(root, exp.Baseline)
	if err != nil {
		return Version{}, err
	}
	if err := Adoptable(candidate, base); err != nil {
		return Version{}, err
	}
	versions, err := ListVersions(root)
	if err != nil {
		return Version{}, err
	}
	v := Version{
		N: len(versions) + 1, ExperimentID: exp.ID, Snapshot: snapshot,
		AdoptedBy: approvedBy, At: time.Now().UTC().Format(time.RFC3339),
	}
	if err := os.MkdirAll(versionsDir(root), 0o755); err != nil {
		return Version{}, err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Version{}, err
	}
	if err := os.WriteFile(filepath.Join(versionsDir(root), fmt.Sprintf("%d.json", v.N)), append(raw, '\n'), 0o644); err != nil {
		return Version{}, err
	}
	exp.History = append(exp.History, ExpHistoryEvent{
		At: v.At, Kind: "adopted", From: exp.State, To: StateAdopted,
		By: approvedBy, Note: fmt.Sprintf("adopted from canary as version %d", v.N),
	})
	exp.State = StateAdopted
	if err := exp.save(root); err != nil {
		return Version{}, err
	}
	return v, nil
}

// rollbackRecordPath is the atomic rollback artifact, kept in a rollbacks/
// subdirectory so ListVersions (versions/*.json) never sees it.
func rollbackRecordPath(root string, n int) string {
	return filepath.Join(versionsDir(root), "rollbacks", fmt.Sprintf("rollback-%d.json", n))
}

// GovernedRollback atomically supersedes version n and moves the
// experiment (CANARY or ADOPTED) to ROLLED_BACK, citing manifest, diff,
// receipts and reason. Validation precedes every write; if a late write
// fails after the record lands, the record is removed so no partial
// rollback survives.
func GovernedRollback(root, expID string, versionN int, by, reason, manifestPath, diff string, receiptIDs []string) (GovernedRollbackRecord, error) {
	if strings.TrimSpace(by) == "" {
		return GovernedRollbackRecord{}, errors.New("evolve: governed rollback needs a human operator")
	}
	if strings.TrimSpace(reason) == "" {
		return GovernedRollbackRecord{}, errors.New("evolve: governed rollback needs a reason")
	}
	if strings.TrimSpace(manifestPath) == "" {
		return GovernedRollbackRecord{}, errors.New("evolve: governed rollback needs a manifest reference")
	}
	if strings.TrimSpace(diff) == "" {
		return GovernedRollbackRecord{}, errors.New("evolve: governed rollback needs a diff")
	}
	if len(receiptIDs) == 0 {
		return GovernedRollbackRecord{}, errors.New("evolve: governed rollback needs at least one receipt")
	}
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return GovernedRollbackRecord{}, err
	}
	if exp.State != StateCanary && exp.State != StateAdopted {
		return GovernedRollbackRecord{}, fmt.Errorf("evolve: governed rollback applies to CANARY or ADOPTED experiments, not %s", exp.State)
	}
	from := exp.State
	rec := GovernedRollbackRecord{
		ExperimentID: exp.ID, VersionN: versionN, By: by, Reason: reason,
		ManifestPath: manifestPath, Diff: diff, ReceiptIDs: append([]string(nil), receiptIDs...),
		At: time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return GovernedRollbackRecord{}, err
	}
	if err := os.MkdirAll(filepath.Join(versionsDir(root), "rollbacks"), 0o755); err != nil {
		return GovernedRollbackRecord{}, err
	}
	recordPath := rollbackRecordPath(root, versionN)
	tmp, err := os.CreateTemp(filepath.Join(versionsDir(root), "rollbacks"), "rollback-*.tmp")
	if err != nil {
		return GovernedRollbackRecord{}, err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return GovernedRollbackRecord{}, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return GovernedRollbackRecord{}, err
	}
	// Supersede the version (same semantics as Rollback) before moving
	// the experiment, so the audit trail never shows an adopted version
	// without a supersede note after a governed rollback.
	v, err := Rollback(root, versionN, reason+" (governed by "+by+")")
	if err != nil {
		os.Remove(tmpName)
		return GovernedRollbackRecord{}, err
	}
	_ = v
	exp.History = append(exp.History, ExpHistoryEvent{
		At: rec.At, Kind: "rolled-back", From: from, To: StateRolledBack,
		By: by, Note: "governed rollback of version " + fmt.Sprintf("%d", versionN) + ": " + reason,
	})
	exp.State = StateRolledBack
	if err := exp.save(root); err != nil {
		os.Remove(tmpName)
		return GovernedRollbackRecord{}, err
	}
	if err := os.Rename(tmpName, recordPath); err != nil {
		os.Remove(tmpName)
		return GovernedRollbackRecord{}, err
	}
	return rec, nil
}

// PostAdoptionCheck re-checks an adopted candidate against its baseline
// (follow-up after adoption). It reuses Adoptable, never a new rule.
func PostAdoptionCheck(root, expID string, candidate Metrics) error {
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return err
	}
	if exp.State != StateAdopted && exp.State != StateCanary {
		return fmt.Errorf("evolve: post-adoption check applies to CANARY or ADOPTED experiments, not %s", exp.State)
	}
	base, err := GetBaseline(root, exp.Baseline)
	if err != nil {
		return err
	}
	return Adoptable(candidate, base)
}

// ResumeCanary reloads a governed experiment without refiring probes or
// duplicating history: the resume-safe read for canary follow-up.
func ResumeCanary(root, expID string) (Experiment, error) {
	exp, err := LoadExperiment(root, expID)
	if err != nil {
		return Experiment{}, err
	}
	switch exp.State {
	case StateEvaluated, StateCanary, StateAdopted, StateRolledBack:
		return exp, nil
	default:
		return Experiment{}, fmt.Errorf("evolve: resume applies to EVALUATED/CANARY/ADOPTED/ROLLED_BACK, not %s", exp.State)
	}
}
