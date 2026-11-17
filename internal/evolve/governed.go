// Governed self-evolution cycle (F12.7, NOV-102): close the improvement
// loop with evidence and governance. Signals from F12.4 quality,
// evals and verified memory feed one hypothesis; versioned candidates
// (K bounded) compete against the baseline through a linked F11.6
// validation plan (repetitions + governed holdout); quality, latency,
// cost, security and regressions are evaluated as separate dimensions;
// proposer/evaluator/approver/deployer are four distinct roles; human
// approval is mandatory for Core/permissions/policies/evaluators bound
// to commit and policy pins; canary runs under preapproved rollback
// conditions with monitoring; an independent adversarial review gates
// approval; the terminal report reproduces the whole cycle.
//
// The cycle never duplicates the validation protocol: comparison,
// holdout sealing, evaluator independence and the reproducible verdict
// come from validation.go / validation_runs.go / validation_report.go
// via the linked plan. This file holds declaration, signals, roles,
// scope and K-bounded candidates; evaluation, review, approval, canary,
// promotion, rollback and the report live in governed_eval.go.
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

// GovernedCycleSchema versions the governed-cycle document.
const GovernedCycleSchema = 1

// Governed states. DECLARED freezes signals, hypothesis, surface,
// roles, sensitivity and K; CANDIDATES accumulates versioned
// candidates; COMPARED pins the multi-dimension evaluation over the
// linked plan verdict; REVIEWED records the independent adversarial
// review; APPROVED records the human promotion approval; CANARY runs
// monitored; PROMOTED / ROLLED_BACK / BLOCKED are terminal. BLOCKED
// (insufficient evidence, regression, rejected review, holdout
// contradiction) never promotes: a new question needs a new cycle.
const (
	GovernedDeclared   = "DECLARED"
	GovernedCandidates = "CANDIDATES"
	GovernedCompared   = "COMPARED"
	GovernedReviewed   = "REVIEWED"
	GovernedApproved   = "APPROVED"
	GovernedCanary     = "CANARY"
	GovernedPromoted   = "PROMOTED"
	GovernedRolledBack = "ROLLED_BACK"
	GovernedBlocked    = "BLOCKED"
)

// GovernedMaxCandidates caps K: at most five versioned candidates per
// cycle. More ideas need a new cycle with its own hypothesis.
const GovernedMaxCandidates = 5

// Governed signal sources: F12.4 quality baselines/detections, eval
// manifests (F11.3 tiers, F12.4 windows), verified memory (F12.6
// learn), bench and trace stores. The cycle keeps pointers (refs),
// never bodies: the evidence stays in its store.
var governedSources = map[string]bool{
	"quality": true, "eval": true, "memory": true, "bench": true, "trace": true,
}

// GovernedSignal is one ingested improvement signal: where it comes
// from, the exact reference (path, id or hash) and a redacted summary.
type GovernedSignal struct {
	Source  string `json:"source"`
	Ref     string `json:"ref"`
	Summary string `json:"summary"`
}

// GovernedRoles pins the four separated duties. Every duty is a
// distinct identity: proposer (builds), evaluator (scores),
// approver (human, owns the risk), deployer (canary/promote/rollback).
// No agent approves its own critical change: the approver is human
// and differs from the other three.
type GovernedRoles struct {
	Proposer  string `json:"proposer"`
	Evaluator string `json:"evaluator"`
	Approver  string `json:"approver"`
	Deployer  string `json:"deployer"`
}

// GovernedSensitivity marks what the change touches. Any set flag —
// or a critical surface — makes the cycle sensitive: promotion then
// needs a human approval bound to a commit SHA and a policy reference.
type GovernedSensitivity struct {
	TouchesCore        bool `json:"touches_core"`
	TouchesPermissions bool `json:"touches_permissions"`
	TouchesPolicy      bool `json:"touches_policy"`
	TouchesEvaluator   bool `json:"touches_evaluator"`
}

// Sensitive reports whether any sensitive surface is touched.
func (s GovernedSensitivity) Sensitive() bool {
	return s.TouchesCore || s.TouchesPermissions || s.TouchesPolicy || s.TouchesEvaluator
}

