package cli

// P4: auto-detect de la raíz (git top-level → regulus.json → cwd).
//
// Given el core usa root="." literal (cwd) y parte el guardado en dos
// desde un subdirectorio
// When cualquier comando corre desde un subdirectorio (de un checkout
// git o con regulus.json hacia arriba)
// Then el core auto-detecta la raíz y todo cae bajo <raíz>/.regulus/

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
)

// cleanEq compara rutas con la misma normalización que sameDir
// (Clean + case-insensitive: git en Windows puede devolver la unidad
// o el case con otra capitalización que Getwd).
func cleanEq(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// noGit fija un techo determinista para que `git rev-parse` falle sin
// depender de dónde viva el TMPDIR del runner (los fixtures sin git
// deben fallar el probe aunque el TMPDIR estuviera dentro de un repo).
func noGit(t *testing.T, root string) {
	t.Helper()
	t.Setenv("GIT_CEILING_DIRECTORIES", root)
}

// gitInit verifica git disponible y levanta un checkout fresco en root
// (todo queda dentro de t.TempDir(); sin commits: rev-parse funciona).
func gitInit(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

// Subdir de un repo git resuelve al top-level.
func TestResolveRootGitSubdir(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	sub := filepath.Join(root, "charts", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveRoot(sub); !cleanEq(got, root) {
		t.Fatalf("ResolveRoot(subdir) = %q, want %q", got, root)
	}
	// En el top-level preserva la entrada (sin reescritura).
	if got := ResolveRoot(root); got != root {
		t.Fatalf("ResolveRoot(root) = %q, want input %q verbatim", got, root)
	}
}

// Subdir con regulus.json arriba (sin git) resuelve ahí.
func TestResolveRootRegulusJsonAbove(t *testing.T) {
	root := t.TempDir()
	noGit(t, root)
	if err := os.WriteFile(filepath.Join(root, "regulus.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "docs", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveRoot(sub); !cleanEq(got, root) {
		t.Fatalf("ResolveRoot(subdir) = %q, want %q", got, root)
	}
	if got := ResolveRoot(root); got != root {
		t.Fatalf("ResolveRoot(root) = %q, want input %q verbatim", got, root)
	}
}

// Fuera de git y sin regulus.json: comporta igual que hoy (cwd).
func TestResolveRootFallbackIsCwd(t *testing.T) {
	dir := t.TempDir()
	noGit(t, dir)
	if got := ResolveRoot(dir); got != dir {
		t.Fatalf("ResolveRoot(plain) = %q, want %q", got, dir)
	}
}

// En la raíz del proyecto "." sigue siendo "." (P3 intacto).
func TestResolveRootDotAtTopStaysDot(t *testing.T) {
	root := t.TempDir()
	noGit(t, root)
	if err := os.WriteFile(filepath.Join(root, "regulus.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if got := ResolveRoot("."); got != "." {
		t.Fatalf("ResolveRoot(.) at top = %q, want \".\"", got)
	}
	if got := ResolveRoot(""); got != "" {
		t.Fatalf("ResolveRoot(\"\") at top = %q, want \"\"", got)
	}
}

// `memory note` desde un subdir (raíz por regulus.json) cae en
// <raíz>/.regulus/ y no crea <subdir>/.regulus/.
func TestMemoryNoteFromSubdirSharesRootStores(t *testing.T) {
	root := t.TempDir()
	noGit(t, root)
	if err := os.WriteFile(filepath.Join(root, "regulus.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "charts")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	var out, errb bytes.Buffer
	c := CLI{Version: "test", Stdout: &out, Stderr: &errb}
	if code := c.Run([]string{"memory", "note", "hello from subdir"}); code != 0 {
		t.Fatalf("memory note exit = %d, want 0 (%s)", code, errb.String())
	}
	data, err := os.ReadFile(almanac.File(root))
	if err != nil {
		t.Fatalf("root store missing: %v", err)
	}
	if !strings.Contains(string(data), "hello from subdir") {
		t.Fatalf("root store lacks the note: %s", data)
	}
	if _, err := os.Stat(filepath.Join(sub, ".regulus")); !os.IsNotExist(err) {
		t.Fatalf("subdir must not gain its own .regulus (split stores)")
	}
}

// `memory note` desde un subdir de un checkout git cae en el top-level.
func TestMemoryNoteFromGitSubdirSharesRootStores(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	sub := filepath.Join(root, "internal", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	var out, errb bytes.Buffer
	c := CLI{Version: "test", Stdout: &out, Stderr: &errb}
	if code := c.Run([]string{"memory", "note", "git subdir note"}); code != 0 {
		t.Fatalf("memory note exit = %d, want 0 (%s)", code, errb.String())
	}
	data, err := os.ReadFile(almanac.File(root))
	if err != nil {
		t.Fatalf("top-level store missing: %v", err)
	}
	if !strings.Contains(string(data), "git subdir note") {
		t.Fatalf("top-level store lacks the note: %s", data)
	}
	if _, err := os.Stat(filepath.Join(sub, ".regulus")); !os.IsNotExist(err) {
		t.Fatalf("subdir must not gain its own .regulus (split stores)")
	}
}

// Nunca falla: un cwd inexistente o un git roto devuelven el input.
func TestResolveRootNeverFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	if got := ResolveRoot(missing); got != missing {
		t.Fatalf("ResolveRoot(missing) = %q, want input %q", got, missing)
	}
}
