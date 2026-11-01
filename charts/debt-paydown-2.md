# debt-paydown-2 — CheckClose, Compose, RunTask + higienes menores

User authorization: plan total 2026-10-31, tercero = SPLITs restantes e higienes. Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: CRAP <30 sostenido + higienes de concurrencia/perf sin cambiar conducta.
- Scope: `internal/sextant/close.go`, `internal/probe/compose.go`, `internal/gates/runtask.go`, `internal/evidence/prune.go` (regex), `internal/gates/gates.go` (capWriter), timeouts git puntuales. Sin cambiar UX ni thresholds.
- Constraints: mismos mensajes/verdictos; helpers cx<15, orquestadores <20; journey verde.
- Acceptance: S1, S2, S3
- Artifacts: receipts `debt-paydown-2/D*`.
- Dependencies: D1–D3 independientes.
- Risk: STANDARD-equivalente por unidad. Class COMPLEX por multi-unidad.
- Verification: `spec check` + `gate run debt-paydown-2/D*`.
- Completion: 3 tasks CLOSED, `spec OK: 3`, crap max <30.

## Acceptance

### Scenario: S1 CheckClose partido

- Given `CheckClose` 98 líneas cx~13 CRAP 29.46
- When se extraen `checkContract/checkDeps/checkEvidence/checkVerifier`
- Then cada función <30, mismos veredictos de cierre

### Scenario: S2 Compose + RunTask partidos

- Given `ComposeWithRunner` ~112 líneas y `RunTask` ~95 líneas
- When se extraen cap-check compartido y `buildManifestParams`/`attachRefs`
- Then cada función <30, receipts idénticos

### Scenario: S3 higienes

- Given regex compilado por llamada, kill doble sin mutex, `git rev-parse` sin timeout
- When se hoistea regex, se protege capWriter, se acota git con timeout
- Then mismos resultados, `go vet`/`-race` verde en los paquetes tocados

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| D1 | S1 | SPLIT CheckClose + tests | @regulus-worker |
| D2 | S2 | SPLIT Compose (cap-check) + RunTask + tests | @regulus-worker |
| D3 | S3 | regex hoist + capWriter mutex + git timeouts + tests `-race` | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31 (plan total, deuda). Scout Go `ses_e6ba201acffeCkBZAhydahK6gi` §1/§3: `close.go:29`, `compose.go:47`, `runtask.go:27`; `prune.go:176` regex en loop; `gates.go:156,360` kill doble; `GitHead` sin timeout (`gates.go:679`, `library.go:43,235`).
- L2: D1–D3 CLOSED 2026-10-31 — D1 CheckClose 28 líneas + 4 fases (`ses_e6b768addffeICLIfxlux7uJ6a`; su gate frenó en typecheck por `compose_merge.go` intermedio de D2, re-run propio 12/12 `run_09bf9a6a57583010`), D2 Compose/RunTask fases compartidas (`ses_e6b768ad8ffeRAor9o2XYJXph4`, 12/12), D3 regex-cache + capWriter mutex + git 30s (`ses_e6b768ad7ffenFdxkbduU8yDLC`, 12/12; `-race` pendiente en CI sin gcc local). `spec OK: 3`. Cierre manual.
