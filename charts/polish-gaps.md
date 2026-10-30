# polish-gaps — cierre F5 + higiene residual

User authorization: "procede con las mejoras" — respuesta a "¿autorizas Chart para F5 + higiene (EVIDENCE local, bash POSIX, cierres Devin, GC)?" tras auditoría 1 media + 4 bajas. Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: F5 queda cerrado o supersedido

- Given `charts/f5-release.md` abierto solo con L1 y gate de 7 capas obsoleto
- When se revisa contra el árbol actual de 12 gates
- Then F5 queda CLOSED con evidencia o marcado SUPERSEDED por `live-plus-gaps`/`health-gaps` con razón en su Log

### Scenario: EVIDENCE auditable en local

- Given `docs/EVIDENCE.md` ausente local (solo se genera en `main`)
- When corre el gauntlet en local sobre `polish-gaps/P2`
- Then `docs/EVIDENCE.md` existe generado desde `evidence list` o queda documentado por qué solo-main con comando local equivalente

### Scenario: install-devin.bash probado

- Given `scripts/install-devin.bash` sin prueba POSIX
- When se ejecuta en shell POSIX o se verifica por revisión + `bash -n`
- Then el espejo instala y `contract` + `initialize` pasan, o queda riesgo documentado

### Scenario: cierres Devin explícitos

- Given `charts/devin-agents.md` y `charts/devin-install.md` con tasks CLOSED sin línea `Chart CERRADO`
- When se reverifican A1/A2 y D1/D2
- Then ambos charts llevan `Chart CERRADO` o se lista qué falta

### Scenario: retención local acotada

- Given `.regulus/traces/` y `.regulus/evidence/` creciendo sin GC
- When se revisa el conteo y la política CI `retention-days 7`
- Then hay política local documentada o comando `gc`/`prune`, o razón explícita de no-GC

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| P1 | S1 | F5 close/supersede + Log | @regulus-worker |
| P2 | S2–S3 | EVIDENCE local + bash POSIX probe | @regulus-worker |
| P3 | S4–S5 | cierres Devin + GC policy/docs | @regulus-worker |

## Log

- L1: Chart opened 2026-10-30 desde auditoría (f5-release abierto, EVIDENCE solo-main, bash sin POSIX, Devin sin CERRADO, traces/evidence sin GC). Autorización "procede con las mejoras".
- L2: P1 CLOSED 2026-10-30 S1 F5 CLOSED con evidencia (status.go:35-135, bin/regulus.js:1-47, package.json:10-16, README.md:14-47/49-87, regulus.json:2-14 12 gates, 12/12 L2/L3 en live-plus-gaps); F5.4 7 capas evoluciona a 12 gates, no SUPERSEDED (live-plus-gaps/health-gaps no lo reemplazan). `spec_check` f5-release + polish-gaps OK. Sin gauntlet aqui (verde en L2/L3). Sin tocar P2/P3.
- L3: P2 CLOSED 2026-10-30 S2+S3 con evidencia local. S2: `docs/EVIDENCE.md` generado local desde `bin/regulus evidence list --task live-plus-gaps/L3` (24 receipts = 12 gates x 2 runs verdes 2026-10-30, todo `pass exit=0`; HEAD `239623f`); encabezado honesto local-vs-CI: CI solo-main `.github/workflows/gauntlet.yml:66-77` (`success() && refs/heads/main`, `[skip ci]`, `retention-days 7` en artefacto PR) queda intacto, sin modificar workflow; comando local equivalente documentado en el propio archivo; fuente `internal/cli/evidence.go`. S3: `bash -n scripts/install-devin.bash` PASS; `dash -n` tambien parsea; probe de ejecucion en shell POSIX real (WSL bash 5.3.9 + dash presente, `/bin/sh -> dash`): el script aborta en paso 0 con `install-devin: Go toolchain not found on PATH` (fail-loud por diseno, sin efectos); S1/S2 del espejo reverificados contra binario sellado 0.8.1 (`regulus contract` = `regulus.contract/v1`, `--version` = `regulus 0.8.1`, `mcp initialize` responde `serverInfo.contract = regulus.contract/v1`) — identicos a los checks del `.ps1` (misma S1+S2; difieren solo install-dir: bash exige `~/.local/bin` en PATH sin fallback, ps1 prefiere `~/bin` con fallback; documentado en ambos encabezados). Riesgo residual: sin end-to-end bash con go+node (WSL no los tiene; no se corre install real para no tocar `~/.local/bin`); script es bash-por-diseno (shebang `env bash`, `set -o pipefail`), no sh/dash; `+x`: fichero untracked en git (modo no almacenado; WSL muestra rwx por mount) — al commitear usar `git add --chmod=+x`; invocacion `bash scripts/...` no lo necesita. `spec_check charts/polish-gaps.md` 5 OK. Sin tocar P1/P3, sin cambiar gates.
- L4: P3 CLOSED 2026-10-30 S4+S5 con evidencia local. S4: `charts/devin-agents.md` y `charts/devin-install.md` llevaban tasks CLOSED (A1/A2 en L2, D1/D2 en L2) sin linea `Chart CERRADO` — reverificados y cerrados: A1/A2 = 11 ficheros `.devin/agents/` con `^name:` 11/11, 0 OpenCode-isms (`@regulus-`/`tools.regulus`/`mode:`/`temperature:`); D1/D2 = `scripts/install-devin.ps1` + `scripts/install-devin.bash` (S1 contract + S2 MCP initialize), `docs/devin.md` Desktop-first (CLI-only marcada, Desktop sin `devin` CLI, robustness note), `.devin/mcp_config.json` (`command: regulus`, `args: [mcp]`). `Chart CERRADO` anadido en ambos Logs (L3). S5: 49 sesiones en `.regulus/traces/`, 39 task-dirs (~369 receipts) en `.regulus/evidence/`; `.regulus/` gitignored, nunca commiteado; sin comando global `gc` por diseno (`trace prune` es por-sesion, `evidence` es list/show solo) — politica local documentada en `docs/troubleshooting.md` (comandos `trace prune --session <s> [--keep N]` + borrado manual por task-dir, retencion CI `retention-days: 7` en `gauntlet.yml:99`, razon explicita de no-GC: no borrar evidencia de tasks cerrados sin autorizacion). Nada borrado. `spec_check` 3 charts: devin-agents 4 OK, devin-install 3 OK, polish-gaps 5 OK. Sin tocar P1/P2, sin cambiar gates.
