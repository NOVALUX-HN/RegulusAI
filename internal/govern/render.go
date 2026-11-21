// Human-readable renders (F13.6/NOV-109): show/list/audit text.
// Pure formatting over sealed records; the records stay the
// authority, these lines are the quote.
package govern

import (
	"fmt"
	"strings"
)

// RenderShow quotes one sealed request: what was asked, under which
// policy, who owns each duty, and — once decided — exactly what the
// human bound, when, and until when.
func RenderShow(req ApprovalRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "approval %s state=%s\n", req.ID, req.State)
	fmt.Fprintf(&b, "  op: %s %s reversible=%v (mission %s)\n", req.Kind, req.Surface, req.Reversible, req.MissionID)
	fmt.Fprintf(&b, "  summary: %s\n", req.Summary)
	fmt.Fprintf(&b, "  diff: %s | plan: %s\n", req.DiffHash, req.PlanVersion)
	fmt.Fprintf(&b, "  risks: %s\n", strings.Join(req.Risks, "; "))
	if len(req.Tests) > 0 {
		fmt.Fprintf(&b, "  tests: %s\n", strings.Join(req.Tests, "; "))
	}
	fmt.Fprintf(&b, "  rollback: %s | cost: %s\n", req.Rollback, req.Cost)
	fmt.Fprintf(&b, "  scope: %s\n", strings.Join(req.Scope, ", "))
	fmt.Fprintf(&b, "  policy: %s@%s\n", req.PolicyName, req.PolicyVersion)
	fmt.Fprintf(&b, "  duties: proposer=%s reviewer=%s executor=%s approver=%s\n",
		req.Proposer, req.Reviewer, req.Executor, req.Approver)
	fmt.Fprintf(&b, "  created: %s expires: %s\n", req.CreatedAt, req.ExpiresAt)
	if req.DecidedAt != "" {
		fmt.Fprintf(&b, "  decided: %s by %s: %s (bound %s)\n",
			req.DecidedAt, req.DecidedBy, req.DecideReason, req.BoundArtifact)
	}
	fmt.Fprintf(&b, "  history:\n")
	for _, h := range req.History {
		fmt.Fprintf(&b, "    - %s %s by %s %s\n", h.At, h.Action, h.By, h.Note)
	}
	return b.String()
}

// RenderList summarizes every sealed request in one line each.
func RenderList(reqs []ApprovalRequest) string {
	var b strings.Builder
	if len(reqs) == 0 {
		return "govern: no approvals sealed\n"
	}
	for _, r := range reqs {
		fmt.Fprintf(&b, "%s %s %s %s (policy %s@%s, expires %s)\n",
			r.ID, r.State, r.Kind, r.Surface, r.PolicyName, r.PolicyVersion, r.ExpiresAt)
	}
	return b.String()
}

// RenderAudit replays the hash-chained log in append order.
func RenderAudit(events []AuditEvent) string {
	var b strings.Builder
	if len(events) == 0 {
		return "govern: audit log empty\n"
	}
	for _, e := range events {
		fmt.Fprintf(&b, "#%d %s %s by %s req=%s %s (hash %s)\n",
			e.Seq, e.At, e.Action, e.Actor, e.RequestID, e.Detail, shortHash(e.Hash))
	}
	return b.String()
}
