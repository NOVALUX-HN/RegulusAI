// NOV-67: the size gate never goes red on a clean tree and always
// names the file, the cap and the fix.
package size

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGo(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCleanTreePasses(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "internal/ok/ok.go", "package ok\n\nfunc Fine() int {\n\treturn 1\n}\n")
	os.MkdirAll(filepath.Join(dir, "internal", "ok"), 0o755)
	if got := Check(dir, "docs/quality-exceptions.md"); len(got) != 0 {
		t.Fatalf("clean tree must pass, got %v", got)
	}
}

func TestCheckFailsOversizedFileAndFunc(t *testing.T) {
	dir := t.TempDir()
	body := "package big\n\nfunc Huge() int {\n" + strings.Repeat("\t_ = 1\n", 300) + "\treturn 1\n}\n"
	writeGo(t, dir, "internal/big/big.go", body)
	got := Check(dir, "docs/quality-exceptions.md")
	var file, fn bool
	for _, v := range got {
		if v.Kind == "file-too-big" {
			file = true
		}
		if v.Kind == "func-too-big" {
			fn = true
		}
	}
	if !file || !fn {
		t.Fatalf("must flag file and func, got %v", got)
	}
}

func TestCheckHonorsAndExpiresWaivers(t *testing.T) {
	dir := t.TempDir()
	body := "package big\n\nfunc Huge() int {\n" + strings.Repeat("\t_ = 1\n", 300) + "\treturn 1\n}\n"
	writeGo(t, dir, "internal/big/big.go", body)
	exc := filepath.Join("docs", "quality-exceptions.md")
	full := filepath.Join(dir, exc)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("internal/big/big.go:Huge test waiver expira:2099-01-01 aprueba:test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Check(dir, exc); len(got) != 0 {
		t.Fatalf("valid waiver must cover, got %v", got)
	}
	if err := os.WriteFile(full, []byte("internal/big/big.go:Huge stale expira:2000-01-01 aprueba:test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Check(dir, exc)
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
