// MCP capability policy (F11.8/NOV-94): server-side authorization for
// every MCP tool call. The stdio wire stays local-trust by design — no
// auth handshake, no tokens — so authorization is derived server-side
// from the process environment and the resolved workspace, never from
// client-supplied tool arguments. Evaluation is deny-by-default: an
// unknown tool, a missing scope, a path that escapes the workspace, or
// a malformed identity value is refused before any side effect.
//
// Enforcement point: (CLI).mcpCallTool in internal/cli/mcp.go calls
// Evaluate before dispatching tool.run, and records the decision with
// mcpAuditDecision (internal/cli/mcp_policy.go) into the F11.1
// execution log (internal/observability/exec.go, schema_version 1,
// redacted pre-disk). This file is pure: no disk, no network, no
// clocks — fully unit-testable.
package mcp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PolicyVersion versions the MCP capability policy. Readers (audit
// queries, migration docs) must pin it exactly; tightening or
// relaxing the matrix ships a new version, never a silent edit.
const PolicyVersion = "regulus.mcp-policy/v1"

// Capability classifies what a tool can touch. A tool carries one or
// more; the matrix below (toolRules) is the authoritative inventory.
type Capability string

const (
	// CapRead touches nothing: pure reads of memory, traces, reports.
	CapRead Capability = "read"
	// CapWrite mutates project state (notes, observations, tick cursor).
	CapWrite Capability = "write"
	// CapExecute runs code (gates): process spawn with receipts.
	CapExecute Capability = "execute"
	// CapSecrets marks tools whose inputs/outputs routinely carry
	// secret-shaped data. No tool takes credentials; the capability
	// exists so the audit layer knows every such surface is
	// redacted pre-disk (arg names only, never values).
	CapSecrets Capability = "secrets"
	// CapMemory marks Almanac memory state (notes, observations).
	CapMemory Capability = "memory"
	// CapEvidence marks evidence writes (gate receipts, tick cursor).
	CapEvidence Capability = "evidence"
	// CapHarness marks harness/evolution state changes (tick cursor).
	// No MCP tool modifies harness code: self-modification
	// (probe_contrast/compose, memory prune/import, surface apply)
	// stays shell-only by policy.
	CapHarness Capability = "harness"
)

// Minimal scopes. A principal holds a set; a tool call needs every
// scope its rule requires (RequiredScopes). Unknown scope names in
// REGULUS_MCP_SCOPES are dropped, never granted: the world is closed.
const (
	ScopeMemoryRead  = "memory:read"
	ScopeMemoryWrite = "memory:write"
	ScopeGateExecute = "gate:execute"
	ScopeSpecRead    = "spec:read"
	ScopeDoctorRead  = "doctor:read"
	ScopeTraceRead   = "trace:read"
	ScopeAuditRead   = "audit:read"
	ScopeEvolveRead  = "evolve:read"
	ScopeEvolveWrite = "evolve:write"
	ScopeSkillsRead  = "skills:read"
)

// knownScopes is the closed scope vocabulary. Anything else in the
// environment is ignored (deny-by-default extends to configuration).
var knownScopes = map[string]bool{
	ScopeMemoryRead: true, ScopeMemoryWrite: true,
	ScopeGateExecute: true, ScopeSpecRead: true,
	ScopeDoctorRead: true, ScopeTraceRead: true,
	ScopeAuditRead: true, ScopeEvolveRead: true,
	ScopeEvolveWrite: true, ScopeSkillsRead: true,
}

// DefaultScopes returns the minimal scopes that preserve the
// pre-policy local-trust behavior: every one of the 17 served tools
// with its least-privilege grant (evolve_tick dry-run only — the wet
// cursor advance additionally needs evolve:write, which the default
// includes so bare-client behavior stays byte-identical; operators
// tighten by setting REGULUS_MCP_SCOPES explicitly).
func DefaultScopes() []string {
	return []string{
		ScopeMemoryRead, ScopeMemoryWrite,
		ScopeGateExecute, ScopeSpecRead, ScopeDoctorRead,
		ScopeTraceRead, ScopeAuditRead,
		ScopeEvolveRead, ScopeEvolveWrite, ScopeSkillsRead,
	}
}

// Principal separates the four authorization dimensions. Agent is the
// calling agent identity (server-attributed, never client-claimed);
// Host/User identify where the server runs; Workspace is the
// server-resolved project root every path check anchors to. Scopes is
// the granted set. Delegation may only restrict (see Delegate).
type Principal struct {
	Agent     string
	Host      string
	User      string
	Workspace string
	Scopes    map[string]bool
}

// Has reports whether the principal holds scope.
func (p Principal) Has(scope string) bool {
	return p.Scopes[scope]
}

// GrantedScopes returns the held scopes, sorted, for audit payloads.
func (p Principal) GrantedScopes() []string {
	out := make([]string, 0, len(p.Scopes))
	for s := range p.Scopes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// sanitizeID forces a server-attributed identity field onto one line:
// trims spaces, folds newlines/control runs into "_", keeps printable
// text. Client input never reaches these fields; this guards the
// environment (and the exec log's single-line ID columns) instead.
func sanitizeID(s, fallback string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return fallback
	}
	var b strings.Builder
	for _, r := range t {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	if out := b.String(); strings.TrimSpace(out) != "" {
		return out
	}
	return fallback
}

// defaultHostUser resolves the server host/user attribution without
// trusting the wire: hostname, then USER/USERNAME, with stable
// fallbacks. Values are sanitized single-line.
func defaultHostUser() (host, user string) {
	host = "unknown-host"
	if h, err := os.Hostname(); err == nil && strings.TrimSpace(h) != "" {
		host = h
	}
	host = sanitizeID(host, "unknown-host")
	user = strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = strings.TrimSpace(os.Getenv("USERNAME"))
	}
	user = sanitizeID(user, "unknown-user")
	return host, user
}

// PrincipalFor derives the calling principal server-side. Workspace
// comes from the caller (the CLI passes its resolved project root);
// Agent/User may be overridden by REGULUS_MCP_AGENT /
// REGULUS_MCP_USER (deployment attribution, still server-side —
// client tool arguments can never set them). Scopes come from
// REGULUS_MCP_SCOPES: unset means the compatible default (every
// minimal scope); set means exactly the known names listed (unknown
// dropped); set-but-empty is the lockdown: no scopes, every
// tools/call denied while tools/list and initialize keep serving.
func PrincipalFor(workspace string) Principal {
	host, user := defaultHostUser()
	p := Principal{
		Agent:     sanitizeID(os.Getenv("REGULUS_MCP_AGENT"), "mcp:stdio-client"),
		Host:      host,
		User:      sanitizeID(os.Getenv("REGULUS_MCP_USER"), user),
		Workspace: workspace,
		Scopes:    map[string]bool{},
	}
	raw, set := os.LookupEnv("REGULUS_MCP_SCOPES")
	if !set {
		for _, s := range DefaultScopes() {
			p.Scopes[s] = true
		}
		return p
	}
	for _, part := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(part); knownScopes[s] {
			p.Scopes[s] = true
		}
	}
	return p
}

