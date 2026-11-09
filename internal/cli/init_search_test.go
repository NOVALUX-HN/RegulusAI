package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitHarnessFromKeepsSearchDefault (obscura-default-search O2):
// copyHarnessConfig must carry the search-default guidance so a
// fresh-install gets obscura as the documented default search path.
// Breaks when tool-guidance.md is dropped from the copy list (target
// file missing or without the ## search-default obscura section).
func TestInitHarnessFromKeepsSearchDefault(t *testing.T) {
	src := t.TempDir()
	oc := filepath.Join(src, ".opencode")
	guidance := "# Regulus tool guidance\n\n## search-default\n\n" +
		"Search default: obscura browser_navigate + browser_extract/markdown + " +
		"browser_links; fallback raw/bing; recipe DDG html + extract + raw.\n"
	files := map[string]string{
		filepath.Join(oc, "discipline.md"):     "# discipline\n",
		filepath.Join(oc, "tool-guidance.md"):  guidance,
		filepath.Join(oc, "agents", "r.md"):    "agent\n",
		filepath.Join(oc, "skills", "s.md"):    "skill\n",
		filepath.Join(oc, "plugins", "reg.ts"): "plugin\n",
	}
	for p, content := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(t.TempDir())
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--harness-from", src}); code != 0 {
		t.Fatalf("exit = %d, stderr %q", code, errb.String())
	}
	got, err := os.ReadFile(filepath.Join(".opencode", "tool-guidance.md"))
	if err != nil {
		t.Fatalf("tool-guidance.md must be copied: %v", err)
	}
	if !strings.Contains(string(got), "## search-default") {
		t.Fatalf("copied guidance must keep the ## search-default section, got:\n%s", got)
	}
	if !strings.Contains(strings.ToLower(string(got)), "obscura") {
		t.Fatalf("copied guidance must name obscura as the default, got:\n%s", got)
	}
}
