// Approval lifecycle (F13.6/NOV-109): requests bound to exactly
// what was approved, with expiry, separation of duties and an
// anti-self-approval rule no profile can waive.
//
// Binding contract (checked by Check, enforced by the edge before
// any side effect):
//   - hashes: the approval binds BoundArtifact (which must equal
//     the requested DiffHash when one was declared); any material
//     change — different bytes — invalidates it.
//   - scope: approved scopes are a ceiling; a widened scope
//     invalidates, a narrowed one stays valid.
//   - policy: the approval pins policy name+version; a rule change
//     invalidates prior approvals (nobody inherits trust across a
//     matrix edit).
//   - identity + date + expiry: approver, decided-at and expires-at
//     travel with the approval; expired approvals never validate.
//   - separation: proposer, reviewer, approver and executor are four
//     distinct identities; the approver is human and never one of
//     the other three. No agent approves its own critical change,
//     and automated actors cannot change the rules (see
//     store SavePolicy).
//
// This file is pure except for ID minting (crypto/rand): no disk,
// no clock injection beyond the `now` the caller passes, so every
// transition is unit-testable with fixed times.
package govern

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Approval states. REQUESTED opens; PAUSED freezes without deciding;
// APPROVED authorizes exactly the bound artifact; REJECTED and
// REVOKED are terminal refusals. Expiry is computed (see Expired),
// not a state: an expired APPROVED simply stops validating.
const (
	ApprovalRequested = "REQUESTED"
	ApprovalPaused    = "PAUSED"
	ApprovalApproved  = "APPROVED"
	ApprovalRejected  = "REJECTED"
	ApprovalRevoked   = "REVOKED"
)

// DefaultApprovalTTL bounds every approval: authorizations rot by
// default after 24h. MaxApprovalTTL caps explicit extensions at 7d;
// longer trust needs a fresh request, not a longer lease.
const (
	DefaultApprovalTTL = 24 * time.Hour
	MaxApprovalTTL     = 7 * 24 * time.Hour
)

