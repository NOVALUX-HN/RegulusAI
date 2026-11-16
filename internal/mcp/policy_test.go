package mcp_test

// F11.8/NOV-94 server-side capability policy: matrix drift, defaults,
// delegation restriction, spoofing inertness, traversal containment.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"regulus-ai/internal/mcp"
)

// fullPrincipal grants every known scope: the compatible default.
func fullPrincipal() mcp.Principal {
	os.Unsetenv("REGULUS_MCP_SCOPES")
	return mcp.PrincipalFor("/ws")
}

func scopedPrincipal(scopes ...string) mcp.Principal {
	p := fullPrincipal()
	p.Scopes = map[string]bool{}
	for _, s := range scopes {
		p.Scopes[s] = true
	}
	return p
}

func TestPolicyMatrixCoversTools(t *testing.T) {
	matrix := mcp.PolicyMatrix()
	byTool := map[string]mcp.ToolPolicy{}
	for _, row := range matrix {
		if _, dup := byTool[row.Tool]; dup {
			t.Fatalf("duplicate matrix row for %q", row.Tool)
		}
		byTool[row.Tool] = row
		if row.Tool == "" || len(row.Capabilities) == 0 || len(row.Scopes) == 0 {
			t.Fatalf("matrix row must carry tool+capabilities+scopes, got %+v", row)
		}
	}
	// Both directions: every served tool has a row, every row serves.
	for _, tool := range mcp.Tools() {
		row, ok := byTool[tool.Name]
		if !ok {
			t.Fatalf("served tool %q has no policy row (matrix must track every dispatch)", tool.Name)
		}
		if _, file, _, ok := runtime.Caller(0); ok && row.Sensitive {
			_ = file
		}
	}
	for name := range byTool {
		found := false
		for _, tool := range mcp.Tools() {
			if tool.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("matrix row %q names no served tool (matrix must expose, never invent)", name)
		}
	}
}

func TestPolicyDefaultAllowsAllServedTools(t *testing.T) {
	p := fullPrincipal()
	benign := map[string]map[string]any{
		"memory_note": {}, "memory_show": {}, "memory_save": {},
		"memory_search": {}, "memory_context": {}, "memory_history": {},
		"gate_run":   {"task": "demo/T1"},
		"spec_check": {}, "doctor": {},
		"trace_show": {"session": "s1"}, "trace_sessions": {}, "trace_files": {"session": "s1"},
		"audit":         {},
		"evolve_report": {}, "skills_list": {}, "evolve_tick": {}, "evolve_propose": {"task": "demo/T1", "domain": "go"},
	}
	for _, tool := range mcp.Tools() {
		args, ok := benign[tool.Name]
		if !ok {
			t.Fatalf("no benign fixture for %q", tool.Name)
		}
		if dec := mcp.Evaluate(tool.Name, args, p); !dec.Allow {
			t.Fatalf("default principal denied %q: %s (defaults must preserve local-trust behavior)", tool.Name, dec.Reason)
		} else if dec.PolicyVersion != mcp.PolicyVersion {
			t.Fatalf("decision for %q carries policy %q, want %q", tool.Name, dec.PolicyVersion, mcp.PolicyVersion)
		}
	}
}

func TestPolicyDenyByDefault(t *testing.T) {
	p := fullPrincipal()
	if dec := mcp.Evaluate("nope_missing", map[string]any{}, p); dec.Allow {
		t.Fatal("unknown tool must be denied even for a full principal")
	}
	empty := scopedPrincipal()
	for _, tool := range []string{"memory_show", "doctor", "skills_list"} {
		if dec := mcp.Evaluate(tool, map[string]any{}, empty); dec.Allow {
			t.Fatalf("locked-down principal (no scopes) allowed %q", tool)
		} else if dec.MissingScope == "" {
			t.Fatalf("scope denial for %q must name the missing scope", tool)
		}
	}
}

