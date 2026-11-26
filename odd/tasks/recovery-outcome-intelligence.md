# recovery-outcome-intelligence — F17 · Universal Recovery Outcome Intelligence

Linear: NOV-129 (padre), NOV-138..NOV-141 (hijas). Baseline: `develop`, nunca `main`.

Estado de arranque verificado: rama `develop`, `HEAD 6f92f9a`, árbol limpio, 7 commits por delante de `origin/develop`.

## Specs

S1 — No duplicar lo existente. Reutilizar NOV-100 (recovery, checkpoints, resume idempotente), NOV-108 (mission failure intelligence), LoopGuard y el executor de F15. Si un componente nombrado no existe en este repo, se reporta y no se inventa.

S2 — Unificar señales observables de progreso y las clasificaciones `Progress` / `Stagnation` / `RepeatedFailure` en el nivel apropiado.

S3 — Detectar ciclos de comandos, herramientas, errores, tests, estrategia y actividad sin progreso.

S4 — Escalera de recovery con límites, budgets, cooldown, escalamiento y host capability checks.

S5 — Registrar el outcome de cada acción de recovery: evidencia antes/después, eficacia, coste/tiempo, reintentos, rollback o escalation.

S6 — Evitar falsos positivos en trabajos legítimamente largos o exploratorios.

S7 — Recovery idempotente, cancelación, checkpoint/resume y seguridad de procesos.

S8 — Evaluar root cause y Attack the Premise cuando una estrategia repite fallos.

S9 — Exclusiones verbatim del padre: "No matar procesos indiscriminadamente; no convertir detection en autorización; no duplicar mission-level failure system; no forzar intervenciones si host solo permite observación."

S10 — Si no hay brecha demostrada, no se inventa trabajo: se reporta y se propone cierre como audit-only.

S11 — Baseline `develop`. Distinguir host real de fixture/mock y medición de estimación/proxy. Nunca debilitar tests o gates para aparentar éxito.

Texto verbatim del DoD del padre: "Matriz de señales, false-positive/negative fixtures, recovery benchmark, evidencia de recovery exitoso en escenarios representativos, budgets/idempotency/cancel tests, multi-host capability-degraded tests, regression, Gauntlet y independent review."

## Tasks

| ID | Linear | Specs | Ruta | Commit |
|----|--------|-------|------|--------|
| T1 | NOV-138 | S1 S2 S3 S6 S10 | inline + explore | `docs: progress and stagnation signal matrix (F17.1 audit)` |
| T2 | NOV-139 | S2 S3 S4 S6 S11 | inline | `feat(recovery): deterministic stagnation classification and budgets` |
| T3 | NOV-140 | S5 S8 | inline | `feat(recovery): strategy selection with before/after outcome evidence` |
| T4 | NOV-141 | S7 S9 | inline | `test(recovery): idempotent, cancellable and host-aware recovery` |

T1 — auditoría: matriz de señales observables de progreso y patrones de estancamiento, con ubicación en código, alcance y limitaciones. Read-only. Su salida condiciona T2–T4: si una brecha no aparece, no se implementa.

T2 — clasificación determinista con budgets, cooldown y criterios de escalamiento. Gap-only.

T3 — selección de estrategia y evidencia de outcome antes/después, con coste, reintentos y rollback o escalation.

T4 — validar idempotencia, cancelación, checkpoint/resume, rollback y seguridad; nunca terminación arbitraria ni intervención duplicada.

## Agent execution protocol (obligatorio, del padre)

* Sólo `develop`; nunca `main` como baseline.
* Confirmar rama, HEAD, árbol y estado remoto antes de empezar.
* Confirmar que cada ruta/API referenciada existe antes de proponer cambios.
* Demostrar la brecha con evidencia; si no existe, no inventar trabajo.
* TDD cuando sea viable; mutation tests para detección y decisiones críticas.
* Reportar archivos, evidencia, comandos y resultados exactos, límites y riesgos.

