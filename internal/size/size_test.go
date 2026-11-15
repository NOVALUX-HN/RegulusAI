// NOV-67: size budgets with critical tiers and expiring waivers.
package size

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string, waiver string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if waiver != "" {
		d := filepath.Join(dir, "docs")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "quality-exceptions.md"), []byte(waiver), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func funcOf(name string, lines int) string {
	var b strings.Builder
	b.WriteString("func " + name + "() int {\n")
	for i := 0; i < lines-2; i++ {
		b.WriteString("\t_ = 1\n")
	}
	b.WriteString("\treturn 1\n}\n")
	return b.String()
}

func TestCheckCleanTreePasses(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/ok/ok.go": "package p\n\n" + funcOf("Small", 10)}, "")
	if got := Check(root, "docs/quality-exceptions.md"); len(got) != 0 {
		t.Fatalf("clean tree must pass, got %v", got)
	}
}

func TestCheckFlagsOversizedFuncAndFile(t *testing.T) {
	body := "package p\n\n" + strings.Repeat("// filler\n", 820) + funcOf("Huge", 300)
	root := writeTree(t, map[string]string{"internal/big/big.go": body}, "")
	got := Check(root, "docs/quality-exceptions.md")
	var file, fn bool
	for _, v := range got {
		if v.Kind == "file-too-big" {
			file = true
		}
		if v.Kind == "func-too-big" && strings.Contains(v.Where, "Huge") {
			fn = true
		}
	}
	if !file || !fn {
		t.Fatalf("must flag file and func, got %v", got)
	}
}

func TestCheckHonorsAndExpiresWaivers(t *testing.T) {
	files := map[string]string{"internal/big/big.go": "package p\n\n" + funcOf("Huge", 300)}
	root := writeTree(t, files, "internal/big/big.go:Huge covered expira:2099-01-01 aprueba:test\n")
	if got := Check(root, "docs/quality-exceptions.md"); len(got) != 0 {
		t.Fatalf("valid waiver must cover, got %v", got)
	}
	root2 := writeTree(t, files, "internal/big/big.go:Huge stale expira:2000-01-01 aprueba:test\n")
	got := Check(root2, "docs/quality-exceptions.md")
	found := false
	for _, v := range got {
		if v.Kind == "waiver-expired" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expired waiver must fail, got %v", got)
	}
}

func TestCheckCriticalTierIsStricter(t *testing.T) {
	body := "package p\n\n" + funcOf("Core", 150)
	root := writeTree(t, map[string]string{"internal/gates/g.go": body}, "")
	got := Check(root, "docs/quality-exceptions.md")
	found := false
	for _, v := range got {
		if v.Kind == "func-too-big" && strings.Contains(v.Where, "gates") {
			found = true
		}
	}
	if !found {
		t.Fatalf("150-line func must fail the critical tier (cap 100), got %v", got)
	}
}

func TestCheckFlagsTooCoupledPackage(t *testing.T) {
	var imps strings.Builder
	for i := 0; i < 35; i++ {
		fmt.Fprintf(&imps, "\t_ \"regulus-ai/internal/dep%02d\"\n", i)
	}
	body := "package p\n\nimport (\n" + imps.String() + ")\n\nfunc F() {}\n"
	root := writeTree(t, map[string]string{"internal/hub/hub.go": body}, "")
	got := Check(root, "docs/quality-exceptions.md")
	found := false
	for _, v := range got {
		if v.Kind == "too-coupled" {
			found = true
		}
	}
	if !found {
		t.Fatalf("35 deps must trip the coupling cap, got %v", got)
	}
}
