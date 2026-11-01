# dashboard-responsive — responsive total (móvil incluido)

User authorization: "El dashboard debe ser totalmente responsiva. osea que debe permitir verse también en móviles" 2026-11-01. Completa design.md §13 (D1 solo hizo apilado de barras ≤640px). Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: el dashboard se usa completo en móvil (360px) sin scroll horizontal ni pérdida de función: tarjetas, gráficos, 6 tablas y selector accesibles y legibles.
- Scope: `internal/httpapi/dashboard_ui.go` (solo CSS + viewport + ajustes mínimos de markup), `docs/dashboard.md` (nota responsive si cambia algo visible).
- Constraints: sin JS nuevo salvo mínimo imprescindible; CSP+`textContent` intactos; tokens Obsidian y veredictos semánticos intactos (D1); desktop idéntico a hoy; accesibilidad §12 (foco visible, labels) preservada en móvil.
- Acceptance: R1, R2
- Artifacts: receipts `dashboard-responsive/R*` (12/12 por tarea).
- Dependencies: R1 tras R2 usable; orden R2 (viewport+base), R1 (tablas y detalle).
- Risk: standard
- Verification: `spec check` + `gate run dashboard-responsive/R*` 12/12 + captura/servido en viewport móvil (best-effort sin navegador: test de CSS/HTML + lectura).
- Completion: R1–R2 CLOSED vía `task close`.

## Acceptance

### Scenario: R1

- Given el HTML servido en `/` hoy no declara viewport móvil y las 6 tablas (`#tb-receipts`, `#tb-obs`, `#tb-lessons`, `#tb-traces`, `#tb-entries`, `#tb-budget`) desbordan a 360px
- When se abre en un viewport de 360×740
- Then existe `<meta name="viewport" content="width=device-width, initial-scale=1">`, cero scroll horizontal en `/` y `/dashboard`, y cada tabla es consultable (columnas priorizadas visibles + resto accesible por apilado o scroll contenido por tabla, nunca página rota)

### Scenario: R2

- Given cabecera con selector de proyecto, controles de paginación/búsqueda y tarjetas `#cards-grid`
- When se usa en táctil móvil (sin hover, sin teclado)
- Then los controles tienen tamaño táctil (≥44px), el selector y paginadores se alcanzan sin zoom, y el foco visible se mantiene; desktop sin cambios visuales

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|--------|
| R2 | R2 | viewport + base fluida + táctil | @regulus-worker | done |
| R1 | R1 | tablas apiladas/contenidas a 360px | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 (D1 solo apiló barras ≤640px; tablas y viewport pendientes; design §13 + §5 radios exigen responsive total).

- CLOSED R2 STANDARD spec OK + 12 receipt(s) PASS (run_e1de9a9400f9e080).
- CLOSED R1 STANDARD spec OK + 12 receipt(s) PASS (run_87dd0d5fa8c62a73).