## Log

L1: "arranca pero siempre viendo la informacion completa de cada tarea y haciendo todo tal como lo piden"

L2: Las cuatro hijas comparten el mismo bloque de Acceptance criteria genérico y las mismas Instrucciones de 6 pasos; el requisito real de cada una vive en su `## Objetivo`. Criterio: cada tarea se juzga contra su Objetivo + las specs de este documento, y el template es checklist mínimo, no alcance.

L3: El padre nombra "LoopGuard" entre los sistemas a no duplicar. Verificado: existe, pero **sólo en TypeScript** (`.opencode/lib/domain-loopguard.ts:10`, `LOOPGUARD_THRESHOLD = 3`), advisory por diseño, sin equivalente Go.

L4: F17.1 entregada como `docs/progress-signals.md`. La revisión independiente refutó tres afirmaciones: son **nueve** estados durables (no diez), los tokens están **excluidos a propósito** de los spans (el deny-list de redacción borra claves con "token"), y el marco de alcance attempt/task/exploration no coincidía con el código (los reales son run, task+agent, host y mission). También aparecieron superficies omitidas que estrechan G1: `evolve` ya promueve a repeated-failures con `MinSupport 2`, `bench` ya reporta `Progress` por intento, `sextant.CheckProgress` ya sigue progreso de plan, `ReconstructRun` ya dictamina crashed/incomplete.

L5: F17.2, `internal/recovery/progress.go`. `Classify` con vocabulario cerrado y budgets 3/4/300000. El orden de las reglas es el guard de falsos positivos: la última observación vence a todo detector, y un presupuesto a medio gastar no escala.

L6: F17.3, `internal/recovery/outcome.go`. `StrategyOutcome` como documento hermano con pin `recovery-outcome-v1`: no se extendió `recovery-v1` para no cambiar un contrato persistido. `Effective` se mide (estado, efectos, compensaciones), nunca por tiempo ni retries.

L7: F17.4, `internal/recovery/safety_test.go`. Guards estructurales: ningún archivo del paquete puede referenciar `os/exec`, `os.Process`, `.Kill(`, `Process.Signal` ni `syscall.Kill`; `progress.go` tampoco. Tres expectativas iniciales eran mías y estaban mal (cancel devuelve error, reassign mueve); se corrigieron los tests, no el código.

L8: Revisión de fase: C1-C7 verificadas, y tres hallazgos corregidos — `Sample.AtMS` era un campo muerto (eliminado), y `knownState` duplicaba la allowlist de `ValidateCheckpoint` (unificada en un solo helper).

## Estado final

| Commit | Contenido |
|---|---|
| `861f99a` | F17.1 matriz de señales |
| `576eb70` | correcciones tras la revisión de F17.1 |
| `2c0f173` | F17.2 clasificador |
| `d513fe5` | F17.3 outcome medido |
| `066ef0a` | F17.4 validación de seguridad |

Gauntlet 6/6 PASS en cada tarea: `f17/T2` `ca65aab09ae4b2d7`, `f17/T3` `ab6bdf20d2156c11`, `f17/T4` `1baea416d5455269`.

### Limitación declarada, no escondida

**F17 no cableó nada.** `progress.go` y `outcome.go` son biblioteca lista y testeada, pero **sin llamador en producción**: ninguna ruta fuera de `internal/recovery` y sus tests usa `Classify` ni `StrategyOutcome`. La fase implementó la capacidad, no la integración; decir lo contrario sería inflar el resultado. El paso siguiente natural es consumirlos desde `cmdx` (un comando que clasifique un run y registre el outcome), igual que `internal/execbackend` espera a F16.2.

Residual anotado por la revisión: conviven cuatro vocabularios de clasificación en el repo (`recovery.ClassifyError`, `failure.Classify`, `gates`/`sextant.Classify`, y ahora el de progreso). Unificarlos es una decisión de contrato, no un refactor: quedó fuera de alcance a propósito.
