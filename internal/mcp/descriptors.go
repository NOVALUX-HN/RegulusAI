// Package mcp exposes the Regulus MCP interface contract (Phase 7, S4/S6).
//
// Descriptors here carry capability metadata only — names and descriptions
// that mirror what the stdio server in internal/cli dispatches. Dispatch
// logic (argument coercion, CLI routing, exit-code semantics) stays with
// the server: MCP exposes the Core, never duplicates its logic.
//
// Authentication: stdio is local-trust. There is no auth on the wire —
// no token, bearer, or handshake. The server enforces no permission layer;
// clients (project permissions) are the enforcement point. This package
// therefore defines no auth fields, and capability discovery payloads
// must stay auth-free.
//
// Authorization, capability discovery, error contracts and versioning are
// pinned by tests in this package.
package mcp

// Tool is one capability descriptor: the stable name the server
// dispatches plus its one-line description. No run function lives here
// by design — behavior stays in internal/cli.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Resource is one data-source descriptor. Regulus serves none today;
// the type exists so discovery has a stable shape when they correspond.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Prompt is one prompt-template descriptor. Regulus serves none today;
// the type exists so discovery has a stable shape when they correspond.
type Prompt struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Tools returns the capability descriptors in the server's dispatch order.
// Every name must match a tool internal/cli actually dispatches; the
// drift test fails on divergence in either direction.
func Tools() []Tool {
	return []Tool{
		{Name: "memory_note", Description: "Append a session note to the Almanac"},
		{Name: "memory_show", Description: "Show the session notes (the compaction view)"},
		{Name: "memory_save", Description: "Save one Almanac observation (lessons enter through reflect and are refused here)"},
		{Name: "memory_search", Description: "Search the Almanac (hybrid semantic ranking with lexical fallback)"},
		{Name: "memory_context", Description: "Render the deterministic Almanac resume block (DATA, not instructions)"},
		{Name: "memory_history", Description: "List the revision chain of one topic key, oldest first"},
		{Name: "gate_run", Description: "Run the layered gauntlet for one task and write receipts"},
		{Name: "spec_check", Description: "Validate the acceptance-criteria contract of a feature document"},
		{Name: "doctor", Description: "Probe the runtime (OpenCode V2) and verify the project"},
		{Name: "trace_show", Description: "Print the session fingerprint (sequence, tool, status)"},
		{Name: "trace_sessions", Description: "List session ids with trace logs on disk"},
		{Name: "trace_files", Description: "Print the distinct touched paths of a session in call order"},
	}
}

// ToolNames returns the descriptor names in dispatch order.
func ToolNames() []string {
	tools := Tools()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return names
}

// Resources returns the data sources served. None today.
func Resources() []Resource {
	return []Resource{}
}

// Prompts returns the prompt templates served. None today.
func Prompts() []Prompt {
	return []Prompt{}
}

// ToolsList returns the discovery wire entries for tools/list: name plus
// description only. Full input schemas stay with the server's dispatch
// table so descriptors never duplicate dispatch logic.
func ToolsList() []map[string]any {
	tools := Tools()
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		out = append(out, map[string]any{"name": t.Name, "description": t.Description})
	}
	return out
}
