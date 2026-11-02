# dashboard-ux — mejorar diseño y presentación de la información

User authorization: "me gustaria que hicieras cambios en el dahsboard" + aclaración "mejorar el diseño y como se muestra la informacion" 2026-11-02. Verificación con MCPs playwright + chrome-devtools tras el cambio. Serve vivo `17817 → 200`, UI `internal/httpapi/dashboard_ui.go` (~30KB), tokens `design.md:103-154`. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: el dashboard se ve más profesional y la información se entiende a golpe de vista: jerarquía visual clara, cada cifra con etiqueta de qué cuenta y de dónde sale, vacíos con mensaje útil, tablas y gráficos legibles.
- Scope: `internal/httpapi/dashboard_ui.go` (CSS + copy de etiquetas + ajustes mínimos de markup), `docs/dashboard.md` (nota si cambia algo visible). Sin cambiar endpoints ni datos.
- Constraints: CSP+`textContent`+`RedactText` intactos (sin JS externo, sin HTML inyectado); tokens Obsidian & Lion Gold intactos (`design.md:103-154`); responsive R1/R2 intacto (viewport, 44px, `.tbl-wrap`); accesibilidad §12 (foco visible, labels, contraste) intacta; desktop y móvil sin regresión.
- Acceptance: U1, U2
- Artifacts: receipts `dashboard-ux/U*` (12/12 por tarea).
- Dependencies: U1 y U2 independientes (misma capa, secciones distintas); orden indistinto.
- Risk: standard
- Verification: `spec check` + `gate run dashboard-ux/U*` 12/12 + verificación viva con MCPs playwright y chrome-devtools (`serve` + captura + lectura de `/` y `/v1/*` clave).
- Completion: U1–U2 CLOSED vía `task close`.

## Acceptance

### Scenario: U1

- Given el panel actual (`header.top`, `#cards-grid`, gráficos de barras CSS, 6 tablas, paginadores) con jerarquía plana y espaciados mejorables
- When se abre `/` en desktop y en 360px
- Then la jerarquía es clara (cabecera, secciones, tarjetas, gráficos, tablas), espaciado y tipografía consistentes, sin scroll horizontal y sin regresión móvil

### Scenario: U2

- Given cifras y tablas que hoy muestran totales sin explicar qué cuentan ni de dónde salen, y proyectos vacíos con ceros secos
- When un usuario abre el dashboard con datos y en uno vacío
- Then cada tarjeta/gráfico/tabla dice qué cuenta y su fuente, los vacíos muestran mensaje de estado útil (nunca tabla rota ni NaN), y `docs/dashboard.md` describe lo visible sin mentir

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| U1 | U1 | jerarquía visual: espaciado, secciones, tarjetas y gráficos | @regulus-worker | done |
| U2 | U2 | claridad info: etiquetas con fuente, vacíos útiles, copy | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-02 (serve 200, UI ~30KB, tokens design.md:103-154). Mejora pedida: diseño + presentación. Verificación MCPs tras el cambio.
- L2: verificación viva con MCPs playwright + chrome-devtools (serve fresco 17840): fuentes/leyenda/paginadores/timestamp renderizan con datos reales (127 tareas, 1955/2003), 0 errores JS tras el fix. Hallazgo: `id="graphs"` duplicado preexistente (section :157 + div :159) → loadGraphs borraba su propia sección y `setError('graphs-error')` lanzaba TypeError cada ciclo. Fix TRIVIAL 1 línea (section → `sec-graphs`); verificado 0 errores en 3 ciclos. Único 404 benigno: favicon.ico (preexistente, sin favicon servido).
- CLOSED U1 STANDARD spec OK + 12 receipt(s) PASS (run_06de5d9cd020c04e).
- CLOSED U2 STANDARD spec OK + 12 receipt(s) PASS (run_4e4eaa24b4f26b32, árbol final con fix sec-graphs). Intento previo: stop en unit-tests por timeout infra (exit −1, todos los paquetes ok) → re-run en caliente 12/12.
