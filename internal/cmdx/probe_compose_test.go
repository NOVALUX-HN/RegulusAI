// Tabla ProbeComposeCmd (CRAP paydown sin cambiar comportamiento):
// requeridos, parse, validTaskID, Compose err, !ran defensivo, igual y
// diferente con Runner falso y t.TempDir. Los splits de parse/finish
// se prueban directo para cubrir la rama defensiva inalcanzable por
// ComposeWithRunner (ran=false siempre llega con err).
package cmdx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type composeFakeResult struct {
	stdout string
	stderr string
	code   int
	err    error
}

type composeFakeRunner struct {
	results map[string]composeFakeResult
	calls   []string
}

func (f *composeFakeRunner) Run(_ context.Context, command string) (string, string, int, error) {
	f.calls = append(f.calls, command)
	if command == "git rev-parse --verify HEAD" {
		return "abc123\n", "", 0, nil
	}
	if res, ok := f.results[command]; ok {
		return res.stdout, res.stderr, res.code, res.err
	}
	return "out:" + command + "\n", "", 0, nil
}

func writeComposeProject(t *testing.T, dir string) {
	t.Helper()
	cfg := `{"gates":[{"name":"typecheck","layer":1,"command":"echo hi"}]}`
	if err := os.WriteFile(filepath.Join(dir, "regulus.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProbeComposeTable(t *testing.T) {
	cases := []struct {
		name     string
		withCfg  bool
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "requeridos",
			withCfg:  false,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--base", "echo base", "--a", "echo a"},
			wantCode: 2,
			wantErr:  "usage: regulus probe compose",
		},
		{
			name:     "parse",
			withCfg:  false,
			args:     []string{"--task", "t1", "--zzz", "x"},
			wantCode: 2,
		},
		{
			name:     "invalido",
			withCfg:  false,
			args:     []string{"--task", "../evil", "--gate", "typecheck", "--base", "echo base", "--a", "echo a", "--b", "echo b"},
			wantCode: 2,
			wantErr:  "invalid task id",
		},
		{
			name:     "compose err",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "nope", "--base", "echo base", "--a", "echo a", "--b", "echo b"},
			wantCode: 1,
			wantErr:  "probe compose:",
		},
		{
			name:     "sin config",
			withCfg:  false,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--base", "echo base", "--a", "echo a", "--b", "echo b"},
			wantCode: 1,
			wantErr:  "probe compose:",
		},
		{
			name:     "igual",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--base", "echo same", "--a", "echo same", "--b", "echo same"},
			wantCode: 0,
			wantOut:  "(no conflicts)",
		},
		{
			name:     "diferente",
			withCfg:  true,
			args:     []string{"--task", "t1", "--gate", "typecheck", "--base", "echo base", "--a", "echo a", "--b", "echo b"},
			wantCode: 0,
			wantOut:  "## Conflicts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.withCfg {
				writeComposeProject(t, dir)
			}
			fake := &composeFakeRunner{results: map[string]composeFakeResult{}}
			io, out, errb := missionIO(dir)
			io.Runner = fake
			if code := io.ProbeComposeCmd(tc.args); code != tc.wantCode {
				t.Fatalf("ProbeComposeCmd %q exit = %d, want %d (out=%q err=%q)", tc.name, code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("ProbeComposeCmd %q stdout debe contener %q, got %q", tc.name, tc.wantOut, out.String())
			}
			if tc.name == "diferente" && strings.Contains(out.String(), "(no conflicts)") {
				t.Fatalf("ProbeComposeCmd diferente no debe reportar sin conflictos, got %q", out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("ProbeComposeCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}

func TestFinishProbeComposeCmdDefensive(t *testing.T) {
	io, _, errb := missionIO(t.TempDir())
	if code := finishProbeComposeCmd(io, "", false, nil); code != 1 {
		t.Fatalf("ran=false sin err exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "runs did not execute") {
		t.Fatalf("la rama defensiva debe avisar, got %q", errb.String())
	}
}
