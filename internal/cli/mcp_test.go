package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/doctor"
)

type mcpResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  map[string]any  `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func mcpTestCLI() CLI {
	return CLI{
		Version: "test",
		Probe: func() doctor.Result {
			return doctor.Result{Reason: "stub: no runtime for tests"}
		},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
}

func mcpCall(id int, method string, params any) string {
	m := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(raw) + "\n"
}

func mcpNotify(method string, params any) string {
	m := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		m["params"] = params
	}
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(raw) + "\n"
}

func runMCP(c CLI, input string) []mcpResp {
	var out bytes.Buffer
	c.serveMCP(strings.NewReader(input), &out)
	text := strings.TrimSpace(out.String())
	if text == "" {
		return nil
	}
	var resps []mcpResp
	for _, line := range strings.Split(text, "\n") {
		var r mcpResp
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			panic("server wrote malformed JSON: " + line)
		}
		resps = append(resps, r)
	}
	return resps
}

func mcpResultText(t *testing.T, r mcpResp) string {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("want result, got error %+v", r.Error)
	}
	content, ok := r.Result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result has no content: %v", r.Result)
	}
	first, ok := content[0].(map[string]any)
	if !ok || first["type"] != "text" {
		t.Fatalf("first content block is not text: %v", content[0])
	}
	text, _ := first["text"].(string)
	return text
}

func TestMCPInitializeHandshake(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "initialize", map[string]any{}))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	r := resps[0]
	if r.JSONRPC != "2.0" || string(r.ID) != "1" {
		t.Fatalf("bad envelope: %+v", r)
	}
	if r.Error != nil {
		t.Fatalf("initialize failed: %+v", r.Error)
	}
	if got := r.Result["protocolVersion"]; got != "2024-11-05" {
		t.Fatalf("protocolVersion = %v, want 2024-11-05", got)
	}
	caps, ok := r.Result["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing: %v", r.Result)
	}
	if _, ok := caps["tools"]; !ok {
		t.Fatalf("capabilities.tools missing: %v", caps)
	}
	info, ok := r.Result["serverInfo"].(map[string]any)
	if !ok || info["name"] != "regulus" || info["version"] != "test" {
		t.Fatalf("serverInfo = %v, want {regulus test}", r.Result["serverInfo"])
	}
}

func TestMCPInitializeExposesContract(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "initialize", map[string]any{}))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	info, ok := resps[0].Result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("serverInfo missing: %v", resps[0].Result)
	}
	if info["contract"] != contract.Version {
		t.Fatalf("serverInfo.contract = %v, want %q", info["contract"], contract.Version)
	}
}

func TestMCPToolsListNames(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "tools/list", nil))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	raw, err := json.Marshal(resps[0].Result["tools"])
	if err != nil {
		t.Fatal(err)
	}
	var tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"inputSchema"`
	}
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"memory_note", "memory_show", "memory_save", "memory_search",
		"memory_context", "memory_history", "gate_run", "spec_check",
		"doctor", "trace_show", "trace_sessions", "trace_files",
	}
	if len(tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(tools), len(want))
	}
	for i, name := range want {
		if tools[i].Name != name {
			t.Fatalf("tool %d = %q, want %q", i, tools[i].Name, name)
		}
		if tools[i].Description == "" {
			t.Fatalf("tool %q has no description", name)
		}
		schema := tools[i].InputSchema
		if schema["type"] != "object" || schema["properties"] == nil || schema["required"] == nil {
			t.Fatalf("tool %q inputSchema must carry type/properties/required: %v", name, schema)
		}
	}
}

