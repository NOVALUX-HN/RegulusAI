// Dashboard RED (T1, S1): /v1/metrics returns Summarize numbers,
// /v1/almanac/summary returns counts by type. Fails before dashboard.go exists.
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardMetricsEndpointExists(t *testing.T) {
	srv := newTestServer()
	code, body := get(t, srv, "/v1/metrics")
	if code != 200 {
		t.Fatalf("GET /v1/metrics = %d, want 200 (%s)", code, body)
	}
	env := decodeEnvelope(t, body)
	if env["ok"] != true {
		t.Fatalf("GET /v1/metrics ok = %v, want true (%s)", env["ok"], body)
	}
}

func TestDashboardAlmanacSummaryExists(t *testing.T) {
	srv := newTestServer()
	code, body := get(t, srv, "/v1/almanac/summary")
	if code != 200 {
		t.Fatalf("GET /v1/almanac/summary = %d, want 200 (%s)", code, body)
	}
}

// D2: the visual is a professional panel: header with the project
// state, cards per area (Guardado/Evidencia/Memoria/Aprendizaje/Coste)
// with plain-language explanations, paginated tables over the read-only
// /v1/* collections and dependency-free mini charts. All dynamic text
// goes through textContent, never innerHTML with data.
func TestDashboardUIServesProfessionalPanels(t *testing.T) {
	srv := newTestServer()
	for _, path := range []string{"/", "/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("GET %s Content-Type = %q, want html", path, ct)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"Guardado", "Evidencia", "Memoria", "Aprendizaje", "Coste",
			"Resumen por áreas", "De un vistazo",
			"/v1/receipts", "/v1/observations", "/v1/lessons", "/v1/traces", "/v1/budget",
			"limit", "offset", "textContent",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET %s dashboard UI missing %q", path, want)
			}
		}
		for _, banned := range []string{"p-harness", "innerHTML", "<script src"} {
			if strings.Contains(body, banned) {
				t.Fatalf("GET %s dashboard UI must not contain %q", path, banned)
			}
		}
	}
}

// D2: every area reads its own endpoint (paginated with ?limit/?offset).
func TestDashboardUIAreasQueryTheirEndpoints(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		"fetchData('/v1/almanac/summary')",
		"fetchData('/v1/receipts?limit=",
		"fetchData('/v1/observations?limit=",
		"fetchData('/v1/lessons?status=",
		"fetchData('/v1/traces?limit=",
		"fetchData('/v1/traces?session=",
		"fetchData('/v1/budget?limit=",
		"fetchData('/v1/status')",
		"fetchData('/v1/metrics')",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI never queries %q", want)
		}
	}
}
