// Guard test: every process launch in the repo goes through the
// centralized executor (internal/pxexec) or a documented exception.
// It walks the repo (skipping .git), parses each non-test .go file
// outside internal/pxexec and fails on two signals:
//
//  1. calling exec.Command / exec.CommandContext (spawning a child
//     without the executor: no classification, no cap, no redaction);
//  2. importing "os/exec" at all (the import is the smoke signal that
//     a file may be launching on its own).
//
// Documented exceptions (allowlist with motivo):
//   - internal/gates/gates_exec.go (ShRunner/StructuredRunner/
//     runShell): env-policy, kill-gate and cpu stats are security
//     features, never plain launches (NOV-81). Audited here, not
//     migrated; the F15.4 runner reuses runShellCap semantics.
//   - *_test.go: test orchestration (fake runners, fixture probes,
//     multiprocess helpers, portability probes).
//   - scripts/: no Go sources (shell/TS tooling only).
package pxexec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedLaunch names non-test files outside internal/pxexec that may
// still call exec.Command/CommandContext, with the motivo.
var allowedLaunch = map[string]string{
	"internal/gates/gates_exec.go": "ShRunner/StructuredRunner/runShell env-policy + kill-gate + cpu stats (NOV-81 security features; F15.4 runner scope)",
}

// allowedImport names non-test files outside internal/pxexec that may
// import "os/exec" WITHOUT calling exec.Command/CommandContext
// (LookPath probes, exec.Error classification, HideWindow infra).
var allowedImport = map[string]string{
	"internal/doctor/doctor.go":         "import-only: clasifica exec.Error de runners inyectados (exigido por doctor_test.go); lanza vía pxexec",
	"internal/bench/agenthost.go":       "import-only: exec.LookPath localiza el binario; el lanzamiento va por pxexec",
	"internal/cli/gate_run.go":          "import-only: exec.LookPath en preflight (sin lanzamiento)",
	"internal/cli/status.go":            "import-only: exec.LookPath probe (sin lanzamiento)",
	"internal/testenv/testenv.go":       "import-only: exec.LookPath helper de tests (sin lanzamiento)",
	"internal/executil/hide_unix.go":    "infra: HideWindow opera sobre *exec.Cmd; la usa el propio pxexec (no lanza)",
	"internal/executil/hide_windows.go": "infra: HideWindow opera sobre *exec.Cmd; la usa el propio pxexec (no lanza)",
}

func TestNoRawProcessLaunch(t *testing.T) {
	root := repoRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/pxexec/") {
			return nil // the executor itself
		}
		if strings.HasSuffix(rel, "_test.go") {
			return nil // test orchestration (documented exception)
		}
		if strings.HasPrefix(rel, "scripts/") {
			return nil // no-Go tooling (documented exception)
		}
		launches, importsExec := scanExecUsage(t, path)
		if launches {
			if _, ok := allowedLaunch[rel]; !ok {
				violations = append(violations, rel+": llama exec.Command/CommandContext fuera de pxexec y sin excepción documentada")
			}
		} else if importsExec {
			if _, ok := allowedImport[rel]; !ok {
				if _, ok2 := allowedLaunch[rel]; !ok2 {
					violations = append(violations, rel+": importa os/exec sin excepción documentada")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("lanzadores fuera del executor:\n- %s", strings.Join(violations, "\n- "))
	}
}

// scanExecUsage reports whether a Go file calls exec.Command or
// exec.CommandContext and whether it imports "os/exec".
func scanExecUsage(t *testing.T, path string) (launches, importsExec bool) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == "os/exec" {
			importsExec = true
		}
	}
	if !importsExec {
		return false, false
	}
	// Imports os/exec: check for actual launch calls with a full parse.
	ff, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	ast.Inspect(ff, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "exec" {
			return true
		}
		if sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext" {
			launches = true
			return false
		}
		return true
	})
	return launches, true
}

// repoRoot climbs from this file to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test working directory")
		}
		dir = parent
	}
}
