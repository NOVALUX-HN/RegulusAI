package cli

// The stdio MCP server (`regulus mcp`): newline-delimited JSON-RPC 2.0
// over stdin/stdout for Devin (CLI, Desktop, Cloud). Zero dependencies
// beyond the standard library. Every tool reuses the CLI's own code
// paths (buffers in, buffers out) — never shell-outs — so MCP semantics
// match CLI stderr semantics exactly.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// mcpProtocolVersion is the MCP wire version the server speaks.
const mcpProtocolVersion = "2024-11-05"

// mcpCmd implements `regulus mcp`: serve one JSON-RPC object per stdin
// line until EOF.
func (c CLI) mcpCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus mcp")
		return 2
	}
	c.serveMCP(os.Stdin, os.Stdout)
	return 0
}

// serveMCP runs the request loop: one JSON object per line, one response
// line per request. Notifications (no id) and the initialized
// notification get no reply; malformed lines get -32700 and serving
// continues.
func (c CLI) serveMCP(in io.Reader, out io.Writer) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if resp := c.handleMCPLine(line); resp != nil {
			out.Write(append(resp, '\n'))
		}
	}
}

// mcpRequest is one inbound JSON-RPC message. ID stays raw so responses
// echo numbers, strings and null back verbatim; nil means notification.
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func mcpOK(id json.RawMessage, result any) []byte {
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
	if err != nil {
		return mcpErr(json.RawMessage("null"), -32603, "internal error")
	}
	return raw
}

func mcpErr(id json.RawMessage, code int, msg string) []byte {
	if id == nil {
		id = json.RawMessage("null")
	}
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(id),
		"error": map[string]any{"code": code, "message": msg},
	})
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return raw
}

func (c CLI) handleMCPLine(line string) []byte {
	var req mcpRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return mcpErr(nil, -32700, "parse error")
	}
	if req.Method == "notifications/initialized" {
		return nil
	}
	if req.ID == nil {
		return nil
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return mcpErr(req.ID, -32600, "invalid request")
	}
	switch req.Method {
	case "initialize":
		return mcpOK(req.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "regulus", "version": c.Version},
		})
	case "tools/list":
		return mcpOK(req.ID, map[string]any{"tools": mcpToolList()})
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
			return mcpErr(req.ID, -32602, "tools/call needs {name, arguments}")
		}
		if params.Arguments == nil {
			params.Arguments = map[string]any{}
		}
		return c.mcpCallTool(req.ID, params.Name, params.Arguments)
	default:
		return mcpErr(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// mcpCallTool runs one named tool. Unknown names are -32601; usage-level
// failures (exit 2, the CLI's bad-params signal) are -32602; every other
// outcome — success or refusal — is a content reply carrying the combined
// CLI output, matching CLI stderr semantics.
func (c CLI) mcpCallTool(id json.RawMessage, name string, args map[string]any) []byte {
	tool := mcpToolByName(name)
	if tool == nil {
		return mcpErr(id, -32601, fmt.Sprintf("unknown tool: %s", name))
	}
	stdout, stderr, code := tool.run(c, args)
	if code == 2 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = fmt.Sprintf("invalid params for tool %s", name)
		}
		return mcpErr(id, -32602, msg)
	}
	return mcpOK(id, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": mcpCombine(stdout, stderr)}},
	})
}

// mcpCombine merges the sub-command buffers the way a terminal shows
// them: stdout first, then stderr, trimmed, never empty.
func mcpCombine(stdout, stderr string) string {
	s := stdout
	if stderr != "" {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += stderr
	}
	if s = strings.TrimSpace(s); s != "" {
		return s
	}
	return "(no output)"
}

// mcpExec runs one CLI code path with private buffers, inheriting the
// server's version, probe and runner.
func (c CLI) mcpExec(fn func(sub CLI) int) (string, string, int) {
	var ob, eb bytes.Buffer
	sub := CLI{Version: c.Version, Probe: c.Probe, Runner: c.Runner, Stdout: &ob, Stderr: &eb}
	code := fn(sub)
	return ob.String(), eb.String(), code
}

// mcpTool is one entry of the tools/list table: schema for the listing,
// run for the call.
type mcpTool struct {
	name     string
	desc     string
	props    map[string]any
	required []string
	run      func(c CLI, args map[string]any) (string, string, int)
}

