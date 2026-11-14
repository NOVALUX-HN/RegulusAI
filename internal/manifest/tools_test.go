package manifest

// NOV-25 canonical tool manifest: tools.json is the single source of
// truth for MCP/plugin dispatch. These tests pin its shape (version 1,
// 14 MCP tools, no duplicates, 5 intentional gaps with reasons) and its
// agreement with the descriptor registry in internal/mcp.

import (
	"encoding/json"
	"os"
	"testing"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/mcp"
)

type toolsFileEntry struct {
	Name     string   `json:"name"`
	Required []string `json:"required"`
}

type toolsFile struct {
	Version         int              `json:"version"`
	MCP             []toolsFileEntry `json:"mcp"`
	IntentionalGaps []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"intentional_gaps"`
}

func loadToolsFile(t *testing.T) toolsFile {
	t.Helper()
	raw, err := os.ReadFile("tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var f toolsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("tools.json: malformed: %v", err)
	}
	return f
}

// TestManifestToolsJSONValid pins the canonical shape: version 1, 14 MCP
// tools with unique names, 5 intentional gaps with non-empty reasons.
func TestManifestToolsJSONValid(t *testing.T) {
	f := loadToolsFile(t)
	if f.Version != 1 {
		t.Fatalf("tools.json version = %d, want 1", f.Version)
	}
	if len(f.MCP) != 14 {
		t.Fatalf("tools.json mcp = %d tools, want 14", len(f.MCP))
	}
	seen := map[string]int{}
	for _, e := range f.MCP {
		if e.Name == "" {
			t.Fatal("tools.json mcp holds an empty name")
		}
		seen[e.Name]++
	}
	for name, n := range seen {
		if n != 1 {
			t.Fatalf("duplicate mcp tool %q x%d", name, n)
		}
	}
	if len(f.IntentionalGaps) != 5 {
		t.Fatalf("tools.json intentional_gaps = %d, want 5", len(f.IntentionalGaps))
	}
	wantGaps := map[string]bool{
		"evolve_propose": true, "probe_contrast": true,
		"probe_compose": true, "skills_list": true,
		"evolve_tick": true,
	}
	for _, g := range f.IntentionalGaps {
		if !wantGaps[g.Name] {
			t.Fatalf("unexpected intentional gap %q", g.Name)
		}
		if g.Reason == "" {
			t.Fatalf("intentional gap %q needs a reason", g.Name)
		}
		delete(wantGaps, g.Name)
	}
	for name := range wantGaps {
		t.Fatalf("missing intentional gap %q", name)
	}
}

// TestManifestVersionMapsToSupportedContracts pins the NOV-37 wiring:
// tools.json version 1 means the pinned contract, mcp contract and wire
// protocol — all three must be in their Supported sets (explicit refuse
// otherwise, never silent fallback).
func TestManifestVersionMapsToSupportedContracts(t *testing.T) {
	f := loadToolsFile(t)
	if f.Version != 1 {
		t.Fatalf("tools.json version = %d, want 1", f.Version)
	}
	if !contract.Supports(contract.Version) {
		t.Fatalf("contract %q not in Supported %v", contract.Version, contract.Supported)
	}
	if !mcp.SupportsContract(mcp.ContractVersion) {
		t.Fatalf("mcp contract %q not in SupportedContracts %v", mcp.ContractVersion, mcp.SupportedContracts)
	}
	if !mcp.SupportsProtocol(mcp.ProtocolVersion) {
		t.Fatalf("protocol %q not in SupportedProtocols %v", mcp.ProtocolVersion, mcp.SupportedProtocols)
	}
}

// TestManifestToolsMatchDescriptors proves the canonical JSON lists
// exactly the tools internal/mcp describes, in dispatch order.
func TestManifestToolsMatchDescriptors(t *testing.T) {
	f := loadToolsFile(t)
	var names []string
	for _, e := range f.MCP {
		names = append(names, e.Name)
	}
	want := mcp.ToolNames()
	if len(names) != len(want) {
		t.Fatalf("tools.json mcp = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tools.json mcp[%d] = %q, want %q (full: %v vs %v)", i, names[i], want[i], names, want)
		}
	}
}
