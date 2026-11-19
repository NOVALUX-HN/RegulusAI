// Table tests for risksFor (F13.2/NOV-105): one row per risk branch.
// Pure: no FS, no network; the mission, the scope hit and the tasks
// are built inline so every line names its terminal effect.
package planning

import (
	"strings"
	"testing"

	"regulus-ai/internal/mission"
)

func risksMission(risk string, tokens int64) *mission.Mission {
	m := &mission.Mission{Risk: risk}
	if tokens != 0 {
		m.Budget.Tokens = tokens
	}
	return m
}

func TestRisksForTable(t *testing.T) {
	cases := []struct {
		name    string
		m       *mission.Mission
		hit     scopeHit
		graphOK bool
		tasks   []PlanTask
		wantLen int
		want    []string
		notWant []string
	}{
		{
			name: "solo mision sin extras",
			m:    risksMission("low", 0), hit: scopeHit{}, graphOK: true,
			tasks:   nil,
			wantLen: 1,
			want:    []string{"riesgo de misión low"},
			notWant: []string{"paquetes críticos", "serializada", "grafo F12.8", "sin correspondencia", "presupuesto estimado"},
		},
		{
			name: "criticos por blocker",
			m:    risksMission("medium", 0), hit: scopeHit{}, graphOK: true,
			tasks: []PlanTask{
				{ID: "t1", Blockers: []string{"guarda crítico: gates"}},
				{ID: "t2", Blockers: []string{"nota menor"}},
			},
			wantLen: 2,
			want:    []string{"riesgo de misión medium", "paquetes críticos bajo el alcance (tareas t1)"},
			notWant: []string{"serializada", "grafo F12.8"},
		},
		{
			name: "criticos por rationale",
			m:    risksMission("low", 0), hit: scopeHit{}, graphOK: true,
			tasks: []PlanTask{
				{ID: "t9", Rationale: "toca ruta crítico del kernel"},
			},
			wantLen: 2,
			want:    []string{"paquetes críticos bajo el alcance (tareas t9)"},
		},
		{
			name: "colision serializa",
			m:    risksMission("low", 0), hit: scopeHit{}, graphOK: true,
			tasks: []PlanTask{
				{ID: "t1", Blockers: []string{"serialización: t1 tras t0 por internal/a/a.go"}},
				{ID: "t2", Blockers: []string{"serialización: t2 tras t1 por internal/a/a.go"}},
			},
			wantLen: 2,
			want:    []string{"2 tarea(s) serializada(s)"},
		},
		{
			name: "grafo ausente",
			m:    risksMission("high", 0), hit: scopeHit{}, graphOK: false,
			tasks:   nil,
			wantLen: 2,
			want:    []string{"grafo F12.8 no disponible"},
		},
		{
			name: "terminos sin correspondencia",
			m:    risksMission("low", 0), hit: scopeHit{unknown: []string{"zorblax", "quux"}}, graphOK: true,
			tasks:   nil,
			wantLen: 2,
			want:    []string{"términos sin correspondencia en el corpus (zorblax, quux)"},
		},
		{
			name: "presion de presupuesto",
			m:    risksMission("low", 5000), hit: scopeHit{}, graphOK: true,
			tasks: []PlanTask{
				{ID: "t1", Estimate: Estimate{Tokens: 1200, Note: EstimateNote}},
				{ID: "t2", Estimate: Estimate{Tokens: 800, Note: EstimateNote}},
			},
			wantLen: 2,
			want:    []string{"presupuesto estimado total 2000 tokens", "techo de misión tokens=5000"},
		},
		{
			name: "todo combinado suma seis lineas",
			m:    risksMission("critical", 9000), hit: scopeHit{unknown: []string{"zorblax"}}, graphOK: false,
			tasks: []PlanTask{
				{ID: "t1", Blockers: []string{"guarda crítico: receipts", "serialización: t1 tras t0"}, Rationale: "x", Estimate: Estimate{Tokens: 100, Note: EstimateNote}},
			},
			wantLen: 6,
			want: []string{
				"riesgo de misión critical",
				"paquetes críticos",
				"serializada",
				"grafo F12.8",
				"sin correspondencia",
				"presupuesto estimado total 100 tokens",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := risksFor(c.m, c.hit, c.graphOK, c.tasks)
			if len(got) != c.wantLen {
				t.Fatalf("risksFor = %d líneas %q, want %d", len(got), got, c.wantLen)
			}
			joined := strings.Join(got, "\n")
			for _, w := range c.want {
				if !strings.Contains(joined, w) {
					t.Errorf("riesgo %q ausente en %q", w, joined)
				}
			}
			for _, w := range c.notWant {
				if strings.Contains(joined, w) {
					t.Errorf("riesgo %q inesperado en %q", w, joined)
				}
			}
			if !strings.HasPrefix(got[0], "riesgo de misión ") {
				t.Errorf("primera línea debe ser el riesgo de misión, got %q", got[0])
			}
		})
	}
}
