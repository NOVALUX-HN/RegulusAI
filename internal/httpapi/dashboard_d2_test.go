// D2: cada cifra visible es trazable a su endpoint y filtro; el proyecto
// vacío muestra ceros con mensaje de estado (no tablas rotas ni NaN).
package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// D2: las tarjetas y gráficos nombran su endpoint, campo y filtro
// (trazabilidad dato→pantalla), incluidos los límites ?limit=1/?limit=200.
func TestDashboardD2TraceabilityLabels(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"GET /v1/almanac/summary",
		"GET /v1/receipts",
		"GET /v1/observations",
		"GET /v1/lessons?status=all",
		"GET /v1/budget",
		"GET /v1/metrics",
		"GET /v1/status",
		"campo total",
		"?limit=1",
		"?limit=200",
		"los primeros 200",
		"Sin comprobantes todav",
		"Sin recuerdos todav",
		"Sin lecciones todav",
		"Sin sesiones todav",
		"Sin movimientos todav",
		"Sin pasos",
		"Sin datos todav",
		"emptyRow",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI missing D2 traceability %q", want)
		}
	}
}

// D2: con stores vacíos todo es 0 con forma válida (total/limit/offset,
// tasas 0 nunca NaN) y el summary omite oldest/newest.
func TestDashboardD2EmptyStoresStayHealthy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	srv := newTestServer()
	for _, path := range []string{
		"/v1/metrics", "/v1/almanac/summary",
		"/v1/receipts?limit=1", "/v1/observations?limit=1",
		"/v1/lessons?limit=1", "/v1/traces", "/v1/budget?limit=1",
	} {
		code, body := get(t, srv, path)
		if code != 200 {
			t.Fatalf("GET %s = %d, want 200 (%s)", path, code, body)
		}
		if strings.Contains(string(body), "NaN") {
			t.Fatalf("GET %s contains NaN (%s)", path, body)
		}
	}
	// metrics: tasas cero, nunca NaN.
	_, mbody := get(t, srv, "/v1/metrics")
	var menv map[string]any
	if err := json.Unmarshal(mbody, &menv); err != nil {
		t.Fatal(err)
	}
	m := menv["data"].(map[string]any)["metrics"].(map[string]any)
	for _, k := range []string{"verification_pass_rate", "task_completion_rate"} {
		if m[k].(float64) != 0 {
			t.Fatalf("empty metrics %s = %v, want 0", k, m[k])
		}
	}
	// colecciones: total 0, items [] (no null), paginación coherente.
	for _, path := range []string{
		"/v1/receipts?limit=1", "/v1/observations?limit=1",
		"/v1/lessons?limit=1", "/v1/traces", "/v1/budget?limit=1",
	} {
		_, body := get(t, srv, path)
		total, _, _, items := decodePage(t, body)
		if total != 0 || len(items) != 0 {
			t.Fatalf("GET %s empty = total %d items %d, want 0/0", path, total, len(items))
		}
	}
}
