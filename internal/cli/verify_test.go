// CLI tests (F13.4/NOV-107): wiring, messages and exit codes for
// `regulus verify ...`. The engine owns the semantics (see
// internal/verify); here the dispatch, the required flags and the
// allow/block exits are pinned.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func verifyFixture(t *testing.T) string {
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
	write("internal/b/b.go", "package b\n\nfunc Shout() string { return \"hi\" }\n")
	write("internal/b/b_test.go", "package b\n\nimport \"testing\"\n\nfunc TestShout(t *testing.T) {}\n")
	return dir
}

func TestVerifyHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		c, out, _ := newCLI(nil)
		if code := c.Run([]string{"verify", flag}); code != 0 {
			t.Fatalf("verify %s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(out.String(), "usage: regulus verify") {
			t.Fatalf("verify %s: stdout = %q", flag, out.String())
		}
	}
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"verify", "frobnicate"}); code != 2 {
		t.Fatalf("unknown subcommand: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), `unknown subcommand "frobnicate"`) {
		t.Fatalf("unknown subcommand must be refused: %q", errb.String())
	}
}

func TestVerifyPlanRequiresChanged(t *testing.T) {
	dir := verifyFixture(t)
	t.Chdir(dir)
	for _, argv := range [][]string{
		{"verify", "plan"},
		{"verify", "run"},
		{"verify", "report"},
		{"verify", "rerun", "--dir", "x"},
	} {
		c, _, _ := newCLI(nil)
		if code := c.Run(argv); code != 2 {
			t.Fatalf("%v: exit = %d, want 2", argv, code)
		}
	}
}

func TestVerifyPlanAndMatrix(t *testing.T) {
	dir := verifyFixture(t)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"verify", "plan", "--changed", "internal/b/b.go"}); code != 0 {
		t.Fatalf("plan exit = %d", code)
	}
	for _, want := range []string{"verify plan (task", "riesgo:", "suite:", "contraste:", "gates ("} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("plan must contain %q, got:\n%s", want, out.String())
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"verify", "matrix"}); code != 0 {
		t.Fatalf("matrix exit = %d", code)
	}
	if !strings.Contains(out2.String(), "BLOCKED/SKIPPED/UNKNOWN") {
		t.Fatalf("matrix must document the never-PASS rule, got:\n%s", out2.String())
	}
}

func TestVerifyRunBlocksOnInjectedFailure(t *testing.T) {
	dir := verifyFixture(t)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"verify", "run", "--changed", "internal/b/b.go", "--fail", "unit:panic en TestShout"})
	if code != 1 {
		t.Fatalf("run with FAIL: exit = %d, want 1 (block)", code)
	}
	if !strings.Contains(out.String(), "FAIL / block") {
		t.Fatalf("run must decide FAIL/block, got:\n%s", out.String())
	}
}

func TestVerifyRunUnknownBlocksWithoutEvidence(t *testing.T) {
	dir := verifyFixture(t)
	t.Chdir(dir)
	c, _, _ := newCLI(nil)
	// Sin evidencia registrada todo gate requerido es UNKNOWN → bloquea.
	if code := c.Run([]string{"verify", "run", "--changed", "internal/b/b.go"}); code != 1 {
		t.Fatalf("run without evidence: exit = %d, want 1 (UNKNOWN blocks)", code)
	}
}

func TestVerifyRunNoToolchainBlocks(t *testing.T) {
	dir := verifyFixture(t)
	t.Chdir(dir)
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"verify", "run", "--changed", "internal/b/b.go", "--no-toolchain"})
	if code != 1 {
		t.Fatalf("run --no-toolchain: exit = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "BLOCKED") {
		t.Fatalf("run --no-toolchain must report BLOCKED, got:\n%s", out.String())
	}
}