func TestMCPIntentionalGapPinsPluginDelta(t *testing.T) {
	// The OpenCode agent surface is 16 tools; MCP deliberately exposes
	// 12. The 4-gap (evolve_propose, probe_contrast, probe_compose,
	// skills_list) is a Devin-side policy decision, not drift: evolving
	// this list means updating docs/devin.md too.
	plugin := []string{
		"gate_run", "spec_check", "doctor", "memory_note",
		"memory_show", "memory_save", "memory_search", "memory_context",
		"memory_history", "evolve_propose", "probe_contrast", "trace_show",
		"skills_list", "trace_sessions", "trace_files", "probe_compose",
	}
	mcp := map[string]bool{}
	for _, tool := range mcpTools() {
		mcp[tool.name] = true
	}
	if len(mcp) != 12 {
		t.Fatalf("MCP tools = %d, want 12", len(mcp))
	}
	var gap []string
	for _, name := range plugin {
		if !mcp[name] {
			gap = append(gap, name)
		}
	}
	want := map[string]bool{
		"evolve_propose": true, "probe_contrast": true,
		"probe_compose": true, "skills_list": true,
	}
	if len(gap) != len(want) {
		t.Fatalf("plugin-MCP gap = %v, want %v", gap, want)
	}
	for _, name := range gap {
		if !want[name] {
			t.Fatalf("plugin-MCP gap = %v, want %v", gap, want)
		}
	}
}

func TestMCPMemoryLifecycle(t *testing.T) {
	t.Chdir(t.TempDir())
	c := mcpTestCLI()

	save := func(id int, args map[string]any) string {
		resps := runMCP(c, mcpCall(id, "tools/call", map[string]any{
			"name": "memory_save", "arguments": args,
		}))
		if len(resps) != 1 {
			t.Fatalf("save: want 1 response, got %d", len(resps))
		}
		return mcpResultText(t, resps[0])
	}

	if got := save(1, map[string]any{
		"type": "decision", "title": "MCP lifecycle probe",
		"content": "the quick brown regulus fox", "topic_key": "mcp-k1",
	}); !strings.Contains(got, "observation saved") {
		t.Fatalf("save text = %q", got)
	}
	if got := save(2, map[string]any{
		"type": "discovery", "title": "MCP lifecycle followup",
		"content": "regulus fox followup detail", "topic_key": "mcp-k1",
	}); !strings.Contains(got, "observation saved") {
		t.Fatalf("save text = %q", got)
	}

	resps := runMCP(c, mcpCall(3, "tools/call", map[string]any{
		"name": "memory_search", "arguments": map[string]any{"terms": []string{"regulus"}},
	}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "MCP lifecycle probe") {
		t.Fatalf("search text = %q", text)
	}

	resps = runMCP(c, mcpCall(4, "tools/call", map[string]any{
		"name": "memory_note", "arguments": map[string]any{"text": "session hello from mcp"},
	}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "note saved") {
		t.Fatalf("note text = %q", text)
	}

	resps = runMCP(c, mcpCall(5, "tools/call", map[string]any{"name": "memory_show"}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "session hello from mcp") {
		t.Fatalf("show text = %q", text)
	}

	resps = runMCP(c, mcpCall(6, "tools/call", map[string]any{
		"name": "memory_history", "arguments": map[string]any{"topic_key": "mcp-k1"},
	}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "MCP lifecycle followup") {
		t.Fatalf("history text = %q", text)
	}

	resps = runMCP(c, mcpCall(7, "tools/call", map[string]any{"name": "memory_context"}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "Almanac") {
		t.Fatalf("context text = %q", text)
	}
}

func TestMCPUnknownTool(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "tools/call", map[string]any{
		"name": "nope_missing", "arguments": map[string]any{},
	}))
	if len(resps) != 1 || resps[0].Error == nil || resps[0].Error.Code != -32601 {
		t.Fatalf("want -32601, got %+v", resps)
	}
}

func TestMCPBadParams(t *testing.T) {
	c := mcpTestCLI()
	input := mcpCall(1, "tools/call", map[string]any{"name": "memory_note", "arguments": map[string]any{}}) +
		mcpCall(2, "tools/call", map[string]any{"name": "memory_history", "arguments": map[string]any{}})
	resps := runMCP(c, input)
	if len(resps) != 2 {
		t.Fatalf("want 2 responses, got %d", len(resps))
	}
	for i, r := range resps {
		if r.Error == nil || r.Error.Code != -32602 {
			t.Fatalf("response %d: want -32602, got %+v", i, r)
		}
	}
}

func TestMCPMalformedLineKeepsServing(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, "this is not json\n"+mcpCall(9, "initialize", map[string]any{}))
	if len(resps) != 2 {
		t.Fatalf("want 2 responses, got %d", len(resps))
	}
	if resps[0].Error == nil || resps[0].Error.Code != -32700 {
		t.Fatalf("first: want -32700, got %+v", resps[0])
	}
	if resps[1].Error != nil || resps[1].Result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("server did not keep serving: %+v", resps[1])
	}
}

func TestMCPNotificationSwallowed(t *testing.T) {
	c := mcpTestCLI()
	resps := runMCP(c, mcpNotify("notifications/initialized", map[string]any{})+mcpCall(1, "tools/list", nil))
	if len(resps) != 1 {
		t.Fatalf("want 1 response (notification swallowed), got %d", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("tools/list failed: %+v", resps[0].Error)
	}
}

func TestMCPDoctorWithoutOpencode(t *testing.T) {
	t.Chdir(t.TempDir())
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "tools/call", map[string]any{"name": "doctor"}))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "stub: no runtime for tests") {
		t.Fatalf("doctor text = %q", text)
	}
}