// Delegate derives a child principal that may only restrict: the
// requested scopes are intersected with the parent's (never widened),
// and Host/User/Workspace are pinned — a delegate cannot re-home the
// workspace or claim a new user to escape containment. The agent
// chain records attribution (parent>child), sanitized single-line.
func (p Principal) Delegate(agent string, requested []string) Principal {
	child := Principal{
		Agent:     sanitizeID(p.Agent+">"+agent, p.Agent),
		Host:      p.Host,
		User:      p.User,
		Workspace: p.Workspace,
		Scopes:    map[string]bool{},
	}
	for _, s := range requested {
		if s = strings.TrimSpace(s); knownScopes[s] && p.Scopes[s] {
			child.Scopes[s] = true
		}
	}
	return child
}

// Decision is the outcome of Evaluate: Allow plus the reason (keys
// only, never arg values), or deny with the missing scope / offending
// key named. PolicyVersion pins which matrix decided.
type Decision struct {
	Allow         bool
	Reason        string
	MissingScope  string
	PolicyVersion string
}

// allowDecision grants with a reason.
func allowDecision(reason string) Decision {
	return Decision{Allow: true, Reason: reason, PolicyVersion: PolicyVersion}
}

// denyDecision refuses with a reason; missingScope names the scope
// whose absence refused ("" for non-scope refusals).
func denyDecision(reason, missingScope string) Decision {
	return Decision{Allow: false, Reason: reason, MissingScope: missingScope, PolicyVersion: PolicyVersion}
}

