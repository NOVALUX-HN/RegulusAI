// Governed evaluation, review, approval, canary, promotion, rollback
// and report (F12.7, NOV-102). Comparison reuses the linked F11.6
// validation plan (CompareValidation freezes the exploration verdict;
// the holdout confirms, it never re-decides); this layer folds the
// five governed dimensions around it, gates promotion on an
// independent adversarial review plus a human approval bound to
// commit/policy pins, and runs a monitored canary with preapproved
// rollback conditions.
package evolve

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/trace"
)

// GovernedEval is the pinned multi-dimension evaluation: the linked
// plan verdict plus quality, latency, cost, security and regression
// lines folded from the plan exploration trials. Every dimension is
// reported with its N; significance is never claimed.
type GovernedEval struct {
	PlanID         string `json:"plan_id"`
	PlanVerdict    string `json:"plan_verdict"`
	GapPassRate    string `json:"gap_pass_rate"`
	Quality        string `json:"quality"`
	Latency        string `json:"latency"`
	Cost           string `json:"cost"`
	Security       string `json:"security"`
	Regressions    string `json:"regressions"`
	Holdout        string `json:"holdout"`
	ComparedAt     string `json:"compared_at"`
	ExploredTrials int    `json:"explored_trials"`
}

// GovernedReview is the independent adversarial review: a reviewer
// outside all four pinned roles probes the leading candidate (control
// expectations, injection markers, domain hold) and passes or rejects.
// A reject blocks the cycle terminally.
type GovernedReview struct {
	Reviewer string `json:"reviewer"`
	Verdict  string `json:"verdict"`
	Reason   string `json:"reason"`
	At       string `json:"at"`
}

// GovernedApproval is the human promotion approval. By must equal the
// pinned approver (never the proposer/evaluator/deployer: roles are
// pairwise distinct since declare). Sensitive cycles additionally bind
// the approval to a commit SHA and a policy reference: the approval
// owns exact bytes under an exact policy.
type GovernedApproval struct {
	By          string `json:"by"`
	Reason      string `json:"reason"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	PolicyRef   string `json:"policy_ref,omitempty"`
	ManifestRef string `json:"manifest_ref,omitempty"`
	At          string `json:"at"`
}

// GovernedCanaryRun bounds the monitored rollout: a limited canary scope
// (prompt/skill/workflow, reversible, non-critical — the promote.go
// rule, not a new one) plus the preapproved rollback conditions the
// deployer may invoke without a new approval.
type GovernedCanaryRun struct {
	Kind       string   `json:"kind"`
	Reversible bool     `json:"reversible"`
	Reason     string   `json:"reason"`
	Conditions []string `json:"conditions"`
	At         string   `json:"at"`
}

// GovernedMonitor is one canary health observation by the deployer.
type GovernedMonitor struct {
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail"`
	By      string `json:"by"`
	At      string `json:"at"`
}

