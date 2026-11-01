// R1: tablas apiladas/contenidas a 360px. El HTML servido en / y
// /dashboard mantiene el viewport móvil (verificado por R2, sin
// duplicar), envuelve cada una de las 6 tablas en un contenedor con
// scroll propio y controla el overflow-x de página. Sin navegador
// real: aserciones sobre el HTML/CSS servido.
package httpapi

import (
	"strings"
	"testing"
)

// R1: el viewport móvil existe una sola vez (R2 lo aportó; R1 no lo
// duplica).
func TestDashboardR1ViewportSingle(t *testing.T) {
	want := `<meta name="viewport" content="width=device-width, initial-scale=1">`
	for _, path := range []string{"/", "/dashboard"} {
		body := servedDashboardBody(t, path)
		if n := strings.Count(body, want); n != 1 {
			t.Fatalf("GET %s viewport meta count = %d, want exactly 1", path, n)
		}
	}
}

// R1: las 6 tablas existen y cada una vive en un contenedor con
// scroll contenido (.tbl-wrap con overflow-x:auto + max-width:100%).
func TestDashboardR1TablesContained(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, id := range []string{
		"tb-receipts", "tb-obs", "tb-lessons",
		"tb-traces", "tb-entries", "tb-budget",
	} {
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Fatalf("dashboard UI missing table body %q", id)
		}
		if !strings.Contains(body, `<div class="tbl-wrap"><table><thead><tr><th>`) {
			t.Fatalf("dashboard UI table %q not wrapped in .tbl-wrap", id)
		}
	}
	if n := strings.Count(body, `<div class="tbl-wrap">`); n != 6 {
		t.Fatalf("dashboard UI .tbl-wrap count = %d, want 6 (una por tabla)", n)
	}
	for _, want := range []string{
		".tbl-wrap",
		"overflow-x:auto",
		"max-width:100%",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI CSS missing containment rule %q", want)
		}
	}
}

// R1: cero scroll horizontal de página — red de seguridad en la raíz
// (overflow-x:hidden + max-width:100vw) sin esconder contenido
// consultable (el scroll vive en .tbl-wrap, nunca en la página).
func TestDashboardR1PageOverflowControlled(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		"overflow-x:hidden",
		"max-width:100vw",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard UI CSS missing page overflow guard %q", want)
		}
	}
}

// R1: texto largo (hashes, títulos, mensajes) se rompe sin empujar el
// ancho de la tabla.
func TestDashboardR1LongTextWraps(t *testing.T) {
	body := servedDashboardBody(t, "/")
	if !strings.Contains(body, "overflow-wrap:anywhere") {
		t.Fatalf("dashboard UI CSS missing overflow-wrap:anywhere for long text")
	}
}