// PolicyDenied renders a denial as the wire error: -32602 (invalid
// params) with the stable "policy denied for tool <name>:" prefix, so
// clients distinguish authorization failures from usage errors by
// prefix while the code space stays unchanged (compatible migration).
func PolicyDenied(tool, reason string) Error {
	if reason == "" {
		reason = "not authorized"
	}
	return Error{Code: CodeInvalidParams, Message: "policy denied for tool " + tool + ": " + reason}
}

// pathKind selects the containment rule for a path-ish argument.
type pathKind int

const (
	// kindProvenance is stored metadata, never read or written: it
	// must be repo-relative (no absolute, no drive letter, no ".."
	// segment) — mirroring checkProvenancePaths in internal/cli.
	kindProvenance pathKind = iota
	// kindFile is a real file read (spec_check file, audit profile,
	// memory_context charts): it must stay inside the workspace.
	// Workspace-contained absolute paths stay allowed (compatible:
	// existing callers pass TempDir-absolute paths); anything
	// escaping is refused.
	kindFile
)

// pathCheck binds one argument key to its containment rule.
type pathCheck struct {
	key  string
	kind pathKind
}

// toolRule is one row of the tool→capability→policy matrix: the
// capabilities the tool exercises, the scopes that authorize it
// (required may depend on arguments, e.g. evolve_tick dry vs wet),
// the path-ish keys to contain, and the ID-ish keys to keep
// traversal-free. Sensitive marks state-changing tools (write or
// execute): no sensitive tool runs without server-side authorization.
type toolRule struct {
	capabilities []Capability
	sensitive    bool
	required     func(args map[string]any) []string
	paths        []pathCheck
	ids          []string
}

func staticScopes(scopes ...string) func(map[string]any) []string {
	return func(map[string]any) []string { return scopes }
}

// tickScopes is the evolve_tick conditional: the default dry run
// (dry_run absent or true) only reads; an explicit wet run
// (dry_run:false) advances the tick cursor — a harness-state write
// that needs evolve:write.
func tickScopes(args map[string]any) []string {
	if b, ok := args["dry_run"].(bool); ok && !b {
		return []string{ScopeEvolveWrite}
	}
	if s, ok := args["dry_run"].(string); ok {
		if v := strings.TrimSpace(strings.ToLower(s)); v == "false" || v == "0" || v == "no" || v == "off" {
			return []string{ScopeEvolveWrite}
		}
	}
	return []string{ScopeEvolveRead}
}

// toolRules is the authoritative tool→capability→policy matrix. Every
// name must match internal/cli's dispatch table (drift pinned both
// ways by TestPolicyMatrixCoversTools here and the descriptor drift
// tests). No MCP tool is destructive and none modifies harness code:
// probe_contrast/compose, memory prune/import and surface apply stay
// shell-only by policy (see docs/mcp-policy.md).
var toolRules = map[string]toolRule{
	"memory_note": {
		capabilities: []Capability{CapWrite, CapSecrets, CapMemory},
		sensitive:    true,
		required:     staticScopes(ScopeMemoryWrite),
	},
	"memory_show": {
		capabilities: []Capability{CapRead, CapMemory},
		required:     staticScopes(ScopeMemoryRead),
	},
	"memory_save": {
		capabilities: []Capability{CapWrite, CapSecrets, CapMemory},
		sensitive:    true,
		required:     staticScopes(ScopeMemoryWrite),
		paths: []pathCheck{
			{key: "paths", kind: kindProvenance},
			{key: "receipts", kind: kindProvenance},
		},
		ids: []string{"topic_key", "domain", "task"},
	},
	"memory_search": {
		capabilities: []Capability{CapRead, CapMemory},
		required:     staticScopes(ScopeMemoryRead),
		ids:          []string{"topic_key", "domain", "path"},
	},
	"memory_context": {
		capabilities: []Capability{CapRead, CapMemory},
		required:     staticScopes(ScopeMemoryRead),
		paths:        []pathCheck{{key: "charts", kind: kindFile}},
		ids:          []string{"session"},
	},
	"memory_history": {
		capabilities: []Capability{CapRead, CapMemory},
		required:     staticScopes(ScopeMemoryRead),
		ids:          []string{"topic_key"},
	},
	"gate_run": {
		capabilities: []Capability{CapExecute, CapEvidence},
		sensitive:    true,
		required:     staticScopes(ScopeGateExecute),
		ids:          []string{"task"},
	},
	"spec_check": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeSpecRead),
		paths:        []pathCheck{{key: "file", kind: kindFile}},
	},
	"doctor": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeDoctorRead),
	},
	"trace_show": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeTraceRead),
		ids:          []string{"session"},
	},
	"trace_sessions": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeTraceRead),
	},
	"trace_files": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeTraceRead),
		ids:          []string{"session"},
	},
	"audit": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeAuditRead),
		paths:        []pathCheck{{key: "profile", kind: kindFile}},
	},
	"evolve_report": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeEvolveRead),
		ids:          []string{"domain"},
	},
	"skills_list": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeSkillsRead),
	},
	"evolve_tick": {
		capabilities: []Capability{CapRead, CapHarness, CapEvidence},
		sensitive:    true, // wet runs write the tick cursor
		required:     tickScopes,
	},
	"evolve_propose": {
		capabilities: []Capability{CapRead},
		required:     staticScopes(ScopeEvolveRead),
		ids:          []string{"task", "domain"},
	},
}

