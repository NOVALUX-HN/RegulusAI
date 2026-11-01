// R2: viewport + base fluida + táctil. El HTML servido en / y
// /dashboard declara el viewport móvil, dimensiona los controles
// táctiles (≥44px, sin zoom iOS) y apila #cards-grid a 1 columna en
// móvil. Sin navegador real: aserciones sobre el HTML/CSS servido.
package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func servedDashboardBody(t *testing.T, path string) string {
	t.Helper()
	srv := newTestServer()
	req := httptest.NewRequest("GET", path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET %s = %d, want 200", path, rec.Code)
	}
	return rec.Body.String()
}

// R2: el <head> servido declara el viewport móvil.
func TestDashboardR2ViewportMeta(t *testing.T) {
	for _, path := range []string{"/", "/dashboard"} {
		body := servedDashboardBody(t, path)
		if !strings.Contains(body, `<meta name="viewport" content="width=device-width, initial-scale=1">`) {
			t.Fatalf("GET %s dashboard UI missing viewport meta", path)
		}
	}
}

// R2: los controles (select#project, paginadores, búsquedas y filtros)
// tienen tamaño táctil ≥44px y a 16px (sin auto-zoom iOS al enfocar).
func TestDashboardR2TouchTargets(t *testing.T) {
	body := servedDashboardBody(t, "/")
	if !strings.Contains(body, "min-height:44px") {
		t.Fatalf("dashboard UI CSS missing tactile min-height:44px rule")
	}
	for _, want := range []string{`<select id="project"`, "#project", "prev-receipts", "next-budget", `type="search"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI missing tactile control %q", want)
		}
	}
}

// R2: #cards-grid cae a 1 columna dentro de la media query móvil y
// #graphs sigue apilando (D1 intacto).
func TestDashboardR2FluidBaseStacksCards(t *testing.T) {
	body := servedDashboardBody(t, "/")
	marker := "@media (max-width:640px)"
	mi := strings.Index(body, marker)
	if mi < 0 {
		t.Fatalf("dashboard UI CSS missing mobile media query %q", marker)
	}
	rest := body[mi:]
	if ni := strings.Index(rest, "@media (pointer:coarse)"); ni >= 0 {
		rest = rest[:ni]
	}
	if !strings.Contains(rest, "#cards-grid") || !strings.Contains(rest, "grid-template-columns:1fr") {
		t.Fatalf("mobile media query must stack #cards-grid to 1 column")
	}
	if !strings.Contains(rest, ".bar-row{grid-template-columns:1fr") {
		t.Fatalf("mobile media query broke D1 #graphs stacking")
	}
}

// R2: el foco visible (D1) sigue declarado y el :hover nunca es el
// único camino de una acción (solo mejora visual: sin display ni
// visibility dentro de reglas :hover).
func TestDashboardR2FocusVisibleAndNoHoverOnlyActions(t *testing.T) {
	body := servedDashboardBody(t, "/")
	if !strings.Contains(body, ":focus-visible") {
		t.Fatalf("dashboard UI CSS missing :focus-visible rule")
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, ":hover") &&
			(strings.Contains(line, "display:") || strings.Contains(line, "visibility:")) {
			t.Fatalf("dashboard UI must not gate actions on :hover: %q", strings.TrimSpace(line))
		}
	}
}
