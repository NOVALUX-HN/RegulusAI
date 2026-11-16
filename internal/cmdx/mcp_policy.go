package cmdx

import (
	"fmt"
	"sort"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

// MCPPrincipal derives the calling principal server-side: the
// resolved project root anchors every path check, and scopes come
// from REGULUS_MCP_SCOPES (unset = compatible default, set =
// exactly the known names, set-but-empty = lockdown). Moved from
// internal/cli (mcp_policy.go) without behavior change.
func (io IO) MCPPrincipal() mcp.Principal {
	return mcp.PrincipalFor(io.Dir)
}

// MCPAuthorize evaluates one tool call against the server principal.
// Pure decision, no side effects, no audit: the caller audits.
func (io IO) MCPAuthorize(name string, args map[string]any) mcp.Decision {
	return mcp.Evaluate(name, args, io.MCPPrincipal())
}

// MCPAuditDecision records one policy decision (allow or deny) as an
// F11.1 decision event. Best-effort: a logging failure is reported on
// the server stderr and never alters the tool outcome. code is the
// CLI exit code for authorized calls, -1 for denied ones.
func (io IO) MCPAuditDecision(p mcp.Principal, name string, args map[string]any, dec mcp.Decision, code int) {
	run, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "mcp: policy audit skipped (no run id): %v\n", err)
		return
	}
	status := observability.ExecSucceeded
	verdict := "allow"
	if !dec.Allow {
		status = observability.ExecFailed
		verdict = "deny"
	}
	scopes := append([]string(nil), p.GrantedScopes()...)
	sort.Strings(scopes)
	payload := map[string]any{
		"policy_version": mcp.PolicyVersion,
		"tool":           name,
		"decision":       verdict,
		"reason":         dec.Reason,
		"agent":          p.Agent,
		"host":           p.Host,
		"user":           p.User,
		"workspace":      p.Workspace,
		"scopes":         scopes,
		"arg_names":      mcp.ArgNames(args),
	}
	if dec.MissingScope != "" {
		payload["missing_scope"] = dec.MissingScope
	}
	if code >= 0 {
		payload["exit_code"] = code
	}
	if _, _, err := observability.RecordExec(io.Dir, observability.ExecEvent{
		SchemaVersion:    observability.ExecSchemaVersion,
		RunID:            run,
		AgentExecutionID: p.Agent,
		Type:             observability.ExecDecision,
		Status:           status,
		Host:             p.Host,
		Payload:          payload,
	}, time.Now().UTC()); err != nil {
		fmt.Fprintf(io.Stderr, "mcp: policy audit skipped: %v\n", err)
	}
}
