// D3: la cabecera nombra el proyecto real (ProjectName, no genérico) y
// dos checkouts nunca mezclan datos (cada uno lee solo su .regulus/).
package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/almanac"
)

// D3: GET /v1/status expone el proyecto que sirve (ProjectName("."):
// git remote o dirname) en el campo `project`, sin romper el contrato
// (stdout/stderr/exit siguen verbatim del core path).
func TestStatusCarriesProjectName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	code, body := get(t, srv, "/v1/status")
	if code != 200 {
		t.Fatalf("GET /v1/status = %d, want 200 (%s)", code, body)
	}
	data := almanacData(t, body)
	got, ok := data["project"].(string)
	if !ok || strings.TrimSpace(got) == "" {
		t.Fatalf("GET /v1/status project = %v, want non-empty ProjectName (%s)", data["project"], body)
	}
	if want := almanac.ProjectName("."); got != want {
		t.Fatalf("GET /v1/status project = %q, want ProjectName %q", got, want)
	}
	// Contrato intacto: stdout/stderr/exit siguen presentes.
	for _, k := range []string{"stdout", "stderr", "exit"} {
		if _, ok := data[k]; !ok {
			t.Fatalf("GET /v1/status must keep field %q (%s)", k, body)
		}
	}
}

// D3: la cabecera pinta el nombre real del proyecto (h1 + <title>
// dinámicos desde GET /v1/status campo project, todo por textContent).
func TestDashboardHeaderShowsRealProject(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="project-name"`,
		"st.project",
		"Qué hay guardado en ",
		"document.title",
		"GET /v1/status campos exit/project",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard header must carry project identity %q", want)
		}
	}
	if strings.Contains(body, "innerHTML") {
		t.Fatalf("project header must use textContent, never innerHTML")
	}
}

// D3: aislamiento por proyecto — dos roots distintos no mezclan:
// ProjectName difiere por dirname y cada store vive bajo su root.
func TestProjectIsolationTwoRootsNeverMix(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	if almanac.ProjectName(rootA) == almanac.ProjectName(rootB) {
		t.Fatalf("two temp roots must differ by dirname, got %q both",
			almanac.ProjectName(rootA))
	}
	if almanac.File(rootA) == almanac.File(rootB) {
		t.Fatalf("File must scope per root, got %q both", almanac.File(rootA))
	}
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if _, err := almanac.Save(rootA, &almanac.Observation{
		Type: almanac.TypeDecision, Title: "solo-A", Content: "a",
	}, now); err != nil {
		t.Fatal(err)
	}
	a, err := almanac.All(rootA)
	if err != nil || len(a) != 1 || a[0].Title != "solo-A" {
		t.Fatalf("rootA = %+v (%v)", a, err)
	}
	b, err := almanac.All(rootB)
	if err != nil || len(b) != 0 {
		t.Fatalf("rootB must stay empty, got %+v (%v)", b, err)
	}
}