// ToolPolicy is the wire-safe projection of one matrix row: the tool,
// its capabilities, its static minimal scopes (dynamic rows like
// evolve_tick report the read grant; the wet grant is documented),
// and whether it is state-changing.
type ToolPolicy struct {
	Tool         string       `json:"tool"`
	Capabilities []Capability `json:"capabilities"`
	Scopes       []string     `json:"scopes"`
	Sensitive    bool         `json:"sensitive"`
}

// PolicyMatrix returns the tool→capability→policy inventory in
// Tools() dispatch order. Unknown tools never appear here:
// deny-by-default keeps the matrix closed.
func PolicyMatrix() []ToolPolicy {
	tools := Tools()
	out := make([]ToolPolicy, 0, len(tools))
	for _, t := range tools {
		r, ok := toolRules[t.Name]
		if !ok {
			continue
		}
		out = append(out, ToolPolicy{
			Tool:         t.Name,
			Capabilities: append([]Capability(nil), r.capabilities...),
			Scopes:       append([]string(nil), r.required(map[string]any{})...),
			Sensitive:    r.sensitive,
		})
	}
	return out
}

// RequiredScopes reports the scopes one call needs (argument-aware:
// evolve_tick wet runs need evolve:write). Unknown tools need the
// unsatisfiable scope "": always denied.
func RequiredScopes(tool string, args map[string]any) []string {
	if r, ok := toolRules[tool]; ok {
		return r.required(args)
	}
	return []string{""}
}

// strArg coerces one argument to a string (strings only, mirroring
// the server's mcpStr: anything else is absent, never an error).
func strArg(args map[string]any, key string) (string, bool) {
	s, ok := args[key].(string)
	if !ok {
		return "", false
	}
	return s, true
}