func mcpProp(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// mcpTools is the full registry in tools/list order.
func mcpTools() []mcpTool {
	str := func(key, desc string) (string, map[string]any) {
		return key, mcpProp("string", desc)
	}
	opt := func(key, desc string) (string, map[string]any) {
		return key, mcpProp("string", desc+" (optional)")
	}
	props := func(pairs ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i].(string)] = pairs[i+1]
		}
		return m
	}
	textKey, textProp := str("text", "the session note text")
	typeKey, typeProp := str("type", "observation type: decision|discovery|preference|session-note")
	titleKey, titleProp := str("title", "short, searchable title")
	contentKey, contentProp := str("content", "the observation body")
	topicKey, topicProp := str("topic_key", "group for evolving topics (revision chain)")
	sessionKey, sessionProp := str("session", "the session id")
	taskKey, taskProp := str("task", "the task id")
	fileKey, fileProp := str("file", "the feature document path")
	tkOptKey, tkOptProp := opt("topic_key", "group for evolving topics")
	domainOptKey, domainOptProp := opt("domain", "domain bucket")
	taskOptKey, taskOptProp := opt("task", "task id for provenance")
	pathsOptKey, pathsOptProp := opt("paths", "affected paths, comma-separated")
	receiptsOptKey, receiptsOptProp := opt("receipts", "receipt paths for provenance, comma-separated")
	_, domainProp := opt("domain", "bucket: domain")
	_, typeOptProp := opt("type", "bucket: observation type")
	_, dimOptProp := opt("dimension", "bucket: lesson dimension (inv | dom)")
	_, pathOptProp := opt("path", "bucket: provenance path (exact match)")
	return []mcpTool{
		{
			name: "memory_note", desc: "Append a session note to the Almanac",
			props: props(textKey, textProp), required: []string{"text"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				text := strings.TrimSpace(mcpStr(args, "text"))
				if text == "" {
					return "", "memory note: missing required param \"text\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.memoryNoteCmd([]string{text}) })
			},
		},
		{
			name: "memory_show", desc: "Show the session notes (the compaction view)",
			props: map[string]any{}, required: []string{},
			run: func(c CLI, args map[string]any) (string, string, int) {
				return c.mcpExec(func(sub CLI) int { return sub.memoryShowCmd(nil) })
			},
		},
		{
			name: "memory_save", desc: "Save one Almanac observation (lessons enter through reflect and are refused here)",
			props: props(typeKey, typeProp, titleKey, titleProp, contentKey, contentProp,
				tkOptKey, tkOptProp, domainOptKey, domainOptProp, taskOptKey, taskOptProp,
				pathsOptKey, pathsOptProp, receiptsOptKey, receiptsOptProp),
			required: []string{"type", "title", "content"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				typ := strings.TrimSpace(mcpStr(args, "type"))
				title := strings.TrimSpace(mcpStr(args, "title"))
				content := strings.TrimSpace(mcpStr(args, "content"))
				if typ == "" || title == "" || content == "" {
					return "", "memory save: missing required params \"type\", \"title\", \"content\"", 2
				}
				cliArgs := []string{"--type", typ, "--title", title}
				for flag, key := range map[string]string{
					"--topic-key": "topic_key", "--domain": "domain",
					"--task": "task", "--path": "paths", "--receipt": "receipts",
				} {
					if v, ok := mcpStrList(args, key); ok && strings.TrimSpace(v) != "" {
						cliArgs = append(cliArgs, flag, v)
					}
				}
				cliArgs = append(cliArgs, content)
				return c.mcpExec(func(sub CLI) int { return sub.memorySaveCmd(cliArgs) })
			},
		},
		{
			name: "memory_search", desc: "Search the Almanac (hybrid semantic ranking with lexical fallback)",
			props: props(
				"terms", map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "terms to match (optional)"},
				"topic_key", mcpProp("string", "bucket: topic key (optional)"),
				"domain", domainProp, "type", typeOptProp, "dimension", dimOptProp, "path", pathOptProp,
				"limit", map[string]any{"type": "integer", "description": "keep the first N results, 0 means all (optional)"},
				"any", map[string]any{"type": "boolean", "description": "any term may match (optional)"},
				"latest", map[string]any{"type": "boolean", "description": "collapse each topic chain to its best hit (optional)"},
				"include_superseded", map[string]any{"type": "boolean", "description": "show observations a supersedes relation hides (optional)"},
			),
			required: []string{},
			run: func(c CLI, args map[string]any) (string, string, int) {
				terms, ok := mcpTerms(args, "terms")
				if !ok {
					return "", "memory search: \"terms\" must be a string or an array of strings", 2
				}
				var cliArgs []string
				for flag, key := range map[string]string{
					"--topic-key": "topic_key", "--domain": "domain", "--type": "type",
					"--dimension": "dimension", "--path": "path",
				} {
					if v := strings.TrimSpace(mcpStr(args, key)); v != "" {
						cliArgs = append(cliArgs, flag, v)
					}
				}
				if n, ok := mcpInt(args, "limit"); ok {
					cliArgs = append(cliArgs, "--limit", strconv.Itoa(n))
				}
				for flag, key := range map[string]string{
					"--any": "any", "--latest": "latest", "--include-superseded": "include_superseded",
				} {
					if b, ok := mcpBool(args, key); ok && b {
						cliArgs = append(cliArgs, flag)
					}
				}
				cliArgs = append(cliArgs, terms...)
				return c.mcpExec(func(sub CLI) int { return sub.memorySearchCmd(cliArgs) })
			},
		},
		{
			name: "memory_context", desc: "Render the deterministic Almanac resume block (DATA, not instructions)",
			props: map[string]any{}, required: []string{},
			run: func(c CLI, args map[string]any) (string, string, int) {
				return c.mcpExec(func(sub CLI) int { return sub.memoryContextCmd(nil) })
			},
		},
		{
			name: "memory_history", desc: "List the revision chain of one topic key, oldest first",
			props: props(topicKey, topicProp), required: []string{"topic_key"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				k := strings.TrimSpace(mcpStr(args, "topic_key"))
				if k == "" {
					return "", "memory history: missing required param \"topic_key\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.memoryHistoryCmd([]string{"--topic-key", k}) })
			},
		},
		{
			name: "gate_run", desc: "Run the layered gauntlet for one task and write receipts",
			props: props(taskKey, taskProp), required: []string{"task"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				task := strings.TrimSpace(mcpStr(args, "task"))
				if task == "" {
					return "", "gate run: missing required param \"task\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.gateCmd([]string{"run", task}) })
			},
		},
		{
			name: "spec_check", desc: "Validate the acceptance-criteria contract of a feature document",
			props: props(fileKey, fileProp), required: []string{"file"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				f := strings.TrimSpace(mcpStr(args, "file"))
				if f == "" {
					return "", "spec check: missing required param \"file\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.specCmd([]string{"check", f}) })
			},
		},
		{
			name: "doctor", desc: "Probe the runtime (OpenCode V2) and verify the project",
			props: map[string]any{}, required: []string{},
			run: func(c CLI, args map[string]any) (string, string, int) {
				return c.mcpExec(func(sub CLI) int { return sub.doctor() })
			},
		},
		{
			name: "trace_show", desc: "Print the session fingerprint (sequence, tool, status)",
			props: props(sessionKey, sessionProp), required: []string{"session"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				s := strings.TrimSpace(mcpStr(args, "session"))
				if s == "" {
					return "", "trace show: missing required param \"session\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.traceShowCmd([]string{"--session", s}) })
			},
		},
		{
			name: "trace_sessions", desc: "List session ids with trace logs on disk",
			props: map[string]any{}, required: []string{},
			run: func(c CLI, args map[string]any) (string, string, int) {
				return c.mcpExec(func(sub CLI) int { return sub.traceSessionsCmd(nil) })
			},
		},
		{
			name: "trace_files", desc: "Print the distinct touched paths of a session in call order",
			props: props(sessionKey, sessionProp), required: []string{"session"},
			run: func(c CLI, args map[string]any) (string, string, int) {
				s := strings.TrimSpace(mcpStr(args, "session"))
				if s == "" {
					return "", "trace files: missing required param \"session\"", 2
				}
				return c.mcpExec(func(sub CLI) int { return sub.traceFilesCmd([]string{"--session", s}) })
			},
		},
	}
}

func mcpToolByName(name string) *mcpTool {
	for _, t := range mcpTools() {
		if t.name == name {
			tool := t
			return &tool
		}
	}
	return nil
}

// mcpToolList renders the registry for tools/list.
func mcpToolList() []any {
	out := make([]any, 0, len(mcpTools()))
	for _, t := range mcpTools() {
		out = append(out, map[string]any{
			"name":        t.name,
			"description": t.desc,
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": t.props,
				"required":   t.required,
			},
		})
	}
	return out
}

// mcpStr coerces one argument to a string (strings only).
func mcpStr(args map[string]any, key string) string {
	if s, ok := args[key].(string); ok {
		return s
	}
	return ""
}

// mcpStrList coerces paths/receipts-style arguments: a raw string passes
// through (the CLI splits commas itself), an array joins with commas.
func mcpStrList(args map[string]any, key string) (string, bool) {
	switch v := args[key].(type) {
	case nil:
		return "", false
	case string:
		return v, true
	case []any:
		parts := make([]string, 0, len(v))
		for _, p := range v {
			s, ok := p.(string)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ","), true
	default:
		return "", false
	}
}

// mcpTerms coerces the search terms: absent means all, a string splits on
// whitespace, an array carries the terms verbatim.
func mcpTerms(args map[string]any, key string) ([]string, bool) {
	switch v := args[key].(type) {
	case nil:
		return nil, true
	case string:
		return strings.Fields(v), true
	case []any:
		terms := make([]string, 0, len(v))
		for _, p := range v {
			s, ok := p.(string)
			if !ok {
				return nil, false
			}
			terms = append(terms, s)
		}
		return terms, true
	default:
		return nil, false
	}
}

// mcpBool coerces boolean flags (bools, plus "true"/"false" strings).
func mcpBool(args map[string]any, key string) (bool, bool) {
	switch v := args[key].(type) {
	case nil:
		return false, false
	case bool:
		return v, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return false, false
		}
		return b, true
	default:
		return false, false
	}
}

// mcpInt coerces integer arguments (JSON numbers, ints, numeric strings).
func mcpInt(args map[string]any, key string) (int, bool) {
	switch v := args[key].(type) {
	case nil:
		return 0, false
	case float64:
		return int(v), true
	case int:
		return v, true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}
