// GovernCmd router cover (CRAP<50 sin cambiar comportamiento): tabla
// mínima help/vacío/unknown/1 subcomando con IO fake, extendida a
// todos los despachos para cubrir cada rama del switch sin tocar
// mensajes ni stores fuera de tmp. Ningún caso necesita seam: los
// subcomandos con flags faltantes rehúsan con 2 antes de IO, y
// matrix/list/audit leen stores vacíos en tmp.
package cmdx

import (
	"strings"
	"testing"
)

func TestGovernRouterCover(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
		wantOut  string
	}{
		{name: "help", args: []string{"--help"}, wantCode: 0, wantOut: "regulus govern"},
		{name: "help short", args: []string{"-h"}, wantCode: 0, wantOut: "regulus govern"},
		{name: "empty", args: nil, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "unknown", args: []string{"frobnicate"}, wantCode: 2, wantErr: `unknown subcommand "frobnicate"`},
		{name: "matrix", args: []string{"matrix"}, wantCode: 0, wantOut: "deny-by-default"},
		{name: "authorize missing flags", args: []string{"authorize"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "request missing flags", args: []string{"request"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "approve missing flags", args: []string{"approve"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "check missing flags", args: []string{"check"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "pause missing flags", args: []string{"pause"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "resume missing flags", args: []string{"resume"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "reject missing flags", args: []string{"reject"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "revoke missing flags", args: []string{"revoke"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "show missing flags", args: []string{"show"}, wantCode: 2, wantErr: "usage: regulus govern"},
		{name: "list empty store", args: []string{"list"}, wantCode: 0},
		{name: "audit empty store", args: []string{"audit"}, wantCode: 0},
		{name: "policy missing flags", args: []string{"policy"}, wantCode: 2, wantErr: "usage: regulus govern"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			io, out, errb := missionIO(dir)
			if code := io.GovernCmd(tc.args); code != tc.wantCode {
				t.Fatalf("GovernCmd %q exit = %d, want %d (out=%q err=%q)", tc.name, code, tc.wantCode, out.String(), errb.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Fatalf("GovernCmd %q stdout debe contener %q, got %q", tc.name, tc.wantOut, out.String())
			}
			if tc.wantErr != "" && !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("GovernCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}
