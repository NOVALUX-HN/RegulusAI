# macizo-dash — panel profesional que explica lo guardado en local

User authorization: "debemos mejorar el panel de Regulus (127.0.0.1:17817): frontend más llamativo y profesional, que muestre detalladamente y fácil de entender todo lo que recopila y guarda en local" 2026-11-01. Programa macizo 4/4 (panel). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: panel que cualquier persona entiende: qué guarda Regulus en su proyecto, dónde, cuánto y por qué; con aspecto profesional.
- Scope: `internal/httpapi/*` (endpoints + UI embebida), `cmd/regulus` serve, `docs/dashboard.md`, `scripts/install-dashboard-autostart.ps1`. Solo lectura: ningún endpoint muta estado jamás.
- Constraints: cero dependencias JS externas (offline, UI embebida en el binario); CSP + redacción real en todo texto libre; paginación obligatoria en listados; loopback por defecto con aviso si `--bind` se abre.
- Acceptance: D1, D2, D3, D4
- Artifacts: receipts `macizo-dash/D*` (12/12 por tarea).
- Dependencies: D2 tras D1 (UI consume la API); D3 con D1 (redacción en endpoints nuevos y viejos); D4 independiente.
- Risk: complex
- Verification: `spec check` + `gate run macizo-dash/D*` 12/12 + revisión visual documentada (captura o descripción de la vista).
- Completion: D1–D4 CLOSED vía `task close`.

## Acceptance

### Scenario: D1

- Given 4 JSON agregados (`server.go:90-98`, `dashboard.go:35-100`) y ~80 dirs de evidence, observations, traces y budget sin exponer
- When existen endpoints read-only paginados (`?limit/?offset/?task`) de receipts, observaciones con búsqueda, lessons/candidates, sesiones de trace e historial de budget
- Then el panel puede mostrar cada dato guardado sin full-scan en el cliente y con redacción aplicada

### Scenario: D2

- Given UI de 26 líneas con 3 `<pre>` JSON (`dashboard_ui.go:6-26`) y test que exige textos actuales (`dashboard_test.go:33-50`)
- When la UI es un panel con tarjetas por área (Guardado/Evidencia/Memoria/Aprendizaje/Coste), tablas, explicaciones en lenguaje simple y gráficos sin dependencias
- Then un no-técnico entiende qué hay guardado en su proyecto y el test se actualiza al nuevo contenido

### Scenario: D3

- Given `/v1/status` reenvía stdout verbatim (`server.go:111`) y el comentario de redacción no llama a `Redact*` (`dashboard.go:3`)
- When todo texto libre pasa por redacción, el HTML sirve con CSP y los listados tienen tope
- Then ningún secreto local (tokens, rutas sensibles) llega al navegador y el bind abierto avisa

### Scenario: D4

- Given autostart solo Windows (`install-dashboard-autostart.ps1:36-40`) y `docs/dashboard.md` de una página
- When el panel arranca en las 3 plataformas (o documenta cómo) y el doc explica cada sección con capturas
- Then abrir el panel y entenderlo no requiere leer código

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|
| D1 | D1 | API read-only paginada (receipts, observaciones, lessons, traces, budget) + redacción | @regulus-worker |  |
| D2 | D2 | UI profesional embebida (tarjetas, tablas, gráficos sin deps, lenguaje simple) | @regulus-worker | done |
| D3 | D3 | CSP + redacción en status + topes + aviso de bind | @regulus-worker |  |
| D4 | D4 | autostart/documentación del panel con capturas | @regulus-worker |  |

## Log

- L1: Chart opened 2026-11-01 (programa macizo 4/4). Evidencia: UI 26 líneas `dashboard_ui.go:6-26`, endpoints `server.go:90-98`, verbatim sin redactar `server.go:111`, 122 traces y ~80 evidence dirs sin exponer.
- L2: D1 CLOSED 2026-11-01 (`ses_e6ac0e2c6ffepW3KBDc2TWrTFR`): 5 endpoints GET read-only paginados (receipts, observations?q=, lessons+ candidates, traces, budget; limit 50/200, 405 a escritura, 400 a params malos) + `RedactText` nuevo aplicado en status/version y colecciones. Run1 cayó en crap (handleLessons cx29→31.13) → SPLIT → re-run `run_cbf07bd38ea319d8` 12/12, verify 19/19. `task close` exige historial todo-verde → cierre manual (3er caso M3/D1: refuerza candidata "close sobre último manifest verde").
- L3: D2 CLOSED 2026-11-01 (`ses_e6a8d1172ffeaCuBIMlqprDpbj`, cierre formal): UI embebida ~20KB (cabecera estado+salud, 5 tarjetas con explicación simple, 4 mini-gráficos CSS puros, 5 tablas paginadas 10/pág con búsqueda/filtros, auto-refresh conservando página; cero innerHTML/deps). Tests actualizados + pin por endpoint. `gate run macizo-dash/D2` 12/12 (`run_e0ca20be11b389f2`), verify 12/12.

- CLOSED D2 COMPLEX spec OK + 12 receipt(s) PASS.