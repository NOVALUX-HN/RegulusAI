// gradeQuality directo: Plan+reg+manifests sin disco ni reloj.
// Tabla early adversarial + 8 checks + fold de Warnings. Portable.
package planning

import (
	"testing"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/orchestrate"
)

func gqReg() []orchestrate.RoutedAgent {
	return []orchestrate.RoutedAgent{{ID: "atlas", Scopes: []string{"s1", "s2"}}}
}

func gqManifests() map[string]bench.HostManifest {
	return map[string]bench.HostManifest{
		"h1": {Host: "h1", Capabilities: []bench.HostCapability{
			{ID: "cap-a", Status: bench.CapVerified, Evidence: "ev fechada"},
		}},
	}
}

func gqBase() *Plan {
	return &Plan{
		Kind: KindSimple,
		Criteria: []Criterion{
			{Text: "Given a When b Then 1", Verifiable: true, CoveringTask: "t1"},
		},
		Tasks: []PlanTask{
			{
				ID: "t1", Agent: "atlas",
				RequiredCapabilities: []string{"cap-a"},
				MinScopes:            []string{"s1"},
				ImpactKnown:          true,
				Estimate:             Estimate{Note: "estimación, no medición"},
				Autonomy:             AutonomyAutonomous,
			},
		},
		DAG: DAG{Order: []string{"t1"}, Levels: [][]string{{"t1"}}},
	}
}

func gqByName(q Quality, name string) (Check, bool) {
	for _, c := range q.Checks {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func TestGradeQualityEarlyAdversarial(t *testing.T) {
	p := &Plan{Kind: KindAdversarial}
	q := gradeQuality(p, gqReg(), gqManifests())
	if len(q.Checks) != 2 {
		t.Fatalf("adversarial = %d checks, want 2", len(q.Checks))
	}
	if c, _ := gqByName(q, "plan-no-vacio"); c.Pass {
		t.Fatalf("adversarial plan-no-vacio debe fallar: %+v", c)
	}
	if c, _ := gqByName(q, "capabilities-verificadas"); !c.Pass {
		t.Fatalf("adversarial capabilities debe pasar: %+v", c)
	}
	if len(q.Warnings) != 0 {
		t.Fatalf("early no pliega warnings: %v", q.Warnings)
	}
}

func TestGradeQualityChecksTable(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(p *Plan)
		check     string
		wantPass  bool
		wantWarns int
	}{
		{name: "ok todo pasa", mutate: func(p *Plan) {}, check: "plan-no-vacio", wantPass: true, wantWarns: 0},
		{name: "plan vacio", mutate: func(p *Plan) { p.Tasks = nil }, check: "plan-no-vacio", wantPass: false, wantWarns: 0},
		{name: "criterio sin covering", mutate: func(p *Plan) { p.Criteria[0].CoveringTask = "" }, check: "criterios-cubiertos", wantPass: false, wantWarns: 0},
		{name: "criterio ambiguo avisa", mutate: func(p *Plan) { p.Criteria[0].Verifiable = false }, check: "criterios-verificables", wantPass: false, wantWarns: 1},
		{name: "cap sin verificar", mutate: func(p *Plan) { p.Tasks[0].RequiredCapabilities = []string{"cap-missing"} }, check: "capabilities-verificadas", wantPass: false, wantWarns: 0},
		{name: "scope excede", mutate: func(p *Plan) { p.Tasks[0].MinScopes = []string{"s3"} }, check: "scopes-minimos", wantPass: false, wantWarns: 0},
		{name: "scope sin agente", mutate: func(p *Plan) { p.Tasks[0].Agent = "" }, check: "scopes-minimos", wantPass: false, wantWarns: 0},
		{name: "scope needs-user sin agente pasa", mutate: func(p *Plan) { p.Tasks[0].Agent = ""; p.Tasks[0].Autonomy = AutonomyNeedsUser }, check: "scopes-minimos", wantPass: true, wantWarns: 0},
		{name: "dag ciclo", mutate: func(p *Plan) { p.DAG.Cycles = [][]string{{"t1"}} }, check: "dag-aciclico", wantPass: false, wantWarns: 0},
		{name: "impacto desconocido avisa", mutate: func(p *Plan) { p.Tasks[0].ImpactKnown = false }, check: "impacto-conocido", wantPass: false, wantWarns: 1},
		{name: "estimacion sin etiqueta", mutate: func(p *Plan) { p.Tasks[0].Estimate.Note = "plain" }, check: "estimaciones-etiquetadas", wantPass: false, wantWarns: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := gqBase()
			tc.mutate(p)
			q := gradeQuality(p, gqReg(), gqManifests())
			if len(q.Checks) != 8 {
				t.Fatalf("checks = %d, want 8", len(q.Checks))
			}
			c, ok := gqByName(q, tc.check)
			if !ok {
				t.Fatalf("falta check %q", tc.check)
			}
			if c.Pass != tc.wantPass {
				t.Fatalf("%s pass = %v, want %v (%q)", tc.check, c.Pass, tc.wantPass, c.Detail)
			}
			if len(q.Warnings) != tc.wantWarns {
				t.Fatalf("warnings = %v, want %d", q.Warnings, tc.wantWarns)
			}
		})
	}
}

func TestGradeQualityWarningsFold(t *testing.T) {
	p := gqBase()
	p.Criteria[0].Verifiable = false
	p.Tasks[0].ImpactKnown = false
	q := gradeQuality(p, gqReg(), gqManifests())
	if len(q.Warnings) != 2 {
		t.Fatalf("warnings = %v, want 2 (solo impacto+verificables)", q.Warnings)
	}
	for _, w := range q.Warnings {
		if w == "" {
			t.Fatal("warning vacio")
		}
	}
}
