// NOV-67: the dashboard shell (split sources, assembled at init)
// keeps its contract: route, headers, CSP and key markers.
package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDashboardShell(t *testing.T) {
	srv := newTestServer()
	for _, path := range []string{"/", "/dashboard"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("GET %s content-type = %q", path, ct)
		}
		if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Fatalf("GET %s CSP = %q, want restrictive policy", path, csp)
		}
		body := rec.Body.String()
		for _, want := range []string{"<!doctype html>", "kpi-ejecuciones-val", "<style>", "</style>", "<script>", "visionKpiTitle"} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET %s body missing %q", path, want)
			}
		}
	}
}