func TestContractWithoutOpencode(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"contract"}); code != 0 {
		t.Fatalf("contract exit = %d, want 0", code)
	}
	if got := out.String(); got != contract.Version+"\n" {
		t.Fatalf("contract output = %q", got)
	}
}

func TestMCPSpecCheckViolation(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("# Bad\n\nno acceptance here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "good.md")
	if err := os.WriteFile(good, []byte("# Good\n\n## Acceptance\n\n### Scenario: happy\n\n- Given a server\n- When it serves\n- Then it passes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := mcpTestCLI()
	resps := runMCP(c, mcpCall(1, "tools/call", map[string]any{
		"name": "spec_check", "arguments": map[string]any{"file": bad},
	}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "Acceptance") {
		t.Fatalf("violation text = %q", text)
	}
	resps = runMCP(c, mcpCall(2, "tools/call", map[string]any{
		"name": "spec_check", "arguments": map[string]any{"file": good},
	}))
	if text := mcpResultText(t, resps[0]); !strings.Contains(text, "spec OK") {
		t.Fatalf("ok text = %q", text)
	}
}

func TestMCPTraceRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	c := mcpTestCLI()
	rec, _, _ := newCLI(nil)
	if code := rec.Run([]string{"trace", "record", "--session", "s1", "--message", "m1", "--call", "c1", "--tool", "Read", "--status", "completed", "--files", "a.go,b.go"}); code != 0 {
		t.Fatalf("trace record exit = %d", code)
	}
	call := func(id int, name string, args map[string]any) string {
		t.Helper()
		resps := runMCP(c, mcpCall(id, "tools/call", map[string]any{"name": name, "arguments": args}))
		if len(resps) != 1 {
			t.Fatalf("%s: want 1 response, got %d", name, len(resps))
		}
		return mcpResultText(t, resps[0])
	}
	if got := call(1, "trace_show", map[string]any{"session": "s1"}); !strings.Contains(got, "Read") {
		t.Fatalf("trace_show = %q", got)
	}
	if got := call(2, "trace_files", map[string]any{"session": "s1"}); !strings.Contains(got, "a.go") {
		t.Fatalf("trace_files = %q", got)
	}
	if got := call(3, "trace_sessions", map[string]any{}); !strings.Contains(got, "s1") {
		t.Fatalf("trace_sessions = %q", got)
	}
}

func TestMCPGateRunPassReportsVerdicts(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"echo hi"},
		{"name":"unit-tests","layer":2,"command":"echo ok"}
	]}`)
	t.Chdir(dir)
	c := mcpTestCLI()
	c.Runner = &fakeGateRunner{results: map[string]fakeGateResult{
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	resps := runMCP(c, mcpCall(1, "tools/call", map[string]any{
		"name": "gate_run", "arguments": map[string]any{"task": "demo/T1"},
	}))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	text := mcpResultText(t, resps[0])
	if text == "" {
		t.Fatal("gate_run content must never be empty")
	}
	if !strings.Contains(text, "gauntlet: 2/2 gates passed") {
		t.Fatalf("pass content must carry the verdicts, got %q", text)
	}
}

func TestMCPGateRunFailureReportsExitCode(t *testing.T) {
	dir := writeGatesProject(t, `{"gates":[
		{"name":"typecheck","layer":1,"command":"boom"},
		{"name":"unit-tests","layer":2,"command":"echo ok"}
	]}`)
	t.Chdir(dir)
	c := mcpTestCLI()
	c.Runner = &fakeGateRunner{results: map[string]fakeGateResult{
		"boom":                        {code: 1, stderr: "type error"},
		"git rev-parse --verify HEAD": {stdout: "abc123\n"},
	}}
	resps := runMCP(c, mcpCall(1, "tools/call", map[string]any{
		"name": "gate_run", "arguments": map[string]any{"task": "demo/T1"},
	}))
	if len(resps) != 1 {
		t.Fatalf("want 1 response, got %d", len(resps))
	}
	// A failing gate is a content reply (not a protocol error) carrying
	// the non-zero exit code — never an empty string or null.
	if resps[0].Error != nil {
		t.Fatalf("gate failure must be a content reply, got error %+v", resps[0].Error)
	}
	text := mcpResultText(t, resps[0])
	if text == "" {
		t.Fatal("gate_run failure content must never be empty")
	}
	if !strings.Contains(text, "exit code: 1") {
		t.Fatalf("failure content must carry the exit code, got %q", text)
	}
	if !strings.Contains(text, "stopped at gate") {
		t.Fatalf("failure content must carry the stop verdict, got %q", text)
	}
}

func TestMCPContentNeverEmpty(t *testing.T) {
	if got := mcpContent("", "", 0); got == "" {
		t.Fatal("empty success must not stay empty")
	}
	got := mcpContent("", "", 1)
	if got == "" || !strings.Contains(got, "exit code: 1") {
		t.Fatalf("empty failure must carry the exit code, got %q", got)
	}
	if got := mcpContent("layer 1 typecheck pass (3ms)", "", 0); !strings.Contains(got, "pass") {
		t.Fatalf("success content must pass through, got %q", got)
	}
}

func TestMCPContentBoundsLargeOutputWithSha(t *testing.T) {
	big := strings.Repeat("x", 20000)
	got := mcpContent(big, "", 1)
	if len(got) >= len(big) {
		t.Fatalf("large output must be bounded, got %d bytes", len(got))
	}
	if !strings.Contains(got, "sha256:") {
		t.Fatalf("bounded output must carry its sha, got %.120q…", got)
	}
	if !strings.Contains(got, "exit code: 1") {
		t.Fatalf("bounded failure must still carry the exit code, got %.120q…", got)
	}
}

func TestMCPUsageLine(t *testing.T) {
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"help"}); code != 0 {
		t.Fatalf("help exit = %d", code)
	}
	if !strings.Contains(out.String(), "mcp") {
		t.Fatal("help must mention the mcp command")
	}
}

func TestMCPIntCoercion(t *testing.T) {
	cases := []struct {
		name   string
		args   map[string]any
		want   int
		wantOK bool
	}{
		{"missing", map[string]any{}, 0, false},
		{"nil", map[string]any{"limit": nil}, 0, false},
		{"float64", map[string]any{"limit": float64(7)}, 7, true},
		{"int", map[string]any{"limit": 5}, 5, true},
		{"numeric string", map[string]any{"limit": "42"}, 42, true},
		{"padded numeric string", map[string]any{"limit": "  12  "}, 12, true},
		{"bad string", map[string]any{"limit": "abc"}, 0, false},
		{"unsupported type", map[string]any{"limit": true}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mcpInt(tc.args, "limit")
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("mcpInt(%v) = (%d,%v), want (%d,%v)", tc.args["limit"], got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