func TestPolicyScopesEnv(t *testing.T) {
	t.Setenv("REGULUS_MCP_SCOPES", "memory:read,trace:read,bogus-scope,admin")
	p := mcp.PrincipalFor("/ws")
	if !p.Has("memory:read") || !p.Has("trace:read") {
		t.Fatalf("listed known scopes must be granted: %v", p.GrantedScopes())
	}
	if p.Has("bogus-scope") || p.Has("admin") || p.Has("memory:write") {
		t.Fatalf("unknown/unlisted scopes must never be granted: %v", p.GrantedScopes())
	}
	// Set-but-empty is the lockdown: nothing granted.
	t.Setenv("REGULUS_MCP_SCOPES", "")
	if got := mcp.PrincipalFor("/ws").GrantedScopes(); len(got) != 0 {
		t.Fatalf("empty REGULUS_MCP_SCOPES must grant nothing, got %v", got)
	}
}

func TestPolicyDelegateOnlyRestricts(t *testing.T) {
	parent := scopedPrincipal("memory:read", "memory:write")
	child := parent.Delegate("worker", []string{"memory:read", "gate:execute"})
	if !child.Has("memory:read") {
		t.Fatal("delegation must keep the intersected scope")
	}
	if child.Has("memory:write") {
		t.Fatal("delegation must drop unrequested scopes")
	}
	if child.Has("gate:execute") {
		t.Fatal("delegation must never grant scopes the parent lacks (no elevation)")
	}
	if child.Workspace != parent.Workspace || child.Host != parent.Host || child.User != parent.User {
		t.Fatal("delegation must pin host/user/workspace (no re-homing)")
	}
	// Over the evaluator: the child cannot save.
	if dec := mcp.Evaluate("memory_save", map[string]any{}, child); dec.Allow {
		t.Fatal("restricted child must be denied memory_save")
	}
	// A hostile agent name cannot smuggle newlines into audit IDs.
	evil := parent.Delegate("x\ny", []string{"memory:read"})
	if strings.ContainsAny(evil.Agent, "\n\r") {
		t.Fatalf("delegated agent chain must stay single-line: %q", evil.Agent)
	}
}

func TestPolicyClientArgsNeverAuthorize(t *testing.T) {
	// Scope/role/admin-shaped keys in args are inert DATA.
	p := scopedPrincipal("memory:read")
	spoof := map[string]any{
		"type": "decision", "title": "t", "content": "c",
		"scope": "memory:write", "scopes": "memory:write,gate:execute",
		"role": "admin", "admin": true, "allow_destructive": true,
		"policy": "allow", "principal": "root", "trusted": true,
	}
	if dec := mcp.Evaluate("memory_save", spoof, p); dec.Allow {
		t.Fatal("spoof keys in args must not authorize: decision comes from the server principal only")
	} else if dec.MissingScope != "memory:write" {
		t.Fatalf("denial must name the real missing scope, got %q (%s)", dec.MissingScope, dec.Reason)
	}
}

func TestPolicyEvolveTickDryVsWet(t *testing.T) {
	reader := scopedPrincipal("evolve:read")
	if dec := mcp.Evaluate("evolve_tick", map[string]any{}, reader); !dec.Allow {
		t.Fatalf("default dry-run tick must need only evolve:read: %s", dec.Reason)
	}
	if dec := mcp.Evaluate("evolve_tick", map[string]any{"dry_run": false}, reader); dec.Allow {
		t.Fatal("wet tick (dry_run:false) must need evolve:write")
	} else if dec.MissingScope != "evolve:write" {
		t.Fatalf("wet denial must name evolve:write, got %q", dec.MissingScope)
	}
	wet := scopedPrincipal("evolve:write")
	if dec := mcp.Evaluate("evolve_tick", map[string]any{"dry_run": false}, wet); !dec.Allow {
		t.Fatalf("evolve:write must authorize a wet tick: %s", dec.Reason)
	}
}

