package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"regulus-ai/internal/mcp"
)

// TestManifestCheckPasses runs the real gate on the clean tree: the
// canonical tools.json must agree with both code registries.
// manifestTestRoot anchors the repo root to this test file, never to the
// process cwd: the workspace runner executes compiled test binaries from
// the repo root while `go test` runs them from the package dir.
func manifestTestRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestManifestCheckPasses(t *testing.T) {
	root := manifestTestRoot(t)
	c, out, errb := newCLI(nil)
	c.Dir = root
	if code := c.Run([]string{"manifest", "check"}); code != 0 {
		t.Fatalf("manifest check: exit = %d, want 0; stderr %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "manifest OK: 17 tools + 2 intentional gaps") {
		t.Fatalf("manifest check output = %q, want the OK line", out.String())
	}
}

// TestManifestCheckUsage pins the subcommand shape: bare and unknown
// subcommands are usage errors, never silent passes.
func TestManifestCheckUsage(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"manifest"}); code != 2 {
		t.Fatalf("bare manifest: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus manifest check") {
		t.Fatalf("bare manifest stderr = %q", errb.String())
	}
	c2, _, errb2 := newCLI(nil)
	if code := c2.Run([]string{"manifest", "frobnicate"}); code != 2 {
		t.Fatalf("unknown manifest subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb2.String(), "usage: regulus manifest check") {
		t.Fatalf("unknown manifest stderr = %q", errb2.String())
	}
}

// TestManifestCheckRejectsUnsupportedVersion pins the NOV-37 wiring:
// tools.json version 1 maps to the supported contract/mcp/protocol.
// Any other version fails with a message naming the supported set.
func TestManifestCheckRejectsUnsupportedVersion(t *testing.T) {
	root := manifestTestRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "internal", "manifest", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file manifestToolsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	file.Version = 2
	tampered, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	problems := checkToolManifest(tampered, mcpTools(), mcp.ToolNames())
	found := false
	for _, p := range problems {
		if strings.Contains(p, "unsupported manifest version") {
			found = true
		}
	}
	if !found {
		t.Fatalf("unsupported version must be refused, got %v", problems)
	}
}

// TestManifestCheckDetectsRequiredMismatch tampers one required list and
// proves the pure comparator names it (RED-capable without touching the
// tree: the failure lists the tool and the mismatch kind).
func TestManifestCheckDetectsRequiredMismatch(t *testing.T) {
	root := manifestTestRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "internal", "manifest", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file manifestToolsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	for i := range file.MCP {
		if file.MCP[i].Name == "memory_note" {
			file.MCP[i].Required = []string{}
		}
	}
	tampered, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	problems := checkToolManifest(tampered, mcpTools(), mcp.ToolNames())
	found := false
	for _, p := range problems {
		if strings.Contains(p, "memory_note") && strings.Contains(p, "required mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("tampered required must be reported, got %v", problems)
	}
}

// TestManifestCheckDetectsMissingTool drops one entry and proves the
// comparator reports it as missing (exit-1 path without tree writes).
func TestManifestCheckDetectsMissingTool(t *testing.T) {
	root := manifestTestRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "internal", "manifest", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file manifestToolsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	kept := file.MCP[:0]
	for _, e := range file.MCP {
		if e.Name != "doctor" {
			kept = append(kept, e)
		}
	}
	file.MCP = kept
	tampered, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	problems := checkToolManifest(tampered, mcpTools(), mcp.ToolNames())
	found := false
	for _, p := range problems {
		if strings.Contains(p, "doctor") && strings.Contains(p, "missing in manifest") {
			found = true
		}
	}
	if !found {
		t.Fatalf("dropped tool must be reported as missing, got %v", problems)
	}
}
