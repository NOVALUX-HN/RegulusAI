// Tabla LoadDiagThresholds (CRAP paydown): fixture temporal por caso
// (read err, >64KiB, malformed, unknown field, validate, defaults y
// parcial). Pura lectura con t.TempDir, portable.
package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeThresholds(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadDiagThresholdsTable(t *testing.T) {
	t.Run("read err", func(t *testing.T) {
		if _, err := LoadDiagThresholds(filepath.Join(t.TempDir(), "ausente.json")); err == nil {
			t.Fatal("fichero ausente debe fallar")
		}
	})
	t.Run("demasiado grande", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "big.json", strings.Repeat("x", 64*1024+1))
		if _, err := LoadDiagThresholds(p); err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf(">64KiB debe fallar con too large, got %v", err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "bad.json", "{bad")
		if _, err := LoadDiagThresholds(p); err == nil || !strings.Contains(err.Error(), "malformed thresholds") {
			t.Fatalf("json roto debe fallar con malformed, got %v", err)
		}
	})
	t.Run("unknown field", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "unknown.json", `{"bogus_field":1}`)
		if _, err := LoadDiagThresholds(p); err == nil || !strings.Contains(err.Error(), "malformed thresholds") {
			t.Fatalf("campo desconocido debe fallar estricto, got %v", err)
		}
	})
	t.Run("validate", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "zero.json", `{"slow_stage_ms":0}`)
		if _, err := LoadDiagThresholds(p); err == nil || !strings.Contains(err.Error(), "slow_stage_ms must be positive") {
			t.Fatalf("cero debe fallar validacion, got %v", err)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "empty.json", `{}`)
		got, err := LoadDiagThresholds(p)
		if err != nil {
			t.Fatalf("objeto vacio usa defaults: %v", err)
		}
		want := DefaultDiagThresholds()
		if got != want {
			t.Fatalf("defaults = %+v, want %+v", got, want)
		}
	})
	t.Run("parcial", func(t *testing.T) {
		dir := t.TempDir()
		p := writeThresholds(t, dir, "partial.json", `{"slow_stage_ms":5000}`)
		got, err := LoadDiagThresholds(p)
		if err != nil {
			t.Fatalf("parcial: %v", err)
		}
		want := DefaultDiagThresholds()
		want.SlowStageMS = 5000
		if got != want {
			t.Fatalf("parcial = %+v, want %+v", got, want)
		}
	})
}
