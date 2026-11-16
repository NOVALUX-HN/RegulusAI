package journey

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// NOV-72: ningún workflow puede usar una etiqueta de runner que GitHub
// no reconozca (ubuntu-24-latest dejaba los jobs en espera eterna).
// Este test fija la lista permitida (ubuntu-24.04 pineado + alias
// canónicos) y falla antes de fusionar si un runs-on se sale de ella.
// Anclado al repo, no al cwd.
func TestWorkflowRunsOnAllowlist(t *testing.T) {
	allowed := map[string]bool{
		"ubuntu-24.04":  true,
		"ubuntu-latest": true,
		"windows-2022":  true,
		"macos-14":      true,
	}
	files, err := filepath.Glob(filepath.Join(journeyRepoRoot(t), ".github", "workflows", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("workflows must exist: %v", files)
	}
	var bad []string
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range workflowRunsOnValues(string(raw)) {
			if !allowed[v] {
				bad = append(bad, filepath.Base(path)+": "+v)
			}
		}
	}
	if len(bad) != 0 {
		t.Fatalf("runs-on fuera de la lista permitida (ubuntu-24.04, ubuntu-latest, windows-2022, macos-14): %v", bad)
	}
}

// workflowRunsOnValues extrae los valores de runs-on de un workflow,
// expandiendo ${{ matrix.os }} con la lista os de su matriz (dos
// pasadas porque la matriz puede declararse después del runs-on).
func workflowRunsOnValues(doc string) []string {
	var matrix []string
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "os: [") {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "os: ["), "]")
		for _, v := range strings.Split(inner, ",") {
			if v = strings.TrimSpace(v); v != "" {
				matrix = append(matrix, v)
			}
		}
	}
	var out []string
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "runs-on:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(trimmed, "runs-on:"))
		if strings.HasPrefix(v, "${{") {
			out = append(out, matrix...)
			continue
		}
		out = append(out, v)
	}
	return out
}
