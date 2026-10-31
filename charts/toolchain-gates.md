# toolchain-gates — mutation hermética + preflight fail-fast

User authorization: "continua hasta mejorar todo lo pendiente" 2026-10-31, último punto = toolchain y gates. Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: que el gauntlet pesado sea hermético y falle rápido — mutation sobre export limpio en código (no solo docs), preflight de binarios con hints.
- Scope: `internal/gates/` (mutation clean-export portable en Go + preflight LookPath), `docs/troubleshooting.md` + `docs/gauntlet.md` (profiles, installs). Sin tocar contratos MCP/plugin/HTTP (divergencia intencional pineada), sin publish npm (decisión owner), sin cambiar thresholds/required.
- Constraints: export vía `git archive HEAD` (falla limpio si no es git); sin POSIX (`os`+`os/exec`, como `runShell`); mismos receipts/verdictos; `mutation` sigue `profiles: [full]`.
- Acceptance: S1, S2, S3
- Artifacts: receipts `toolchain-gates/T*`.
- Dependencies: T1 primero; T2–T3 en paralelo tras T1.
- Risk: STANDARD-equivalente por unidad (mutation ya corre 1m01s en este árbol; export solo cambia el cwd del run). Class COMPLEX por multi-unidad.
- Verification: `spec check charts/toolchain-gates.md` + `gate run toolchain-gates/T*`.
- Completion: 3 tasks CLOSED, `spec OK: 3`, capa 7 pass.

## Acceptance

### Scenario: S1 mutation sobre export limpio

- Given árbol con `node_modules` presente y `git archive HEAD` funcional
- When corre la capa mutation (`gremlins unleash ./internal/receipts ...`)
- Then gremlins corre sobre export temporal sin `node_modules` (veredicto eficacia, no timeouts por bloat) y el receipt lo indica

### Scenario: S2 preflight fail-fast

- Given `gremlins` o `govulncheck` o `bun` ausentes del PATH
- When `bin/regulus gate run <t>` arranca
- Then falla antes de la capa 1 listando binarios faltantes con hints de install, sin correr capas

### Scenario: S3 docs toolchain

- Given `docs/troubleshooting.md` + `docs/gauntlet.md`
- When se leen las secciones toolchain/profiles
- Then documentan installs (gremlins, govulncheck, bun), `quick` vs `full`, export hermético, y los wontfix (npm, gap-4 MCP) con su evidencia

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| T1 | S1 | mutation hermética: export `git archive HEAD` a tmp + `gremlins` ahí (Go portable, fallback a run in-situ con warning si no-git) + tests | @regulus-worker |
| T2 | S2 | preflight en `gate run` (exec): LookPath por comando seleccionado, fail-fast con hints | @regulus-worker |
| T3 | S3 | docs toolchain/profiles/wontfix | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31. Scout `ses_e6c56a9b8ffeIJ7E3J35yZLjyI`: npm no-publicado por decisión owner (`CHANGELOG.md:331-333` G3.8, `README.md:19-23`); todo `required:true` (`regulus.json:7-18`), `quick` no omite required (`gates.go:93-98`); gremlins bloat mitigado solo en docs/agente (`git archive HEAD`, `troubleshooting.md:59-63`, `regulus-mutation-auditor.md:35-37`), 0 hits en `internal/gates/`; gap plugin-16/MCP-12 intencional pineado (`mcp_test.go:179-215` exige tocar `docs/devin.md`); HTTP read-only por diseño (`server.go:90-98`); H2/H3 cerrados.
- L2: T1–T3 CLOSED 2026-10-31 — T1 mutation hermética (`mutation_export.go`, efficacy 84.62% sobre export, 12/12); T2 preflight fail-fast + fix builtins/`regulus` skip (bug hallado por T3: `echo` builtin tumbaba 5 journey; fix + journey verde, 12/12); T3 docstoolchain/profiles/wontfix (130 ins, verificación propia 12/12 `manifest-run_843d81ddc5404b70`). `spec OK: 3`. Nota: `task close` inexistente en 0.9.0, cierre manual. Wontfix honrados: npm publish, gap-4 MCP, bash-allow, unstamped.
