// Split validators for ValidateParallelTasks (CRAP paydown): direct
// unit tables per group (count, ids, titles, files, deps, command,
// scopes, agent, estimates, unknown deps). The parent only wires the
// groups in field order; every message stays byte-identical.
package orchestrate

import (
	"strings"
	"testing"
)

func splitValidTask(id string) ParallelTask {
	return ParallelTask{ID: id, Title: "Task " + id, Files: []string{"pkg/" + id + ".go"}}
}

func TestValidateParallelGroupsTable(t *testing.T) {
	t.Run("count", func(t *testing.T) {
		if err := validateParallelCount(nil); err == nil || !strings.Contains(err.Error(), "at least one task") {
			t.Fatalf("vacio debe fallar, got %v", err)
		}
		many := make([]ParallelTask, 0, MaxParallelTasks+1)
		for i := 0; i <= MaxParallelTasks; i++ {
			many = append(many, splitValidTask("t"))
		}
		if err := validateParallelCount(many); err == nil || !strings.Contains(err.Error(), "at most 64") {
			t.Fatalf("exceso debe fallar, got %v", err)
		}
		if err := validateParallelCount([]ParallelTask{splitValidTask("a")}); err != nil {
			t.Fatalf("uno valido: %v", err)
		}
	})
	t.Run("ids", func(t *testing.T) {
		ids := map[string]bool{"a": true}
		if err := validateParallelIDs(splitValidTask("b"), ids); err != nil {
			t.Fatalf("id nuevo: %v", err)
		}
		if err := validateParallelIDs(ParallelTask{ID: "a b"}, ids); err == nil || !strings.Contains(err.Error(), "invalid task id") {
			t.Fatalf("id con espacio debe fallar, got %v", err)
		}
		if err := validateParallelIDs(splitValidTask("a"), ids); err == nil || !strings.Contains(err.Error(), `duplicate task id "a"`) {
			t.Fatalf("duplicado debe fallar, got %v", err)
		}
	})
	t.Run("titulos", func(t *testing.T) {
		if err := validateParallelTitle(splitValidTask("a")); err != nil {
			t.Fatalf("titulo valido: %v", err)
		}
		bad := splitValidTask("a")
		bad.Title = "  "
		if err := validateParallelTitle(bad); err == nil || !strings.Contains(err.Error(), "needs a title") {
			t.Fatalf("titulo en blanco debe fallar, got %v", err)
		}
		bad.Title = "x\ny"
		if err := validateParallelTitle(bad); err == nil || !strings.Contains(err.Error(), "single line under 256") {
			t.Fatalf("titulo multilinea debe fallar, got %v", err)
		}
	})
	t.Run("files", func(t *testing.T) {
		if err := validateParallelFiles(splitValidTask("a")); err != nil {
			t.Fatalf("files validos: %v", err)
		}
		many := splitValidTask("a")
		for i := 0; i < 33; i++ {
			many.Files = append(many.Files, "f.go")
		}
		if err := validateParallelFiles(many); err == nil || !strings.Contains(err.Error(), "at most 32 files") {
			t.Fatalf("demasiados files deben fallar, got %v", err)
		}
		bad := splitValidTask("a")
		bad.Files = []string{"/abs/x.go"}
		if err := validateParallelFiles(bad); err == nil || !strings.Contains(err.Error(), "repo-relative, not absolute") {
			t.Fatalf("file absoluto debe fallar, got %v", err)
		}
	})
	t.Run("deps", func(t *testing.T) {
		if err := validateParallelDeps(splitValidTask("a")); err != nil {
			t.Fatalf("sin deps: %v", err)
		}
		self := splitValidTask("a")
		self.DependsOn = []string{"a"}
		if err := validateParallelDeps(self); err == nil || !strings.Contains(err.Error(), "must not depend on itself") {
			t.Fatalf("auto-dep debe fallar, got %v", err)
		}
		inv := splitValidTask("a")
		inv.DependsOn = []string{"a b"}
		if err := validateParallelDeps(inv); err == nil || !strings.Contains(err.Error(), "invalid dependency") {
			t.Fatalf("dep invalida debe fallar, got %v", err)
		}
		dup := splitValidTask("a")
		dup.DependsOn = []string{"b", "b"}
		if err := validateParallelDeps(dup); err == nil || !strings.Contains(err.Error(), "duplicates dependency") {
			t.Fatalf("dep duplicada debe fallar, got %v", err)
		}
	})
	t.Run("command", func(t *testing.T) {
		if err := validateParallelCommand(splitValidTask("a")); err != nil {
			t.Fatalf("sin command: %v", err)
		}
		long := splitValidTask("a")
		long.Command = strings.Repeat("c", 4001)
		if err := validateParallelCommand(long); err == nil || !strings.Contains(err.Error(), "exceeds 4000") {
			t.Fatalf("command largo debe fallar, got %v", err)
		}
		nul := splitValidTask("a")
		nul.Command = "run\x00x"
		if err := validateParallelCommand(nul); err == nil || !strings.Contains(err.Error(), "must not contain NUL") {
			t.Fatalf("command NUL debe fallar, got %v", err)
		}
	})
	t.Run("scopes", func(t *testing.T) {
		if err := validateParallelScopes(splitValidTask("a")); err != nil {
			t.Fatalf("sin scopes: %v", err)
		}
		bad := splitValidTask("a")
		bad.Scopes = []string{""}
		if err := validateParallelScopes(bad); err == nil || !strings.Contains(err.Error(), "invalid scope") {
			t.Fatalf("scope vacio debe fallar, got %v", err)
		}
	})
	t.Run("agent", func(t *testing.T) {
		if err := validateParallelAgent(splitValidTask("a")); err != nil {
			t.Fatalf("sin agent: %v", err)
		}
		bad := splitValidTask("a")
		bad.Agent = "a\nb"
		if err := validateParallelAgent(bad); err == nil || !strings.Contains(err.Error(), "single line under 64") {
			t.Fatalf("agent multilinea debe fallar, got %v", err)
		}
	})
	t.Run("estimates", func(t *testing.T) {
		if err := validateParallelEstimates(splitValidTask("a")); err != nil {
			t.Fatalf("sin estimates: %v", err)
		}
		bad := splitValidTask("a")
		bad.EstimateMS = -1
		if err := validateParallelEstimates(bad); err == nil || !strings.Contains(err.Error(), "estimate_ms") {
			t.Fatalf("estimate_ms negativo debe fallar, got %v", err)
		}
		bad = splitValidTask("a")
		bad.EstimateTokens = -1
		if err := validateParallelEstimates(bad); err == nil || !strings.Contains(err.Error(), "estimate_tokens") {
			t.Fatalf("estimate_tokens negativo debe fallar, got %v", err)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		a := splitValidTask("a")
		a.DependsOn = []string{"b"}
		ids := map[string]bool{"a": true}
		if err := validateParallelUnknownDeps([]ParallelTask{a}, ids); err == nil || !strings.Contains(err.Error(), "unknown task") {
			t.Fatalf("dep desconocida debe fallar, got %v", err)
		}
		b := splitValidTask("b")
		ok := []ParallelTask{{ID: "a", Title: "A", DependsOn: []string{"b"}}, b}
		ids = map[string]bool{"a": true, "b": true}
		if err := validateParallelUnknownDeps(ok, ids); err != nil {
			t.Fatalf("deps conocidas: %v", err)
		}
	})
	t.Run("tarea valida", func(t *testing.T) {
		ids := map[string]bool{}
		if err := validateParallelTask(splitValidTask("a"), ids); err != nil {
			t.Fatalf("tarea minima valida: %v", err)
		}
	})
}
