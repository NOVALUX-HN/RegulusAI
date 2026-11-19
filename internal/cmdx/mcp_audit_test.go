// MCP audit decision cover (CRAP paydown sin cambiar comportamiento):
// buildDecisionPayload puro y testeable sin MintRunID ni RecordExec.
// La tabla pinean allow/deny, missing_scope y exit_code.
package cmdx

import (
	"testing"

	"regulus-ai/internal/mcp"
	"regulus-ai/internal/observability"
)

func auditTestPrincipal() mcp.Principal {
	return mcp.Principal{
		Agent: "agent-1", Host: "host-1", User: "user-1", Workspace: "/repo",
		Scopes: map[string]bool{"tools:call": true, "memory:read": true},
	}
}

func TestBuildDecisionPayloadTable(t *testing.T) {
	t.Run("allow carries exit code", func(t *testing.T) {
		dec := mcp.Decision{Allow: true, Reason: "matrix allows"}
		audit := buildDecisionPayload(auditTestPrincipal(), "memory_context", map[string]any{"path": "c"}, dec, 0)
		if audit.status != observability.ExecSucceeded || audit.verdict != "allow" {
			t.Fatalf("allow must succeed, got %+v", audit)
		}
		p := audit.payload
		if p["tool"] != "memory_context" || p["decision"] != "allow" || p["reason"] != "matrix allows" {
			t.Fatalf("payload must cite tool/decision/reason, got %v", p)
		}
		if p["policy_version"] != mcp.PolicyVersion {
			t.Fatalf("payload must pin the policy version, got %v", p)
		}
		if p["exit_code"] != 0 {
			t.Fatalf("authorized calls carry the exit code, got %v", p)
		}
		if _, ok := p["missing_scope"]; ok {
			t.Fatalf("allow must not name a missing scope, got %v", p)
		}
		scopes, ok := p["arg_names"].([]string)
		if !ok || len(scopes) != 1 || scopes[0] != "path" {
			t.Fatalf("arg names must list keys, got %v", p["arg_names"])
		}
	})
	t.Run("deny names the missing scope", func(t *testing.T) {
		dec := mcp.Decision{Allow: false, Reason: "needs scope", MissingScope: "tools:call"}
		audit := buildDecisionPayload(auditTestPrincipal(), "exec_run", map[string]any{}, dec, -1)
		if audit.status != observability.ExecFailed || audit.verdict != "deny" {
			t.Fatalf("deny must fail, got %+v", audit)
		}
		if audit.payload["missing_scope"] != "tools:call" {
			t.Fatalf("deny must name the missing scope, got %v", audit.payload)
		}
		if _, ok := audit.payload["exit_code"]; ok {
			t.Fatalf("denied calls (-1) carry no exit code, got %v", audit.payload)
		}
	})
	t.Run("scopes sort and attribution pins", func(t *testing.T) {
		dec := mcp.Decision{Allow: true, Reason: "ok"}
		audit := buildDecisionPayload(auditTestPrincipal(), "tool", nil, dec, 2)
		scopes, ok := audit.payload["scopes"].([]string)
		if !ok || len(scopes) != 2 || scopes[0] != "memory:read" || scopes[1] != "tools:call" {
			t.Fatalf("scopes must sort, got %v", audit.payload["scopes"])
		}
		for _, k := range []string{"agent", "host", "user", "workspace"} {
			if audit.payload[k] == nil || audit.payload[k] == "" {
				t.Fatalf("payload must pin %q, got %v", k, audit.payload)
			}
		}
	})
}
