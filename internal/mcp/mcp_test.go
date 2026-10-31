package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"regulus-ai/internal/mcp"
)

// repoRoot resolves the repository root from this file's location, so the
// drift tests work regardless of the caller's working directory (go test
// runs with CWD set to this package's directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate this test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// cliServerSource reads internal/cli/mcp.go. Any read failure fails the
// test: a missing server is drift.
func cliServerSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "cli", "mcp.go"))
	if err != nil {
		t.Fatalf("reading internal/cli/mcp.go: %v", err)
	}
	return string(data)
}

var (
	cliToolNameRe = regexp.MustCompile(`name:\s*"([^"]+)"`)
	cliProtoRe    = regexp.MustCompile(`mcpProtocolVersion\s*=\s*"([^"]+)"`)
)

// cliToolNames extracts the tool-table names from internal/cli/mcp.go in
// source order.
func cliToolNames(t *testing.T) []string {
	t.Helper()
	matches := cliToolNameRe.FindAllStringSubmatch(cliServerSource(t), -1)
	if len(matches) == 0 {
		t.Fatal("no tool names found in internal/cli/mcp.go")
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	return names
}

func TestContractVersionPinned(t *testing.T) {
	const want = "regulus.mcp/v1"
	if mcp.ContractVersion != want {
		t.Fatalf("ContractVersion = %q, want pinned %q", mcp.ContractVersion, want)
	}
}

func TestNegotiateVersionAcceptsPinned(t *testing.T) {
	got, err := mcp.NegotiateVersion("regulus.mcp/v1")
	if err != nil {
		t.Fatalf("NegotiateVersion(pinned) error = %v", err)
	}
	if got != mcp.ContractVersion {
		t.Fatalf("NegotiateVersion(pinned) = %q, want %q", got, mcp.ContractVersion)
	}
}

func TestNegotiateVersionRefusesAnythingElse(t *testing.T) {
	cases := []string{"", "regulus.mcp/v2", "regulus.mcp/v1 ", " REGULUS.MCP/V1", "regulus.contract/v1", "2024-11-05"}
	for _, client := range cases {
		if got, err := mcp.NegotiateVersion(client); err == nil {
			t.Fatalf("NegotiateVersion(%q) = %q, want refusal (compatibility is explicit, never silent drift)", client, got)
		}
	}
}

func TestProtocolVersionPinned(t *testing.T) {
	const want = "2024-11-05"
	if mcp.ProtocolVersion != want {
		t.Fatalf("ProtocolVersion = %q, want pinned %q", mcp.ProtocolVersion, want)
	}
	m := cliProtoRe.FindStringSubmatch(cliServerSource(t))
	if m == nil {
		t.Fatal("mcpProtocolVersion not found in internal/cli/mcp.go")
	}
	if m[1] != mcp.ProtocolVersion {
		t.Fatalf("protocol drift: internal/mcp speaks %q but internal/cli serves %q", mcp.ProtocolVersion, m[1])
	}
}

func TestToolDescriptorsMatchServerTable(t *testing.T) {
	server := cliToolNames(t)
	described := mcp.ToolNames()

	if len(described) != len(server) {
		t.Fatalf("descriptor/server drift: described %d tools %v, server dispatches %d %v", len(described), described, len(server), server)
	}
	for i := range server {
		if described[i] != server[i] {
			t.Fatalf("descriptor/server drift at index %d: described %q, server dispatches %q (full: %v vs %v)", i, described[i], server[i], described, server)
		}
	}

	// Set comparison both directions: no extra, no missing, no duplicates.
	seen := map[string]int{}
	for _, n := range described {
		seen[n]++
	}
	for n, c := range seen {
		if c != 1 {
			t.Fatalf("duplicate descriptor %q x%d", n, c)
		}
	}
	serverSet := map[string]bool{}
	for _, n := range server {
		serverSet[n] = true
	}
	for _, n := range described {
		if !serverSet[n] {
			t.Fatalf("descriptor %q has no server dispatch (descriptors must expose, never invent)", n)
		}
	}
	describedSet := map[string]bool{}
	for _, n := range described {
		describedSet[n] = true
	}
	for _, n := range server {
		if !describedSet[n] {
			t.Fatalf("server tool %q has no descriptor (descriptors must track every dispatch)", n)
		}
	}
}

func TestToolDescriptorsCarryMetadataWithoutLogic(t *testing.T) {
	tools := mcp.Tools()
	if len(tools) != 12 {
		t.Fatalf("Tools() = %d entries, want 12 to mirror the server table", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" || strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool descriptor must carry name+description, got %+v", tool)
		}
		names[tool.Name] = true
	}
	// The descriptor type must stay metadata-only: no func fields that
	// could duplicate dispatch logic.
	toolType := reflect.TypeOf(mcp.Tool{})
	for i := 0; i < toolType.NumField(); i++ {
		f := toolType.Field(i)
		if f.Type.Kind() == reflect.Func {
			t.Fatalf("Tool.%s is func-typed: descriptors must expose capabilities without duplicating logic", f.Name)
		}
	}
	// Sorted-set sanity for error messages elsewhere.
	var sorted []string
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	if len(sorted) != 12 {
		t.Fatalf("want 12 unique tool names, got %v", sorted)
	}
}

