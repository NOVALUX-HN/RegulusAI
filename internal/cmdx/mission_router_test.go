// MissionCmd router cover (CRAP<30 sin cambiar comportamiento): tabla
// mínima help/vacío/unknown más un caso por cada uno de los 15
// subcomandos, con IO{Dir:tmp}. Los subcomandos con flags faltantes
// rehúsan con 2 antes de IO (salvo list, que lee un store vacío en
// tmp y sale 0); report y verify-report exigen --id con su mensaje
// propio en vez del usage. Ningún caso necesita red, shell ni git.
package cmdx

import (
	"strings"
	"testing"
)

func TestMissionRouterCover(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
		wantOut  string
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0, wantOut: "regulus mission"},
		{name: "help short", args: []string{"-h"}, wantCode: 0, wantOut: "regulus mission"},
		{name: "empty", args: nil, wantCode: 2, wantErr: "usage: regulus mission"},
		{name: "unknown", args: []string{"frobnicate"}, wantCode: 2, wantErr: `unknown subcommand "frobnicate"`},
		{name: "create", args: []string{"create"}, wantCode: 2, wantErr: "usage:"},
		{name: "show", args: []string{"show"}, wantCode: 2, wantErr: "usage:"},
		{name: "list empty store", args: []string{"list"}, wantCode: 0},
		{name: "status", args: []string{"status"}, wantCode: 2, wantErr: "usage:"},
		{name: "advance", args: []string{"advance"}, wantCode: 2, wantErr: "usage:"},
		{name: "pause", args: []string{"pause"}, wantCode: 2, wantErr: "usage:"},
		{name: "resume", args: []string{"resume"}, wantCode: 2, wantErr: "usage:"},
		{name: "cancel", args: []string{"cancel"}, wantCode: 2, wantErr: "usage:"},
		{name: "link", args: []string{"link"}, wantCode: 2, wantErr: "usage:"},
		{name: "reopen", args: []string{"reopen"}, wantCode: 2, wantErr: "usage:"},
		{name: "plan", args: []string{"plan"}, wantCode: 2, wantErr: "usage:"},
		{name: "replan", args: []string{"replan"}, wantCode: 2, wantErr: "usage:"},
		{name: "explain", args: []string{"explain"}, wantCode: 2, wantErr: "usage:"},
		{name: "report", args: []string{"report"}, wantCode: 2, wantErr: "mission report: --id is required"},
		{name: "verify-report", args: []string{"verify-report"}, wantCode: 2, wantErr: "mission verify-report: --id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			io, out, errb := missionIO(dir)
			if code := io.MissionCmd(tc.args); code != tc.wantCode {
				t.Fatalf("MissionCmd %q exit = %d, want %d (out=%q err=%q)", tc.name, code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("MissionCmd %q stdout debe contener %q, got %q", tc.name, tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("MissionCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}
