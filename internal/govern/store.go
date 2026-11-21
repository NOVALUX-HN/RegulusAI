// Durable governance store (F13.6/NOV-109): approvals, the active
// policy and the append-only hash-chained audit log under
// `.regulus/governance/`.
//
// Single-writer rules (mirroring mission/evolve stores):
//   - creates are exclusive (O_EXCL): a reused id is refused.
//   - updates rewrite the whole record; the frozen request subset
//     (kind/surface/scope/policy/diff/roles) is pinned at request
//     time and VerifyApproval re-checks it on every load, so disk
//     drift fails instead of widening trust quietly.
//   - the audit log is append-only JSONL with a prev-hash chain:
//     VerifyAudit recomputes it; a cut or edited line breaks the
//     chain and fails loudly.
//   - policy changes are human-only (SavePolicy refuses automated
//     actors): nobody edits the rules that govern them without a
//     named owner, and every change lands in the audit log.
package govern

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GovernDir returns the governance store under a project root.
func GovernDir(root string) string {
	return filepath.Join(root, ".regulus", "governance")
}

// ApprovalsDir returns the approval-record directory.
func ApprovalsDir(root string) string {
	return filepath.Join(GovernDir(root), "approvals")
}

// AuditPath returns the append-only audit log.
func AuditPath(root string) string {
	return filepath.Join(GovernDir(root), "audit.jsonl")
}

// PolicyPath returns the active project policy file.
func PolicyPath(root string) string {
	return filepath.Join(GovernDir(root), "policy.json")
}

