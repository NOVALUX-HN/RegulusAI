// Router EvolveValidationCmd (CRAP<30 sin cambiar comportamiento):
// tabla con los 9 verbos principales más vacío/desconocido y los 11
// cycle-* con Dir tmp, sin partir subcomandos. Cada despacho rehúsa
// con 2 antes de IO, portable (t.TempDir, sin shell ni red).
package cmdx

import (
	"strings"
	"testing"
)

func TestEvolveValidationRouterTable(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{name: "vacio", args: nil, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "desconocido", args: []string{"frobnicate"}, wantCode: 2, wantErr: `unknown subcommand "frobnicate"`},
		{name: "declare", args: []string{"declare"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "run", args: []string{"run"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "holdout-run", args: []string{"holdout-run"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "control", args: []string{"control"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "discard", args: []string{"discard"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "link", args: []string{"link"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "compare", args: []string{"compare"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "finalize", args: []string{"finalize"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "report", args: []string{"report"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-declare", args: []string{"cycle-declare"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-propose", args: []string{"cycle-propose"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-link", args: []string{"cycle-link"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-compare", args: []string{"cycle-compare"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-review", args: []string{"cycle-review"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-approve", args: []string{"cycle-approve"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-canary", args: []string{"cycle-canary"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-monitor", args: []string{"cycle-monitor"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-promote", args: []string{"cycle-promote"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-rollback", args: []string{"cycle-rollback"}, wantCode: 2, wantErr: "regulus evolve validation"},
		{name: "cycle-report", args: []string{"cycle-report"}, wantCode: 2, wantErr: "regulus evolve validation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			io, _, errb := missionIO(dir)
			if code := io.EvolveValidationCmd(tc.args); code != tc.wantCode {
				t.Fatalf("EvolveValidationCmd %q exit = %d, want %d (err=%q)", tc.name, code, tc.wantCode, errb.String())
			}
			if !strings.Contains(errb.String(), tc.wantErr) {
				t.Fatalf("EvolveValidationCmd %q stderr debe contener %q, got %q", tc.name, tc.wantErr, errb.String())
			}
		})
	}
}