// ApprovalEvent is one inline transition for reconstruction: the
// append-only audit log (store.go) is the authority, this history
// is the portable quote.
type ApprovalEvent struct {
	At     string `json:"at"`
	By     string `json:"by"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

// ApprovalRequest is the sealed authorization: what was asked, under
// which policy, by whom, and — once decided — exactly what the
// human approved, when, and until when.
type ApprovalRequest struct {
	ID            string          `json:"id"`
	MissionID     string          `json:"mission_id,omitempty"`
	Kind          string          `json:"kind"`
	Surface       string          `json:"surface"`
	Reversible    bool            `json:"reversible"`
	Summary       string          `json:"summary"`
	DiffHash      string          `json:"diff_hash,omitempty"`
	PlanVersion   string          `json:"plan_version"`
	Risks         []string        `json:"risks"`
	Tests         []string        `json:"tests,omitempty"`
	Rollback      string          `json:"rollback"`
	Cost          string          `json:"cost"`
	Scope         []string        `json:"scope"`
	PolicyName    string          `json:"policy_name"`
	PolicyVersion string          `json:"policy_version"`
	Proposer      string          `json:"proposer"`
	Reviewer      string          `json:"reviewer"`
	Executor      string          `json:"executor"`
	Approver      string          `json:"approver,omitempty"`
	State         string          `json:"state"`
	CreatedAt     string          `json:"created_at"`
	ExpiresAt     string          `json:"expires_at"`
	DecidedAt     string          `json:"decided_at,omitempty"`
	DecidedBy     string          `json:"decided_by,omitempty"`
	DecideReason  string          `json:"decide_reason,omitempty"`
	BoundArtifact string          `json:"bound_artifact,omitempty"`
	History       []ApprovalEvent `json:"history"`
}

// ApprovalSpec is the request input: everything the spec names
// (summary/diff-hash/plan/risks/tests/rollback/cost/scope) plus the
// three build-side roles. The approver is never part of the spec:
// naming your own approver is the first self-approval smell.
type ApprovalSpec struct {
	MissionID   string
	Kind        string
	Surface     string
	Reversible  bool
	Summary     string
	DiffHash    string
	PlanVersion string
	Risks       []string
	Tests       []string
	Rollback    string
	Cost        string
	Scope       []string
	PolicyName  string
	Proposer    string
	Reviewer    string
	Executor    string
	TTLHours    float64
}

// IsHuman reports whether s names a human owner. Mirrors the
// evolve validation gate and the cli adopt gate (automated
// approvers never own a decision): kept as an explicit mirror so
// govern never imports evolve for one predicate.
func IsHuman(s string) bool {
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

// bypassMarkers are approval-skip signals: prompt injection trying
// to route around the human, or free text claiming its own
// authority. Any hit refuses the request/approval at the door.
var bypassMarkers = []string{
	"skip approval", "without approval", "auto-approve", "auto approve",
	"approve this", "pre-approved", "ignore policy", "bypass approval",
	"override", "ignore previous instructions", "system:",
	"jailbreak", "self-grade", "exfiltrate",
}

// ContainsBypass reports whether text carries an approval-bypass or
// injection signal. Flagged content is refused, never screened and
// signed.
func ContainsBypass(text string) bool {
	lower := strings.ToLower(text)
	for _, m := range bypassMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func singleLine(name, v string, required bool) error {
	if strings.TrimSpace(v) == "" {
		if required {
			return fmt.Errorf("govern: approval needs %s", name)
		}
		return nil
	}
	if strings.ContainsAny(v, "\n\r") {
		return fmt.Errorf("govern: approval %s must be a single line", name)
	}
	return nil
}

func namedRole(duty, v string) (string, error) {
	t := strings.TrimSpace(v)
	if len(t) < 2 {
		return "", fmt.Errorf("govern: approval %s must be a named identity", duty)
	}
	return t, nil
}

// mintApprovalID mints a unique request id: apr_ + 8 hex chars.
func mintApprovalID() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("govern: cannot mint approval id: %w", err)
	}
	return "apr_" + hex.EncodeToString(b[:]), nil
}

// RequestApproval seals one approval request. Validates the closed
// vocabularies, the three build-side roles (pairwise distinct),
// the required evidence fields (tests + diff hash mandatory from
// high risk up), the bypass screen and the TTL cap.
func RequestApproval(spec ApprovalSpec, pol Policy, now time.Time) (ApprovalRequest, error) {
	if pol.resolved.Name == "" {
		return ApprovalRequest{}, errors.New("govern: refusing request under an unresolved policy (unknown profile denies by default)")
	}
	kind, err := ParseOpKind(spec.Kind)
	if err != nil {
		return ApprovalRequest{}, err
	}
	surface, err := ParseSurface(spec.Surface)
	if err != nil {
		return ApprovalRequest{}, err
	}
	op := Operation{Kind: kind, Surface: surface, Reversible: spec.Reversible, Scopes: spec.Scope}
	level, _, err := ClassifyRisk(op)
	if err != nil {
		return ApprovalRequest{}, err
	}
	if err := singleLine("summary", spec.Summary, true); err != nil {
		return ApprovalRequest{}, err
	}
	if ContainsBypass(spec.Summary) {
		return ApprovalRequest{}, errors.New("govern: request summary carries an approval-bypass signal: refused, never signed")
	}
	if err := singleLine("plan version", spec.PlanVersion, true); err != nil {
		return ApprovalRequest{}, err
	}
	if len(spec.Risks) == 0 {
		return ApprovalRequest{}, errors.New("govern: approval needs explicit risks (what the human accepts)")
	}
	for _, r := range spec.Risks {
		if err := singleLine("risk", r, true); err != nil {
			return ApprovalRequest{}, err
		}
		if ContainsBypass(r) {
			return ApprovalRequest{}, errors.New("govern: request risk carries an approval-bypass signal: refused, never signed")
		}
	}
	if err := singleLine("rollback", spec.Rollback, true); err != nil {
		return ApprovalRequest{}, err
	}
	if ContainsBypass(spec.Rollback) {
		return ApprovalRequest{}, errors.New("govern: request rollback carries an approval-bypass signal: refused, never signed")
	}
	if err := singleLine("cost", spec.Cost, true); err != nil {
		return ApprovalRequest{}, err
	}
	if len(spec.Scope) == 0 {
		return ApprovalRequest{}, errors.New("govern: approval needs an explicit scope (files/components the authorization covers)")
	}
	for _, s := range spec.Scope {
		if err := singleLine("scope entry", s, true); err != nil {
			return ApprovalRequest{}, err
		}
	}
	if riskRank(level) >= riskRank("high") {
		if strings.TrimSpace(spec.DiffHash) == "" {
			return ApprovalRequest{}, fmt.Errorf("govern: risk %s needs a diff hash: approvals bind to exact bytes", level)
		}
		if len(spec.Tests) == 0 {
			return ApprovalRequest{}, fmt.Errorf("govern: risk %s needs explicit tests: what gates the human trusts", level)
		}
	}
	for _, t := range spec.Tests {
		if err := singleLine("test", t, true); err != nil {
			return ApprovalRequest{}, err
		}
	}
	proposer, err := namedRole("proposer", spec.Proposer)
	if err != nil {
		return ApprovalRequest{}, err
	}
	reviewer, err := namedRole("reviewer", spec.Reviewer)
	if err != nil {
		return ApprovalRequest{}, err
	}
	executor, err := namedRole("executor", spec.Executor)
	if err != nil {
		return ApprovalRequest{}, err
	}
	seen := map[string]string{}
	for _, duty := range []struct{ name, value string }{
		{"proposer", proposer}, {"reviewer", reviewer}, {"executor", executor},
	} {
		lower := strings.ToLower(duty.value)
		if other, dup := seen[lower]; dup {
			return ApprovalRequest{}, fmt.Errorf("govern: proposer, reviewer and executor must be distinct (got %q as %s and %s: no self-governance)", duty.value, other, duty.name)
		}
		seen[lower] = duty.name
	}
	ttl := time.Duration(spec.TTLHours * float64(time.Hour))
	if spec.TTLHours <= 0 {
		ttl = DefaultApprovalTTL
	}
	if ttl > MaxApprovalTTL {
		return ApprovalRequest{}, fmt.Errorf("govern: approval TTL %v exceeds the %v cap: re-request instead of leasing longer trust", ttl, MaxApprovalTTL)
	}
	id, err := mintApprovalID()
	if err != nil {
		return ApprovalRequest{}, err
	}
	now = now.UTC()
	req := ApprovalRequest{
		ID: id, MissionID: strings.TrimSpace(spec.MissionID),
		Kind: kind, Surface: surface, Reversible: spec.Reversible,
		Summary: strings.TrimSpace(spec.Summary), DiffHash: strings.TrimSpace(spec.DiffHash),
		PlanVersion: strings.TrimSpace(spec.PlanVersion),
		Risks:       append([]string(nil), spec.Risks...), Tests: append([]string(nil), spec.Tests...),
		Rollback: strings.TrimSpace(spec.Rollback), Cost: strings.TrimSpace(spec.Cost),
		Scope:      append([]string(nil), spec.Scope...),
		PolicyName: pol.Name, PolicyVersion: pol.Version,
		Proposer: proposer, Reviewer: reviewer, Executor: executor,
		State:     ApprovalRequested,
		CreatedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(ttl).Format(time.RFC3339),
		History: []ApprovalEvent{{At: now.Format(time.RFC3339), By: proposer, Action: "requested",
			Note: fmt.Sprintf("risk %s under %s@%s", level, pol.Name, pol.Version)}},
	}
	return req, nil
}

// Approve records the human decision. The approver must be human
// and stand outside proposer/reviewer/executor (four distinct
// duties — no self-approval by construction); the bound artifact
// must equal the requested diff when one was declared
// (bait-and-switch refused); the reason is screened like the
// request. Paused requests resume first; decided ones never
// re-decide (a replay returns the stored record, it does not mint
// new trust).
func Approve(req ApprovalRequest, approver, artifactSHA, reason string, now time.Time) (ApprovalRequest, error) {
	if req.State != ApprovalRequested {
		return ApprovalRequest{}, fmt.Errorf("govern: approval %s is %s, not REQUESTED (resume a paused one; decided ones never re-decide)", req.ID, req.State)
	}
	a := strings.TrimSpace(approver)
	if !IsHuman(a) {
		return ApprovalRequest{}, fmt.Errorf("govern: approver %q must be a human owner (never auto/bot)", approver)
	}
	for _, other := range []struct{ duty, value string }{
		{"proposer", req.Proposer}, {"reviewer", req.Reviewer}, {"executor", req.Executor},
	} {
		if strings.EqualFold(a, other.value) {
			return ApprovalRequest{}, fmt.Errorf("govern: approver %q is already the %s: no self-approval (four distinct duties)", a, other.duty)
		}
	}
	if err := singleLine("approval reason", reason, true); err != nil {
		return ApprovalRequest{}, err
	}
	if ContainsBypass(reason) {
		return ApprovalRequest{}, errors.New("govern: approval reason carries a bypass signal: refused, never signed")
	}
	art := strings.TrimSpace(artifactSHA)
	if req.DiffHash != "" && art != req.DiffHash {
		return ApprovalRequest{}, fmt.Errorf("govern: bound artifact %q differs from the approved diff %q: material change invalidates, re-request", shortHash(art), shortHash(req.DiffHash))
	}
	if art == "" {
		return ApprovalRequest{}, errors.New("govern: approval binds to an artifact hash (what bytes the human signs)")
	}
	now = now.UTC()
	req.Approver = a
	req.State = ApprovalApproved
	req.DecidedAt = now.Format(time.RFC3339)
	req.DecidedBy = a
	req.DecideReason = strings.TrimSpace(reason)
	req.BoundArtifact = art
	req.History = append(req.History, ApprovalEvent{At: now.Format(time.RFC3339), By: a, Action: "approved",
		Note: fmt.Sprintf("bound to %s under %s@%s until %s", shortHash(art), req.PolicyName, req.PolicyVersion, req.ExpiresAt)})
	return req, nil
}

// Pause freezes a request without deciding it; only the proposer
// or reviewer pauses (the approver never pre-touches a request,
// the executor never freezes what it will run).
func Pause(req ApprovalRequest, by, reason string, now time.Time) (ApprovalRequest, error) {
	if req.State != ApprovalRequested {
		return ApprovalRequest{}, fmt.Errorf("govern: approval %s is %s, not REQUESTED: only open requests pause", req.ID, req.State)
	}
	b := strings.TrimSpace(by)
	if !strings.EqualFold(b, req.Proposer) && !strings.EqualFold(b, req.Reviewer) {
		return ApprovalRequest{}, fmt.Errorf("govern: only the proposer or reviewer pauses %s (got %q)", req.ID, by)
	}
	if err := singleLine("pause reason", reason, true); err != nil {
		return ApprovalRequest{}, err
	}
	now = now.UTC()
	req.State = ApprovalPaused
	req.History = append(req.History, ApprovalEvent{At: now.Format(time.RFC3339), By: b, Action: "paused", Note: strings.TrimSpace(reason)})
	return req, nil
}

// Resume reopens a paused request; only the pauser or the reviewer
// resumes, so a freeze cannot be silently lifted by the executor.
func Resume(req ApprovalRequest, by, reason string, now time.Time) (ApprovalRequest, error) {
	if req.State != ApprovalPaused {
		return ApprovalRequest{}, fmt.Errorf("govern: approval %s is %s, not PAUSED: nothing to resume", req.ID, req.State)
	}
	b := strings.TrimSpace(by)
	if !strings.EqualFold(b, req.Proposer) && !strings.EqualFold(b, req.Reviewer) {
		return ApprovalRequest{}, fmt.Errorf("govern: only the proposer or reviewer resumes %s (got %q)", req.ID, by)
	}
	if err := singleLine("resume reason", reason, true); err != nil {
		return ApprovalRequest{}, err
	}
	now = now.UTC()
	req.State = ApprovalRequested
	req.History = append(req.History, ApprovalEvent{At: now.Format(time.RFC3339), By: b, Action: "resumed", Note: strings.TrimSpace(reason)})
	return req, nil
}

// Reject terminally refuses a request with its reason; only the
// pinned reviewer or the future approver rejects (the executor
// never judges what it would run, the proposer never buries its
// own request silently — a reject states why).
func Reject(req ApprovalRequest, by, reason string, now time.Time) (ApprovalRequest, error) {
	if req.State != ApprovalRequested && req.State != ApprovalPaused {
		return ApprovalRequest{}, fmt.Errorf("govern: approval %s is %s: decided approvals never re-decide", req.ID, req.State)
	}
	b := strings.TrimSpace(by)
	if !IsHuman(b) {
		return ApprovalRequest{}, fmt.Errorf("govern: rejector %q must be a human owner", by)
	}
	if strings.EqualFold(b, req.Proposer) || strings.EqualFold(b, req.Executor) {
		return ApprovalRequest{}, fmt.Errorf("govern: %q cannot reject %s (proposer/executor never judge: reviewer or approver decides)", b, req.ID)
	}
	if err := singleLine("reject reason", reason, true); err != nil {
		return ApprovalRequest{}, err
	}
	now = now.UTC()
	req.State = ApprovalRejected
	req.DecidedAt = now.Format(time.RFC3339)
	req.DecidedBy = b
	req.DecideReason = strings.TrimSpace(reason)
	req.History = append(req.History, ApprovalEvent{At: now.Format(time.RFC3339), By: b, Action: "rejected", Note: strings.TrimSpace(reason)})
	return req, nil
}

// Revoke stops a live authorization: from REQUESTED/PAUSED (never
// asked again) or APPROVED (pending actions stop — the edge checks
// Check before every side effect, so revocation bites
// immediately). The proposer, reviewer, approver or a human
// operator revokes with a reason; the executor never un-signs
// what authorizes it.
func Revoke(req ApprovalRequest, by, reason string, now time.Time) (ApprovalRequest, error) {
	if req.State != ApprovalRequested && req.State != ApprovalPaused && req.State != ApprovalApproved {
		return ApprovalRequest{}, fmt.Errorf("govern: approval %s is %s: terminal, nothing to revoke", req.ID, req.State)
	}
	b := strings.TrimSpace(by)
	allowed := strings.EqualFold(b, req.Proposer) || strings.EqualFold(b, req.Reviewer) ||
		strings.EqualFold(b, req.Approver) || IsHuman(b)
	if !allowed {
		return ApprovalRequest{}, fmt.Errorf("govern: %q cannot revoke %s (proposer, reviewer, approver or a human operator)", b, req.ID)
	}
	if err := singleLine("revoke reason", reason, true); err != nil {
		return ApprovalRequest{}, err
	}
	now = now.UTC()
	req.State = ApprovalRevoked
	req.DecidedAt = now.Format(time.RFC3339)
	req.DecidedBy = b
	req.DecideReason = strings.TrimSpace(reason)
	req.History = append(req.History, ApprovalEvent{At: now.Format(time.RFC3339), By: b, Action: "revoked",
		Note: "pending actions stop: " + strings.TrimSpace(reason)})
	return req, nil
}

// Expired reports whether the approval lease ran out. Expiry is
// computed against `now` so a stored APPROVED visibly stops
// validating without a writer racing the clock.
func Expired(req ApprovalRequest, now time.Time) bool {
	exp, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		return true
	}
	return !now.UTC().Before(exp)
}

// Check validates a live authorization before a side effect: state,
// expiry, artifact binding, scope ceiling and policy binding. Any
// material change — bytes, widened scope, edited rules, lapsed
// lease — invalidates, never warns-and-continues.
func Check(req ApprovalRequest, artifactSHA string, scopes []string, policyVersion string, now time.Time) error {
	if req.State != ApprovalApproved {
		return fmt.Errorf("govern: approval %s is %s, not APPROVED: pending actions stop", req.ID, req.State)
	}
	if Expired(req, now) {
		return fmt.Errorf("govern: approval %s expired at %s: re-request, leases never extend themselves", req.ID, req.ExpiresAt)
	}
	// Exact bytes: Check is the enforcement point, so it canonicalizes
	// nothing. Input edges (RequestApproval/Approve) already trimmed
	// their values; any difference here is a material change.
	if artifactSHA != req.BoundArtifact {
		return fmt.Errorf("govern: artifact %q is not the approved %q: material change invalidates", shortHash(artifactSHA), shortHash(req.BoundArtifact))
	}
	ceiling := map[string]bool{}
	for _, s := range req.Scope {
		ceiling[strings.TrimSpace(s)] = true
	}
	for _, s := range scopes {
		if t := strings.TrimSpace(s); t != "" && !ceiling[t] {
			return fmt.Errorf("govern: scope %q exceeds the approved ceiling (%s): widened scope invalidates", t, strings.Join(req.Scope, ", "))
		}
	}
	if pv := strings.TrimSpace(policyVersion); pv != "" && pv != req.PolicyVersion {
		return fmt.Errorf("govern: policy %s supersedes the approved %s: rules changed, re-approve under the new matrix", pv, req.PolicyVersion)
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	if h == "" {
		return "unknown"
	}
	return h
}
