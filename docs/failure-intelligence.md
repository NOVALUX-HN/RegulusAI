# Mission Failure Intelligence (F13.5 / NOV-108)

Extender F12.5 Autonomous Recovery desde la recuperación de
tareas individuales a **decisiones de recuperación del plan y de
la misión completa**: el Failure Coordinator consume estados,
errores, gates, traces y receipts de la misión, clasifica el
fallo (10 tipos), selecciona una de 7 acciones con
budgets/backoff/circuit breaker y escalamiento humano, y registra
causa, evidencia, decisión, alternativas descartadas y resultado.

> El coordinador **propone, nunca ejecuta** (precedentes
> `orchestrate decide`, `quality check`): los retries corren por
> `recovery retry`/`resume`, las reasignaciones por
> `recovery resume --new-agent`, los replans por
> `mission replan`, las compensaciones por `recovery rollback`, y
> las lecciones entran a F12.6 como hipótesis, nunca como hechos.

## Reutilización (sin duplicar — prohibido un segundo mecanismo)

| Necesidad | Fuente reutilizada |
|---|---|
| Checkpoints, ledger idempotente, idempotency keys, budgets/backoff/circuit, clasificación base, subset de scopes, rollback LIFO | `internal/recovery` (F12.5): la política de misión **incrusta** `recovery.RetryPolicy` verbatim; `VerifyReassign` cita `CheckScopesSubset` y el circuito del run vivo; la ejecución sigue en `recovery resume/retry/rollback` |
| Estados y transiciones de misión, validación de ids | `internal/mission` (F13.1): la decisión **sugiere** el edge auditado (`CheckTransition`), nunca muta la misión |
| Aprendizajes etiquetados | `internal/almanac` (F12.6): `BuildLesson` solo construye `hypothesis` con confianza low/medium; la promoción sigue en `learn verify` |
| Anexado durable bajo lock | `internal/fsx.AppendLine` para `decisions.jsonl`; trazas por `observability.RecordExec` citando ids (F11.1) |
| Cableado CLI | Patrón F12.4: lógica en `internal/cmdx/failure.go`, forwarder delgado + `init` en `internal/cli/failure.go` |

## Clasificación (10 tipos, cerrada)

`transient`, `permanent`, `permission_denied`,
`budget_exceeded`, `merge_conflict`, `validation_failed`,
`corrupted_state`, `unknown`, `agent_offline`, `host_offline`.
La base F12.5 corre primero (`crash/network/timeout →
transient`, `host_offline → host_offline`, `permanent →
permanent`, `auth → permission_denied`, `partial → transient con
nota de resume-por-ledger`, `cancelled/unknown → unknown`) y la
evidencia de misión gana por precedencia: corrupción >
permiso > presupuesto > merge > agente caído > base F12.5 >
gate rojo → `validation_failed`. Solo `transient` reintenta
(acotado); todo lo demás reasume, replanifica, aísla, revierte o
se detiene por su propia vía.

## Matriz decisión → acción

| Clase | Decisión | Dentro/fuera de budget |
|---|---|---|
| `transient` | `retry` (resume por ledger, backoff determinista citado) | agotado o flag de budget → `pause_for_approval` |
| `agent_offline` / `host_offline` | `reassign` (bid verificado: caps + verificadas, contexto, worktree `clean`, scopes que estrechan, circuito cerrado) | bid ausente o no verificado → `isolate` |
| `merge_conflict` | `isolate` (serializar/manual; nunca auto-resolver) | — |
| `validation_failed` (gate rojo) | `replan` (guarda de confirmados intacta; con compensaciones propias pendientes → `rollback`) | replans agotados → `pause_for_approval` (o `rollback` si hay unidad compensable) |
| `budget_exceeded` | `pause_for_approval` (los techos nunca reintentan) | — |
| `permanent` | `rollback` (unidad transaccional, claves propias, LIFO) o `fail_closed → failed` | — |
| `permission_denied` / `corrupted_state` / `unknown` | `fail_closed → blocked` (aparcar y pedir intervención) | — |

