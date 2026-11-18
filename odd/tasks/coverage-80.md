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
- L2: unidad B integrada (mbench+govern, 25 tests; incl. fix real: Revoke aceptaba al executor — ahora lo rechaza; validado estático, ejecución pendiente CI).
- L6: unidades D+E integradas (cmdx 45 tests incl. routers/lifecycles/handlers + testenv 6; context 26 + orchestrate 27 + execute 24 + mission 11; vet/build verdes, ejecución pendiente CI).
- L3: unidades A+C integradas (pxexec 16 tests, testflow 14, mission/report 14, cmdx assemble 11, recovery 7, context 4 — todo *_test.go nuevos, portables, vet/build verdes) + fix shellFlag con rutas absolutas (LookPath devolvía path absoluto que caía a `-c`) + test de paridad de rutas. Ejecución pendiente CI.
- L4 (2026-11-18): CI 44204618668 falló en unit-tests con el código nuevo: blockText sin normalizar espacios (test propio), short-write real en limitedWriter (exec lo convierte en fallo de lanzamiento), go env no refleja vars desconocidas, discover con cwd de paquete. Corregido: normalización de espacios, Write siempre completo, helper-process hermético (sin binarios externos), discover anclado a go.mod, checks aun con discovery roto. Verificado local 2 rondas verdes (pxexec+testflow).
- L5: CI 44228921545: unit-tests PASS (incl. pxexec 100%, testflow 94.5%, govern 95.8%); coverage-floor sube 69.90→71.3 pero sigue bajo 80. Lastre principal: cmdx 17.6% + testenv 50% + context 55.1%. Segunda ronda de tests enfocada.
