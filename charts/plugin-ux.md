# plugin-ux — timeouts, launcher unificado, task cableado, mensajes honestos

User authorization: plan total 2026-10-31, segundo = P1 UX. Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: plugin sin abortos por timeout, launcher y plugin de acuerdo, `task` real, mensajes que no mienten.
- Scope: `.opencode/plugins/regulus.ts` (timeouts), `bin/regulus.js` (resolución), `internal/cli/` (wire `task`, yolo titular, trace mensajes), tests TS/Go. Sin cambiar gates ni contratos MCP/HTTP.
- Constraints: orden first-match-wins intacto; `task close` valida como `sextant.CheckClose` (misma que el core usaría); exit codes intactos.
- Acceptance: S1, S2, S3, S4
- Artifacts: receipts `plugin-ux/U*`.
- Dependencies: U1–U4 independientes.
- Risk: STANDARD-equivalente por unidad. Class COMPLEX por multi-unidad.
- Verification: `spec check` + `gate run plugin-ux/U*` + `bun test` verde.
- Completion: 4 tasks CLOSED, `spec OK: 4`.

## Acceptance

### Scenario: S1 timeouts largos donde hay gates

- Given `probe_contrast`/`probe_compose`/`evolve_propose`/`memory_context` con trabajo >30s
- When el tool corre
- Then usa timeout largo (override `REGULUS_*_TIMEOUT_MS` o el de gate_run) en vez de abortar a 30s

### Scenario: S2 launcher de acuerdo con el plugin

- Given `REGULUS_BIN` o `bin/regulus[.exe]` o `~/.regulus/bin` presentes sin `binaries/`
- When `bin/regulus.js` resuelve
- Then encuentra el binario con el mismo orden del plugin y su error cita las 6 fuentes

### Scenario: S3 task cableado

- Given `bin/regulus task close <chart> <id>` (documentado en `sextant.md:57`)
- When corre contra un chart con receipts PASS
- Then cierra (flips Status + CLOSED record) o lista condiciones sin cumplir; `task status/check` igual al `internal/cli/task.go` existente

### Scenario: S4 mensajes honestos

- Given `yolo status` con bash/edit en desacuerdo, `trace files/show` sin sesión, `.md` corrupto
- When the affected commands run
- Then yolo deriva el titular de ambos o advierte desacuerdo (+hinto `/reload`); trace distingue "sin sesión" de "sin ficheros"; fallbacks del plugin loguean a stderr qué archivo falló

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| U1 | S1 | timeouts largos `probe_*`/`evolve_propose`/`memory_context` + tests | @regulus-worker |
| U2 | S2 | `bin/regulus.js` mismo orden que plugin + error con 6 fuentes | @regulus-worker |
| U3 | S3 | wire `task` (close/status/check) en `cli.go` + tests + docs | @regulus-worker |
| U4 | S4 | yolo titular + trace distinción + plugin warn-fallbacks + tests | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31 (plan total, P1). Scout TS `ses_e6ba201aaffee5hldG8jzvjMKv` §1/§3/§6: timeouts 30s (`regulus.ts:120,235,446-539`); launcher solo `binaries/` (`regulus.js:21-32`); `task` en `cli.go:176` listado pero `unknown command`; yolo (`yolo.go:340-386`), trace (`trace.go:175-201`), fallbacks silentes (`regulus.ts:23-95,588-654`).
- L2: U1–U4 CLOSED 2026-10-31 — U1 timeouts largos compartidos (`ses_e6b832e73ffe82kS5hOxRs1j87`, bun 68/0, 12/12); U2 launcher mismo orden + error 6 fuentes (`ses_e6b832e6dffe68FhYQyDRMNDxG`, 12/12); U3 reducido honestamente: `task` YA estaba cableado (`cli.go:75-76`, evidencia scout desactualizada) + test cierre-sin-mutar (`ses_e6b832e6cffe2xyKfGanSE8wxt`, 12/12); U4 yolo mixed + trace distinción + warn-fallbacks (`ses_e6b7d35f1ffekNdQ9alio31cDV`, bun 72/0, 12/12). `spec OK: 4`. Corrección: el `task close inexistente` citado en charts viejos no rige en este árbol. Cierre manual.