Cada registro sella además el edge de misión sugerido
(`--to recovering|blocked|failed`, vacío si el estado grabado no
lo admite), las alternativas descartadas con su motivo, y el
`result` como propuesta concreta (`proposed: recovery resume
--new-agent …`, `proposed: mission pause, …`, etc.).

## Guardas

- **Reasignación**: solo con worktree `clean`, checkpoint vivo,
  required-caps ⊆ anunciadas ∩ verificadas (F12.1), scopes que
  estrechan lo guardado (F11.8) y circuito destino cerrado. La
  ejecución y su validación atómica quedan en
  `recovery resume --new-agent`.
- **Replan**: el set confirmado debe persistir intacto en el plan
  propuesto (ni caídos ni modificados); si la guarda rehúsa, la
  decisión degrada a `pause_for_approval`.
- **Rollback**: solo claves bajo prefijos propios (misión/run);
  una sola clave externa rehúsa todo el rollback antes de
  deshacer nada. La ejecución LIFO con `rolled_back_partial`
  nombrando el resto vive en `recovery rollback`.
- **Presupuesto**: `Retry` (F12.5: 3 retries, base 1s ×2 techo
  30s, timeout 60s, circuito 3 fallos/5min) + `MaxReplans` (0..5,
  default 2). Sin budget, sin reintento; agotado, escalamiento
  humano estructural (`pause_for_approval` / `fail_closed`).

## F12.6: hipótesis etiquetadas, nunca hechos

`--feed-learn` guarda **una** observación `hypothesis`
(`failure-coordinator`, scope `task`, confianza
medium en transient/validación/offline/conflicto y low en el
resto, proveniencia misión + receipts, `topic-key
failure/<clase>`). Nunca `verified-solution`; la promoción exige
`learn verify` con evidencia independiente.

## Comandos

```sh
# Clasificar sin tocar ningún store
regulus failure classify --error "<texto>" [--gate red] [--class <c>] [--budget-exhausted] [--json]

# Sellar una decisión (la misión debe cargar: corrupta = fail closed, exit 1)
regulus failure decide --mission <id> [--error t] [--class c] [--gate v] \
  [--run r --task t --agent a] [--budget-exhausted] \
  [--retries-used N] [--replans-used N] [--effects-applied N] [--compensations N] \
  [--worktree-state clean|dirty|unknown] \
  [--confirmed a,b] [--plan-tasks a,b] [--modified a,b] \
  [--compensate-keys a,b] [--owned a,b] \
  [--to-agent b --to-caps a,b --verified-caps a,b --scope a,b --to-host h --reason r] \
  [--max-retries N] [--timeout-ms N] [--max-replans N] \
  [--feed-learn] [--actor a] [--idem-key k] [--json]

# Listar el log de decisiones y explicar una
regulus failure status --mission <id> [--json]
regulus failure explain --mission <id> --decision <n|id> [--json]
```

`--idem-key` repetido re-juega el registro guardado sin anexar
(recuperación repetida sin duplicar). Sin `--owned`, los
prefijos propios son misión + run enlazado.

## Escenarios de fallo cubiertos (tests)

Crash del coordinador, worker desaparecido, host offline, merge
conflict, gate rojo, presupuesto agotado, permiso denegado,
estado corrupto, rollback parcial y recuperación repetida —
a nivel paquete (`internal/failure`) y E2E (`internal/cmdx`:
decide→status→explain, reasignación verificada contra run real,
`--feed-learn` como hipótesis, replay idempotente y rechazo ante
misión tamperada).

## Límites conocidos

- El coordinador no muta misiones ni ledgers: si el estado de la
  misión cambió entre la lectura y la ejecución propuesta, el
  comando ejecutor rehúsa con su propio conflicto (generación,
  transición ilegal) y se decide de nuevo.
- `unknown`/`corrupted_state` siempre aparcan: ante duda, cerrado
  y con humano, nunca adivinanza.
- La lección alimentada es conjetura operativa (`medium` como
  máximo): quien la cite debe verificarla.
