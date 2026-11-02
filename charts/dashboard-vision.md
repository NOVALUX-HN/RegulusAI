# dashboard-vision — réplica del mockup Obsidian & Gold

User authorization: "el dashboard de la imagen esta espectacular. si es posible hacer nuestro dashboard de esa forma seria genial asi que analizalo y repliquemos ese diseño para el nuestro" 2026-11-02. Referencia: `docs/assets/dashboard-vision.png` (mockup generado, NO captura real). Read down to `## Log` before working.

Class: COMPLEX

## Contract

- Objective: el dashboard replica el mockup — sidebar de navegación, topbar con pill de estado del último ciclo, 5 KPI cards con deltas, 2 gráficos (barras 7 días + línea tasa de éxito), 2 tablas (últimas ejecuciones + compuertas del último ciclo), tarjeta de proyecto — con datos reales del backend, sin inventar.
- Scope: `internal/httpapi/dashboard_ui.go` (HTML/CSS/JS), `internal/httpapi/dashboard.go` + `collections.go` solo si falta un endpoint para datos que el mockup exige y existen en stores (nunca datos inventados: lo que no existe se omite con nota o se deriva de receipts). `docs/dashboard.md` + README (sección Dashboard con captura real).
- Constraints: CSP+`textContent`+`RedactText` intactos; tokens Obsidian & Lion Gold intactos; responsive R1/R2 intacto (sidebar colapsa en móvil); a11y §12 intacta; lo que el mockup muestra y no existe en stores (ramas activas, deltas 7 días si no computables) se calcula de receipts o se omite declarando el límite en el Log — prohibido hardcodear cifras del mockup.
- Acceptance: V1, V2, V3, V4
- Artifacts: receipts `dashboard-vision/V*` (12/12 por tarea).
- Dependencies: V1 (layout+datos) antes que V2 (docs+captura real); V3 (backend) antes que V4 (UI idéntica).
- Risk: complex
- Verification: `spec check` + `gate run dashboard-vision/V*` 12/12 + verificación viva con MCPs playwright + chrome-devtools contra checklist del mockup + captura real desktop/móvil.
- Completion: V1–V4 CLOSED vía `task close`.

## Acceptance

### Scenario: V1

- Given el panel actual de secciones apiladas y el mockup `docs/assets/dashboard-vision.png`
- When se abre `/` en desktop (1280) y en 360px
- Then existe navegación lateral (colapsada en móvil), topbar con pill del último ciclo, 5 KPIs con deltas calculados de datos reales, 2 gráficos (barras + línea) con ejes, 2 tablas (ejecuciones + compuertas del último ciclo) con datos del backend, cero scroll horizontal y 0 errores de consola

### Scenario: V2

- Given V1 en verde y verificado en vivo
- When se lee el README y `docs/dashboard.md`
- Then existe sección Dashboard con captura real del panel nuevo y el copy describe lo visible sin mentir

### Scenario: V3

- Given receipts con `started_at/duration_ms/path` pero sin número de ciclo ni rama (`receipts.go:38-52`, sin campo branch)
- When se piden `/v1/executions` y `/v1/git`
- Then executions numera ciclos (# secuencial por fecha, task, resultado, compuertas, duración, fecha) desde manifests+receipts, git devuelve rama+head del checkout, y el manifest guarda `branch` hacia adelante (históricos sin rama muestran "—")

### Scenario: V4

- Given V3 en verde y el mockup como referencia pixel a pixel
- When se abre `/` en desktop (1280) y en 360px
- Then el layout es idéntico al mockup (sidebar 7 entradas con iconos, topbar con pill verde, 5 KPIs con deltas y formato `1,248`/`98.8%`, barras 7 días con eje, línea de tasa con banda, tablas #/Rama/Resultado/Compuertas/Duración/Fecha y #/Compuerta/Estado/Tiempo/Evidencia), cada cifra con copy masticado (qué es + de dónde sale, lenguaje simple), 0 errores de consola y MCPs confirman cada checklist

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| V1 | V1 | layout mockup: sidebar, KPIs, gráficos, tablas con datos reales | @regulus-worker | done |
| V2 | V2 | README sección Dashboard + captura real + docs | @regulus-worker | done |
| V3 | V3 | backend: /v1/executions (ciclos numerados) + /v1/git (rama) + branch en manifest | @regulus-worker | done |
| V4 | V4 | UI idéntica al mockup: 7 sidebar, KPIs exactos, tablas #/Rama, copy masticado | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-02. Mockup: sidebar (Resumen/Evidencia/Compuertas/Agentes/Métricas/Almanaque/Configuración) + proyecto/ramas; KPIs (evidencias, ejecuciones, compuertas PASS %, lecciones, ramas); barras 7 días; línea tasa éxito; tablas últimas ejecuciones y compuertas #482. Límite honesto: ramas/deltas solo si derivables de receipts+metrics; nada hardcodeado del mockup.
- CLOSED V1 STANDARD spec OK + 12 receipt(s) PASS (unit-tests pass 366071ms tras 2 timeouts infra; sin manifest-run_*.json — cierre sobre 12 receipts clusterizados 04:10–04:17Z, git_head 3af4fba).
- CLOSED V3 STANDARD spec OK + 12 receipt(s) PASS (manifest run_50bd7cdeebad5e2e, todo PASS): /v1/executions + /v1/git + branch en manifest.
- CLOSED V4 STANDARD spec OK + 12 receipt(s) PASS (run_faf4c2668c785b56): UI idéntica al mockup con datos reales, 0 errores consola en vivo, capturas desktop/móvil en docs/assets.
- CLOSED V2 STANDARD spec OK + 12 receipt(s) PASS (manifest run_82fa179e7d1842d0; intento previo timeout infra exit -1 todo-ok → re-run caliente): README ## Dashboard + docs/dashboard.md al día.
