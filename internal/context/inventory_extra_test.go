// Cobertura directa de Inventory (CRAP<30 sin cambiar
// comportamiento): un único root mínimo en t.TempDir() que ejerce
// defaults (cfg cero), truncado (MaxFileBytes pequeño), issues
// (fichero JSON explícito) y memoria (Almanac). Todo en proceso,
// portable Linux+Windows: sin red, shell ni git.
package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

// extraWrite guarda un fichero bajo root, creando los padres.
func extraWrite(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExtraInventoryDirectMinimal(t *testing.T) {
	root := t.TempDir()
	extraWrite(t, root, "hello.go", "package p\n\nfunc Hello() {}\n")
	bigBody := "package big\n\n// " + strings.Repeat("x", 200) + "\n"
	extraWrite(t, root, "big.go", bigBody)
	issuesPath := filepath.Join(root, "issues.json")
	if err := os.WriteFile(issuesPath, []byte(`[{"id":"i1","title":"First","body":"details"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	saved, err := almanac.Save(root, &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: "note", Content: "plain body",
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	// Defaults: cfg cero aplica los topes por defecto, la versión es
	// "worktree" y el cuerpo cabe sin truncar (8KB por defecto).
	defCorpus, _, err := Inventory(root, InventoryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defByID := map[string]Fragment{}
	for _, f := range defCorpus.Fragments {
		defByID[f.ID] = f
	}
	hello, ok := defByID["code/hello.go"]
	if !ok {
		t.Fatalf("defaults: falta code/hello.go (%d fragmentos)", len(defCorpus.Fragments))
	}
	if hello.Version != "worktree" || hello.Tokens <= 0 || hello.Trust != TrustTrusted {
		t.Fatalf("defaults: fragmento hello deriva: %+v", hello)
	}
	if big, ok := defByID["code/big.go"]; !ok || big.Content != bigBody {
		t.Fatalf("defaults: big.go debe caber sin truncar: %+v", big)
	}

	// Truncado: un tope pequeño acota el cuerpo y avisa.
	_, truncWarn, err := Inventory(root, InventoryConfig{MaxFileBytes: 16, IncludeMemory: false})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(truncWarn, "\n"), "truncado") {
		t.Fatalf("truncado: debe avisar, got %v", truncWarn)
	}
	truncCorpus, _, err := Inventory(root, InventoryConfig{MaxFileBytes: 16, IncludeMemory: false})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range truncCorpus.Fragments {
		if f.ID == "code/big.go" && int64(len(f.Content)) > 16 {
			t.Fatalf("truncado: cuerpo = %d bytes, tope 16", len(f.Content))
		}
	}

	// Issues: el fichero explícito pliega una entrada por fragmento.
	issCorpus, _, err := Inventory(root, InventoryConfig{IssuesFile: issuesPath, IncludeMemory: false})
	if err != nil {
		t.Fatal(err)
	}
	foundIssue := false
	for _, f := range issCorpus.Fragments {
		if f.ID == "issue/i1" && f.Title == "First" {
			foundIssue = true
		}
	}
	if !foundIssue {
		t.Fatalf("issues: falta issue/i1 (%d fragmentos)", len(issCorpus.Fragments))
	}

	// Memoria: el Almanac pliega la nota con scope y confianza.
	memCorpus, _, err := Inventory(root, InventoryConfig{IncludeMemory: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	mem, ok := func() (Fragment, bool) {
		for _, f := range memCorpus.Fragments {
			if f.ID == "memory/"+saved.ID {
				return f, true
			}
		}
		return Fragment{}, false
	}()
	if !ok {
		t.Fatalf("memoria: falta memory/%s (%d fragmentos)", saved.ID, len(memCorpus.Fragments))
	}
	if mem.RequiredScope != RequiredScopeFor(KindMemory) || mem.Trust != TrustTrusted {
		t.Fatalf("memoria: scope/confianza derivan: %+v", mem)
	}
}
