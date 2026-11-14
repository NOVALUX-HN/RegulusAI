package sextant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopeDoc builds a chart whose ## Contract Scope carries the given
// Allow:/Deny: continuation lines (the documented docs/sextant.md form).
func scopeDoc(lines ...string) string {
	var b strings.Builder
	b.WriteString("# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: worker boundary\n")
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString("- Acceptance: s\n- Verification: g\n\n## Acceptance\n\n### Scenario: s\n\n- Given a ready fixture\n- When the checker runs\n- Then it passes\n")
	return b.String()
}

func mustParseScope(t *testing.T, doc string) Scope {
	t.Helper()
	sc, err := ParseScope(doc)
	if err != nil {
		t.Fatalf("ParseScope: %v", err)
	}
	return sc
}

func TestScopeNoDeclarationMeansNoEnforcement(t *testing.T) {
	for _, doc := range []string{
		"# F\n\nClass: STANDARD\n",
		"# F\n\nClass: STANDARD\n\n## Contract\n\n- Objective: x\n- Scope: prose only\n",
	} {
		sc, err := ParseScope(doc)
		if err != nil {
			t.Fatalf("ParseScope: %v", err)
		}
		if sc.Declared {
			t.Fatalf("prose-only Scope must not declare machine scope")
		}
		// Legacy charts keep working: even hostile lexemes pass without
		// a declared scope.
		if err := sc.CheckScopeFiles([]string{"../x", "/abs", `C:\evil`, "anything.go"}); err != nil {
			t.Fatalf("undeclared scope must not enforce, got %v", err)
		}
	}
}

func TestScopeAllowBasic(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Allow: internal/sextant"))
	if !sc.Declared {
		t.Fatal("Allow line must declare the scope")
	}
	for _, ok := range []string{"internal/sextant/scope.go", "internal/sextant/sub/deep.go"} {
		if err := sc.CheckScopeFiles([]string{ok}); err != nil {
			t.Fatalf("in-scope file %q must pass, got %v", ok, err)
		}
	}
	err := sc.CheckScopeFiles([]string{"internal/cli/task.go"})
	if err == nil || !strings.Contains(err.Error(), "fuera de scope: internal/cli/task.go") {
		t.Fatalf("out-of-scope file must name itself, got %v", err)
	}
}

func TestScopeDenyWinsOverAllow(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Allow: internal", "Deny: internal/secret"))
	if err := sc.CheckScopeFiles([]string{"internal/other/f.go"}); err != nil {
		t.Fatalf("allowed file must pass, got %v", err)
	}
	err := sc.CheckScopeFiles([]string{"internal/secret/f.go"})
	if err == nil || !strings.Contains(err.Error(), "fuera de scope: internal/secret/f.go") {
		t.Fatalf("denied file must be refused by name, got %v", err)
	}
}

func TestScopeDenyWithoutAllow(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Deny: internal/secret"))
	if err := sc.CheckScopeFiles([]string{"docs/guide.md"}); err != nil {
		t.Fatalf("empty Allow means everything but Deny passes, got %v", err)
	}
	if err := sc.CheckScopeFiles([]string{"internal/secret/f.go"}); err == nil {
		t.Fatal("denied file must be refused")
	}
}

func TestScopeRejectsTraversalAbsoluteAndDrive(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Allow: docs"))
	for _, evil := range []string{"../x", "a/../b", "/etc/passwd", `C:\evil`, "C:/evil", "C:relative", ""} {
		err := sc.CheckScopeFiles([]string{evil})
		if err == nil || !strings.Contains(err.Error(), "fuera de scope:") {
			t.Fatalf("file %q must be refused as fuera de scope, got %v", evil, err)
		}
		if evil != "" && !strings.Contains(err.Error(), strings.TrimSpace(evil)) {
			t.Fatalf("refusal must name the file %q, got %v", evil, err)
		}
	}
}

func TestScopeInvalidDeclaration(t *testing.T) {
	for _, line := range []string{"Allow: ../evil", "Allow: /abs", `Allow: C:\evil`, "Deny: a/../b", "Allow:"} {
		if _, err := ParseScope(scopeDoc(line)); err == nil || !strings.Contains(err.Error(), "scope: invalid") {
			t.Fatalf("declaration %q must be rejected, got %v", line, err)
		}
	}
}

func TestScopeCaseInsensitive(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Allow: docs/"))
	if err := sc.CheckScopeFiles([]string{"Docs/Readme.md"}); err != nil {
		t.Fatalf("comparison is case-insensitive, got %v", err)
	}
	deny := mustParseScope(t, scopeDoc("Allow: docs", "Deny: DOCS/secret"))
	if err := deny.CheckScopeFiles([]string{"docs/secret/key.md"}); err == nil {
		t.Fatal("deny matches case-insensitively too")
	}
}

func TestScopeRenameBothSidesEvaluated(t *testing.T) {
	sc := mustParseScope(t, scopeDoc("Allow: docs"))
	// Rename entries contribute both paths: each side is judged alone.
	if err := sc.CheckScopeFiles([]string{"docs/old.md -> docs/new.md"}); err != nil {
		t.Fatalf("in-scope rename must pass, got %v", err)
	}
	err := sc.CheckScopeFiles([]string{"outside/old.md -> docs/new.md"})
	if err == nil || !strings.Contains(err.Error(), "fuera de scope: outside/old.md") {
		t.Fatalf("rename with an out-of-scope side must name it, got %v", err)
	}
	err = sc.CheckScopeFiles([]string{"docs/a.md", "outside/b.md"})
	if err == nil || !strings.Contains(err.Error(), "fuera de scope: outside/b.md") {
		t.Fatalf("separate entries are evaluated separately, got %v", err)
	}
}

func TestScopeGlobForms(t *testing.T) {
	star := mustParseScope(t, scopeDoc("Allow: internal/*"))
	if err := star.CheckScopeFiles([]string{"internal/a/b.go"}); err != nil {
		t.Fatalf("trailing /* covers the tree, got %v", err)
	}
	exact := mustParseScope(t, scopeDoc("Allow: cmd/tool/main.go"))
	if err := exact.CheckScopeFiles([]string{"cmd/tool/main.go"}); err != nil {
		t.Fatalf("exact file allow must pass, got %v", err)
	}
	if err := exact.CheckScopeFiles([]string{"cmd/tool/other.go"}); err == nil {
		t.Fatal("exact file allow must not cover siblings")
	}
}

func TestScopeSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"docs", "outside"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "outside", "secret.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "outside", "secret.md"), filepath.Join(root, "docs", "link.md")); err != nil {
		t.Skipf("host forbids symlinks: %v", err)
	}
	sc := mustParseScope(t, scopeDoc("Allow: docs"))
	err := sc.CheckScopeFilesIn(root, []string{"docs/link.md"})
	if err == nil || !strings.Contains(err.Error(), "fuera de scope: docs/link.md") {
		t.Fatalf("symlink escaping the scope must be refused by link name, got %v", err)
	}
	// A benign symlink staying inside the scope passes.
	if err := os.WriteFile(filepath.Join(root, "docs", "real.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", filepath.Join(root, "docs", "ok.md")); err != nil {
		t.Skipf("host forbids symlinks: %v", err)
	}
	if err := sc.CheckScopeFilesIn(root, []string{"docs/ok.md"}); err != nil {
		t.Fatalf("in-scope symlink must pass, got %v", err)
	}
}
