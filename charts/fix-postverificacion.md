# fix-postverificacion — corrige deriva post-0.11.0 + re-mide tokens

User authorization: "corrije los puntos encontrados. también revisa el consumo de token si hemos mejorado eso con el token optimizar que usamos, sino investigar si es viable usar Caverman" 2026-11-01. Corrige los 6 hallazgos de la verificación 2026-11-01 (docs/charts, cero código prod) y re-mide el ahorro de tokens T1/T2. Caverman: sin resultados públicos (2 búsquedas 0 hits) — se reporta y se pide enlace, no se adopta nada. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: dejar los docs/charts coherentes con el código 0.11.0 (versión, links, citas de líneas, completion) y confirmar que el ahorro de tokens T1/T2 sigue pagando en este repo.
- Scope: `docs/dashboard.md` (1 línea versión), `design.md` (1 link), `charts/release-0-11-0.md` (Given duplicados), `charts/dashboard-pro.md` (citas líneas), `charts/dashboard-projects.md` (completion + citas). Re-medición: `memory context` full vs acotado + `parseToolGuidance`/`toolsetTools` (solo lectura, sin cambiar código). Sin tocar `internal/`, `bin/`, gates o conducta.
- Constraints: solo docs/charts; no cambiar código prod ni gates; no inventar baseline de tokens (proxies auditables como en `docs/token-savings.md`); no adoptar Caverman sin enlace del usuario; `spec check` debe seguir OK.
- Acceptance: F1, F2
- Artifacts: receipts `fix-postverificacion/F*` (12/12 por tarea).
- Dependencies: F1 y F2 independientes (ficheros distintos); orden indistinto.
- Risk: standard
- Verification: `spec check` + `gate run fix-postverificacion/F*` 12/12 + `serve` vivo verificado + re-medición tokens con comandos reales.
- Completion: F1–F2 CLOSED vía `task close`.

## Acceptance

### Scenario: F1

- Given `docs/dashboard.md:325` muestra `"regulus 0.10.0"` mientras `package.json:3` es `0.11.0` y `design.md:3` enlaza `[design.md](http://design.md)` a esquema http
- When se corrigen a `0.11.0` y a link relativo al repo
- Then `grep 0.10.0 docs/dashboard.md` queda vacío en el ejemplo de versión, el link apunta al fichero local, y `spec check charts/` sigue OK

### Scenario: F2

- Given `charts/release-0-11-0.md:26-28,34-36,42-44` repite Given 2 veces por escenario, `charts/dashboard-pro.md:24` cita `dashboard.go:104-109/116-133` (hoy `112-117` + `124-136`), `charts/dashboard-projects.md:29` cita `server.go:63-71` (hoy `68-90`) y `charts/dashboard-projects.md:17` dice `P1–P3 CLOSED` con P1+P2+P3+P4 done
- When se dejan en 1 Given por escenario y las citas apuntan a las líneas actuales sin cambiar el concepto
- Then `spec check` da `spec OK` en los 3 charts y el texto describe el estado real (P1–P4)

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| F1 | F1 | docs: versión dashboard.md + link design.md | @regulus-worker | done |
| F2 | F2 | charts: Given únicos + citas al día + completion P1–P4 | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 desde verificación post-0.11.0+R2 (6 hallazgos menores, `main==origin/main` limpio, R2 12/12 `run_e1de9a9400f9e080`). Tokens: T1 −40.4% / T2 −81% minimal en `docs/token-savings.md`; re-medición pendiente en F1/F2. Caverman: 0 hits en 2 búsquedas web — se pide enlace, no se toca código.
- CLOSED F1 STANDARD spec OK + 12 receipt(s) PASS (run_34202881e61275b0).
- CLOSED F2 STANDARD spec OK + 12 receipt(s) PASS (run_e878b5a22ae632f1).
- L2: tokens re-medidos 2026-11-01: T1 bounded 14723 chars/168 líneas vs full 38156/361 = −23433 (−61.4%) PAGA más fuerte; T2 `toolset.test.ts` 13/13 PASS (3/16 defs pin intacto). Gremlins: `go run` y `bin/regulus status` dicen `available` (fix `e201e43` vigente) — hallazgo 6 ya verde, sin cambio. Caverman: 0 web + 0 repo — pendiente enlace usuario.
