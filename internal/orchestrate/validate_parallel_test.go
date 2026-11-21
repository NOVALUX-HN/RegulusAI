// ValidateParallelTasks: tabla por rama (vacío, máximo, id inválido o
// duplicado, título, files, depends, command, scopes, agent,
// estimates) más los límites válidos. Pura: sin IO ni runner, y la
// función no se parte: cada negativa exige su mensaje y cada borde
// válido exige éxito.
package orchestrate

import (
	"fmt"
	"strings"
	"testing"
)

func validParallelTask(id string) ParallelTask {
	return ParallelTask{ID: id, Title: "Task " + id, Files: []string{"pkg/" + id + ".go"}}
}

func manyParallelTasks(n int) []ParallelTask {
	tasks := make([]ParallelTask, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, validParallelTask(fmt.Sprintf("t%d", i)))
	}
	return tasks
}

func TestValidateParallelTasksTable(t *testing.T) {
	longID := strings.Repeat("a", 129)
	longTitle := strings.Repeat("t", 257)
	okTitle := strings.Repeat("t", 256)
	longFile := strings.Repeat("f", 513)
	longCmd := strings.Repeat("c", 4001)
	okCmd := strings.Repeat("c", 4000)
	longAgent := strings.Repeat("g", 65)
	okAgent := strings.Repeat("g", 64)
	longScope := strings.Repeat("s", 65)

	files32 := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		files32 = append(files32, fmt.Sprintf("pkg/f%d.go", i))
	}
	scopes16 := make([]string, 0, 16)
	for i := 0; i < 16; i++ {
		scopes16 = append(scopes16, fmt.Sprintf("s%d", i))
	}
	tooManyFiles := validParallelTask("a")
	for i := 0; i < 33; i++ {
		tooManyFiles.Files = append(tooManyFiles.Files, fmt.Sprintf("f%d.go", i))
	}
	tooManyDeps := validParallelTask("a")
	for i := 0; i < 33; i++ {
		tooManyDeps.DependsOn = append(tooManyDeps.DependsOn, fmt.Sprintf("d%d", i))
	}
	tooManyScopes := validParallelTask("a")
	for i := 0; i < 17; i++ {
		tooManyScopes.Scopes = append(tooManyScopes.Scopes, fmt.Sprintf("s%d", i))
	}
	boundary := validParallelTask("edge")
	boundary.Title = okTitle
	boundary.Files = files32
	boundary.Scopes = scopes16
	boundary.Command = okCmd
	boundary.Agent = okAgent
	chain := []ParallelTask{}
	deps := []string{}
	for i := 1; i <= 32; i++ {
		id := fmt.Sprintf("d%d", i)
		deps = append(deps, id)
		chain = append(chain, validParallelTask(id))
	}
	head := validParallelTask("d0")
	head.DependsOn = deps
	chain = append([]ParallelTask{head}, chain...)

	with := func(base ParallelTask, mutate func(*ParallelTask)) []ParallelTask {
		mutate(&base)
		return []ParallelTask{base}
	}

	cases := []struct {
		name  string
		tasks []ParallelTask
		want  string // "" = éxito esperado
	}{
		{name: "vacío", tasks: nil, want: "at least one task is required"},
		{name: "máximo excedido", tasks: manyParallelTasks(MaxParallelTasks + 1), want: "at most 64 tasks"},
		{name: "id vacío", tasks: []ParallelTask{{ID: "", Title: "T"}}, want: "invalid task id"},
		{name: "id con recorrido", tasks: []ParallelTask{{ID: "../x", Title: "T"}}, want: "invalid task id"},
		{name: "id con espacio", tasks: []ParallelTask{{ID: "a b", Title: "T"}}, want: "invalid task id"},
		{name: "id con signo", tasks: []ParallelTask{{ID: "a!b", Title: "T"}}, want: "invalid task id"},
		{name: "id largo", tasks: []ParallelTask{{ID: longID, Title: "T"}}, want: "invalid task id"},
		{name: "id duplicado", tasks: []ParallelTask{validParallelTask("a"), validParallelTask("a")}, want: `duplicate task id "a"`},
		{name: "título vacío", tasks: []ParallelTask{{ID: "a", Title: ""}}, want: "needs a title"},
		{name: "título en blanco", tasks: []ParallelTask{{ID: "a", Title: "   "}}, want: "needs a title"},
		{name: "título multilínea", tasks: []ParallelTask{{ID: "a", Title: "a\nb"}}, want: "single line under 256"},
		{name: "título largo", tasks: []ParallelTask{{ID: "a", Title: longTitle}}, want: "single line under 256"},
		{name: "demasiados files", tasks: []ParallelTask{tooManyFiles}, want: "at most 32 files"},
		{name: "file vacío", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{""} }), want: "empty file entry"},
		{name: "file largo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{longFile} }), want: "exceeds 512"},
		{name: "file con NUL", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{"a\x00b"} }), want: "must be a single line"},
		{name: "file con salto", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{"a\nb"} }), want: "must be a single line"},
		{name: "file absoluto", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{"/abs/x.go"} }), want: "repo-relative, not absolute"},
		{name: "file con unidad", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{"C:/x.go"} }), want: "repo-relative, not absolute"},
		{name: "file con recorrido", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Files = []string{"a/../../b"} }), want: "'..'"},
		{name: "demasiados depends", tasks: []ParallelTask{tooManyDeps}, want: "at most 32 dependencies"},
		{name: "dependencia propia", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.DependsOn = []string{"a"} }), want: "must not depend on itself"},
		{name: "dependencia inválida", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.DependsOn = []string{"a b"} }), want: "invalid dependency"},
		{name: "dependencia duplicada", tasks: []ParallelTask{with(validParallelTask("a"), func(p *ParallelTask) { p.DependsOn = []string{"b", "b"} })[0], validParallelTask("b")}, want: "duplicates dependency"},
		{name: "command largo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Command = longCmd }), want: "exceeds 4000"},
		{name: "command con NUL", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Command = "run\x00x" }), want: "must not contain NUL"},
		{name: "demasiados scopes", tasks: []ParallelTask{tooManyScopes}, want: "at most 16 scopes"},
		{name: "scope vacío", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Scopes = []string{""} }), want: "invalid scope"},
		{name: "scope largo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Scopes = []string{longScope} }), want: "invalid scope"},
		{name: "scope multilínea", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Scopes = []string{"a\nb"} }), want: "invalid scope"},
		{name: "agent largo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Agent = longAgent }), want: "single line under 64"},
		{name: "agent multilínea", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.Agent = "a\nb" }), want: "single line under 64"},
		{name: "estimate_ms negativo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.EstimateMS = -1 }), want: "estimate_ms"},
		{name: "estimate_tokens negativo", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.EstimateTokens = -1 }), want: "estimate_tokens"},
		{name: "dependencia desconocida", tasks: with(validParallelTask("a"), func(p *ParallelTask) { p.DependsOn = []string{"b"} }), want: "unknown task"},
		{name: "mínima válida", tasks: []ParallelTask{{ID: "a", Title: "A"}}, want: ""},
		{name: "límites válidos", tasks: []ParallelTask{boundary}, want: ""},
		{name: "32 depends válidos", tasks: chain, want: ""},
		{name: "64 tareas válidas", tasks: manyParallelTasks(MaxParallelTasks), want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateParallelTasks(tc.tasks)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("esperaba éxito, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("esperaba error con %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q debe contener %q", err.Error(), tc.want)
			}
		})
	}
}
