# Evals con agente real (NOV-68 / T43)

Suite `agent-tasks-v3` (`bench/agent-splits-v3.json`): 34 exploracion
(14 Go, 9 node, 5 shell, 6 python, 2 trampas de gaming con checks
ocultos) + 10 holdout (4 Go, 3 node, 1 shell, 2 python, 22.7%).
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

Estado 2026-11-15 (corpus v3, NOV-77): fixtures verificados (44/44 fallan pre-fix), trampas verificadas (hardcode falla, fix real pasa). Corridas completas con reps y multi-modelo pendientes de ejecucion con creditos (ver bench/evidence/ + agent-report).

Ciclo v1 cerrado: ver `docs/f7-cycle-real.md` (2026-11-14, veredicto TIE + adopción humana registrada). Sin conflicto con este estado: aquel fue v1 de 5 tareas, este es v3 vigente de 44. Rutas `bench/evidence/v2-*`: corridas legacy v1/v2; la evidencia v3 vigente vive en los subdirectorios `v3-*-beamfree` (anidados bajo `v2-exploration-beamfree/` por la migración, se leen igual con `agent-report --evidence-dir <dir>`).

## Reproducir una corrida (v3)

Requiere: Go toolchain, node, bash y `python3` en el PATH, más un host
headless usable (`opencode` en el PATH; sin host → NOT RUN, nunca un
pase falso).

```bash
# Exploración con 3 reps y evidencia versionada (lo que pide NOV-77):
bin/regulus bench agent-tasks --reps 3 --split exploration \
  --model opencode-go/beam-free --host opencode \
  --evidence-dir bench/evidence/v2-baseline-beam-free
# Holdout (misma config, split holdout):
bin/regulus bench agent-tasks --reps 3 --split holdout \
  --model opencode-go/beam-free --host opencode \
  --evidence-dir bench/evidence/v2-holdout-beam-free
# Reporte generado desde la evidencia (verifica sha256, no edita):
bin/regulus bench agent-report --evidence-dir bench/evidence/v2-baseline-beam-free
```

Costo de referencia (medido, free tier, runner local): cada tarea
consume su `BudgetS` como techo (60–180 s); una corrida completa de
44 tareas × 3 reps ronda los 132 intentos. Timeouts del tier gratuito:
un intento que excede su presupuesto queda en `timeout` y los que
nunca corren en `infra`/`not-run` — ambos buckets no evaluables, nunca
pasan. Si el host responde 402/quota, la corrida entera es NOT RUN
(exit 3): hay que reintentar en otra ventana, no relajar umbrales.

Anti-gaming y holdout: las trampas (AT28, AT29) llevan checks ocultos
que el agente nunca ve; un hardcode pasa lo visible y falla lo
oculto. El holdout (10/44, 4 lenguajes) se evalúa aparte para que un
timeout aislado no lo deje sin evaluables.