func TestPolicyPathTraversalRefused(t *testing.T) {
	p := fullPrincipal()
	evil := []string{"..", "../x", "a/../../b", "/etc/passwd", `C:\Windows\system.ini`, "..\\x"}
	for _, v := range evil {
		cases := map[string]map[string]any{
			"memory_save":    {"paths": v},
			"spec_check":     {"file": v},
			"audit":          {"profile": v},
			"memory_context": {"charts": v},
		}
		for tool, args := range cases {
			if dec := mcp.Evaluate(tool, args, p); dec.Allow {
				t.Fatalf("%s(%q) allowed with a full principal: traversal must be refused for everyone", tool, v)
			}
		}
		if dec := mcp.Evaluate("gate_run", map[string]any{"task": v}, p); dec.Allow {
			t.Fatalf("gate_run task %q allowed: traversal-shaped ids must be refused", v)
		}
		if dec := mcp.Evaluate("trace_show", map[string]any{"session": v}, p); dec.Allow {
			t.Fatalf("trace_show session %q allowed: traversal-shaped ids must be refused", v)
		}
	}
}

func TestPolicyWorkspaceContainedAbsoluteAllowed(t *testing.T) {
	ws := string(filepath.Separator) + "ws"
	if runtime.GOOS == "windows" {
		ws = `C:\ws`
	}
	p := mcp.PrincipalFor(ws)
	inside := filepath.Join(ws, "docs", "spec.md")
	if dec := mcp.Evaluate("spec_check", map[string]any{"file": inside}, p); !dec.Allow {
		t.Fatalf("workspace-contained absolute must stay allowed (compatible): %s", dec.Reason)
	}
	outside := filepath.Join(string(filepath.Separator)+"other", "x.md")
	if runtime.GOOS == "windows" {
		outside = `D:\other\x.md`
	}
	if dec := mcp.Evaluate("spec_check", map[string]any{"file": outside}, p); dec.Allow {
		t.Fatalf("outside-workspace absolute %q must be refused", outside)
	}
	// Provenance stays repo-relative even inside the workspace (CLI parity).
	if dec := mcp.Evaluate("memory_save", map[string]any{"paths": inside}, p); dec.Allow {
		t.Fatal("absolute provenance must be refused even when workspace-contained (metadata is repo-relative)")
	}
	if dec := mcp.Evaluate("memory_save", map[string]any{"paths": "docs/spec.md"}, p); !dec.Allow {
		t.Fatalf("repo-relative provenance must stay allowed: %s", dec.Reason)
	}
}

func TestPolicyReasonsCarryNoValues(t *testing.T) {
	p := scopedPrincipal("memory:read")
	dec := mcp.Evaluate("memory_save", map[string]any{"type": "x", "content": "api_key=SECRET-1"}, p)
	if dec.Allow {
		t.Fatal("must deny (missing scope)")
	}
	if strings.Contains(dec.Reason, "SECRET-1") {
		t.Fatalf("denial reason must never echo arg values: %q", dec.Reason)
	}
	if got := mcp.ArgNames(map[string]any{"type": "x", "content": "y"}); len(got) != 2 || got[0] != "content" || got[1] != "type" {
		t.Fatalf("ArgNames must sort keys (audit key list), got %v", got)
	}
}

func TestPolicyDotWorkspaceAnchorsCwd(t *testing.T) {
	// The CLI resolves to "." at the project root by contract:
	// containment must anchor it to the process cwd, or every
	// workspace-absolute path would be refused (regression: the
	// wire serves "." workspaces on every TempDir test).
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	}()
	p := mcp.PrincipalFor(".")
	inside := filepath.Join(dir, "spec.md")
	if dec := mcp.Evaluate("spec_check", map[string]any{"file": inside}, p); !dec.Allow {
		t.Fatalf("dot workspace must anchor to cwd (allow %q): %s", inside, dec.Reason)
	}
	outside := filepath.Join(cwd, "spec.md")
	if outside == inside {
		t.Skip("cwd equals temp dir; nothing outside to probe")
	}
	if dec := mcp.Evaluate("spec_check", map[string]any{"file": outside}, p); dec.Allow {
		t.Fatalf("dot workspace must still refuse outside paths (%q)", outside)
	}
}

func TestPolicyDeniedWireShape(t *testing.T) {
	err := mcp.PolicyDenied("gate_run", "tool gate_run needs scope gate:execute")
	if err.Code != -32602 {
		t.Fatalf("PolicyDenied code = %d, want -32602 (compatible code space)", err.Code)
	}
	if !strings.HasPrefix(err.Message, "policy denied for tool gate_run:") {
		t.Fatalf("PolicyDenied message must carry the stable prefix, got %q", err.Message)
	}
}