// GovernedSpec is the declaration input: signals, hypothesis,
// reversible surface, separated roles, sensitivity and the K bound.
// Outcomes, candidates and decisions are recorded later, never here.
type GovernedSpec struct {
	ID            string              `json:"id,omitempty"`
	Signals       []GovernedSignal    `json:"signals"`
	Hypothesis    Hypothesis          `json:"hypothesis"`
	Surface       ValidationSurface   `json:"surface"`
	Roles         GovernedRoles       `json:"roles"`
	Sensitivity   GovernedSensitivity `json:"sensitivity"`
	MaxCandidates int                 `json:"max_candidates"`
}

// GovernedCandidate is one versioned candidate: monotonic N, content
// hash, author (always the pinned proposer) and supersede flag.
// Versions are append-only: rollback supersedes, never deletes.
type GovernedCandidate struct {
	ID         string `json:"id"`
	N          int    `json:"n"`
	ContentSHA string `json:"content_sha"`
	By         string `json:"by"`
	At         string `json:"at"`
	Superseded bool   `json:"superseded"`
	Note       string `json:"note,omitempty"`
}

// GovernedCycle is the frozen declaration plus its recorded evidence.
// The frozen subset (signals, hypothesis, surface, roles, sensitivity,
// K) is pinned by CycleSHA256; VerifyGovernedCycle recomputes it.
type GovernedCycle struct {
	ID            string              `json:"id"`
	Schema        int                 `json:"schema"`
	State         string              `json:"state"`
	DeclaredAt    string              `json:"declared_at"`
	CycleSHA256   string              `json:"cycle_sha256"`
	Signals       []GovernedSignal    `json:"signals"`
	Hypothesis    Hypothesis          `json:"hypothesis"`
	Surface       ValidationSurface   `json:"surface"`
	Roles         GovernedRoles       `json:"roles"`
	Sensitivity   GovernedSensitivity `json:"sensitivity"`
	MaxCandidates int                 `json:"max_candidates"`

	Candidates []GovernedCandidate `json:"candidates,omitempty"`
	PlanID     string              `json:"plan_id,omitempty"`
	Evaluation *GovernedEval       `json:"evaluation,omitempty"`
	Review     *GovernedReview     `json:"review,omitempty"`
	Approval   *GovernedApproval   `json:"approval,omitempty"`
	Canary     *GovernedCanaryRun  `json:"canary,omitempty"`
	Monitors   []GovernedMonitor   `json:"monitors,omitempty"`
	Promotion  *GovernedDecision   `json:"promotion,omitempty"`
	Rollback   *GovernedDecision   `json:"rollback,omitempty"`
	Blocked    *GovernedDecision   `json:"blocked,omitempty"`
	History    []ExpHistoryEvent   `json:"history"`
}

// GovernedDir returns the governed-cycle store under a project root.
func GovernedDir(root string) string {
	return filepath.Join(root, ".regulus", "evolution", "governed")
}

// mintGovID mints a unique governed-cycle id: gov_ + 8 hex chars.
func mintGovID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("evolve: cannot mint governed id: %w", err)
	}
	return "gov_" + hex.EncodeToString(b[:]), nil
}

// validateGovernedSignal refuses empty refs, unknown sources and
// multiline summaries (summaries are single-line pointers, bodies stay
// in their store).
func validateGovernedSignal(s GovernedSignal) error {
	if !governedSources[strings.TrimSpace(s.Source)] {
		return fmt.Errorf("evolve: governed signal source %q must be quality, eval, memory, bench or trace", s.Source)
	}
	if strings.TrimSpace(s.Ref) == "" {
		return errors.New("evolve: governed signal needs a ref (path, id or hash into its store)")
	}
	if strings.TrimSpace(s.Summary) == "" {
		return errors.New("evolve: governed signal needs a summary")
	}
	if strings.ContainsAny(s.Summary, "\n\r") {
		return errors.New("evolve: governed signal summary must be a single line")
	}
	return nil
}

