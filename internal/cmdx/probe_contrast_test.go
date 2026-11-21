// Tabla ProbeContrastCmd (CRAP<30 sin cambiar comportamiento): cubre
// missing/inválido/error/!ran/igual/diferente con Runner falso y
// t.TempDir, sin partir la función. Los casos de éxito usan
// regulus.json mínimo y un Runner que no toca shell ni red.
package cmdx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type probeContrastFakeResult struct {
	stdout string
	stderr string
	code   int
	err    error
}

type probeContrastFakeRunner struct {
	results map[string]probeContrastFakeResult
	calls   []string
}

func (f *probeContrastFakeRunner) Run(_ context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	if command == "git rev-parse --verify HEAD" {
		return "abc123\n", "", 0, nil
	}
	if res, ok := f.results[command]; ok {
		return res.stdout, res.stderr, res.code, res.err
	}
	return "out:" + command + "\n", "", 0, nil
}

func writeProbeProject(t *testing.T, dir string) {
	t.Helper()
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProbeContrastTable(t *testing.T) {
	cases := []struct {
		name     string
		withCfg  bool
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "missing",
			withCfg:  false,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--a", "echo a"},
			wantCode: 2,
			wantErr:  "usage: regulus probe contrast",
		},
		{
			name:     "invalido",
			withCfg:  false,
			args:     []string{"--task", "../evil", "--gate", "typecheck", "--a", "echo a", "--b", "echo b"},
			wantCode: 2,
			wantErr:  "invalid task id",
		},
		{
			name:     "error",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "nope", "--a", "echo a", "--b", "echo b"},
			wantCode: 1,
			wantErr:  "probe contrast:",
		},
		{
			name:     "no-ran",
			withCfg:  false,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--a", "echo a", "--b", "echo b"},
			wantCode: 1,
			wantErr:  "probe contrast:",
		},
		{
			name:     "igual",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--a", "echo same", "--b", "echo same"},
			wantCode: 0,
			wantOut:  "no differences: receipts identical",
		},
		{
			name:     "diferente",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--a", "echo hi", "--b", "echo bye"},
			wantCode: 0,
			wantOut:  "contrast",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.withCfg {
				writeProbeProject(t, dir)
			}
			fake := &probeContrastFakeRunner{results: map[string]probeContrastFakeResult{}}
			io, out, errb := missionIO(dir)
			io.Runner = fake
			if code := io.ProbeContrastCmd(tc.args); code != tc.wantCode {
				t.Fatalf("ProbeContrastCmd %q exit = %d, want %d (out=%q err=%q)", tc.name, code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("ProbeContrastCmd %q stdout debe contener %q, got %q", tc.name, tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("ProbeContrastCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}

// El aviso defensivo `runs did not execute` solo ocurre con
// ran=false y err=nil, combinación que ContrastWithRunner nunca
// produce (todo ran=false llega con error y sale por el prefijo
// "probe contrast:"). Este test pinnea que los rechazos salen con 1
// por la rama de error, sin inventar un runner imposible.
func TestProbeContrastRanFalseComesWithError(t *testing.T) {
	dir := t.TempDir()
	fake := &probeContrastFakeRunner{results: map[string]probeContrastFakeResult{}}
	io, _, errb := missionIO(dir)
	io.Runner = fake
	args := []string{"--task", "t1", "--gate", "typecheck", "--a", "echo a", "--b", "echo b"}
	if code := io.ProbeContrastCmd(args); code != 1 {
		t.Fatalf("sin regulus.json exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "probe contrast:") {
		t.Fatalf("el rechazo debe prefijar probe contrast:, got %q", errb.String())
	}
}