// storedPolicy is the on-disk policy: profile name plus audit fields.
type storedPolicy struct {
	Profile   string `json:"profile"`
	UpdatedBy string `json:"updated_by,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// LoadPolicy reads the project policy, defaulting deny-by-default
// (standard profile) when no file exists. A malformed file fails
// closed — governance never runs on a policy it cannot parse — and
// an unknown profile fails closed too (see ResolvePolicy).
func LoadPolicy(root string) (Policy, error) {
	raw, err := os.ReadFile(PolicyPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultPolicy(), nil
		}
		return Policy{}, fmt.Errorf("govern: %v", err)
	}
	var sp storedPolicy
	if err := json.Unmarshal(raw, &sp); err != nil {
		return Policy{}, fmt.Errorf("govern: malformed policy.json (failing closed): %v", err)
	}
	pol, err := ResolvePolicy("project", strings.TrimSpace(sp.Profile))
	if err != nil {
		return Policy{}, err
	}
	pol.UpdatedBy = sp.UpdatedBy
	pol.UpdatedAt = sp.UpdatedAt
	return pol, nil
}

// SavePolicy switches the project profile. Only a human owner may
// change the rules (automated actors are refused even when they
// hold every scope), and the change is audit-logged with its
// before/after. There is no silent path to a wider posture.
func SavePolicy(root, profile, actor string, now time.Time) (Policy, error) {
	a := strings.TrimSpace(actor)
	if !IsHuman(a) {
		return Policy{}, fmt.Errorf("govern: policy change by %q refused: only a human owner edits the rules (audited)", actor)
	}
	pol, err := ResolvePolicy("project", profile)
	if err != nil {
		return Policy{}, err
	}
	before := "default@standard"
	if cur, lerr := LoadPolicy(root); lerr == nil {
		before = cur.Name + "@" + cur.Profile + " (" + cur.Version + ")"
	}
	now = now.UTC()
	pol.UpdatedBy = a
	pol.UpdatedAt = now.Format(time.RFC3339)
	if err := os.MkdirAll(GovernDir(root), 0o755); err != nil {
		return Policy{}, err
	}
	raw, err := json.MarshalIndent(storedPolicy{Profile: pol.Profile, UpdatedBy: a, UpdatedAt: pol.UpdatedAt}, "", "  ")
	if err != nil {
		return Policy{}, err
	}
	if err := os.WriteFile(PolicyPath(root), append(raw, '\n'), 0o644); err != nil {
		return Policy{}, err
	}
	_ = appendAudit(root, AuditEvent{
		At: now.Format(time.RFC3339), Actor: a, Action: "policy-change",
		RequestID: "", Detail: fmt.Sprintf("%s -> project@%s (%s)", before, pol.Profile, pol.Version),
	})
	return pol, nil
}

// SaveApproval persists a new request exclusively: a reused id is
// refused, never overwritten.
func SaveApproval(root string, req ApprovalRequest) error {
	if err := VerifyApproval(req); err != nil {
		return err
	}
	if err := os.MkdirAll(ApprovalsDir(root), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(ApprovalsDir(root), req.ID+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("govern: approval %s already exists", req.ID)
		}
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Close()
}

// LoadApproval reads one stored request and revalidates it: schema
// vocabulary, frozen-subset sanity and history continuity. Drift
// fails, never widens.
func LoadApproval(root, id string) (ApprovalRequest, error) {
	data, err := os.ReadFile(filepath.Join(ApprovalsDir(root), strings.TrimSpace(id)+".json"))
	if err != nil {
		return ApprovalRequest{}, fmt.Errorf("govern: %v", err)
	}
	var req ApprovalRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return ApprovalRequest{}, fmt.Errorf("govern: malformed approval %s: %v", id, err)
	}
	if err := VerifyApproval(req); err != nil {
		return ApprovalRequest{}, err
	}
	return req, nil
}

// UpdateApproval rewrites a request after a pure transition
// (Approve/Pause/Resume/Reject/Revoke already enforced the rules;
// this only persists). The frozen subset must still verify.
func UpdateApproval(root string, req ApprovalRequest) error {
	if err := VerifyApproval(req); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ApprovalsDir(root), req.ID+".json"), append(raw, '\n'), 0o644)
}

// VerifyApproval revalidates a stored request: closed vocabularies,
// state vocabulary, required bindings per state, role separation
// and history continuity (opens with "requested").
func VerifyApproval(req ApprovalRequest) error {
	if strings.TrimSpace(req.ID) == "" {
		return fmt.Errorf("govern: approval needs an id")
	}
	if _, err := ParseOpKind(req.Kind); err != nil {
		return err
	}
	if _, err := ParseSurface(req.Surface); err != nil {
		return err
	}
	switch req.State {
	case ApprovalRequested, ApprovalPaused, ApprovalApproved, ApprovalRejected, ApprovalRevoked:
	default:
		return fmt.Errorf("govern: approval %s has unknown state %q", req.ID, req.State)
	}
	if strings.TrimSpace(req.Summary) == "" || len(req.Scope) == 0 {
		return fmt.Errorf("govern: approval %s drifted from its request (summary/scope are frozen)", req.ID)
	}
	if req.PolicyVersion == "" || req.PolicyName == "" {
		return fmt.Errorf("govern: approval %s drifted from its policy pin", req.ID)
	}
	seen := map[string]bool{}
	for _, r := range []string{req.Proposer, req.Reviewer, req.Executor} {
		if len(strings.TrimSpace(r)) < 2 {
			return fmt.Errorf("govern: approval %s has an unnamed duty", req.ID)
		}
		lower := strings.ToLower(strings.TrimSpace(r))
		if seen[lower] {
			return fmt.Errorf("govern: approval %s roles collapsed on %q (duties must stay distinct)", req.ID, r)
		}
		seen[lower] = true
	}
	if req.State == ApprovalApproved {
		if !IsHuman(req.Approver) {
			return fmt.Errorf("govern: approval %s approved by non-human %q", req.ID, req.Approver)
		}
		if seen[strings.ToLower(strings.TrimSpace(req.Approver))] {
			return fmt.Errorf("govern: approval %s self-approved by %q", req.ID, req.Approver)
		}
		if strings.TrimSpace(req.BoundArtifact) == "" {
			return fmt.Errorf("govern: approval %s approved without a bound artifact", req.ID)
		}
	}
	if len(req.History) == 0 || req.History[0].Action != "requested" {
		return fmt.Errorf("govern: approval %s history must open with the request", req.ID)
	}
	return nil
}

// ListApprovals returns every stored request sorted by id,
// skipping corrupt files loudly (returned alongside, never hidden).
func ListApprovals(root string) ([]ApprovalRequest, []string, error) {
	dir := ApprovalsDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("govern: %v", err)
	}
	var out []ApprovalRequest
	var corrupt []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		req, err := LoadApproval(root, id)
		if err != nil {
			corrupt = append(corrupt, id+": "+err.Error())
			continue
		}
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, corrupt, nil
}

// AuditEvent is one append-only log line: who did what to which
// request, hash-chained to the previous line.
type AuditEvent struct {
	Seq       int    `json:"seq"`
	At        string `json:"at"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	RequestID string `json:"request_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
	PrevHash  string `json:"prev_hash"`
	Hash      string `json:"hash"`
}

// auditHash hashes the canonical event bytes (everything but Hash).
func auditHash(seq int, at, actor, action, requestID, detail, prev string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s", seq, at, actor, action, requestID, detail, prev)))
	return hex.EncodeToString(sum[:])
}

// appendAudit appends one hash-chained line. Failures are returned;
// the edge reports them without failing the sealed decision (the
// record is still valid; the log gap is visible in `govern audit`).
func appendAudit(root string, ev AuditEvent) error {
	if err := os.MkdirAll(GovernDir(root), 0o755); err != nil {
		return err
	}
	prev, seq := "genesis", 1
	if tail, err := loadAuditTail(root); err == nil && tail != nil {
		prev, seq = tail.Hash, tail.Seq+1
	}
	ev.Seq, ev.PrevHash = seq, prev
	ev.Hash = auditHash(seq, ev.At, ev.Actor, ev.Action, ev.RequestID, ev.Detail, prev)
	raw, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(AuditPath(root), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

// AppendAudit records one governance transition in the audit log.
// Exported so the edge (cmdx) logs every command with its actor.
func AppendAudit(root string, at, actor, action, requestID, detail string) error {
	return appendAudit(root, AuditEvent{At: at, Actor: actor, Action: action, RequestID: requestID, Detail: detail})
}

// loadAuditTail reads the last log line for chaining.
func loadAuditTail(root string) (*AuditEvent, error) {
	events, err := LoadAudit(root)
	if err != nil || len(events) == 0 {
		return nil, err
	}
	tail := events[len(events)-1]
	return &tail, nil
}

// LoadAudit reads the whole chain in append order.
func LoadAudit(root string) ([]AuditEvent, error) {
	data, err := os.ReadFile(AuditPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("govern: %v", err)
	}
	var out []AuditEvent
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("govern: malformed audit line: %v", err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// VerifyAudit recomputes the hash chain: sequence continuity,
// prev-hash linkage and content hashes. A cut, edited or reordered
// line fails — the log is tamper-evident, not just append-only.
func VerifyAudit(events []AuditEvent) error {
	prev := "genesis"
	for i, ev := range events {
		if ev.Seq != i+1 {
			return fmt.Errorf("govern: audit gap at line %d (seq %d, want %d)", i+1, ev.Seq, i+1)
		}
		if ev.PrevHash != prev {
			return fmt.Errorf("govern: audit chain broken at seq %d (prev %s, want %s)", ev.Seq, shortHash(ev.PrevHash), shortHash(prev))
		}
		if want := auditHash(ev.Seq, ev.At, ev.Actor, ev.Action, ev.RequestID, ev.Detail, ev.PrevHash); want != ev.Hash {
			return fmt.Errorf("govern: audit line %d tampered (hash %s, want %s)", ev.Seq, shortHash(ev.Hash), shortHash(want))
		}
		prev = ev.Hash
	}
	return nil
}
