package cli

import (
	"os"
	"path/filepath"
	"testing"

	"regulus-ai/internal/sextant"
)

// The chart scaffold must be born valid: a fresh chart parses as a
// STANDARD contract with zero violations and one traced scenario.
func TestChartScaffoldIsValidContract(t *testing.T) {
	t.Chdir(t.TempDir())
	c, _, _ := newCLI(nil)
	if code := c.Run([]string{"chart", "init", "fresh"}); code != 0 {
		t.Fatalf("init: exit = %d", code)
	}
	data, err := os.ReadFile(filepath.Join("charts", "fresh.md"))
	if err != nil {
		t.Fatal(err)
	}
	class, ok := sextant.ParseClassLine(string(data))
	if !ok || class != sextant.ClassStandard {
		t.Fatalf("scaffold must declare Class STANDARD, got %v/%v", class, ok)
	}
	ct, violations := sextant.ParseContract(string(data))
	if len(violations) != 0 {
		t.Fatalf("scaffold contract must validate, got %v", violations)
	}
	if !ct.Present || len(ct.AcceptanceNames) != 1 {
		t.Fatalf("scaffold contract must trace one scenario, got %+v", ct)
	}
}