// validateGovernedRoles enforces the separation of duties: every duty
// named, all four pairwise distinct (no self-approval by construction),
// and a human approver (never auto/bot — the same human rule the
// validation finalize owns).
func validateGovernedRoles(r GovernedRoles) error {
	seen := map[string]string{}
	for _, duty := range []struct {
		name, value string
	}{
		{"proposer", r.Proposer}, {"evaluator", r.Evaluator},
		{"approver", r.Approver}, {"deployer", r.Deployer},
	} {
		v := strings.TrimSpace(duty.value)
		if len(v) < 2 {
			return fmt.Errorf("evolve: governed %s must be a named identity (not empty, not automated)", duty.name)
		}
		lower := strings.ToLower(v)
		if other, dup := seen[lower]; dup {
			return fmt.Errorf("evolve: governed roles must be four distinct identities (%s and proposer/%s share %q: no self-approval)",
				duty.name, other, v)
		}
		seen[lower] = duty.name
	}
	if !validValidationApprover(r.Approver) {
		return fmt.Errorf("evolve: governed approver %q must be a human owner (never auto/bot)", strings.TrimSpace(r.Approver))
	}
	return nil
}

// governedFrozen is the frozen declaration subset for the cycle pin.
type governedFrozen struct {
	Schema        int
	Signals       []GovernedSignal
	Hypothesis    Hypothesis
	Surface       ValidationSurface
	Roles         GovernedRoles
	Sensitivity   GovernedSensitivity
	MaxCandidates int
}

// governedFrozenSHA hashes the canonical frozen JSON.
func governedFrozenSHA(f governedFrozen) (string, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// DeclareGovernedCycle freezes a governed declaration before any
// candidate exists. Writes are exclusive: a reused id is refused.
// Free text (signal summaries) is secret-redacted before the store.
func DeclareGovernedCycle(root string, spec GovernedSpec) (GovernedCycle, error) {
	if len(spec.Signals) == 0 {
		return GovernedCycle{}, errors.New("evolve: governed cycle needs at least one ingested signal (quality, eval or memory)")
	}
	for _, s := range spec.Signals {
		if err := validateGovernedSignal(s); err != nil {
			return GovernedCycle{}, err
		}
	}
	if err := spec.Hypothesis.Validate(); err != nil {
		return GovernedCycle{}, err
	}
	if err := validateSurface(spec.Surface); err != nil {
		return GovernedCycle{}, err
	}
	if err := validateGovernedRoles(spec.Roles); err != nil {
		return GovernedCycle{}, err
	}
	if spec.MaxCandidates < 1 || spec.MaxCandidates > GovernedMaxCandidates {
		return GovernedCycle{}, fmt.Errorf("evolve: governed max_candidates must be 1..%d (K bounded)", GovernedMaxCandidates)
	}
	id := strings.TrimSpace(spec.ID)
	if id == "" {
		var err error
		if id, err = mintGovID(); err != nil {
			return GovernedCycle{}, err
		}
	}
	signals := make([]GovernedSignal, 0, len(spec.Signals))
	for _, s := range spec.Signals {
		s.Source = strings.TrimSpace(s.Source)
		s.Ref = strings.TrimSpace(s.Ref)
		s.Summary = RedactFreeText(s.Summary)
		signals = append(signals, s)
	}
	c := GovernedCycle{
		ID: id, Schema: GovernedCycleSchema, State: GovernedDeclared,
		DeclaredAt: time.Now().UTC().Format(time.RFC3339),
		Signals:    signals, Hypothesis: spec.Hypothesis, Surface: spec.Surface,
		Roles: GovernedRoles{
			Proposer: strings.TrimSpace(spec.Roles.Proposer), Evaluator: strings.TrimSpace(spec.Roles.Evaluator),
			Approver: strings.TrimSpace(spec.Roles.Approver), Deployer: strings.TrimSpace(spec.Roles.Deployer),
		},
		Sensitivity: spec.Sensitivity, MaxCandidates: spec.MaxCandidates,
		History: []ExpHistoryEvent{{
			At: time.Now().UTC().Format(time.RFC3339), Kind: "declared",
			From: "", To: GovernedDeclared, By: strings.TrimSpace(spec.Roles.Proposer),
			Note: "signals, hypothesis, reversible surface, roles and K frozen before candidates",
		}},
	}
	sha, err := governedFrozenSHA(governedFrozen{
		Schema: c.Schema, Signals: c.Signals, Hypothesis: c.Hypothesis,
		Surface: c.Surface, Roles: c.Roles, Sensitivity: c.Sensitivity,
		MaxCandidates: c.MaxCandidates,
	})
	if err != nil {
		return GovernedCycle{}, err
	}
	c.CycleSHA256 = sha
	if err := os.MkdirAll(GovernedDir(root), 0o755); err != nil {
		return GovernedCycle{}, err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return GovernedCycle{}, err
	}
	path := filepath.Join(GovernedDir(root), id+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return GovernedCycle{}, fmt.Errorf("evolve: governed cycle %s already exists", id)
		}
		return GovernedCycle{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return GovernedCycle{}, err
	}
	return c, f.Close()
}

// LoadGovernedCycle reads one stored cycle.
func LoadGovernedCycle(root, id string) (GovernedCycle, error) {
	data, err := os.ReadFile(filepath.Join(GovernedDir(root), strings.TrimSpace(id)+".json"))
	if err != nil {
		return GovernedCycle{}, fmt.Errorf("evolve: %v", err)
	}
	var c GovernedCycle
	if err := json.Unmarshal(data, &c); err != nil {
		return GovernedCycle{}, fmt.Errorf("evolve: malformed governed cycle %s: %v", id, err)
	}
	return c, nil
}

// saveGovernedCycle writes the cycle back (candidates and evidence
// grow; the frozen subset never changes, so the pin still verifies).
func (c GovernedCycle) saveGovernedCycle(root string) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(GovernedDir(root), c.ID+".json"), append(raw, '\n'), 0o644)
}

