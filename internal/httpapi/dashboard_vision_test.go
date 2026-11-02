// V4 visión (charts/dashboard-vision.md): réplica pixel a pixel del mockup
// docs/assets/dashboard-vision.png con datos reales — sidebar con 7
// entradas con iconos, tarjeta de proyecto (nombre/rama de /v1/git,
// última ejecución), topbar con pill verde del último ciclo (#N de
// /v1/executions), 5 KPIs exactos del mockup con formato 1,248/98.8% y
// copy masticado, 2 gráficos SVG con ejes y banda, 2 tablas #/Rama y
// #/Compuerta con datos del backend. Agentes = lista fija de
// docs/agents.md; Configuración = lista fija de regulus.json con estado
// vivo del último ciclo. Sin datos inventados: históricos sin manifest
// muestran rama "—".
package httpapi

import (
	"strings"
	"testing"
)

// V4: la navegación lateral tiene las 7 entradas del mockup con iconos,
// cada una con ancla a una sección real que existe.
func TestVisionSidebarAnchors(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`<nav class="sidebar"`,
		`aria-label="Secciones del panel"`,
		`href="#sec-resumen">`,
		`>Resumen`,
		`href="#sec-evidencia">`,
		`>Evidencia`,
		`href="#sec-compuertas">`,
		`>Compuertas`,
		`href="#sec-agentes">`,
		`>Agentes`,
		`href="#sec-metricas">`,
		`>Métricas`,
		`href="#sec-memoria">`,
		`>Almanaque`,
		`href="#sec-config">`,
		`>Configuración`,
		`id="sec-agentes"`,
		`id="sec-config"`,
		`<svg`,
		`aria-hidden="true"`,
		`id="proj-name"`,
		`id="proj-branch"`,
		`id="proj-activity"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision sidebar missing %q", want)
		}
	}
}

// V4: topbar con el pill del último ciclo numerado (#N de /v1/executions)
// con fecha, en verde cuando pasa.
func TestVisionTopbarPill(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`id="last-cycle"`,
		`id="last-cycle-text"`,
		`Último ciclo:`,
		`role="status"`,
		`is-pass`,
		`Último ciclo: ' + (okCycle ? 'PASS' : 'FAIL') + ' #'`,
		`/v1/executions?limit=`,
		`offset=' + (total - 1)`,
		`/v1/git`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision topbar missing %q", want)
		}
	}
}

// V4: los 5 KPIs exactos del mockup con ids estables, formato con miles
// (en-US: 1,248) y % con 1 decimal, deltas 7d con cap 1000 y aviso
// parcial, cada uno con su "qué significa" y su fuente real.
func TestVisionKPIs(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`id="kpi-evidencias-val"`,
		`id="kpi-evidencias-delta"`,
		`id="kpi-ejecuciones-val"`,
		`id="kpi-ejecuciones-delta"`,
		`id="kpi-pass-val"`,
		`id="kpi-pass-sub"`,
		`id="kpi-lecciones-val"`,
		`id="kpi-lecciones-delta"`,
		`id="kpi-ramas-val"`,
		`id="kpi-ramas-delta"`,
		`id="kpi-ramas-actual"`,
		`Qué significa:`,
		`verification_pass_rate`,
		`lessons_admitted`,
		`GET /v1/executions`,
		`GET /v1/git`,
		`toLocaleString('en-US')`,
		`toFixed(1)`,
		`VISION_CAP`,
		`parcial`,
		`id="vision-partial"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision KPIs missing %q", want)
		}
	}
}

// V4: 2 gráficos SVG con ejes dibujados (grid + etiquetas + días), la
// línea con banda y dominio 90–100% cuando los datos están ahí arriba.
func TestVisionChartsHaveAxes(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`id="chart-bars"`,
		`id="chart-pass"`,
		`role="img"`,
		`grid-line`,
		`axis-line`,
		`createElementNS`,
		`viewBox`,
		`opacity`,
		`Qué significa:`,
		`chart-bars-title`,
		`chart-pass-title`,
		`toLowerCase() === 'title'`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision charts missing %q", want)
		}
	}
}

// V4: las tablas son #/Rama/Resultado/Compuertas/Duración/Fecha y
// #/Compuerta/Estado/Tiempo/Evidencia con filas numeradas y el enlace a
// la evidencia completa. Rama y # salen de /v1/executions; la evidencia
// de cada compuerta es el path del receipt.
func TestVisionTables(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`id="tb-runs"`,
		`id="tb-lastgates"`,
		`id="last-task"`,
		`<th scope="col">#</th>`,
		`<th scope="col">Rama</th>`,
		`<th scope="col">Resultado</th>`,
		`<th scope="col">Compuertas</th>`,
		`<th scope="col">Duración</th>`,
		`<th scope="col">Fecha</th>`,
		`<th scope="col">Compuerta</th>`,
		`<th scope="col">Estado</th>`,
		`<th scope="col">Tiempo</th>`,
		`<th scope="col">Evidencia</th>`,
		`href="#sec-evidencia">Ver todas`,
		`visionGroupCycles`,
		`run_id`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision tables missing %q", want)
		}
	}
	for _, banned := range []string{"<script src"} {
		if strings.Contains(body, banned) {
			t.Fatalf("vision must not load external scripts %q", banned)
		}
	}
}

// V4: la sección Agentes lista a los guardianes y a la tripulación con
// sus nombres reales de docs/agents.md (información del repo, no datos
// inventados del panel).
func TestVisionAgentsSection(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`docs/agents.md`,
		`@regulus-gate-verifier`,
		`@regulus-spec-keeper`,
		`@regulus-mutation-auditor`,
		`@regulus-crap-analyst`,
		`@regulus-evidence-auditor`,
		`@regulus-reviewer`,
		`@regulus-scout`,
		`@regulus-worker`,
		`@regulus-verifier`,
		`@regulus-proposer`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision agentes missing %q", want)
		}
	}
}

// V4: la sección Configuración ancla las 12 puertas del gauntlet con su
// nombre real de regulus.json y declara que el estado es vivo (último
// ciclo) mientras la lista es fija.
func TestVisionConfigSection(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`regulus.json`,
		`id="cfg-typecheck"`,
		`id="cfg-lint"`,
		`id="cfg-build"`,
		`id="cfg-unit-tests"`,
		`id="cfg-coverage-floor"`,
		`id="cfg-mutation"`,
		`id="cfg-crap"`,
		`id="cfg-vulncheck"`,
		`id="cfg-spec"`,
		`id="cfg-ts-typecheck"`,
		`id="cfg-ts-unit-tests"`,
		`id="cfg-gofmt"`,
		`— en este ciclo`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision config missing %q", want)
		}
	}
}

// V4: la sidebar colapsa en móvil (apilada, sin scroll horizontal extra).
func TestVisionSidebarCollapsesOnMobile(t *testing.T) {
	body := servedDashboardBody(t, "/")
	for _, want := range []string{
		`.app{flex-direction:column}`,
		`.sidebar{position:static`,
		`.kpi-grid{grid-template-columns:1fr}`,
		`.panels{grid-template-columns:1fr}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("vision mobile CSS missing %q", want)
		}
	}
}
