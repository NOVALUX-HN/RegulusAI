# crap-paydown — bajar CRAP max <30 para gauntlet verde

User authorization: "continua con las mejoras que recomiendas para dejar todo cheque" 2026-10-31. Desbloquea `charts/win-hardening.md` H1–H4 (6/7, frenan en capa 7). Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: CRAP max <30 en `go run ./cmd/regulus crap --profile .regulus/coverage.out --max 30 --top 5` para que `gate run` llegue a 12/12.
- Scope: `cmd/regulus/main.go:42 serve` + test, `internal/decisions/resolve.go:76`, `internal/decisions/assess.go:188`, `internal/gates/gates.go:659`, `internal/cli/gates.go:22` + tests. Sin cambiar comportamiento, solo SPLIT + COVER.
- Constraints: sin cambios de UX/CLI; helpers puros extraídos con mismos errores; `go test ./...` verde; Necessity Ladder (mínimo que baja cx).
- Acceptance: S1, S2, S3, S4, S5, S6
- Artifacts: receipts `crap-paydown/C*` con capa 7 pass.
- Dependencies: C1 primero (apalancamiento 156→~12); C2–C5 en paralelo tras C1; V1 verifica al final.
- Risk: CRITICAL (contratos consumidos: gate runner, decisions, CLI) — requiere verifier.
- Verification: `gate run crap-paydown/C*` por unidad + `crap --top 12` max <30 al cierre + verifier independiente.
- Completion: 7 tasks CLOSED + V1 PASS, `spec OK: 6`, capa 7 pass.

## Acceptance

### Scenario: S1 serve cubierto

- Given `cmd/regulus/main.go:42 serve` con CRAP 156 (cx 12, cov 0%)
- When se añade `cmd/regulus/main_test.go` cubriendo `--help`, `--port=`, `--bind=`, inválidos y uso
- Then CRAP serve <30 (solo COVER, sin tocar `main.go` salvo testabilidad mínima)

### Scenario: S2 Resolve partido

- Given `internal/decisions/resolve.go:76 Resolve` cx 38 cov 75.4% CRAP 59.54 (ni al 100% baja de 30)
- When se extraen helpers (validación una-acción, vía reject, vía inconclusive, puertas accept, supersede, commit)
- Then cada función <30 y cov ≥80%, comportamiento idéntico

### Scenario: S3 run partido

- Given `internal/gates/gates.go:659 run` cx 57 cov 91.6% CRAP 58.91
- When se extraen helpers (selección/orden, skip/when, política refusar, ejecución por runner, métrica, adjudicación+receipt)
- Then cada función <30, comportamiento idéntico

### Scenario: S4 gateCmd partido

- Given `internal/cli/gates.go:22 gateCmd` cx 40 cov 77.5% CRAP 58.34
- When se extraen helpers (parseo/validación, render dry-run, ejecución+reporte)
- Then cada función <30, CLI idéntica

### Scenario: S5 Assess partido

- Given `internal/decisions/assess.go:188 Assess` cx 54 cov 90.4% CRAP 56.58
- When se extraen helpers (percepción, evidencia+task, obsolete, criterios por bloque, recomendación)
- Then cada función <30, veredictos idénticos

### Scenario: S6 resto CRAP <30

- Given `crap --top 12` con 6 offenders >30 tras C1–C5 (`runWithStats` 50.67, `handleAlmanacSummary` 38.50, `validate` 37.04, `RunExperiments` 35.61, `flipStatusCell` 31.86, `verifyDirs` 30.73)
- When se aplica COVER/SPLIT mínimo por función
- Then `crap --max 30 --top 12` max <30

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| C1 | S1 | COVER serve: `main_test.go` para `serve` (help/port/bind/inválidos) | @regulus-worker |
| C2 | S2 | SPLIT+COVER Resolve en `resolve.go` + tests errores | @regulus-worker |
| C3 | S3 | SPLIT+COVER run en `gates.go:659` + tests | @regulus-worker |
| C4 | S4 | SPLIT+COVER gateCmd en `cli/gates.go:22` + tests dry-run | @regulus-worker |
| C5 | S5 | SPLIT+COVER Assess en `assess.go:188` + tests criterios raros | @regulus-worker |
| C6 | S6 | gates resto: `runWithStats` COVER + `validate` SPLIT en `internal/gates/` | @regulus-worker |
| C7 | S6 | resto: `handleAlmanacSummary` COVER + `verifyDirs` COVER + `Note` COVER + `RunExperiments` SPLIT leve + `flipStatusCell` COVER | @regulus-worker |
| V1 | S1-S6 | verificación independiente: reproduce `crap --top 12` max <30 y `gate run` capa 7 pass | @regulus-verifier |

## Log

- L1: Chart opened 2026-10-31 desde "dejar todo cheque". Scout `ses_e6cdbee99ffeDiROBo3p0yGwFb`: serve 156 cov 0 (COVER puro), Resolve 38/gateCmd 40/run 57/Assess 54 exigen SPLIT (c>30). Gate `regulus.json:13` sobre profile capa 4.
- L2: REPLAN 2026-10-31 — C1 CLOSED-code (serve 15.02), C2 (Resolve 11.01), C3 (run 9.00), C4 (gateCmd 8.00), C5 (Assess ≤7) verificados a nivel paquete + gate 6/7. Emergen 6 offenders >30 (runWithStats 50.67, handleAlmanacSummary 38.50, validate 37.04, RunExperiments 35.61, flipStatusCell 31.86, verifyDirs 30.73). Se añade S6/C6-C7.
- L3: C1–C7 CLOSED + V1 PASS 2026-10-31 — C6 (runWithStats 12.00, validate 9.00), C7 (handleAlmanacSummary 14.00, verifyDirs 6.01, Note 5.00, RunExperiments 18.00, flipStatusCell 20.00). Verificación propia `gate run crap-paydown/C7` 12/12 + `win-hardening/H1` 12/12. V1 `ses_e6cac3507ffezeh03paJk2KqA5` CONFIRMED S1–S6 (crap max 29.46, probe top-12 idéntico). `spec OK: 6`. Nota: `task close` inexistente en 0.9.0, cierre manual en log.
