// Exhaustive table tests for ValidatePlan (F13.2/NOV-105): one row
// per error group plus the happy paths. Table-only: every mutation
// starts from the same valid plan so a message change fails loudly.
package planning

import (
	"fmt"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

func validValidateTask(id string) PlanTask {
	return PlanTask{
		ID: id, Title: "título " + id, Objective: "objetivo", Deliverable: "entregable",
		Scope:                Scope{Files: []string{"internal/a/a.go"}},
		RequiredCapabilities: []string{bench.CapTools}, MinScopes: []string{"spec:read"},
		Estimate: Estimate{Complexity: "low", Risk: "low", Note: EstimateNote},
		Autonomy: AutonomyAutonomous, ImpactKnown: true,
	}
}

func validValidatePlan() *Plan {
	return &Plan{
		Version: PlanVersion, PlanVersion: 1, MissionID: "m1",
		Objective: "objetivo suficiente para validar", Kind: KindSimple, Status: StatusReady,
		Tasks: []PlanTask{validValidateTask("t1")},
	}
}

func TestValidatePlanTable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Plan)
		want   string
		isNil  bool
	}{
		{name: "nulo", mutate: nil, want: "plan nulo"},
		{name: "version", mutate: func(p *Plan) { p.Version = "plan-v0" }, want: "se exige"},
		{name: "kind", mutate: func(p *Plan) { p.Kind = "raro" }, want: `kind "raro" desconocido`},
		{name: "status", mutate: func(p *Plan) { p.Status = "raro" }, want: `status "raro" desconocido`},
		{
			name: "adversarial con tareas", mutate: func(p *Plan) { p.Kind = KindAdversarial },
			want: "adversarial con",
		},
		{
			name: "adversarial vacio ok", mutate: func(p *Plan) { p.Kind = KindAdversarial; p.Status = StatusRefused; p.Tasks = nil },
			isNil: true,
		},
		{name: "vacio", mutate: func(p *Plan) { p.Tasks = nil }, want: "vacío no es ejecutable"},
		{
			name: "maximo", mutate: func(p *Plan) {
				p.Tasks = nil
				for i := 0; i <= MaxTasks; i++ {
					p.Tasks = append(p.Tasks, validValidateTask(fmt.Sprintf("tx%d", i)))
				}
			}, want: "máximo",
		},
		{name: "sin id", mutate: func(p *Plan) { p.Tasks[0].ID = "  " }, want: "sin id"},
		{
			name: "duplicada", mutate: func(p *Plan) { p.Tasks = append(p.Tasks, validValidateTask("t1")) },
			want: `duplicada "t1"`,
		},
		{name: "sin objetivo", mutate: func(p *Plan) { p.Tasks[0].Objective = " " }, want: `sin objetivo`},
		{name: "sin entregable", mutate: func(p *Plan) { p.Tasks[0].Deliverable = "" }, want: "sin entregable"},
		{name: "autonomia", mutate: func(p *Plan) { p.Tasks[0].Autonomy = "volar" }, want: "autonomía"},
		{
			name: "sin scope", mutate: func(p *Plan) { p.Tasks[0].Scope.Files = nil },
			want: "sin scope de ficheros",
		},
		{
			name:   "scope needs-user sin ficheros ok",
			mutate: func(p *Plan) { p.Tasks[0].Scope.Files = nil; p.Tasks[0].Autonomy = AutonomyNeedsUser },
			isNil:  true,
		},
		{name: "fichero con puntos", mutate: func(p *Plan) { p.Tasks[0].Scope.Files = []string{"../x.go"} }, want: "inválido"},
		{name: "fichero vacio", mutate: func(p *Plan) { p.Tasks[0].Scope.Files = []string{"  "} }, want: "inválido"},
		{name: "depende de si misma", mutate: func(p *Plan) { p.Tasks[0].DependsOn = []string{"t1"} }, want: "sí misma"},
		{name: "dependencia desconocida", mutate: func(p *Plan) { p.Tasks[0].DependsOn = []string{"t9"} }, want: "desconocida"},
		{
			name: "dependencia forward valida",
			mutate: func(p *Plan) {
				t2 := validValidateTask("t2")
				p.Tasks[0].DependsOn = []string{"t2"}
				p.Tasks = append(p.Tasks, t2)
			},
			isNil: true,
		},
		{name: "capability sin verificar", mutate: func(p *Plan) { p.Tasks[0].RequiredCapabilities = []string{"telepatia"} }, want: "sin evidencia probada"},
		{name: "agente fuera del registry", mutate: func(p *Plan) { p.Tasks[0].Agent = "nadie" }, want: "fuera del registry"},
		{name: "scopes superiores", mutate: func(p *Plan) { p.Tasks[0].Agent = "baseline"; p.Tasks[0].MinScopes = []string{"memory:write"} }, want: "superiores"},
		{
			name:   "agente baseline ok",
			mutate: func(p *Plan) { p.Tasks[0].Agent = "baseline"; p.Tasks[0].MinScopes = []string{"spec:read"} },
			isNil:  true,
		},
		{name: "estimacion sin etiqueta", mutate: func(p *Plan) { p.Tasks[0].Estimate.Note = "medición exacta" }, want: "sin etiqueta"},
		{
			name: "ciclo",
			mutate: func(p *Plan) {
				t1 := validValidateTask("t1")
				t1.DependsOn = []string{"t2"}
				t2 := validValidateTask("t2")
				t2.DependsOn = []string{"t1"}
				p.Tasks = []PlanTask{t1, t2}
			},
			want: "cycl",
		},
		{name: "feliz simple", mutate: func(p *Plan) {}, isNil: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p *Plan
			if c.mutate == nil {
				p = nil
			} else {
				p = validValidatePlan()
				c.mutate(p)
			}
			err := ValidatePlan(p, testRegistry(), testManifests())
			if c.isNil {
				if err != nil {
					t.Fatalf("ValidatePlan(%s) = %v, want nil", c.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidatePlan(%s) = nil, want %q", c.name, c.want)
			}
			if c.name == "ciclo" {
				lower := strings.ToLower(err.Error())
				if !strings.Contains(lower, "cycle") && !strings.Contains(err.Error(), "ciclo") {
					t.Fatalf("ciclo debe nombrar el bucle, got %v", err)
				}
				return
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidatePlan(%s) = %v, want %q", c.name, err, c.want)
			}
			if !strings.HasPrefix(err.Error(), "planning: ") {
				t.Fatalf("el prefijo debe ser planning:, got %v", err)
			}
		})
	}
}