func TestCapabilityDiscoveryServesDescriptors(t *testing.T) {
	disc := mcp.CapabilityDiscovery()
	if disc.ProtocolVersion != mcp.ProtocolVersion {
		t.Fatalf("Capabilities.ProtocolVersion = %q, want %q", disc.ProtocolVersion, mcp.ProtocolVersion)
	}
	if disc.ContractVersion != mcp.ContractVersion {
		t.Fatalf("Capabilities.ContractVersion = %q, want %q", disc.ContractVersion, mcp.ContractVersion)
	}
	gotNames := make([]string, 0, len(disc.Tools))
	for _, tool := range disc.Tools {
		gotNames = append(gotNames, tool.Name)
	}
	wantNames := mcp.ToolNames()
	if len(gotNames) != len(wantNames) {
		t.Fatalf("discovery tools = %v, want %v", gotNames, wantNames)
	}
	for i := range wantNames {
		if gotNames[i] != wantNames[i] {
			t.Fatalf("discovery tool %d = %q, want %q", i, gotNames[i], wantNames[i])
		}
	}
	if len(disc.Resources) != len(mcp.Resources()) {
		t.Fatalf("discovery resources = %d, want %d", len(disc.Resources), len(mcp.Resources()))
	}
	if len(disc.Prompts) != len(mcp.Prompts()) {
		t.Fatalf("discovery prompts = %d, want %d", len(disc.Prompts), len(mcp.Prompts()))
	}
}

func TestErrorContractCodes(t *testing.T) {
	if mcp.CodeParseError != -32700 {
		t.Fatalf("CodeParseError = %d, want -32700", mcp.CodeParseError)
	}
	if mcp.CodeMethodNotFound != -32601 {
		t.Fatalf("CodeMethodNotFound = %d, want -32601", mcp.CodeMethodNotFound)
	}
	if mcp.CodeInvalidParams != -32602 {
		t.Fatalf("CodeInvalidParams = %d, want -32602", mcp.CodeInvalidParams)
	}
	if mcp.CodeInternal != -32603 {
		t.Fatalf("CodeInternal = %d, want -32603", mcp.CodeInternal)
	}

	// Unknown methods must return -32601 with the stable server shape.
	err := mcp.UnknownMethod("nope_missing")
	if err.Code != -32601 {
		t.Fatalf("UnknownMethod code = %d, want -32601", err.Code)
	}
	if want := "method not found: nope_missing"; err.Message != want {
		t.Fatalf("UnknownMethod message = %q, want %q", err.Message, want)
	}

	toolErr := mcp.UnknownTool("nope_missing")
	if toolErr.Code != -32601 {
		t.Fatalf("UnknownTool code = %d, want -32601", toolErr.Code)
	}
	if want := "unknown tool: nope_missing"; toolErr.Message != want {
		t.Fatalf("UnknownTool message = %q, want %q", toolErr.Message, want)
	}

	if got := mcp.ParseError(); got.Code != -32700 || got.Message == "" {
		t.Fatalf("ParseError() = %+v, want -32700 with a message", got)
	}
	if got := mcp.InvalidParams("bad"); got.Code != -32602 || !strings.Contains(got.Message, "bad") {
		t.Fatalf("InvalidParams() = %+v, want -32602 carrying the detail", got)
	}
	if got := mcp.InternalError("boom"); got.Code != -32603 || !strings.Contains(got.Message, "boom") {
		t.Fatalf("InternalError() = %+v, want -32603 carrying the detail", got)
	}
}

func TestErrorContractRegulusRefusalMapping(t *testing.T) {
	// The CLI's usage signal (exit 2) is the only exit that becomes a
	// JSON-RPC error (-32602). A regulus refusal (exit 1) stays a content
	// reply carrying the exit code — never a protocol error.
	if code, isErr := mcp.ClassifyExit(2); !isErr || code != -32602 {
		t.Fatalf("ClassifyExit(2) = (%d,%v), want (-32602,true)", code, isErr)
	}
	for _, exit := range []int{0, 1, 3} {
		if code, isErr := mcp.ClassifyExit(exit); isErr {
			t.Fatalf("ClassifyExit(%d) = (%d,true), want protocol error only for exit 2 (refusals are content replies)", exit, code)
		}
	}
}

func TestStdioLocalTrustNoAuthHandshake(t *testing.T) {
	// stdio is local-trust: serving the discovery payload must add no
	// auth handshake. The payload carries protocol + capabilities only.
	disc := mcp.CapabilityDiscovery()
	raw, err := json.Marshal(disc)
	if err != nil {
		t.Fatal(err)
	}
	lowered := strings.ToLower(string(raw))
	for _, needle := range []string{"auth", "token", "bearer", "credential", "password", "secret"} {
		if strings.Contains(lowered, needle) {
			t.Fatalf("capability discovery must add no auth handshake, payload contains %q: %s", needle, raw)
		}
	}

	// Descriptor types carry no auth fields either: permission enforcement
	// lives with the client, never on the stdio wire.
	for _, typ := range []reflect.Type{
		reflect.TypeOf(mcp.Tool{}),
		reflect.TypeOf(mcp.Resource{}),
		reflect.TypeOf(mcp.Prompt{}),
		reflect.TypeOf(mcp.Capabilities{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, needle := range []string{"auth", "token", "bearer", "credential", "password", "secret"} {
				if strings.Contains(name, needle) {
					t.Fatalf("%s.%s carries an auth field: stdio adds no auth handshake", typ.Name(), typ.Field(i).Name)
				}
			}
		}
	}

	// tools/list entries derived from descriptors must also stay
	// auth-free.
	entries := mcp.ToolsList()
	rawEntries, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(rawEntries)), "auth") {
		t.Fatalf("tools/list payload must add no auth handshake: %s", rawEntries)
	}
}
