// Tabla TraceTimelineCmd (CRAP<30 sin cambiar comportamiento): cubre
// sin-filtro, json-vacío y rebuild-sin-run con IO{Dir:tmp}, sin partir
// la función. Cada caso usa t.TempDir() y semillas vía TraceEmitCmd
// (portable, sin shell ni red) para elevar la cobertura por encima del
// umbral sin tocar mensajes ni stores.
package cmdx

import (
	"strings"
	"testing"
)

func emitExec(t *testing.T, io IO, args ...string) {
	t.Helper()
	if code := io.TraceEmitCmd(args); code != 0 {
		t.Fatalf("emit %v exit = %d", args, code)
	}
}

func TestTraceTimelineTable(t *testing.T) {
	cases := []struct {
		name     string
		seed     func(t *testing.T, io IO)
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "sin-filtro",
			seed:     nil,
			args:     nil,
			wantCode: 2,
			wantErr:  "usage: regulus trace timeline",
		},
		{
			name:     "json-vacio",
			seed:     nil,
			args:     []string{"--run", "ghost", "--json"},
			wantCode: 0,
			wantOut:  `"events": []`,
		},
		{
			name: "rebuild-sin-run",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "r1", "--type", "task.start", "--task", "t1")
			},
			args:     []string{"--task", "t1", "--rebuild"},
			wantCode: 2,
			wantErr:  "--rebuild needs --run",
		},
		{
			name:     "ayuda",
			seed:     nil,
			args:     []string{"--help"},
			wantCode: 0,
			wantOut:  "usage: regulus trace timeline",
		},
		{
			name:     "flag-error",
			seed:     nil,
			args:     []string{"--bogus"},
			wantCode: 2,
		},
		{
			name:     "texto-vacio",
			seed:     nil,
			args:     []string{"--run", "ghost"},
			wantCode: 0,
			wantOut:  "(no execution events match)",
		},
		{
			name: "texto-con-eventos",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "r1", "--type", "task.start", "--task", "t1", "--agent", "a1")
			},
			args:     []string{"--run", "r1"},
			wantCode: 0,
			wantOut:  "r1",
		},
		{
			name: "json-con-eventos",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "r1", "--type", "task.start", "--task", "t1")
			},
			args:     []string{"--run", "r1", "--json"},
			wantCode: 0,
			wantOut:  "r1",
		},
		{
			name: "rebuild-con-run",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "r1", "--type", "run.start")
				emitExec(t, io, "--run", "r1", "--type", "run.end", "--status", "succeeded")
			},
			args:     []string{"--run", "r1", "--rebuild"},
			wantCode: 0,
			wantOut:  "status succeeded",
		},
		{
			name: "rebuild-crashed",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "rCrash", "--type", "run.start")
			},
			args:     []string{"--run", "rCrash", "--rebuild"},
			wantCode: 0,
			wantOut:  "crashed",
		},
		{
			name: "rebuild-con-delegaciones",
			seed: func(t *testing.T, io IO) {
				emitExec(t, io, "--run", "r1", "--type", "run.start")
				emitExec(t, io, "--run", "r1", "--type", "delegation.request", "--delegation", "d1", "--agent", "a1", "--payload", `{"to":"a2"}`)
				emitExec(t, io, "--run", "r1", "--type", "delegation.result", "--delegation", "d1", "--status", "succeeded")
				emitExec(t, io, "--run", "r1", "--type", "delegation.request", "--delegation", "d2")
				emitExec(t, io, "--run", "r1", "--type", "run.end", "--status", "succeeded")
			},
			args:     []string{"--run", "r1", "--rebuild"},
			wantCode: 0,
			wantOut:  "delegation",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			io, out, errb := missionIO(dir)
			if tc.seed != nil {
				tc.seed(t, io)
				out.Reset()
				errb.Reset()
			}
			if code := io.TraceTimelineCmd(tc.args); code != tc.wantCode {
				t.Fatalf("TraceTimelineCmd %q exit = %d, want %d (out=%q err=%q)", tc.name, code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("TraceTimelineCmd %q stdout debe contener %q, got %q", tc.name, tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("TraceTimelineCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}

func TestTraceTimelineHelpShort(t *testing.T) {
	dir := t.TempDir()
	io, out, _ := missionIO(dir)
	if code := io.TraceTimelineCmd([]string{"-h"}); code != 0 {
		t.Fatalf("-h exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage: regulus trace timeline") {
		t.Fatalf("-h debe mostrar el uso, got %q", out.String())
	}
}
