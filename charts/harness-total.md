# harness-total — precisión, velocidad y tokens de todo Regulus

User authorization: "es correcto. procede con el plan y avisame hasta que finalices todo" 2026-11-03. Blanket noche completa. Cubre evaluación total (no solo buscador). Read down to `## Log` before working.

Class: COMPLEX

## Contract

- Objective: mejorar precisión, velocidad y consumo de tokens de todo el harness con 4 unidades acotadas y verificadas, sin romper denies ni guardians.
- Scope: `internal/trace/cost.go` (medir tokens local), `.opencode/plugins/regulus.ts` + `lib/domain.ts` (memoizar loadSkills, batch trace-hook, cache dominios), `internal/almanac/context.go` (acotar resume), `internal/receipts/receipts.go` (puntero recuperable), `internal/spec/spec.go` (endurecer Given/When/Then), `internal/bench/bench.go` (golden + proxy tokens). Sin tocar `opencode.json`, `regulus.json#allowlist`, `yolo.go`, ni habilitar `webfetch`. Paralelizar gates queda fuera (requiere CRITICAL + verifier dedicado, se propone después).
- Constraints: 1 unidad por worker; spec_check OK antes; gate_run 12/12 por tarea; claims de concurrencia/contratos van a @regulus-verifier; intermedios a memory_note.
- Acceptance: T1, T2, V1, P1
- Artifacts: receipts `harness-total/T*|V*|P*` (12/12 por tarea).
- Dependencies: T1→T2 (medir antes de recortar); V1 y P1 independientes de T; orden sugerido T1,T2,V1,P1.
- Risk: complex
- Verification: `spec check` + `gate run harness-total/*` 12/12 + verifier en V1/P1 si tocan concurrencia/contratos.
- Completion: T1–P1 CLOSED vía `task close`.

## Acceptance

### Scenario: T1

- Given `internal/trace/cost.go:1-13` reporta tokens `unknown` permanente porque el host no expone conteos
- When se mide proxy local (chars/4 o tokenizer ligero) en `CostByTask` manteniendo honestidad (distinguir medido vs host)
- Then `bench B7` y `budgets` muestran tokens medidos locales y `spec check` sigue OK

### Scenario: T2

- Given cada assembly inyecta disciplina + 3 lecciones + 2 skills sin dedup (`regulus.ts:587-608`) y resume incluye 20+20 (`context.go:115-146`)
- When se memoiza `loadSkills` 1 vez por proceso y se acota resume con dedup contra historial
- Then re-medición T1 muestra baja de chars/líneas vs full y `bun test` 118/118 + `tsc` clean

### Scenario: V1

- Given 1 spawn `trace record` por tool-call (`regulus.ts:542-561`) y `resolveDomain` con N×M `memory search` secuenciales (`domain.ts:443-487`)
- When trace-hook hace batch/async (cola+flush) y `domainFromFiles` paraleliza+cachea sin romper orden
- Then wall-time por Oi baja, `gate_run` 12/12, y verifier confirma sin pérdida de trazas

### Scenario: P1

- Given `spec.go:58-72` acepta `Given a` y `bench.go:96-124` son 7 checks triviales sin golden
- When `spec.Check` exige orden Given→When→Then + longitud mínima y bench añade golden prompts + proxy tokens
- Then `spec OK` rechaza escenarios vacíos y bench sigue `Compare ADOPT` solo sin regresiones

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| T1 | T1 | tokens: medir proxy local en cost/budgets | @regulus-worker | done |
| T2 | T2 | tokens: memoizar skills + acotar resume con dedup | @regulus-worker | done |
| V1 | V1 | velocidad: batch trace-hook + cache dominios | @regulus-worker | done |
| P1 | P1 | precisión: endurecer spec + bench golden | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-03 desde mapeo total (8 candidatos scout ses_e607f4b01fferLg4TvK1d0nvZI). O1 en curso con layer-4 bajo diagnóstico (infra→determinista, background go test lanzado). Gates paralelos excluidos a propósito (piden CRITICAL aparte).

- CLOSED T1 COMPLEX spec OK + 12 receipt(s) PASS (run_4e7bac468366c23b).
- CLOSED T2 COMPLEX spec OK + 12 receipt(s) PASS (run_ef84fd84fc33f749).
- CLOSED V1 COMPLEX spec OK + 12 receipt(s) PASS (run_abbf033b7b0faceb).
- CLOSED P1 COMPLEX spec OK + 12 receipt(s) PASS (run_37c81b6a432e8b79).