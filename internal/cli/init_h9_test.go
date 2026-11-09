package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
)

// H9 (S2,S3): init refuses nested scaffolds, --force overwrites, and
// every success prints a best-effort validation that never fails init.

// Ancestor refusal: running init in a subdirectory of a configured
// project must refuse (exit 1), name the ancestor regulus.json path,
// and write nothing (no split-brain sub/regulus.json + sub/charts/).
func TestInitRefusesAncestorNamesPath(t *testing.T) {
	root := t.TempDir()
	ancestorCfg := filepath.Join(root, "regulus.json")
	if err := os.WriteFile(ancestorCfg, []byte(`{"gates":[{"name":"presence","layer":1,"command":"echo repo-present"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	c, _, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--stack", "smoke"}); code != 1 {
		t.Fatalf("nested init exit = %d, want 1 (refusal)", code)
	}
	if !strings.Contains(errb.String(), ancestorCfg) {
		t.Fatalf("refusal must name the ancestor path %q, got %q", ancestorCfg, errb.String())
	}
	if _, err := os.Stat(filepath.Join(sub, "regulus.json")); !os.IsNotExist(err) {
		t.Fatal("refused init must not write sub/regulus.json (split-brain)")
	}
	if _, err := os.Stat(filepath.Join(sub, "charts")); !os.IsNotExist(err) {
		t.Fatal("refused init must not write sub/charts/ (split-brain)")
	}
}

// --force overwrites a regulus.json in the cwd deliberately.
func TestInitForceOverwritesCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	sentinel := `{"gates":[{"name":"old","layer":1,"command":"true"}]}`
	if err := os.WriteFile("regulus.json", []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--stack", "smoke", "--force"}); code != 0 {
		t.Fatalf("init --force exit = %d, stderr %q, want 0", code, errb.String())
	}
	cfg, err := gates.Load("regulus.json")
	if err != nil {
		t.Fatalf("regulus.json must load after --force: %v", err)
	}
	if len(cfg.Gates) != 1 || cfg.Gates[0].Name != "presence" {
		t.Fatalf("--force must overwrite with the smoke preset, got %+v", cfg.Gates)
	}
	if !strings.Contains(out.String(), "validate:") {
		t.Fatalf("forced init must still print validation, got %q", out.String())
	}
}

// --force in a subdirectory scaffolds the cwd deliberately and leaves
// the ancestor config untouched.
func TestInitForceBelowAncestorScaffoldsCwd(t *testing.T) {
	root := t.TempDir()
	ancestorCfg := filepath.Join(root, "regulus.json")
	before := `{"gates":[{"name":"presence","layer":1,"command":"echo repo-present"}]}`
	if err := os.WriteFile(ancestorCfg, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--stack", "smoke", "--force"}); code != 0 {
		t.Fatalf("nested init --force exit = %d, stderr %q, want 0", code, errb.String())
	}
	data, err := os.ReadFile(filepath.Join(sub, "regulus.json"))
	if err != nil {
		t.Fatalf("nested --force must write sub/regulus.json: %v", err)
	}
	if !strings.Contains(string(data), "presence") {
		t.Fatalf("sub/regulus.json must carry the smoke preset, got %q", data)
	}
	after, err := os.ReadFile(ancestorCfg)
	if err != nil || string(after) != before {
		t.Fatalf("nested --force must leave the ancestor config untouched, got %q (%v)", after, err)
	}
	if !strings.Contains(out.String(), "validate:") {
		t.Fatalf("forced init must still print validation, got %q", out.String())
	}
}

// Validation: a failing doctor must not fail init — the summary lines
// print (doctor + gate run --dry-run) and init still exits 0.
func TestInitValidationPrintsDespiteDoctorFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(func() doctor.Result {
		return doctor.Result{Reason: "opencode binary not found on PATH — install OpenCode V2"}
	})
	if code := c.Run([]string{"init", "--stack", "smoke"}); code != 0 {
		t.Fatalf("init exit = %d, stderr %q, want 0 (validation never fails init)", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "validate:") {
		t.Fatalf("init must print validation lines, got %q", s)
	}
	if !strings.Contains(s, "doctor") {
		t.Fatalf("validation must summarize doctor, got %q", s)
	}
	if !strings.Contains(s, "dry-run") {
		t.Fatalf("validation must summarize gate run --dry-run, got %q", s)
	}
	if !strings.Contains(s, "presence") {
		t.Fatalf("validation must include the dry-run plan (presence gate), got %q", s)
	}
}

// Validation with no runtime probe (nil) must neither panic nor fail init.
func TestInitValidationNilProbeNeverFails(t *testing.T) {
	t.Chdir(t.TempDir())
	c, out, errb := newCLI(nil)
	if code := c.Run([]string{"init", "--stack", "smoke"}); code != 0 {
		t.Fatalf("init exit = %d, stderr %q, want 0", code, errb.String())
	}
	if !strings.Contains(out.String(), "validate:") {
		t.Fatalf("init must print validation lines even without a probe, got %q", out.String())
	}
}
