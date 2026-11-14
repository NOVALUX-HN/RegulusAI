// NOV-63: docs-drift gate. Fixture trees prove each drift type
// fails naming file:line and the clean tree passes.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDriftTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cleanDriftTree() map[string]string {
	plugin := "const x = 1;\n"
	for i := 0; i < 19; i++ {
		plugin += "      add({\n        name: \"tool" + string(rune('a'+i)) + "\",\n      });\n"
	}
	return map[string]string{
		"package.json":                 `{"name":"r","version":"0.14.0"}`,
		"versioninfo.json":             `{"FixedFileInfo":{"FileVersion":{"Major":0,"Minor":14,"Patch":0},"ProductVersion":{"Major":0,"Minor":14,"Patch":0}},"StringFileInfo":{"FileVersion":"0.14.0.0","ProductVersion":"0.14.0.0"}}`,
		"cmd/regulus/regulus.manifest": `<assembly version="0.14.0.0">`,
		"CHANGELOG.md":                 "# Log\n\n## [0.14.0] - 2026-11-14\n",
		"README.md":                    "> Status: v0.14.0\n\n![The 15-gate gauntlet](x.png)\n\n| `regulus status` | one look |\n",
		"regulus.json":                 `{"gates":[{"name":"a"},{"name":"b"},{"name":"c"},{"name":"d"},{"name":"e"},{"name":"f"},{"name":"g"},{"name":"h"},{"name":"i"},{"name":"j"},{"name":"k"},{"name":"l"},{"name":"m"},{"name":"n"},{"name":"o"}]}`,
		"docs/ok.md":                   "MCP exposes 17 tools here.\n",
		".opencode/plugins/regulus.ts": plugin,
		".devin/mcp_config.json":       `{}`,
	}
}

func TestDocDriftCleanTreePasses(t *testing.T) {
	// Real registries: 17 MCP tools, dispatch keys incl. status.
	if got := checkDocDrift(writeDriftTree(t, cleanDriftTree()), testSurface()); len(got) != 0 {
		t.Fatalf("clean tree must pass, got %v", got)
	}
}

func TestDocDriftVersionMismatch(t *testing.T) {
	files := cleanDriftTree()
	files["README.md"] = "> Status: v0.13.0\n"
	got := checkDocDrift(writeDriftTree(t, files), testSurface())
	if !containsDrift(got, "README.md Status") {
		t.Fatalf("must flag README version, got %v", got)
	}
}

func TestDocDriftGateCountMismatch(t *testing.T) {
	files := cleanDriftTree()
	files["README.md"] = "> Status: v0.14.0\n\n![The 12-gate gauntlet](x.png)\n"
	got := checkDocDrift(writeDriftTree(t, files), testSurface())
	if !containsDrift(got, "12-gate") {
		t.Fatalf("must flag stale gate legend, got %v", got)
	}
}

func TestDocDriftToolCountMismatch(t *testing.T) {
	files := cleanDriftTree()
	files["docs/ok.md"] = "MCP exposes 18 tools here.\n"
	if got := checkDocDrift(writeDriftTree(t, files), testSurface()); !containsDrift(got, "18 tools") {
		t.Fatalf("must flag MCP count, got %v", got)
	}
	files["docs/ok.md"] = "plugin surface is 20 tools wide.\n"
	if got := checkDocDrift(writeDriftTree(t, files), testSurface()); !containsDrift(got, "20 tools") {
		t.Fatalf("must flag plugin count, got %v", got)
	}
}

func TestDocDriftRuntimeClaim(t *testing.T) {
	files := cleanDriftTree()
	files["README.md"] = "> Status: v0.14.0\n\nOnly supported runtime is OpenCode.\n"
	if got := checkDocDrift(writeDriftTree(t, files), testSurface()); !containsDrift(got, "only supported runtime") {
		t.Fatalf("must flag stale runtime claim, got %v", got)
	}
}

func TestDocDriftUnknownCommand(t *testing.T) {
	files := cleanDriftTree()
	files["README.md"] = "> Status: v0.14.0\n\n| `regulus frobnicate` | does things |\n"
	if got := checkDocDrift(writeDriftTree(t, files), testSurface()); !containsDrift(got, "frobnicate") {
		t.Fatalf("must flag unknown command, got %v", got)
	}
}

func TestDocDriftH10Skipped(t *testing.T) {
	files := cleanDriftTree()
	files["docs/old.md"] = "> **Nota (H10):** frozen record.\n\nOnly supported runtime. 12-gate, 99 tools.\n"
	if got := checkDocDrift(writeDriftTree(t, files), testSurface()); len(got) != 0 {
		t.Fatalf("H10 file must be skipped, got %v", got)
	}
}

func testSurface() map[string]bool {
	return map[string]bool{"status": true, "gate": true, "contract": true, "serve": true}
}

func containsDrift(got []string, want string) bool {
	for _, g := range got {
		if strings.Contains(g, want) {
			return true
		}
	}
	return false
}
