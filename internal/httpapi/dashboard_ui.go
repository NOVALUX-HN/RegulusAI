package httpapi

// V4 vision (charts/dashboard-vision.md): réplica pixel a pixel del mockup
// docs/assets/dashboard-vision.png — sidebar con 7 entradas con iconos
// (Resumen/Evidencia/Compuertas/Agentes/Métricas/Almanaque/Configuración),
// tarjeta de proyecto (nombre, rama de /v1/git, última ejecución), topbar
// con pill verde del último ciclo (#N de /v1/executions), 5 KPIs exactos
// del mockup con formato 1,248/98.8% y copy masticado, 2 gráficos SVG con
// ejes y banda, 2 tablas #/Rama y #/Compuerta con datos del backend.
// Agentes = sección real con la lista fija de guardians+crew de
// docs/agents.md. Configuración = ancla a la lista fija de las 12 puertas
// de regulus.json con estado vivo del último ciclo. Sin datos inventados:
// históricos sin manifest muestran rama "—"; si solo hay 1 checkout se
// avisa; deltas 7d calculados en cliente con cap 1000 ("parcial" si el
// total lo supera).
import (
	"embed"
	"strings"
)

// Dashboard visual sources (NOV-67): the single-blob const is split
// into parts under dashstatic/ so no file exceeds the size budget.
// Assembly order is fixed and byte-identical to the old blob: no
// route, header, CSP or pixel changes. TestDashboardShell pins the
// shell (status, content-type, CSP, markers).
//
//go:embed dashstatic/head.html dashstatic/style.css dashstatic/body.html dashstatic/app.js dashstatic/tail.html
var dashStatic embed.FS

// dashboardHTML is the assembled visual, byte-identical to the
// pre-split blob: head + css + body + js + tail.
var dashboardHTML = mustAssembleDashboard()

func mustAssembleDashboard() string {
	parts := []string{"head.html", "style.css", "body.html", "app.js", "tail.html"}
	var b strings.Builder
	for _, p := range parts {
		raw, err := dashStatic.ReadFile("dashstatic/" + p)
		if err != nil {
			panic("dashboard static part missing: " + p)
		}
		b.Write(raw)
	}
	return b.String()
}
