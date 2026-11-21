// CLI tests (F12.8/NOV-103): wiring, messages and exit codes for
// `regulus impact ...`. The engine owns the semantics (see
// internal/impact); here the dispatch, the required flags and the
// byte-identical surfaces are pinned.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func impactFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/demo\n\ngo 1.27\n")
	write("internal/gates/g.go", "package gates\n\nfunc Check() bool { return true }\n")
	write("internal/gates/g_test.go", "package gates\n\nimport \"testing\"\n\nfunc TestCheck(t *testing.T) {}\n")
	write("internal/app/app.go", "package app\n\nimport \"example.com/demo/internal/gates\"\n\nfunc Run() bool { return gates.Check() }\n")
	write("internal/app/app_test.go", "package app\n\nimport \"testing\"\n\nfunc TestRun(t *testing.T) {}\n")
	return dir
}

func TestImpactHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		c, out, errb := newCLI(nil)
		if code := c.Run([]string{"impact", flag}); code != 0 {
			t.Fatalf("impact %s: exit = %d, want 0 (stderr: %q)", flag, code, errb.String())
		}
		if !strings.Contains(out.String(), "usage: regulus impact") {
			t.Fatalf("impact %s: stdout = %q", flag, out.String())
		}
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"impact", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("unknown subcommand must be refused: %q", errb.String())
	}
}

func TestImpactAffectRequiresChanged(t *testing.T) {
	dir := impactFixture(t)
	t.Chdir(dir)
	for _, argv := range [][]string{
		{"impact", "affect"},
		{"impact", "affect", "--severity", "2"},
		{"impact", "suite"},
	} {
		c, _, _ := newCLI(nil)
		if code := c.Run(argv); code != 2 {
			t.Fatalf("%v: exit = %d, want 2", argv, code)
		}
	}
}

func TestImpactAffectAndSuite(t *testing.T) {
	dir := impactFixture(t)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"impact", "affect", "--changed", "internal/gates/g.go"}); code != 0 {
		t.Fatalf("affect exit = %d", code)
	}
	got := out.String()
	for _, want := range []string{"consumers: pkg:internal/app", "critical: pkg:internal/gates", "risk:", "high", "tests: test:internal/app, test:internal/gates"} {
		if !strings.Contains(got, want) {
			t.Fatalf("affect must contain %q, got:\n%s", want, got)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"impact", "suite", "--changed", "internal/gates/g.go", "--failed", "test:internal/app"}); code != 0 {
		t.Fatalf("suite exit = %d", code)
	}
	got2 := out2.String()
	for _, want := range []string{"focused: test:internal/app, test:internal/gates", "precision: 0.50", "recall: 1.00"} {
		if !strings.Contains(got2, want) {
			t.Fatalf("suite must contain %q, got:\n%s", want, got2)
		}
	}
}

func TestImpactSuiteWidens(t *testing.T) {
	dir := impactFixture(t)
	t.Chdir(dir)
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"impact", "suite", "--changed", "zzz/desconocido.go"}); code != 0 {
		t.Fatalf("suite exit = %d", code)
	}
	if !strings.Contains(out.String(), "[ampliada]") {
		t.Fatalf("unknown change must widen, got:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "ampliada") {
		t.Fatalf("widen must warn on stderr, got %q", errb.String())
	}
}

func TestImpactCyclesBreakingBenchmark(t *testing.T) {
	dir := impactFixture(t)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"impact", "cycles"}); code != 0 {
		t.Fatalf("cycles exit = %d", code)
	}
	if !strings.Contains(out.String(), "cycles:") || !strings.Contains(out.String(), "hidden:") {
		t.Fatalf("cycles output = %q", out.String())
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"impact", "inventory", "--json"}); code != 0 {
		t.Fatalf("inventory exit = %d", code)
	}
	before := filepath.Join(dir, "before.json")
	if err := os.WriteFile(before, []byte(out2.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	c3, out3, _ := newCLI(nil)
	if code := c3.Run([]string{"impact", "breaking", "--before", before}); code != 0 {
		t.Fatalf("breaking exit = %d", code)
	}
	if !strings.Contains(out3.String(), "breaking: none") {
		t.Fatalf("breaking output = %q", out3.String())
	}
	c4, out4, _ := newCLI(nil)
	if code := c4.Run([]string{"impact", "benchmark"}); code != 0 {
		t.Fatalf("benchmark exit = %d", code)
	}
	if !strings.Contains(out4.String(), "decisión: adoptar grafo propio") {
		t.Fatalf("benchmark output = %q", out4.String())
	}
}
