# dashboard-projects — selector de proyecto en el dashboard

User authorization: "cuando se entra a la pantalla del dashboard automáticamente me muestra información de Novalux-HN/regulusAI, pero que pasa si comienzo a usar Regulus en otro proyecto? va a almacenar la información de otro proyecto y el dashboard debe permitirme poder seleccionar de que proyecto quiero ver la información" 2026-11-01. Read down to `## Log` before working.

Class: COMPLEX

## Contract

- Objective: un solo `serve` muestra cualquier proyecto allowlistado y el dashboard deja elegir cuál ver; sin flag nuevo, comporta igual que hoy (solo el proyecto actual).
- Scope: `internal/httpapi/server.go` (flag `--projects`, ruta `/v1/projects`, `?project=`), `internal/httpapi/collections.go` + `dashboard.go` (scoping por proyecto), `internal/httpapi/dashboard_ui.go` (selector), `docs/dashboard.md`.
- Constraints: read-only se preserva (solo `GET`, ningún `POST` nuevo); allowlist declarada en arranque (nada fuera de ella es servible); jail estilo `TaskDirSafe` (traversal/`..`/absoluto = 400, fail-closed); `RedactText`+CSP+`textContent` intactos; cada proyecto sigue guardando en su propio `.regulus/` (el selector es solo lectura, no mueve datos); accesibilidad §12 (selector con label y teclado).
- Acceptance: P1, P2, P3, P4
- Artifacts: receipts `dashboard-projects/P*` (12/12 por tarea).
- Dependencies: P1 tras P2 es usable; P3 es regresión (no rompe lo actual); P4 independiente del UI (toca resolución de root en CLI+serve). Orden: P2 (jail+scoping), P1 (lista+selector), P4 (auto-detect), P3 (default intacto + docs) al final con todo.
- Risk: complex
- Verification: `spec check` + `gate run dashboard-projects/P*` 12/12 + @regulus-verifier intenta traversal (`..`, absoluto, fuera de allowlist) y confirma 400 + prueba viva dos proyectos.
- Completion: P1–P4 CLOSED vía `task close`.

## Acceptance

### Scenario: P1

- Given `serve` arranca con `--projects <dirA>,<dirB>` (además del actual `.`) y expone `GET /v1/projects` con `[{name, root}]` donde `name` es `almanac.ProjectName(dir)`
- When se abre `/` y se elige otro proyecto en el selector
- Then todas las tarjetas/gráficos/tablas recargan con `?project=<name>` y muestran los datos de ese proyecto (totales del otro `.regulus/`), y el nombre en cabecera cambia al proyecto elegido

### Scenario: P2

- Given todos los handlers leen stores con root fijo `"."` (`server.go:68-90` `New()` allowlist, `collections.go:165-501` `evidence.List(".",…)`, `almanac.Search(".",…)`, `trace.*(".",…)`, `budget.Show(".",…)`)
- When se pide `/v1/receipts?project=..`, `?project=/abs`, `?project=otro-fuera-de-allowlist` o falta `project` con allowlist activa
- Then traversal y absolutos responden 400 sin tocar disco fuera de allowlist, proyecto desconocido responde 404, y `project` válido se resuelve solo dentro de su root (jail: el path final debe quedar dentro del root allowlistado); ningún test nuevo escribe fuera de `t.TempDir`

### Scenario: P3

- Given el comportamiento actual sin flags (`serve --port=N` muestra el checkout actual, `docs/dashboard.md:10-11`)
- When se arranca sin `--projects` y se usa `/` y `/v1/*` sin `?project=`
- Then todo funciona idéntico a hoy (mismo HTML, mismos totales, `GET /v1/projects` devuelve solo el proyecto actual) y `docs/dashboard.md` documenta el modo multi-proyecto con ejemplos

### Scenario: P4

- Given el core usa `root="."` literal (cwd): correr `memory note` o `gate run` desde un subdirectorio del proyecto crea `<subdir>/.regulus/` y parte el guardado en dos (verificado por lectura: `almanac.go:137-160`, `evidence.go:22-32`, `trace.go:64-72`, `budget.go:96-98` resuelven contra el root recibido; ningún walk-up a git top-level en `internal/cli`)
- When cualquier comando corre desde cualquier subdirectorio de un checkout git (o con `regulus.json` hacia arriba)
- Then el core auto-detecta la raíz (git top-level vía `git rev-parse --show-toplevel`, fallback al primer ancestro con `regulus.json`, fallback cwd) y TODO — Almanac, receipts, traces, budget, library — cae bajo `<raíz>/.regulus/`; correr desde subdir y desde raíz del mismo proyecto comparte exactamente los mismos stores; fuera de git/regulus.json comporta igual que hoy

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|--------|
| P2 | P2 | scoping ?project= + jail + tests traversal | @regulus-worker | done |
| P1 | P1 | /v1/projects + selector + recarga por proyecto | @regulus-worker | done |
| P4 | P4 | auto-detect raíz (git top-level → regulus.json → cwd) | @regulus-worker | done |
| P3 | P3 | default intacto + docs multi-proyecto | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 (serve mono-raíz `server.go:63-71`; stores por root `.regulus/`; `ProjectName` `export.go:14-54`).

- CLOSED P2 COMPLEX spec OK + 12 receipt(s) PASS (run_bd4b43ab85bfb263).
- CLOSED P1 COMPLEX spec OK + 12 receipt(s) PASS (run_3b208c88c08bcdf1).
- CLOSED P4 COMPLEX spec OK + 12 receipt(s) PASS (run_b6198f3c2ad71e2b).
- P3 worker 2026-11-01: verificado default intacto sin cambio de código (full `go test ./internal/httpapi/ -count=1` OK; `TestP1ProjectsListsAllowlisted` single → `[{root .}]`, `TestP1DashboardHasProjectSelector` selector hidden + `if(state.project && path !== '/v1/projects')`); documentado multi-proyecto en `docs/dashboard.md` (sección «Varios proyectos en un solo `serve`»: `--projects`, `GET /v1/projects`, selector, `?project=`, tabla fail-closed 400/404, auto-detect P4 con ejemplo desde subdir, aislamiento + qué commitear). Pendiente cierre por orquestador.
- CLOSED P3 COMPLEX spec OK + 12 receipt(s) PASS (run_199da87e7acd1291).