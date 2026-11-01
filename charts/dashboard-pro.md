# dashboard-pro — retema Obsidian & Lion Gold, info correcta y por proyecto

User authorization: "modifica el dashboard a usar esos colores que están definidos en design y haz que sea un dashboard profesional. revisa que también se esté mostrando la información correcta y fácil de entender por el usuario. también recuerda que debemos guardar la información por proyecto de ser necesario así como engram" + aclaración "Ra = typo: el dashboard" 2026-11-01. Read down to `## Log` before working.

Class: COMPLEX

## Contract

- Objective: el dashboard de `regulus serve` usa los tokens de `design.md` (Obsidian & Lion Gold), se ve profesional, muestra información correcta y comprensible, y refleja el aislamiento por proyecto con bundle portable estilo Engram.
- Scope: `internal/httpapi/dashboard_ui.go`, `internal/httpapi/dashboard.go`, `internal/httpapi/collections.go` (solo si un endpoint miente), `internal/httpapi/server.go` (solo header/proyecto si hace falta), `docs/dashboard.md`.
- Constraints: CSP estricta + `textContent` + `RedactText` se preservan (sin JS externo, sin HTML inyectado); read-only se preserva (ningún `POST` nuevo); append-only y cuarentena intactos; accesibilidad design.md §12 (labels de texto en veredictos, contraste AA, teclado); responsive §13.
- Acceptance: D1, D2, D3
- Artifacts: receipts `dashboard-pro/D*` (12/12 por tarea).
- Dependencies: D1–D3 independientes entre sí (tocan capas distintas del mismo UI); D2 fija el mapa dato→pantalla que D1 viste y D3 etiqueta por proyecto.
- Risk: complex
- Verification: `spec check` + `gate run dashboard-pro/D*` 12/12 + revisión visual servida (`serve` + captura de `/` y `/v1/*` clave) + @regulus-verifier reproduce un endpoint.
- Completion: D1–D3 CLOSED vía `task close`.

## Acceptance

### Scenario: D1

- Given el panel embebido en `internal/httpapi/dashboard_ui.go:3-8` (tarjetas `#cards-grid`, gráficos `#graphs`, 6 tablas, fetch a 8 `GET /v1/*` en `dashboard_ui.go:145-329`) con estilos previos fuera de `design.md` (tokens: `brand-gold #D9A441`, `bg #0C1018/#171E29/#202A38`, semánticos PASS `#55B889`/FAIL `#E16B68`/BLOCKED `#E69A42` en `design.md:100-154`, tipografías §4, radios/moción §5)
- When se abre `/` o `/dashboard` (CSP `dashboard.go:104-109`, solo `GET` en `dashboard.go:116-133`)
- Then todo color sale de variables `:root` Obsidian & Lion Gold (cero hex hardcodeados fuera de tokens), veredictos usan color semántico + etiqueta de texto (nunca color solo), headings Space Grotesk / cuerpo Inter / datos JetBrains Mono, y `gofmt`+`crap` del paquete siguen ≤ umbrales

### Scenario: D2

- Given cada tarjeta/gráfico/tabla pinta un endpoint (`dashboard_ui.go:145-153` estado/salud, `:161-175` 5 tarjetas de `summary.total`/`receipts.total`/`observations.total`/`lessons.total`/`budget.total`, `:181-204` 4 gráficos de `by_type`/lecciones/budget/verdicts, `:210-329` tablas receipts/observations/lessons/traces/budget con paginación `collections.go:24-29` y redacción `collections.go:85-156`)
- When un usuario abre el dashboard en un proyecto con datos y en uno vacío (`observability/metrics.go:305-310` ceros nunca NaN)
- Then cada cifra visible es trazable a su endpoint y filtro (etiqueta dice qué cuenta y de dónde: p.ej. "Evidencia: N receipts (veredicto pass/total)"), proyecto vacío muestra ceros con mensaje de estado (no tablas rotas ni NaN), y `docs/dashboard.md:69-130` describe el mapa dato→pantalla sin mentir

### Scenario: D3

- Given el aislamiento por proyecto es `root="."` (stores en `<root>/.regulus/{almanac,library,evidence,traces,budget.json}`, `almanac.go:137-160`, `evidence.go:22-32`, `trace.go:64-72`, `budget.go:96-98`, `.gitignore:1-2` runtime nunca commiteado) con `ProjectName` de git remote o dirname (`export.go:14-54`) y bundle portable por dominio estilo `.engram/` (`export_domain.go:3-110`, `memory.go:596-627`, import idempotente `import.go:34-137`)
- When se sirve el dashboard en un proyecto y se exporta/importa su memoria
- Then la cabecera muestra el nombre del proyecto (`ProjectName`, no "tu proyecto" genérico), `memory export [--domain]` + `memory import` redondos por proyecto están documentados y probados en `docs/dashboard.md`, y servir dos checkouts distintos nunca mezcla datos (cada uno lee solo su `.regulus/`)

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|--------|
| D1 | D1 | retema tokens Obsidian & Gold + typo/accesibilidad | @regulus-worker | done |
| D2 | D2 | mapa dato→pantalla correcto + vacíos sanos + docs | @regulus-worker | done |
| D3 | D3 | ProjectName en cabecera + export/import redondo + aislamiento | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 (scout ses_e68c5fcd5ffekjLzvRwfnOrmfv: serve `main.go:98-106`, 8 endpoints `server.go:91-103`, UI `dashboard_ui.go`, stores por root, bundle `.engram/` en `export_domain.go`).

- CLOSED D1 COMPLEX spec OK + 12 receipt(s) PASS (run_39ce2f22584c43eb).
- CLOSED D2 COMPLEX spec OK + 12 receipt(s) PASS (run_97664638e81b9dce).
- CLOSED D3 COMPLEX spec OK + 12 receipt(s) PASS (run_352143476ea2aa98).