// VerifyGovernedCycle revalidates a stored cycle: schema, state
// vocabulary, frozen-subset hash agreement and history continuity.
// Any drift in the declaration fails, never compares quietly.
func VerifyGovernedCycle(c GovernedCycle) error {
	if c.Schema != GovernedCycleSchema {
		return fmt.Errorf("evolve: governed schema %d, want %d", c.Schema, GovernedCycleSchema)
	}
	switch c.State {
	case GovernedDeclared, GovernedCandidates, GovernedCompared, GovernedReviewed,
		GovernedApproved, GovernedCanary, GovernedPromoted, GovernedRolledBack, GovernedBlocked:
	default:
		return fmt.Errorf("evolve: governed cycle %s has unknown state %q", c.ID, c.State)
	}
	sha, err := governedFrozenSHA(governedFrozen{
		Schema: c.Schema, Signals: c.Signals, Hypothesis: c.Hypothesis,
		Surface: c.Surface, Roles: c.Roles, Sensitivity: c.Sensitivity,
		MaxCandidates: c.MaxCandidates,
	})
	if err != nil {
		return err
	}
	if sha != c.CycleSHA256 {
		return fmt.Errorf("evolve: governed cycle %s drifted from its declaration (pin %s, disk %s)",
			c.ID, shortHash(c.CycleSHA256), shortHash(sha))
	}
	if len(c.History) == 0 {
		return fmt.Errorf("evolve: governed cycle %s has empty history", c.ID)
	}
	if c.History[0].Kind != "declared" || c.History[0].To != GovernedDeclared {
		return fmt.Errorf("evolve: governed cycle %s history must open with the declaration", c.ID)
	}
	prev := c.History[0].To
	for i, h := range c.History[1:] {
		if h.From != prev {
			return fmt.Errorf("evolve: governed cycle %s broken history at event %d (%s -> %s)", c.ID, i+1, prev, h.From)
		}
		prev = h.To
	}
	if c.State != prev {
		return fmt.Errorf("evolve: governed cycle %s state/history mismatch (state %s, history ends %s)", c.ID, c.State, prev)
	}
	return nil
}

// GovernedSensitive reports whether the cycle owns sensitive risk: a
// critical surface or any touched Core/permission/policy/evaluator
// flag. Sensitive cycles promote only behind a human approval bound
// to a commit SHA and a policy reference.
func (c GovernedCycle) GovernedSensitive() bool {
	return c.Surface.Critical || c.Sensitivity.Sensitive()
}