// strListArg coerces provenance/charts-style arguments: a raw string
// passes through (callers split commas), an array joins with commas
// (mirroring the server's mcpStrList). Non-string elements are
// refused as malformed.
func strListArg(args map[string]any, key string) ([]string, bool) {
	switch v := args[key].(type) {
	case nil:
		return nil, false
	case string:
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out, true
	case []any:
		out := make([]string, 0, len(v))
		for _, p := range v {
			s, ok := p.(string)
			if !ok {
				return nil, false
			}
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// hasNUL reports embedded NUL bytes: never legitimate in a path or
// id, always refused.
func hasNUL(s string) bool { return strings.IndexByte(s, 0) >= 0 }

// isAbsLike reports absolute-shaped values on either platform:
// leading / or \, or a Windows drive letter.
func isAbsLike(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "\\") {
		return true
	}
	return len(s) >= 2 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':'
}

// hasDotDot reports a ".." path segment (either separator).
func hasDotDot(s string) bool {
	for _, seg := range strings.Split(strings.ReplaceAll(s, "\\", "/"), "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// provenanceClean mirrors checkProvenancePaths (internal/cli): stored
// metadata stays repo-relative — no absolute, no drive letter, no
// "..", no empty, no NUL/newline. Empty input means absent (the CLI's
// usage errors stay authoritative for missing params).
func provenanceClean(v string) bool {
	p := strings.TrimSpace(v)
	if p == "" || hasNUL(p) || strings.ContainsAny(p, "\n\r") {
		return false
	}
	if isAbsLike(p) || hasDotDot(p) {
		return false
	}
	return true
}

// withinWorkspace reports whether v, resolved against the
// server-resolved workspace, stays inside it. Relative values without
// traversal are inside by construction; absolute values must be
// contained (compatible: existing callers pass workspace-absolute
// paths); drive-relative "/x" is anchored at the volume root so it
// cannot smuggle an escape on Windows. An unresolvable workspace
// ("", ".") fails absolute values closed and keeps clean relative
// ones (they resolve against the process cwd, which IS the workspace
// there).
func withinWorkspace(workspace, v string) bool {
	p := strings.TrimSpace(v)
	if p == "" || hasNUL(p) || strings.ContainsAny(p, "\n\r") {
		return false
	}
	if hasDotDot(p) {
		return false
	}
	// Anchor the workspace: the CLI resolves to "." at the project
	// root by contract, which filepath.Rel cannot anchor lexically —
	// resolve it against the process cwd (which IS the workspace
	// there) before containing absolutes.
	base := filepath.Clean(workspace)
	if base == "" || base == "." {
		if wd, err := os.Getwd(); err == nil && strings.TrimSpace(wd) != "" {
			base = filepath.Clean(wd)
		}
	}
	if !isAbsLike(p) && !filepath.IsAbs(p) {
		return true
	}
	if base == "" || base == "." {
		return false
	}
	var target string
	switch {
	case filepath.IsAbs(p):
		target = filepath.Clean(p)
	case base == "" || base == ".":
		return false
	default:
		// Leading / or \: anchor at the workspace volume root so a
		// drive-relative value cannot resolve outside by accident.
		vol := filepath.VolumeName(base)
		rest := strings.TrimLeft(p, `/\`)
		target = filepath.Clean(filepath.Join(vol+string(filepath.Separator), rest))
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// idClean keeps ID-ish arguments traversal-free: no NUL/newlines, no
// ".." segments, never absolute-shaped. Empty means absent (the CLI's
// missing-param errors stay authoritative). Legit ids ("s1",
// "demo/T1", "mcp-k1", "go") always pass.
func idClean(v string) bool {
	p := strings.TrimSpace(v)
	if p == "" {
		return true
	}
	if hasNUL(p) || strings.ContainsAny(p, "\n\r") {
		return false
	}
	return !isAbsLike(p) && !hasDotDot(p)
}

// Evaluate authorizes one tool call against a server-derived
// principal. Client arguments are DATA: they are validated for shape
// (containment, traversal) but never grant anything — authorization
// comes only from p.Scopes. Order: known tool → scopes → paths →
// ids. Missing params pass through to the CLI's pinned usage errors.
func Evaluate(tool string, args map[string]any, p Principal) Decision {
	r, ok := toolRules[tool]
	if !ok {
		return denyDecision("unknown tool "+tool, "")
	}
	if args == nil {
		args = map[string]any{}
	}
	for _, s := range r.required(args) {
		if !p.Scopes[s] {
			return denyDecision("tool "+tool+" needs scope "+s, s)
		}
	}
	for _, pc := range r.paths {
		switch pc.kind {
		case kindProvenance:
			vals, present := strListArg(args, pc.key)
			if !present {
				continue
			}
			for _, v := range vals {
				if !provenanceClean(v) {
					return denyDecision("argument "+pc.key+" must be repo-relative, no absolute paths or ..", "")
				}
			}
		case kindFile:
			vals, present := strListArg(args, pc.key)
			if !present {
				if s, ok := strArg(args, pc.key); ok && strings.TrimSpace(s) != "" {
					vals = []string{s}
					present = true
				}
			}
			if !present {
				continue
			}
			ws := p.Workspace
			for _, v := range vals {
				if strings.TrimSpace(v) == "" {
					continue
				}
				if !withinWorkspace(ws, v) {
					return denyDecision("argument "+pc.key+" must stay inside the workspace", "")
				}
			}
		}
	}
	for _, key := range r.ids {
		if s, ok := strArg(args, key); ok && !idClean(s) {
			return denyDecision("argument "+key+" must be a plain id, no paths or ..", "")
		}
	}
	return allowDecision("tool " + tool + " authorized")
}

// ArgNames returns the sorted argument keys for audit payloads: what
// was passed, never the values (secrets never reach the log).
func ArgNames(args map[string]any) []string {
	out := make([]string, 0, len(args))
	for k := range args {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
