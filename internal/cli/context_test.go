// CLI tests for `regulus context` (F12.3/NOV-98): byte-identical
// usage, descriptor gating, deterministic dry-run selection and the
// conservative fallback on an empty match.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		c, out, _ := newCLI(nil)
		if code := c.Run([]string{"context", flag}); code != 0 {
			t.Fatalf("context %s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(out.String(), "usage: regulus context inventory") {
			t.Fatalf("context %s: stdout = %q", flag, out.String())
		}
	}
}

func TestContextUnknownSubcommand(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"context", "frobnicate"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("must refuse the unknown subcommand, got %q", errb.String())
	}
	if !strings.Contains(errb.String(), "usage: regulus context inventory") {
		t.Fatalf("refusal must carry the byte-identical usage, got %q", errb.String())
	}
}

func TestContextSelectNeedsDescriptor(t *testing.T) {
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"context", "select", "--domain", "x"}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus context inventory") {
		t.Fatalf("refusal must carry the byte-identical usage, got %q", errb.String())
	}
}

// writeContextFixture seeds a minimal repo root: one Go source, one
// doc and one issues file.
func writeContextFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "adr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "route.go"),
		[]byte("package route\n\n// Decide routes one deterministic routing decision.\nfunc Decide() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "adr", "001.md"),
		[]byte("# ADR-001: deterministic routing\n\nDecisions route deterministically.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues := `[{"id":"7","title":"routing decisionflake","body":"routing decision must stay deterministic","critical":true}]`
	issuesPath := filepath.Join(root, "issues.json")
	if err := os.WriteFile(issuesPath, []byte(issues), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func selectArgs(root string) []string {
	return []string{"context", "select",
		"--domain", "orchestration", "--lang", "go",
		"--complexity", "medium", "--risk", "medium",
		"--constraint", "deterministic routing",
		"--issues", filepath.Join(root, "issues.json"),
		"--dry-run", "--json"}
}

func TestContextSelectDryRunDeterministic(t *testing.T) {
	root := writeContextFixture(t)
	var first string
	for i := 0; i < 2; i++ {
		c, out, _ := newCLI(nil)
		c.Dir = root
		if code := c.Run(selectArgs(root)); code != 0 {
			t.Fatalf("select: exit = %d, want 0", code)
		}
		if first == "" {
			first = out.String()
		} else if out.String() != first {
			t.Fatal("dry-run selection is not deterministic")
		}
	}
	var sel struct {
		Essential []struct {
			ID       string `json:"id"`
			Source   string `json:"source"`
			Reason   string `json:"reason"`
			Critical bool   `json:"critical"`
		} `json:"essential"`
		Report struct {
			CriticalRecall float64 `json:"critical_recall"`
			SavedPct       float64 `json:"saved_pct"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(first), &sel); err != nil {
		t.Fatalf("select --json must print a selection: %v", err)
	}
	if sel.Report.CriticalRecall != 1 {
		t.Fatalf("critical recall = %v, want 1", sel.Report.CriticalRecall)
	}
	for _, f := range sel.Essential {
		if f.Source == "" || f.Reason == "" {
			t.Fatalf("essential fragment %s lacks source/reason", f.ID)
		}
	}
}

func TestContextSelectFallbackKeepsCritical(t *testing.T) {
	root := writeContextFixture(t)
	c, out, _ := newCLI(nil)
	c.Dir = root
	args := []string{"context", "select",
		"--domain", "orchestration", "--lang", "go",
		"--complexity", "low", "--risk", "low",
		"--constraint", "chocolate cake recipes",
		"--issues", filepath.Join(root, "issues.json"),
		"--dry-run", "--json"}
	if code := c.Run(args); code != 0 {
		t.Fatalf("select: exit = %d, want 0", code)
	}
	var sel struct {
		Essential []struct {
			ID       string   `json:"id"`
			Critical bool     `json:"critical"`
			Flags    []string `json:"flags"`
		} `json:"essential"`
	}
	if err := json.Unmarshal([]byte(out.String()), &sel); err != nil {
		t.Fatalf("select --json must print a selection: %v", err)
	}
	if len(sel.Essential) == 0 {
		t.Fatal("fallback must promote instead of delivering an empty essential layer")
	}
	seenCritical := false
	for _, f := range sel.Essential {
		if f.Critical {
			seenCritical = true
		}
	}
	if !seenCritical {
		t.Fatal("critical context must survive an unrelated query")
	}
}
