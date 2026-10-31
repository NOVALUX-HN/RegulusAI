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

// T2 (S1-S2): the visual serves three views with auto-refresh, no envelope.
func TestDashboardUIServesThreeViews(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("GET / Content-Type = %q, want html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"Harness", "Guardado", "Aprendizaje", "/v1/metrics", "/v1/almanac/summary"} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI missing %q", want)
		}
	}
}
