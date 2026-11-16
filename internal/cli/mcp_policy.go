// MCP server-side authorization glue (F11.8/NOV-94).
//
// Every tools/call passes mcpAuthorize (mcp.Evaluate over a
// server-derived principal) BEFORE tool.run executes: unknown tools
// keep the pinned -32601 shape, denied tools get the pinned
// -32602 "policy denied for tool <name>:" shape, and only authorized
// calls reach the CLI code paths — so refusal happens before any
// side effect. Each decision (allow or deny) is recorded best-effort
// into the F11.1 execution log (observability.RecordExec,
// schema_version 1, redacted pre-disk) with the policy version, the
// server-attributed principal, the granted scopes and the argument
// NAMES only — never values, so secrets cannot leak into the audit
// trail. Audit failures never change the wire outcome: availability
// stays local-tool-first (a disk failure must not brick the server),
// and the failure surfaces on the server's stderr, not the wire.
package cli

import (
	"fmt"
	"sort"
	"time"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

// mcpPrincipal derives the calling principal server-side: the
// resolved project root anchors every path check, and scopes come
// from REGULUS_MCP_SCOPES (unset = compatible default, set =
// exactly the known names, set-but-empty = lockdown). Client tool
// arguments are never consulted: scope/role/admin-shaped keys in
// args are inert DATA (spoofing tests pin this).
func (c CLI) mcpPrincipal() mcp.Principal {
	return mcp.PrincipalFor(c.dir())
}

// mcpAuthorize evaluates one tool call against the server principal.
// Pure decision, no side effects, no audit: the caller audits.
func (c CLI) mcpAuthorize(name string, args map[string]any) mcp.Decision {
	return mcp.Evaluate(name, args, c.mcpPrincipal())
}

// mcpAuditDecision records one policy decision (allow or deny) as an
// F11.1 decision event. Best-effort: a logging failure is reported on
// the server stderr and never alters the tool outcome. code is the
// CLI exit code for authorized calls, -1 for denied ones (refused
// before running, so there is no exit code).
func (c CLI) mcpAuditDecision(p mcp.Principal, name string, args map[string]any, dec mcp.Decision, code int) {
	run, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(c.Stderr, "mcp: policy audit skipped (no run id): %v\n", err)
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
	if _, _, err := observability.RecordExec(c.dir(), observability.ExecEvent{
		SchemaVersion:    observability.ExecSchemaVersion,
		RunID:            run,
		AgentExecutionID: p.Agent,
		Type:             observability.ExecDecision,
		Status:           status,
		Host:             p.Host,
		Payload:          payload,
	}, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "mcp: policy audit skipped: %v\n", err)
	}
}
