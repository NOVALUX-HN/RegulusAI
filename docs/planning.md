# Mission Planner (F13.2 / NOV-105)

Planificador autónomo que transforma un objetivo de ingeniería en un
plan ejecutable, verificable y con riesgo/presupuesto explícitos.
Vive en `internal/planning`; el cableado CLI usa `mission
plan|replan|explain` (sin top-level nuevo).

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Entidad durable, estados, store, concurrencia | `internal/mission` (F13.1): el planner lee la misión, nunca la redefine |
| Inventario corpus (código/símbolos/docs/...)| `internal/context` Inventory (F12.3): el planner puntúa fragmentos para inferir scope |
| Grafo, diff→consumidores, tests, ciclos | `internal/impact` Census/BuildGraph/MapDiff/DetectCycles (F12.8) |
| Descriptores, routing, scoring, DAG, colisiones | `internal/orchestrate` TaskDescriptor/Decide/PlanParallelLevels (F12.1/F12.2): cada tarea se mapea a ParallelTask |
| Capabilities verificadas | `internal/bench` DefaultHostManifests (F11.7): solo `probada` con evidencia fechada |
| Scopes mínimos | `internal/mcp` (F11.8): subconjunto de los del agente, nunca superiores |
| Lock inter-proceso | `internal/fsx` (LockDir) |
| Trazabilidad | Log F11.1 (`observability.RecordExec` vía `emitMissionEvent`); dry-run no registra nada |

## Modelo

`planning.Plan` (`plan-v1`): `plan_version` (entero, el store ordena
por él), `mission_id`, `objective`, `kind`
(simple/compleja/ambigua/adversarial), `status`
(ready/needs-clarification/needs-approval/refused), hipótesis,
supuestos, riesgos, preguntas abiertas, entregables, criterios
`Given/When/Then`, tareas, DAG, exclusiones, confirmados y reporte
de calidad.

`planning.PlanTask`: id, título, objetivo, entregable,
dependencias, exclusiones, scope (ficheros repo-relativos +
componentes), agente/capability F11.7-verificada con evidencia,
permisos mínimos (F11.8), contexto (refs F12.3/F12.8), tests,
verificación (estrategia + criterios), presupuesto etiquetado
(`EstimateNote`: «estimación, no medición») y autonomía
(autonomous/reversible/needs-approval/needs-user) con bloqueos.

## Reglas

- Solo capabilities `probada` con evidencia; scopes ⊆ agente.
- Ciclos y criterios ambiguos se detectan antes de ejecutar
  (`ValidatePlan` rehúsa; el plan pide aclaración/aprobación).
- Estimaciones siempre etiquetadas; coste desconocido = desconocido,
  nunca cero.
- Replan exige `--change` (motivo conservado por versión), nunca
  reescribe ni borra: el historial es append-only y los resultados
  confirmados viajan intactos (`--confirm tarea=resultado`).
- Sin scope identificable, sin criterios verificables o con
  intención adversarial: el plan pide aclaración o se rehúsa; no
  inventa supuestos.

## Comandos

```sh
regulus mission plan --id <id> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json]
regulus mission replan --id <id> --change <motivo> [--objective <o>] [--constraint <a,b>] [--criteria <a,b>] [--confirm <tarea=resultado,a,b>] [--author <a>] [--actor <a>] [--dry-run] [--json]
regulus mission explain --id <id> [--version N] [--json]
```

Códigos: `ready`/`needs-approval` → 0 (el operador actúa sobre el
status); `needs-clarification`/`refused` → 1 (nunca un cero que
esconda un plan bloqueado); defectos estructurales → 1;
uso malformado → 2.

## Calidad del plan

Gates con nombre: `plan-no-vacio`, `criterios-cubiertos`
(no-omisión: todo criterio tiene tarea covering),
`criterios-verificables`, `capabilities-verificadas`,
`scopes-minimos`, `dag-aciclico`, `impacto-conocido`,
`estimaciones-etiquetadas`. `mission explain` muestra por qué se
dividió el trabajo, por qué se serializa cada colisión, la tabla de
autonomía y el diff contra el plan original y la revisión anterior.

## Corpus de evaluación

`internal/planning/planning_test.go`: solicitud simple (plan
ejecutable con criterios verificables), compleja (DAG multinivel
con colisiones serializadas), ambigua (pide aclaración, nada
autónomo), adversarial (rehusado, cero tareas), DAG cíclico,
colisiones, scopes insuficientes, impacto desconocido, plan
reversionado (historial + motivo + confirmados) y no-omisión de
requisitos críticos.
