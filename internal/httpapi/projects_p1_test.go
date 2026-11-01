// P1: GET /v1/projects + <select id="project"> que recarga todo con
// ?project=<name>.
//
// Given `serve` arranca con `--projects <dirA>,<dirB>` (además del actual `.`)
// y expone `GET /v1/projects` con `[{name, root}]` donde `name` es
// `almanac.ProjectName(dir)`
// When se abre `/` y se elige otro proyecto en el selector
// Then todas las tarjetas/gráficos/tablas recargan con `?project=<name>` y
// muestran los datos de ese proyecto (totales del otro `.regulus/`), y el
// nombre en cabecera cambia al proyecto elegido
package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/mcp"
)

// La ruta lista los N allowlistados en orden (primero = default) con el
// nombre ProjectName y el root declarado; sin flag, solo el actual.
func TestP1ProjectsListsAllowlisted(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	beta := p2Project(t, base, "proj-beta")
	srv, nameA, nameB := p2Server(t, alpha, beta)

	code, body := get(t, srv, "/v1/projects")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/projects = %d, want 200 (%s)", code, body)
	}
	var env struct {
		OK   bool      `json:"ok"`
		Data []Project `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || !env.OK {
		t.Fatalf("GET /v1/projects bad envelope (%s): %v", body, err)
	}
	if len(env.Data) != 2 {
		t.Fatalf("GET /v1/projects = %d items, want 2 (%s)", len(env.Data), body)
	}
	if env.Data[0].Name != nameA || env.Data[0].Root != alpha {
		t.Fatalf("projects[0] = %+v, want {name %q root %q}", env.Data[0], nameA, alpha)
	}
	if env.Data[1].Name != nameB || env.Data[1].Root != beta {
		t.Fatalf("projects[1] = %+v, want {name %q root %q}", env.Data[1], nameB, beta)
	}
	if env.Data[0].Name != almanac.ProjectName(alpha) {
		t.Fatalf("name = %q, want almanac.ProjectName %q", env.Data[0].Name, almanac.ProjectName(alpha))
	}

	// Sin flag: solo el proyecto actual (P3 intacto).
	solo := newTestServer()
	code, body = get(t, solo, "/v1/projects")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/projects (single) = %d, want 200 (%s)", code, body)
	}
	var single struct {
		OK   bool      `json:"ok"`
		Data []Project `json:"data"`
	}
	if err := json.Unmarshal(body, &single); err != nil || !single.OK {
		t.Fatalf("single bad envelope (%s): %v", body, err)
	}
	if len(single.Data) != 1 || single.Data[0].Root != "." {
		t.Fatalf("single projects = %+v, want exactly [{root .}]", single.Data)
	}
}

// Solo GET: cualquier otro verbo es 405 con el envelope compartido.
func TestP1ProjectsMethodNotAllowed(t *testing.T) {
	srv := newTestServer()
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		code, body := doMethod(t, srv, m, "/v1/projects")
		if code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /v1/projects = %d, want 405 (%s)", m, code, body)
		}
		env := decodeEnvelope(t, body)
		if env["ok"] != false {
			t.Fatalf("%s /v1/projects ok = true, want false (%s)", m, body)
		}
		errObj, ok := env["error"].(map[string]any)
		if !ok || int(errObj["code"].(float64)) != mcp.CodeInvalidRequest {
			t.Fatalf("%s /v1/projects error = %v, want code %d (%s)", m, env["error"], mcp.CodeInvalidRequest, body)
		}
	}
}

// El dashboard trae el selector accesible (label + teclado nativo +
// :focus-visible global): carga /v1/projects, fija state.project y
// recarga todo con ?project=<name>; con un solo proyecto no añade
// ?project= (P3 intacto). Sin innerHTML: opciones por textContent.
func TestP1DashboardHasProjectSelector(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<label for="project">`,
		`id="project"`,
		`id="project-row"`,
		`aria-label="Proyecto"`,
		`/v1/projects`,
		`state.project`,
		`project=' + encodeURIComponent(state.project)`,
		`addEventListener('change'`,
		`document.createElement('option')`,
		`loadProjects`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard must carry project selector piece %q", want)
		}
	}
	if strings.Contains(body, "innerHTML") {
		t.Fatalf("project selector must use textContent, never innerHTML")
	}
	// Con un solo proyecto el selector nace oculto y fetchData no
	// añade ?project= (mismo comportamiento que hoy).
	for _, want := range []string{
		`id="project-row" hidden`,
		`if(state.project && path !== '/v1/projects')`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("single-project dashboard must keep %q", want)
		}
	}
	// Los endpoints existentes siguen colgando de fetchData (D2
	// intacto): el ?project= se añade en el único punto de salida.
	for _, want := range []string{
		`fetchData('/v1/almanac/summary')`,
		`fetchData('/v1/receipts?limit='`,
		`fetchData('/v1/observations?limit=1')`,
		`fetchData('/v1/status')`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI lost data source %q", want)
		}
	}
}

// El cambio de proyecto refetchea con el query a nivel handler: los
// nombres que devuelve /v1/projects sirven tal cual como ?project= y
// cada valor muestra solo los totales de su propio .regulus/ (la
// cabecera /v1/status nombra al proyecto elegido).
func TestP1ProjectSwitchServesThatProject(t *testing.T) {
	base := t.TempDir()
	alpha := p2Project(t, base, "proj-alpha")
	beta := p2Project(t, base, "proj-beta")
	p2Seed(t, alpha, "ALPHA")
	srv, nameA, nameB := p2Server(t, alpha, beta)

	// Round-trip: los nombres listados son los que acepta ?project=.
	code, body := get(t, srv, "/v1/projects")
	if code != 200 {
		t.Fatalf("GET /v1/projects = %d, want 200 (%s)", code, body)
	}
	var env struct {
		OK   bool      `json:"ok"`
		Data []Project `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || !env.OK || len(env.Data) != 2 {
		t.Fatalf("bad projects envelope (%s): %v", body, err)
	}
	listed := map[string]bool{env.Data[0].Name: true, env.Data[1].Name: true}
	if !listed[nameA] || !listed[nameB] {
		t.Fatalf("listed names must round-trip as ?project= values, got %+v want %q %q", env.Data, nameA, nameB)
	}

	if total := p2Total(t, srv, "/v1/receipts?project="+nameA); total != 2 {
		t.Fatalf("alpha receipts = %d, want 2", total)
	}
	if total := p2Total(t, srv, "/v1/receipts?project="+nameB); total != 0 {
		t.Fatalf("beta receipts = %d, want 0 (empty, never alpha's)", total)
	}
	if _, body := get(t, srv, "/v1/receipts?project="+nameA); !strings.Contains(string(body), "ALPHA") {
		t.Fatalf("alpha receipts must carry the alpha marker (%s)", body)
	}
	if _, body := get(t, srv, "/v1/receipts?project="+nameB); strings.Contains(string(body), "ALPHA") {
		t.Fatalf("beta receipts must never leak alpha data (%s)", body)
	}
	if got := p2StatusProject(t, srv, "?project="+nameB); got != almanac.ProjectName(beta) {
		t.Fatalf("header project after switch = %q, want %q", got, almanac.ProjectName(beta))
	}
}
