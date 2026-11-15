package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"regulus-ai/internal/testenv"
	"strings"
	"testing"
)

// scopeChartDoc is a closeable STANDARD chart whose machine scope is the
// given Allow:/Deny: continuation lines under - Scope:.
func scopeChartDoc(lines ...string) string {
	var b strings.Builder
	b.WriteString("# demo\n\nDomain: demo\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: worker boundary\n")
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString("- Acceptance: the happy path\n- Verification: g\n\n## Acceptance\n\n### Scenario: the happy path\n\n- Given a ready fixture\n- When the checker runs\n- Then it passes\n\n## Tasks\n\n| ID | Spec | Work | Route | Depends | Status |\n|----|------|------|-------|---------|--------|\n| T1 | the happy path | build | inline | - | todo |\n\n## Log\n\n- L1: authorized.\n")
	return b.String()
}

func scopeGit(t *testing.T) string {
	t.Helper()
	testenv.Require(t, "git")
	return "git"
}

// initScopeRepo builds a committed git fixture: chart + one in-scope
// file, .gitignore for the harness-owned .regulus dir, and a passing
// receipt so only the scope verdict varies between cases.
func initScopeRepo(t *testing.T, chartDoc string) string {
	t.Helper()
	git := scopeGit(t)
	root := t.TempDir()
	t.Chdir(root)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.MkdirAll(filepath.Join("charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("charts", "demo.md"), []byte(chartDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("internal", "ok.go"), []byte("package ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitignore", []byte(".regulus/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "init")
	writeTaskReceipt(t, "demo/T1", "build", true)
	return filepath.Join("charts", "demo.md")
}

func TestTaskCloseRefusesOutOfScopeFile(t *testing.T) {
	chart := initScopeRepo(t, scopeChartDoc("Allow: internal/"))
	before, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("outside.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 1 {
		t.Fatalf("out-of-scope worktree must block close, exit = %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "fuera de scope: outside.txt") {
		t.Fatalf("refusal must name the file, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Scope") || !strings.Contains(out.String(), "REPLAN") {
		t.Fatalf("refusal must name the fix (Scope + REPLAN), got:\n%s", out.String())
	}
	after, err := os.ReadFile(chart)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("refused close must leave the chart byte-identical")
	}
}

func TestTaskCloseAllowsInScopeFile(t *testing.T) {
	chart := initScopeRepo(t, scopeChartDoc("Allow: internal/"))
	if err := os.WriteFile(filepath.Join("internal", "new.go"), []byte("package ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("in-scope worktree must close, exit = %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "CLOSED T1") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTaskCloseLegacyChartWithoutScope(t *testing.T) {
	doc := strings.Replace(scopeChartDoc("Allow: internal/"), "  Allow: internal/\n", "", 1)
	chart := initScopeRepo(t, doc)
	if err := os.WriteFile("outside.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 0 {
		t.Fatalf("chart without machine scope must not enforce, exit = %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "CLOSED T1") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestTaskCloseRejectsInvalidScopeDeclaration(t *testing.T) {
	chart := initScopeRepo(t, scopeChartDoc("Allow: ../evil"))
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"task", "close", chart, "T1"}); code != 1 {
		t.Fatalf("invalid Scope declaration must block close, exit = %d, output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "scope: invalid Allow") {
		t.Fatalf("refusal must name the bad declaration, got:\n%s", out.String())
	}
}
