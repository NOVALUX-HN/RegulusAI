# macizo-tokens — compaction acotada + tool pruning + loop guard

User authorization: "crea un plan para ejecutar cada uno de ellos e integrarlos a Regulus para hacer un producto macizo" 2026-11-01. Programa macizo 3/3 (economía de tokens, mecanismos TokenEconomizer adaptados a nuestro plugin + fugas propias). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: recortar tokens por sesión con mecanismos reversibles y reporte de ahorro honesto (coste propio incluido).
- Scope: `.opencode/plugins/regulus.ts` (hooks, registro de tools), `.opencode/lib/domain.ts`, `internal/almanac/context.go`, `internal/trace/*`, `docs/*`. Sin cambiar conducta de gates; todo mecanismo reversible y desactivable por env (`TE_*`-style).
- Constraints: sin nuevas dependencias; nada que dependa de versiones pineadas de otros agentes; primero baseline (F1), sin baseline no hay claim de ahorro.
- Acceptance: T1, T2, T3, T4
- Artifacts: receipts `macizo-tokens/T*` (12/12 por tarea).
- Dependencies: T4 tras F1+T1+T2+T3; T1–T3 independientes.
- Risk: complex
- Verification: `spec check` + `gate run macizo-tokens/T*` 12/12.
- Completion: T1–T4 CLOSED vía `task close`.

## Acceptance

### Scenario: T1

- Given la compaction inyecta `memory context` completo (`regulus.ts:569-574`, `context.go:45-46`) y decisions van íntegras (`context.go:267-284`)
- When los defaults usan budget/limit + scope y el resume mantiene utilidad (test de fidelidad del resume)
- Then los tokens de resume quedan acotados sin perder la capacidad de continuar trabajo

### Scenario: T2

- Given 16 tools registradas incondicionalmente (`regulus.ts:138-456`) y TokenEconomizer en opencode "nothing yet"
- When se prueba registro condicional por modo (chart activo vs sesión simple) con fallback total si la API no lo permite
- Then las sesiones simples pagan menos definiciones, o queda documentado por qué no con alternativa (guidance lazy)

### Scenario: T3

- Given sin detección de bucles y traces con fingerprint (`trace.go:212-217`)
- When un detector determinista (mismo fallo ×3 → redirect de 1 línea, reversible, nunca detiene solo) vive en el plugin
- Then los bucles se cortan con nudge y el progreso normal sigue intacto (tests de no-interferencia)

### Scenario: T4

- Given F1 (contabilidad) + T1–T3 activos
- When se mide antes/después en este repo con coste propio incluido
- Then el reporte de ahorro es honesto por mecanismo, con reversión documentada si alguno no paga

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| T1 | T1 | defaults budget/limit en compaction+context + test fidelidad | @regulus-worker |
| T2 | T2 | probe registro condicional + subsets por modo o alternativa | @regulus-worker |
| T3 | T3 | loop guard determinista (×3→redirect, reversible) + tests | @regulus-worker |
| T4 | T4 | medición antes/después + reporte honesto por mecanismo | @regulus-worker |

## Log

- L1: Chart opened 2026-11-01 (programa macizo 3/3). Evidencia: fuga compaction `regulus.ts:569-574`, 16 tools `regulus.ts:138-456`, fingerprint `trace.go:212-217`, TE settings/coste-con-cache.
