# Cobertura global ≥80% (gate coverage-floor)

Header: el gate `coverage-floor` (`regulus.json`, `go tool cover -func`, last-line ≥80%) falla en 69.90 (CI run 44195831777). Las fases F11–F15 agregaron código con cobertura parcial. Prohibido: bajar el umbral, excluir paquetes, tests vacíos o `t.Skip` injustificados. Solo tests reales que ejecutan código.

## Specs

- S1: umbral global ≥80% medido como última línea de `go tool cover -func .regulus/coverage.out` sobre `go test ./... -coverprofile`.
- S2: los tests nuevos deben pasar en Linux (CI) y Windows (sin `powershell`/`ping -n` hardcodeados: resolver shell por `LookPath` con skip, sleep por SO).
- S3: prioridad a código F13–F15 (pxexec, testflow, mbench, govern, mission/report, cmdx nuevo); secundarios: context, planning, recovery, testenv, mission, fsx, execute, impact.
- S4: verificación local = gofmt + vet + build (los binarios frescos están bloqueados por Device Guard en este host); la prueba real es CI.

## Tasks

| ID | Spec | Paquetes | Route | Commit |
|----|------|----------|-------|--------|
| T1 | S1-S4 | pxexec (77.5→90+), testflow (20.7→85+) | worker A | — |
| T2 | S1-S4 | mbench (67.6→90+), govern (69.3→85+) | worker B | — |
| T3 | S1-S4 | mission/report.go, cmdx mission_report (+stretch: context/planning/recovery/testenv) | worker C | — |
| T4 | — | scripts untracked: decidir commit/descarte | parent | — |
| T5 | — | push + CI verde | usuario (parent verifica) | — |

## Log

- L1 (2026-11-17): gate coverage-floor en FAIL (69.90 vs 80.00, run 44195831777). unit-tests ya en PASS. Causa: código nuevo F11–F15 con cobertura parcial.
- L2: unidad B integrada (mbench+govern, 25 tests; incl. fix real: Revoke rechazaba auto-revoke solo en Approve — ahora el executor tampoco puede revocar; validado estático, ejecución pendiente CI).
