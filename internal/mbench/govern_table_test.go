// Govern axis tests (CRAP split): pure op builder, routine decisions
// and one minimal RunCase per axis (routine vs approval vs MB4-deny
// vs MB9-revoke). In-process, isolated roots, no network.
package mbench

import (
	"strings"
	"testing"

	"regulus-ai/internal/govern"
)

func TestGovernOpTable(t *testing.T) {
	cases := []struct {
		name       string
		c          MissionCase
		kind       string
		surface    string
		reversible bool
		scope      string
	}{
		{"routine read", MissionCase{ID: "MB1"}, govern.KindRead, govern.SurfaceCode, true, "spec:read"},
		{"approval deploy", MissionCase{ID: "MB5", NeedsApproval: true}, govern.KindDeploy, govern.SurfaceCore, false, "spec:read"},
		{"adversarial deny", MissionCase{ID: "MB4"}, govern.KindDeploy, govern.SurfaceCore, false, "admin:root"},
		{"adversarial wins over approval flag", MissionCase{ID: "MB4", NeedsApproval: true}, govern.KindDeploy, govern.SurfaceCore, false, "admin:root"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := governOpFor(tc.c)
			if op.Kind != tc.kind || op.Surface != tc.surface || op.Reversible != tc.reversible {
				t.Fatalf("op = %+v (want %s/%s reversible=%v)", op, tc.kind, tc.surface, tc.reversible)
			}
			if len(op.Scopes) != 1 || op.Scopes[0] != tc.scope {
				t.Fatalf("scopes = %v (want [%s])", op.Scopes, tc.scope)
			}
			if op.Label != tc.c.ID {
				t.Fatalf("label = %q (want %q)", op.Label, tc.c.ID)
			}
		})
	}
}

func TestGovernRoutineDecisions(t *testing.T) {
	j := &journey{}
	if !j.checkRoutineOp(govern.Decision{Allow: true}) {
		t.Fatal("allowed routine op must pass")
	}
	if !j.checkRoutineOp(govern.Decision{NeedsApproval: true}) {
		t.Fatal("routine op that needs approval still passes the routine gate")
	}
	denied := &journey{}
	if denied.checkRoutineOp(govern.Decision{Reason: "missing scope"}) {
		t.Fatal("denied routine op must not pass")
	}
	if denied.res.Violations != 1 {
		t.Fatalf("violations = %d (want 1)", denied.res.Violations)
	}
	if !strings.Contains(denied.res.Detail, "routine op denied") {
		t.Fatalf("detail must quote the denial: %q", denied.res.Detail)
	}
}

func TestGovernRunCaseAxes(t *testing.T) {
	byID := map[string]MissionCase{}
	for _, c := range MissionCorpus() {
		byID[c.ID] = c
	}
	axes := []struct {
		id            string
		wantSimulated bool
	}{
		{"MB1", false}, // routine: autonomous read
		{"MB5", false}, // approval: bound human approval
		{"MB4", true},  // deny: adversarial op refused
		{"MB9", true},  // revoke: revoked grant stops the journey
	}
	for _, ax := range axes {
		t.Run(ax.id, func(t *testing.T) {
			r := RunCase(byID[ax.id])
			if !r.AssistedOK || !r.EvidenceValid || !r.BaselineOK || r.Violations != 0 {
				t.Fatalf("case %s not green: %+v", ax.id, r)
			}
			if ax.wantSimulated && len(r.Simulated) == 0 {
				t.Fatalf("case %s must label its simulated signal: %+v", ax.id, r)
			}
			if !ax.wantSimulated && len(r.Simulated) != 0 {
				t.Fatalf("case %s must run without simulated signals: %+v", ax.id, r)
			}
		})
	}
}
