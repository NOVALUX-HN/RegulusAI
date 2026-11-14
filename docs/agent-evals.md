# Evals con agente real (NOV-68 / T43)

Suite `agent-tasks-v1` (`bench/agent-splits-v1.json`): 4 exploración
(AT1 Go, AT2 Go, AT3 node, AT4 shell) + 1 holdout (AT5 Go, 20%).
Cada tarea = fixture autocontenido + prompt + check oculto
(runner-only, el agente nunca lo ve). `regulus bench agent-tasks`
corre la suite con un host headless (`opencode run --format json`).

Reglas: infra/timeout/no-evaluable en buckets propios, nunca pasan;
sin host usable → `NOT RUN` exit 3 (nunca falso positivo); sin
evidencia evaluable → `INCONCLUSIVE` exit 1. Splits validados
(≥20% holdout, sin overlap, cobertura total); exploración nunca toca
holdout (anti-leakage). Resultados mapean a `CompareRobust` (NOV-59):
control negativo (todo fail) no adopta, positivo (mejora) adopta,
fallo de guardrail rechaza.

Estado 2026-11-14: sin créditos de modelo en este host (`opencode run`
→ 402 quota), la baseline real queda pendiente de una corrida con
créditos (`--reps 3`, exploración y holdout). Fixtures y checks
verificados manualmente en ambas direcciones (fix pasa, roto falla);
`go vet` + build limpios. Ejecución de `bench.test.exe` bloqueada
este ciclo por App Control (eventos 3077/3118, ver `docs/windows-diagnosis.md`);
reintentar en ventana limpia antes de cerrar NOV-68.