// GovernedDecision is a terminal record: promotion (with the winning
// candidate), rollback (with the invoked preapproved condition) or the
// block (with the evidence that refused promotion).
type GovernedDecision struct {
	Verdict string `json:"verdict"`
	By      string `json:"by"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
	At      string `json:"at"`
}

// foldGovernedDims folds latency/cost lines from exploration trials:
// latency mean per arm (evaluable attempts), measured token totals
// with unknown trials counted but never summed (F11.5).
func foldGovernedDims(trials []ValidationTrial) (latency, cost string) {
	var baseLat, candLat int64
	var baseN, candN int
	var baseTok, candTok int64
	var baseUnk, candUnk int
	for _, t := range trials {
		if t.Status != ManifestPass && t.Status != ManifestFail {
			continue
		}
		if t.Arm == ValidationArmBaseline {
			baseN++
			baseLat += t.LatencyMS
			if t.TokenSource == trace.SourceUnknown {
				baseUnk++
			} else {
				baseTok += t.Tokens
			}
			continue
		}
		candN++
		candLat += t.LatencyMS
		if t.TokenSource == trace.SourceUnknown {
			candUnk++
		} else {
			candTok += t.Tokens
		}
	}
	baseMean, candMean := 0.0, 0.0
	if baseN > 0 {
		baseMean = float64(baseLat) / float64(baseN)
	}
	if candN > 0 {
		candMean = float64(candLat) / float64(candN)
	}
	latency = fmt.Sprintf("baseline mean %.1fms (N=%d) vs candidate mean %.1fms (N=%d)", baseMean, baseN, candMean, candN)
	cost = fmt.Sprintf("baseline measured %d tokens (unknown %d, never summed) vs candidate measured %d (unknown %d)", baseTok, baseUnk, candTok, candUnk)
	return latency, cost
}

// governedBlock seals the cycle BLOCKED with its evidence. Terminal:
// a blocked cycle never promotes; a new question needs a new cycle.
func governedBlock(c *GovernedCycle, by, verdict, reason, detail string) GovernedDecision {
	now := time.Now().UTC().Format(time.RFC3339)
	d := GovernedDecision{Verdict: verdict, By: by, Reason: reason, Detail: detail, At: now}
	c.Blocked = &d
	from := c.State
	c.State = GovernedBlocked
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "blocked", From: from, To: GovernedBlocked,
		By: by, Note: fmt.Sprintf("%s: %s", verdict, reason),
	})
	return d
}

// loadVerifiedGovernedCycle loads the stored cycle and revalidates its
// frozen pin and history continuity.
func loadVerifiedGovernedCycle(root, cycleID string) (GovernedCycle, error) {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return GovernedCycle{}, err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return GovernedCycle{}, err
	}
	return c, nil
}

// checkGovernedCompareReady refuses compares outside
// DECLARED/CANDIDATES, by anyone but the pinned evaluator, without
// a linked plan or without a versioned candidate.
func checkGovernedCompareReady(c GovernedCycle, by string) error {
	if c.State != GovernedDeclared && c.State != GovernedCandidates {
		return fmt.Errorf("evolve: governed cycle %s compares from DECLARED/CANDIDATES, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(by) != c.Roles.Evaluator {
		return fmt.Errorf("evolve: only the pinned evaluator %q compares (got %q: proposer never scores its own change)",
			c.Roles.Evaluator, strings.TrimSpace(by))
	}
	if strings.TrimSpace(c.PlanID) == "" {
		return errors.New("evolve: governed cycle compares through its linked validation plan (link one first)")
	}
	if len(c.Candidates) == 0 {
		return errors.New("evolve: governed cycle needs at least one versioned candidate before comparing")
	}
	return nil
}

// buildGovernedEval folds the five governed dimensions around the
// frozen plan verdict. Candidates were injection-screened at propose
// time; the review probes them again independently, so the security
// line records the posture, not a second gate.
func buildGovernedEval(c GovernedCycle, cmp ValidationComparison, p ValidationPlan, exp []ValidationTrial, holdTrials []ValidationTrial) GovernedEval {
	latency, cost := foldGovernedDims(exp)
	security := "no injection markers in candidate contents; domain holds (non-regression)"
	hold := "holdout: not run (sealed; confirms only when present)"
	if len(holdTrials) > 0 {
		if holdoutContradicts(holdTrials) {
			hold = "holdout CONTRADICTS the exploration verdict: adoption refused"
		} else {
			hold = "holdout does not contradict the exploration verdict (confirmation-only)"
		}
	}
	regressions := "no regression: candidate gap >= 0 on the primary metric"
	return GovernedEval{
		PlanID: p.ID, PlanVerdict: cmp.Verdict,
		GapPassRate: fmt.Sprintf("%.4f descriptive (%s)", cmp.GapPassRate, cmp.Honesty),
		Quality:     fmt.Sprintf("baseline pass-rate %.4f (evaluable %d) vs candidate %.4f (evaluable %d)", cmp.Baseline.PassRate, cmp.Baseline.Evaluable, cmp.Candidate.PassRate, cmp.Candidate.Evaluable),
		Latency:     latency, Cost: cost, Security: security,
		Regressions: regressions, Holdout: hold,
		ComparedAt: time.Now().UTC().Format(time.RFC3339), ExploredTrials: len(exp),
	}
}

// blockGovernedCompareIfNeeded seals the cycle BLOCKED when the
// verdict cannot promote: insufficient evidence, regression/stall
// or a contradicting holdout. It reports handled=true with the
// terminal error; otherwise handled=false.
func blockGovernedCompareIfNeeded(root string, c *GovernedCycle, by string, cmp ValidationComparison, holdTrials []ValidationTrial, ev GovernedEval) (GovernedEval, bool, error) {
	switch {
	case cmp.Verdict == RobustInsufficient:
		governedBlock(c, by, cmp.Verdict, "insufficient evaluable evidence: missing/infra outcomes are never scored as pass", ev.Quality)
		if err := c.saveGovernedCycle(root); err != nil {
			return GovernedEval{}, true, err
		}
		return ev, true, fmt.Errorf("evolve: governed cycle %s BLOCKED: %s", c.ID, "insufficient evaluable evidence")
	case cmp.Verdict == RobustKeepBaseline:
		governedBlock(c, by, cmp.Verdict, "candidate regresses or stalls below MinDelta: non-regression rule keeps the baseline", ev.GapPassRate)
		if err := c.saveGovernedCycle(root); err != nil {
			return GovernedEval{}, true, err
		}
		return ev, true, fmt.Errorf("evolve: governed cycle %s BLOCKED: %s", c.ID, cmp.Reason)
	case holdoutContradicts(holdTrials) && cmp.Verdict == RobustAdoptCandidate:
		governedBlock(c, by, cmp.Verdict, "holdout contradicts the exploration verdict: revisit in a new cycle, do not adopt", ev.Holdout)
		if err := c.saveGovernedCycle(root); err != nil {
			return GovernedEval{}, true, err
		}
		return ev, true, fmt.Errorf("evolve: governed cycle %s BLOCKED: holdout contradicts the exploration verdict", c.ID)
	}
	return ev, false, nil
}

// sealGovernedCompared records the successful comparison and moves
// the cycle to COMPARED.
func sealGovernedCompared(root string, c *GovernedCycle, by string, ev GovernedEval, cmpReason string, expN int) error {
	c.Evaluation = &ev
	from := c.State
	c.State = GovernedCompared
	c.History = append(c.History, ExpHistoryEvent{
		At: ev.ComparedAt, Kind: "compared", From: from, To: GovernedCompared,
		By: by, Note: fmt.Sprintf("plan verdict %s over %d exploration trials; %s", ev.PlanVerdict, expN, cmpReason),
	})
	return c.saveGovernedCycle(root)
}

// CompareGovernedCycle evaluates the cycle over its linked plan: the
// plan verdict decides (frozen by CompareValidation when still
// RUNNING), the five dimensions are folded around it, and anything
// but a promotable verdict blocks with its evidence. Only the pinned
// evaluator compares.
func CompareGovernedCycle(root, cycleID, by string) (GovernedEval, error) {
	c, err := loadVerifiedGovernedCycle(root, cycleID)
	if err != nil {
		return GovernedEval{}, err
	}
	if err := checkGovernedCompareReady(c, by); err != nil {
		return GovernedEval{}, err
	}
	cmp, err := CompareValidation(root, c.PlanID)
	if err != nil {
		return GovernedEval{}, err
	}
	p, err := LoadGovernedPlanForCycle(root, c.PlanID)
	if err != nil {
		return GovernedEval{}, err
	}
	exp := explorationTrials(p)
	holdTrials := holdoutTrials(p)
	ev := buildGovernedEval(c, cmp, p, exp, holdTrials)
	if blockedEv, blocked, err := blockGovernedCompareIfNeeded(root, &c, by, cmp, holdTrials, ev); blocked {
		return blockedEv, err
	}
	if err := sealGovernedCompared(root, &c, by, ev, cmp.Reason, len(exp)); err != nil {
		return GovernedEval{}, err
	}
	return ev, nil
}

// ReviewGovernedCycle records the independent adversarial review. The
// reviewer must stand outside all four pinned roles (independent of
// build, score, approval and deploy alike); a reject blocks the cycle
// terminally with its reason.
func ReviewGovernedCycle(root, cycleID, reviewer, verdict, reason string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedCompared {
		return fmt.Errorf("evolve: governed cycle %s reviews from COMPARED, not %s", c.ID, c.State)
	}
	r := strings.TrimSpace(reviewer)
	if len(r) < 2 {
		return errors.New("evolve: adversarial review needs a named reviewer")
	}
	for _, role := range []string{c.Roles.Proposer, c.Roles.Evaluator, c.Roles.Approver, c.Roles.Deployer} {
		if strings.EqualFold(r, role) {
			return fmt.Errorf("evolve: reviewer %q must stand outside the pinned roles (independent of build, score, approval and deploy)", r)
		}
	}
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "pass":
	case "reject":
		if strings.TrimSpace(reason) == "" {
			return errors.New("evolve: a rejecting review needs its reason (silent rejection is not evidence)")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		c.Review = &GovernedReview{Reviewer: r, Verdict: "reject", Reason: strings.TrimSpace(reason), At: now}
		governedBlock(&c, r, "REVIEW_REJECTED", strings.TrimSpace(reason), "independent adversarial review rejected the candidate")
		return c.saveGovernedCycle(root)
	default:
		return fmt.Errorf("evolve: review verdict %q must be pass or reject", strings.TrimSpace(verdict))
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("evolve: a passing review still states what it probed (reason required)")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.Review = &GovernedReview{Reviewer: r, Verdict: "pass", Reason: strings.TrimSpace(reason), At: now}
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "reviewed", From: c.State, To: GovernedReviewed,
		By: r, Note: "independent adversarial review passed: " + strings.TrimSpace(reason),
	})
	c.State = GovernedReviewed
	return c.saveGovernedCycle(root)
}

// ApproveGovernedCycle records the human promotion approval. By must
// equal the pinned approver — and the pin is pairwise distinct from
// proposer/evaluator/deployer since declare, so no agent approves its
// own critical change. Sensitive cycles bind the approval to a commit
// SHA and a policy reference: exact bytes under an exact policy.
func ApproveGovernedCycle(root, cycleID string, a GovernedApproval) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedReviewed {
		return fmt.Errorf("evolve: governed cycle %s approves from REVIEWED, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(a.By) != c.Roles.Approver {
		return fmt.Errorf("evolve: only the pinned approver %q approves (got %q: no self-approval)",
			c.Roles.Approver, strings.TrimSpace(a.By))
	}
	if !validValidationApprover(a.By) {
		return fmt.Errorf("evolve: governed approver %q must be a human owner (never auto/bot)", strings.TrimSpace(a.By))
	}
	if strings.TrimSpace(a.Reason) == "" {
		return errors.New("evolve: governed approval needs a reason (what risk the human accepts)")
	}
	if c.GovernedSensitive() && (strings.TrimSpace(a.CommitSHA) == "" || strings.TrimSpace(a.PolicyRef) == "") {
		return errors.New("evolve: sensitive change (Core/permissions/policy/evaluator or critical surface) needs --commit <sha> and --policy <ref>: approvals bind to hashes and policies")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a.By = strings.TrimSpace(a.By)
	a.Reason = strings.TrimSpace(a.Reason)
	a.CommitSHA = strings.TrimSpace(a.CommitSHA)
	a.PolicyRef = strings.TrimSpace(a.PolicyRef)
	a.ManifestRef = strings.TrimSpace(a.ManifestRef)
	a.At = now
	c.Approval = &a
	pin := "human approval by " + a.By
	if a.CommitSHA != "" {
		pin += fmt.Sprintf(" bound to commit %s under policy %s", a.CommitSHA, a.PolicyRef)
	}
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "approved", From: c.State, To: GovernedApproved,
		By: a.By, Note: pin + ": " + a.Reason,
	})
	c.State = GovernedApproved
	return c.saveGovernedCycle(root)
}

// EnterGovernedCanary opens the monitored rollout behind the recorded
// approval. The scope obeys the governed canary rule (prompt, skill
// or workflow; reversible; non-critical — reused from promote.go);
// at least one rollback condition is preapproved here, and only a
// preapproved condition can trigger the rollback later.
func EnterGovernedCanary(root, cycleID, by string, scope CanaryScope, conditions []string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedApproved {
		return fmt.Errorf("evolve: governed cycle %s enters canary from APPROVED, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(by) != c.Roles.Deployer {
		return fmt.Errorf("evolve: only the pinned deployer %q runs canary (got %q)", c.Roles.Deployer, strings.TrimSpace(by))
	}
	if err := ValidateCanaryScope(scope); err != nil {
		return err
	}
	var conds []string
	for _, k := range conditions {
		if t := strings.TrimSpace(k); t != "" {
			conds = append(conds, t)
		}
	}
	if len(conds) == 0 {
		return errors.New("evolve: canary needs at least one preapproved rollback condition (rollback without a preapproved trigger is refused)")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.Canary = &GovernedCanaryRun{
		Kind: strings.ToLower(strings.TrimSpace(scope.Kind)), Reversible: scope.Reversible,
		Reason: strings.TrimSpace(scope.Reason), Conditions: conds, At: now,
	}
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "canary-enter", From: c.State, To: GovernedCanary,
		By: by, Note: fmt.Sprintf("canary scope kind=%s reversible=%v; rollback preapproved on: %s",
			c.Canary.Kind, c.Canary.Reversible, strings.Join(conds, "; ")),
	})
	c.State = GovernedCanary
	return c.saveGovernedCycle(root)
}

// RecordGovernedMonitor appends one canary health observation. Only the
// pinned deployer monitors; promotion needs healthy signal and zero
// unhealthy ones, so every observation counts.
func RecordGovernedMonitor(root, cycleID, by string, healthy bool, detail string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedCanary {
		return fmt.Errorf("evolve: governed cycle %s monitors in CANARY, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(by) != c.Roles.Deployer {
		return fmt.Errorf("evolve: only the pinned deployer %q monitors canary (got %q)", c.Roles.Deployer, strings.TrimSpace(by))
	}
	if strings.TrimSpace(detail) == "" {
		return errors.New("evolve: canary monitor needs detail (what was observed)")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.Monitors = append(c.Monitors, GovernedMonitor{Healthy: healthy, Detail: strings.TrimSpace(detail), By: strings.TrimSpace(by), At: now})
	health := "healthy"
	if !healthy {
		health = "UNHEALTHY"
	}
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "monitor", From: c.State, To: c.State,
		By: strings.TrimSpace(by), Note: health + ": " + strings.TrimSpace(detail),
	})
	return c.saveGovernedCycle(root)
}

// PromoteGovernedCycle seals the cycle PROMOTED on the winning
// candidate. The deployer promotes only from a fully healthy canary:
// at least one monitor observation and zero unhealthy ones — any
// unhealthy signal forces the rollback path instead.
func PromoteGovernedCycle(root, cycleID, by, winnerID string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedCanary {
		return fmt.Errorf("evolve: governed cycle %s promotes from CANARY, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(by) != c.Roles.Deployer {
		return fmt.Errorf("evolve: only the pinned deployer %q promotes (got %q)", c.Roles.Deployer, strings.TrimSpace(by))
	}
	found := false
	for _, cand := range c.Candidates {
		if cand.ID == strings.TrimSpace(winnerID) && !cand.Superseded {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("evolve: winner %q is not an active versioned candidate of cycle %s", strings.TrimSpace(winnerID), c.ID)
	}
	if len(c.Monitors) == 0 {
		return errors.New("evolve: promotion needs canary evidence: at least one monitor observation (unmonitored canary never promotes)")
	}
	for _, m := range c.Monitors {
		if !m.Healthy {
			return errors.New("evolve: canary shows unhealthy signal: rollback instead of promote")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.Promotion = &GovernedDecision{Verdict: "PROMOTED", By: strings.TrimSpace(by),
		Reason: fmt.Sprintf("winner %s promoted after %d healthy canary observation(s)", strings.TrimSpace(winnerID), len(c.Monitors)),
		Detail: "approval " + c.Approval.By + " bound to commit " + c.Approval.CommitSHA, At: now}
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "promoted", From: c.State, To: GovernedPromoted,
		By: strings.TrimSpace(by), Note: "promoted " + strings.TrimSpace(winnerID) + " (monitored, approved, reversible)",
	})
	c.State = GovernedPromoted
	return c.saveGovernedCycle(root)
}

// RollbackGovernedCycle returns the cycle to safety. It runs from
// CANARY or PROMOTED, only on a preapproved condition from canary
// entry (no ad-hoc triggers), and supersedes the winning candidate so
// the evidence shows exactly what stopped. The rollback path is real:
// canary-entry refuses empty conditions, and promote refuses an
// unhealthy canary, so this is the only way back.
func RollbackGovernedCycle(root, cycleID, by, reason, condition string) error {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return err
	}
	if c.State != GovernedCanary && c.State != GovernedPromoted {
		return fmt.Errorf("evolve: governed cycle %s rolls back from CANARY or PROMOTED, not %s", c.ID, c.State)
	}
	if strings.TrimSpace(by) != c.Roles.Deployer {
		return fmt.Errorf("evolve: only the pinned deployer %q rolls back (got %q)", c.Roles.Deployer, strings.TrimSpace(by))
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("evolve: governed rollback needs a reason")
	}
	if c.Canary == nil {
		return errors.New("evolve: governed rollback needs its canary record (conditions were preapproved at entry)")
	}
	preapproved := false
	for _, k := range c.Canary.Conditions {
		if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(condition)) {
			preapproved = true
		}
	}
	if !preapproved {
		return fmt.Errorf("evolve: rollback condition %q was not preapproved at canary entry (preapproved: %s)",
			strings.TrimSpace(condition), strings.Join(c.Canary.Conditions, "; "))
	}
	now := time.Now().UTC().Format(time.RFC3339)
	winner := ""
	if c.Promotion != nil {
		// Promotion reason opens with "winner <id> promoted after ...".
		if fields := strings.Fields(c.Promotion.Reason); len(fields) >= 2 && fields[0] == "winner" {
			winner = fields[1]
		}
	}
	for i := range c.Candidates {
		if winner == "" || c.Candidates[i].ID == winner {
			c.Candidates[i].Superseded = true
		}
	}
	c.Rollback = &GovernedDecision{Verdict: "ROLLED_BACK", By: strings.TrimSpace(by),
		Reason: strings.TrimSpace(reason), Detail: "preapproved condition: " + strings.TrimSpace(condition), At: now}
	from := c.State
	c.State = GovernedRolledBack
	c.History = append(c.History, ExpHistoryEvent{
		At: now, Kind: "rolled-back", From: from, To: GovernedRolledBack,
		By: strings.TrimSpace(by), Note: "rollback on preapproved condition " + strings.TrimSpace(condition) + ": " + strings.TrimSpace(reason),
	})
	return c.saveGovernedCycle(root)
}

// RenderGovernedReport prints the reproducible F12.7 report: signals,
// hypothesis, reversible surface, roles, versioned candidates with
// SHAs, the linked plan verdict, the five evaluated dimensions,
// independent review, human approval with its hashes, canary scope
// with preapproved conditions and monitor evidence, and the terminal
// verdict with its next step. Only terminal cycles render: the report
// seals a decision, never a draft.
func RenderGovernedReport(root, cycleID string) (string, error) {
	c, err := LoadGovernedCycle(root, cycleID)
	if err != nil {
		return "", err
	}
	if err := VerifyGovernedCycle(c); err != nil {
		return "", err
	}
	if c.State != GovernedPromoted && c.State != GovernedRolledBack && c.State != GovernedBlocked {
		return "", fmt.Errorf("evolve: governed cycle %s renders its report from a terminal state, not %s", c.ID, c.State)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Governed evolution report %s\n\n", c.ID)
	fmt.Fprintf(&b, "state: %s (declared %s, pin %s)\n", c.State, c.DeclaredAt, c.CycleSHA256)
	fmt.Fprintf(&b, "sensitive: %v (critical=%v core=%v permissions=%v policy=%v evaluator=%v)\n",
		c.GovernedSensitive(), c.Surface.Critical, c.Sensitivity.TouchesCore,
		c.Sensitivity.TouchesPermissions, c.Sensitivity.TouchesPolicy, c.Sensitivity.TouchesEvaluator)
	fmt.Fprintf(&b, "\n## Ingested signals\n\n")
	for _, s := range c.Signals {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", s.Source, s.Ref, s.Summary)
	}
	fmt.Fprintf(&b, "\n## Hypothesis (pre-registered)\n\n")
	fmt.Fprintf(&b, "- problem: %s\n- root cause: %s\n- change: %s\n", c.Hypothesis.Problem, c.Hypothesis.RootCause, c.Hypothesis.Change)
	fmt.Fprintf(&b, "- benefit: %s\n- regressions: %s\n- criteria: %s\n", c.Hypothesis.Benefit, c.Hypothesis.PotentialRegressions, c.Hypothesis.EvaluationCriteria)
	fmt.Fprintf(&b, "\n## Reversible surface\n\n")
	fmt.Fprintf(&b, "- kind: %s, reversible: %v, critical: %v\n- description: %s\n- revert: %s\n",
		c.Surface.Kind, c.Surface.Reversible, c.Surface.Critical, c.Surface.Description, c.Surface.RevertCommand)
	fmt.Fprintf(&b, "\n## Roles (separated, no self-approval)\n\n")
	fmt.Fprintf(&b, "- proposer: %s | evaluator: %s | approver: %s | deployer: %s\n",
		c.Roles.Proposer, c.Roles.Evaluator, c.Roles.Approver, c.Roles.Deployer)
	fmt.Fprintf(&b, "\n## Candidates (K<=%d, versioned)\n\n", c.MaxCandidates)
	if len(c.Candidates) == 0 {
		fmt.Fprintf(&b, "- none\n")
	}
	for _, cand := range c.Candidates {
		fmt.Fprintf(&b, "- %s v%d sha %s by %s at %s superseded=%v\n",
			cand.ID, cand.N, cand.ContentSHA, cand.By, cand.At, cand.Superseded)
	}
	fmt.Fprintf(&b, "\n## Comparison (linked plan decides, holdout confirms)\n\n")
	if c.Evaluation == nil {
		fmt.Fprintf(&b, "- evaluation: none recorded (blocked before comparing)\n")
	} else {
		e := c.Evaluation
		fmt.Fprintf(&b, "- plan: %s verdict %s over %d exploration trials at %s\n", e.PlanID, e.PlanVerdict, e.ExploredTrials, e.ComparedAt)
		fmt.Fprintf(&b, "- gap: %s\n- quality: %s\n- latency: %s\n- cost: %s\n- security: %s\n- regressions: %s\n- %s\n",
			e.GapPassRate, e.Quality, e.Latency, e.Cost, e.Security, e.Regressions, e.Holdout)
	}
	fmt.Fprintf(&b, "\n## Independent adversarial review\n\n")
	if c.Review == nil {
		fmt.Fprintf(&b, "- none recorded\n")
	} else {
		fmt.Fprintf(&b, "- reviewer %s (outside all pinned roles): %s at %s — %s\n",
			c.Review.Reviewer, c.Review.Verdict, c.Review.At, c.Review.Reason)
	}
	fmt.Fprintf(&b, "\n## Human approval (bound to hashes/policies)\n\n")
	if c.Approval == nil {
		fmt.Fprintf(&b, "- none recorded\n")
	} else {
		fmt.Fprintf(&b, "- approved by %s at %s: %s\n", c.Approval.By, c.Approval.At, c.Approval.Reason)
		fmt.Fprintf(&b, "- commit: %s | policy: %s | manifest: %s\n", c.Approval.CommitSHA, c.Approval.PolicyRef, c.Approval.ManifestRef)
	}
	fmt.Fprintf(&b, "\n## Canary and monitoring\n\n")
	if c.Canary == nil {
		fmt.Fprintf(&b, "- canary: not entered\n")
	} else {
		fmt.Fprintf(&b, "- scope kind=%s reversible=%v at %s: %s\n", c.Canary.Kind, c.Canary.Reversible, c.Canary.At, c.Canary.Reason)
		fmt.Fprintf(&b, "- preapproved rollback conditions: %s\n", strings.Join(c.Canary.Conditions, "; "))
	}
	if len(c.Monitors) == 0 {
		fmt.Fprintf(&b, "- monitors: none\n")
	}
	for _, m := range c.Monitors {
		fmt.Fprintf(&b, "- monitor by %s at %s healthy=%v: %s\n", m.By, m.At, m.Healthy, m.Detail)
	}
	fmt.Fprintf(&b, "\n## Terminal\n\n")
	switch c.State {
	case GovernedPromoted:
		fmt.Fprintf(&b, "- PROMOTED by %s at %s: %s\n", c.Promotion.By, c.Promotion.At, c.Promotion.Reason)
		fmt.Fprintf(&b, "next: regulus evolve validation cycle-rollback --cycle %s --by <deployer> --reason <text> --condition <preapproved> on drift\n", c.ID)
	case GovernedRolledBack:
		fmt.Fprintf(&b, "- ROLLED_BACK by %s at %s: %s (%s)\n", c.Rollback.By, c.Rollback.At, c.Rollback.Reason, c.Rollback.Detail)
		fmt.Fprintf(&b, "next: propose a new cycle when evidence supports it; the superseded candidate stays pinned, never deleted\n")
	default:
		fmt.Fprintf(&b, "- BLOCKED by %s at %s: %s %s\n", c.Blocked.By, c.Blocked.At, c.Blocked.Verdict, c.Blocked.Reason)
		fmt.Fprintf(&b, "next: a blocked cycle never promotes; a new question needs a new cycle\n")
	}
	return b.String(), nil
}
