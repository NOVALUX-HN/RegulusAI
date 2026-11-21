// RenderExplain cover (CRAP<50 sin cambiar comportamiento): table-driven
// puro, sin FS. Cubre why split (con/sin componentes), colisiones
// 0/>0, autonomy loop (con/sin bloqueos), history 0/1/>0 con y sin
// diff (incluido el fallback prev==current), y Confirmed 0/>0.
package planning

import (
	"strings"
	"testing"
	"time"
)

func explainFixedTime() time.Time {
	return time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
}

func explainTask(id, autonomy string, comps, blockers []string) PlanTask {
	return PlanTask{
		ID:       id,
		Title:    "task " + id,
		Autonomy: autonomy,
		Scope:    Scope{Files: []string{"internal/" + id + "/x.go"}, Components: comps},
		Blockers: blockers,
	}
}

func TestRenderExplainCover(t *testing.T) {
	fixed := explainFixedTime()
	cases := []struct {
		name    string
		current *Plan
		history []*Plan
		want    []string
		absent  []string
	}{
		{
			name: "minimal unknown scope no collisions no history no confirmed",
			current: &Plan{
				PlanVersion: 1, MissionID: "m1", Status: StatusReady,
				CreatedAt: fixed, Quality: Quality{},
			},
			history: nil,
			want: []string{
				"explain plan v1 mission m1",
				"why split", "(alcance por delimitar)",
				"why serialize: sin colisiones",
				"autonomy:", "quality:",
			},
			absent: []string{"history:", "confirmed results", "diff vs", "bloquea:", "(motivo:"},
		},
		{
			name: "components collisions blockers quality mixed",
			current: &Plan{
				PlanVersion: 1, MissionID: "m2", Status: StatusReady, CreatedAt: fixed,
				Tasks: []PlanTask{
					explainTask("t1", AutonomyAutonomous, []string{"api"}, nil),
					explainTask("t2", AutonomyNeedsApproval, []string{"api", "web"}, []string{"db locked"}),
				},
				DAG: DAG{Collisions: [][2]string{{"t1", "t2"}}},
				Quality: Quality{
					Checks:   []Check{{Name: "g1", Pass: true, Detail: "ok"}, {Name: "g2", Pass: false, Detail: "bad"}},
					Warnings: []string{"watch out"},
				},
			},
			history: nil,
			want: []string{
				"api", "web",
				"1 colisión(es)", "t1 antes que t2",
				"bloquea: db locked",
				"[PASS] g1", "[FAIL] g2", "warning: watch out",
			},
			absent: []string{"history:", "confirmed results", "sin colisiones"},
		},
		{
			name: "history single with diff and confirmed",
			current: &Plan{
				PlanVersion: 2, MissionID: "m3", Status: StatusNeedsApproval,
				CreatedAt: fixed, ChangeReason: "expand",
				Tasks:     []PlanTask{explainTask("t2", AutonomyAutonomous, []string{"api"}, nil)},
				Quality:   Quality{},
				Confirmed: []ConfirmedResult{{TaskID: "t1", Result: "ok", At: fixed}},
			},
			history: []*Plan{
				{
					PlanVersion: 1, MissionID: "m3", Status: StatusReady,
					CreatedAt: fixed, ChangeReason: "init",
					Tasks: []PlanTask{explainTask("t1", AutonomyAutonomous, []string{"api"}, nil)},
				},
			},
			want: []string{
				"history: 1", "(motivo: init)",
				"diff vs original (v1 → v2)", "añadidas", "eliminadas", "estado",
				"confirmed results", "t1",
			},
			absent: []string{"diff vs previous"},
		},
		{
			name: "history single no diff no confirmed no reason",
			current: &Plan{
				PlanVersion: 1, MissionID: "m4", Status: StatusReady,
				CreatedAt: fixed,
				Tasks:     []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
			},
			history: []*Plan{
				{
					PlanVersion: 1, MissionID: "m4", Status: StatusReady,
					CreatedAt: fixed,
					Tasks:     []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
				},
			},
			want:   []string{"history: 1"},
			absent: []string{"diff vs original", "diff vs previous", "confirmed results", "(motivo:"},
		},
		{
			name: "history two normal previous diff",
			current: &Plan{
				PlanVersion: 3, MissionID: "m5", Status: StatusNeedsApproval,
				CreatedAt: fixed, ChangeReason: "v3",
				Tasks: []PlanTask{
					explainTask("t1", AutonomyAutonomous, nil, nil),
					explainTask("t2", AutonomyAutonomous, nil, nil),
					explainTask("t3", AutonomyAutonomous, nil, nil),
				},
			},
			history: []*Plan{
				{
					PlanVersion: 1, Status: StatusReady, CreatedAt: fixed, ChangeReason: "v1",
					Tasks: []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
				},
				{
					PlanVersion: 2, Status: StatusReady, CreatedAt: fixed, ChangeReason: "v2",
					Tasks: []PlanTask{
						explainTask("t1", AutonomyAutonomous, nil, nil),
						explainTask("t2", AutonomyAutonomous, nil, nil),
					},
				},
			},
			want: []string{
				"history: 2",
				"diff vs original (v1 → v3)",
				"diff vs previous (v2 → v3)",
			},
		},
		{
			name: "history two fallback prev equals current",
			current: &Plan{
				PlanVersion: 2, MissionID: "m6", Status: StatusNeedsApproval,
				CreatedAt: fixed,
				Tasks: []PlanTask{
					explainTask("t1", AutonomyAutonomous, nil, nil),
					explainTask("t2", AutonomyAutonomous, nil, nil),
					explainTask("t3", AutonomyAutonomous, nil, nil),
				},
			},
			history: []*Plan{
				{
					PlanVersion: 1, Status: StatusReady, CreatedAt: fixed,
					Tasks: []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
				},
				{
					PlanVersion: 2, Status: StatusReady, CreatedAt: fixed,
					Tasks: []PlanTask{
						explainTask("t1", AutonomyAutonomous, nil, nil),
						explainTask("t2", AutonomyAutonomous, nil, nil),
					},
				},
			},
			want: []string{
				"history: 2",
				"diff vs original (v1 → v2)",
				"diff vs previous (v1 → v2)",
			},
		},
		{
			name: "history two no diff when versions equal",
			current: &Plan{
				PlanVersion: 2, MissionID: "m7", Status: StatusReady,
				CreatedAt: fixed,
				Tasks:     []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
			},
			history: []*Plan{
				{
					PlanVersion: 2, Status: StatusReady, CreatedAt: fixed,
					Tasks: []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
				},
				{
					PlanVersion: 2, Status: StatusReady, CreatedAt: fixed,
					Tasks: []PlanTask{explainTask("t1", AutonomyAutonomous, nil, nil)},
				},
			},
			want:   []string{"history: 2"},
			absent: []string{"diff vs original", "diff vs previous"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderExplain(tc.current, tc.history)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Fatalf("RenderExplain %q debe contener %q, got:\n%s", tc.name, w, got)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Fatalf("RenderExplain %q no debe contener %q, got:\n%s", tc.name, a, got)
				}
			}
		})
	}
}