// ProposeGovernedCandidate records one versioned candidate. Only the
// pinned proposer builds (evaluator never implements, approver never
// proposes): --by must equal the declared proposer. Content is treated
// as untrusted: injection markers are refused at the door, secrets are
// redacted before the store, and the SHA pins the exact bytes scored.
func ProposeGovernedCandidate(root, cycleID, candidateID, content, by string) (GovernedCandidate, error) {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return GovernedCandidate{}, err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return GovernedCandidate{}, err
	}
	if c.State != GovernedDeclared && c.State != GovernedCandidates {
		return GovernedCandidate{}, fmt.Errorf("evolve: governed cycle %s proposes candidates while DECLARED or CANDIDATES, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(candidateID) == "" {
		return GovernedCandidate{}, errors.New("evolve: governed candidate needs an id")
	}
	if strings.TrimSpace(content) == "" {
		return GovernedCandidate{}, errors.New("evolve: governed candidate needs content (what the evaluator scores)")
	}
	if strings.TrimSpace(by) != c.Roles.Proposer {
		return GovernedCandidate{}, fmt.Errorf("evolve: only the pinned proposer %q builds candidates (got %q: evaluator/approver never implement)",
			c.Roles.Proposer, strings.TrimSpace(by))
	}
	if FlagsAdversarial(content) {
		return GovernedCandidate{}, errors.New("evolve: governed candidate carries injection markers: refused, never scored")
	}
	active := 0
	for _, cand := range c.Candidates {
		if cand.ID == strings.TrimSpace(candidateID) {
			return GovernedCandidate{}, fmt.Errorf("evolve: governed candidate %s already exists (versions are append-only)", candidateID)
		}
		if !cand.Superseded {
			active++
		}
	}
	if active >= c.MaxCandidates {
		return GovernedCandidate{}, fmt.Errorf("evolve: governed cycle %s at K=%d: no more candidates (a new question needs a new cycle)", c.ID, c.MaxCandidates)
	}
	safe := RedactFreeText(content)
	sum := sha256.Sum256([]byte(safe))
	now := time.Now().UTC().Format(time.RFC3339)
	cand := GovernedCandidate{
		ID: strings.TrimSpace(candidateID), N: len(c.Candidates) + 1,
		ContentSHA: hex.EncodeToString(sum[:]), By: c.Roles.Proposer, At: now,
	}
	c.Candidates = append(c.Candidates, cand)
	from := c.State
	c.State = GovernedCandidates
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "candidate", From: from, To: GovernedCandidates,
		By: c.Roles.Proposer, Note: fmt.Sprintf("candidate %s v%d pinned sha %s", cand.ID, cand.N, shortHash(cand.ContentSHA)),
	})
	if err := c.saveGovernedCycle(root); err != nil {
		return GovernedCandidate{}, err
	}
	return cand, nil
}

// LinkGovernedPlan attaches the F11.6 validation plan that decides the
// cycle: comparison with repetitions and the governed holdout come
// from the plan, never from a second rule. The binding is checked:
// the plan evaluator must be the pinned evaluator and the implementer
// the pinned proposer, so independence holds on both layers.
func LinkGovernedPlan(root, cycleID, planID string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedDeclared && c.State != GovernedCandidates {
		return fmt.Errorf("evolve: governed cycle %s links its plan while DECLARED or CANDIDATES, not %s", c.ID, c.State)
	}
	p, err := LoadGovernedPlanForCycle(root, planID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(p.Evaluator) != c.Roles.Evaluator {
		return fmt.Errorf("evolve: plan evaluator %q must be the pinned evaluator %q (independence on both layers)",
			p.Evaluator, c.Roles.Evaluator)
	}
	if strings.TrimSpace(p.Implementer) != c.Roles.Proposer {
		return fmt.Errorf("evolve: plan implementer %q must be the pinned proposer %q (who builds is who the evaluator checks)",
			p.Implementer, c.Roles.Proposer)
	}
	c.PlanID = p.ID
	c.History = append(c.History, ExpHistoryEvent{
		At: time.Now().UTC().Format(time.RFC3339), Kind: "linked",
		From: c.State, To: c.State, By: c.Roles.Evaluator,
		Note: fmt.Sprintf("linked validation plan %s (verdict %s decides, holdout confirms)", p.ID, p.ComparedVerdict),
	})
	return c.saveGovernedCycle(root)
}

// LoadGovernedPlanForCycle loads and integrity-checks a validation
// plan for cycle binding.
func LoadGovernedPlanForCycle(root, planID string) (ValidationPlan, error) {
	if strings.TrimSpace(planID) == "" {
		return ValidationPlan{}, errors.New("evolve: governed cycle needs a validation plan id")
	}
	p, err := LoadValidationPlan(root, planID)
	if err != nil {
		return ValidationPlan{}, err
	}
	if err := VerifyValidationPlan(p); err != nil {
		return ValidationPlan{}, err
	}
	return p, nil
}
