# Mission Execution (F13.3 / NOV-106)

Ejecutor autónomo de misiones: consume un plan aprobado de F13.2 y
lo despacha coordinando F12.1 (Adaptive Orchestration) y F12.2
(Parallel Engineering), con control de estado, permisos y
presupuesto a nivel de misión. Es el dueño del ledger por tarea;
no reemplaza los estados de F13.1 (misión), F12.2 (worktrees) ni
F12.5 (recovery): los cita por id.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Máquina de estados global, concurrencia optimista, ledger exactly-once | `internal/mission` (F13.1): el executor avanza la misión y sella integraciones con `ApplyEffect`, nunca redefine el modelo |
| Plan versionado, DAG, scopes, presupuestos etiquetados | `internal/planning` (F13.2): `PlanTask` se mapea a `ParallelTask`, nunca se re-planifica |
| Orden DAG, colisiones, ciclos, concurrencia, backpressure, timeouts, cancelación, worktrees, merge, gates | `internal/orchestrate` (F12.1 `Decide` + F12.2 `RunParallel`): el executor invoca el scheduler por ola, nunca implementa un segundo |
| Routing y scopes mínimos | `orchestrate.Decide` revalida cada ola (worker offline o capability perdida bloquea con motivo, nunca despacha a ciegas) |
| Trazabilidad | `observability.RecordExec` (F11.1) bajo el run id de la ola; dry-run no registra nada |
| Lock inter-proceso y escritura atómica | `internal/fsx` (`LockDir`) + temp+rename+manifest sha, la misma disciplina del store de misión |
| Cableado CLI | Patrón F12.4: lógica en `internal/cmdx/execute.go`, forwarder delgado + `init` en `internal/cli/execute.go` |

## Modelo

`internal/execute.Ledger` (`execution-v1`), en
`.regulus/missions/<id>/execution.json` + `execution-manifest.json`
(sha256; el manifiesto propio evita colisionar con el `manifest.json`
de la misión en el mismo directorio):

- 9 estados por tarea: `pending → ready → dispatched → running →
  succeeded|failed|blocked|cancelled → integrated`. `integrated` es
  terminal (reanudar la salta, jamás la reabre); el resto reabre solo
  por retry (`failed → ready`), approve (`blocked → ready`) o
  cancelación.
- Cada registro: `task_id`, `plan_version`, `state`, `agent`,
  `attempt`, `op_key`, resultado estructurado (outcome, duración,
  ficheros, commit, scopes, motivo, run id), aprobado-por y auditoría.
- `generation` con chequeo optimista: dos coordinadores en carrera
  fallan con conflicto en vez de despachar dos veces.

## Olas y doble dispatch

Una ola es una invocación a `orchestrate.RunParallel` sobre las
tareas que el ledger marca listas. El flujo por ola:

1. Cargar misión + plan aprobado (`ready`, o `needs-approval` con
   `--approve` explícito; `refused`/`needs-clarification` y misiones
   terminales rehúsan).
2. Sembrar el ledger (idempotente) y aparcar aprobaciones
   (`needs-approval`/`needs-user`, o todo en `--supervised`).
3. Pre-chequeo de dependencias: una dependencia no sellada bloquea
   con motivo (propagado en cascada); los ciclos los rehúsa el
   planificador F12.2 nombrando el bucle (sin deadlocks).
4. Revalidar routing (`Decide` por tarea): agentes caídos o sin
   capability verificada bloquean; sus dependientes también.
5. Chequeo de budgets: agregado de la ola contra el global, cada
   tarea contra su techo (estimaciones etiquetadas, nunca medidas;
   coste desconocido ni limita ni se ceroiza).
6. Sellar `dispatched` con clave `exec/<misión>/v<plan>/<tarea>/a<intento>`;
   una tarea que ya porta clave sellada en `dispatched`/`running`
   no se re-clavea (reanudación tras crash re-conduce el intento).
7. Correr el scheduler (concurrencia, timeouts, cancelación,
   backpressure por `MaxQueued`). Integración: merge limpio +
   ficheros dentro del scope + gate en verde sellan `integrated`
   bajo la clave exactly-once `integrate/<misión>/v<plan>/<tarea>`
   (un reintento replays sin re-mergear). Conflictos, suciedad,
   fuera-de-scope o gate rojo aparcan en `blocked` para revisión
   con worktrees preservados — nunca resolución silenciosa.
8. Avanzar la misión: primera integración `executing → integrating`;
   todo integrado `→ validating`. Olas parciales quedan en
   `executing` para resume/approve/retry.

Cancelar (`execute cancel --reason`) cierra la misión y aparca el
ledger vivo en `cancelled`; la cancelación en vuelo alcanza workers
y descendientes vía contexto (dependientes saltan nombrando).

## CLI

```sh
regulus execute run --id <misión> [--version N] [--approve] [--supervised] [--dry-run] \
  [--max-concurrency N] [--per-task-timeout-ms N] [--wall-budget-ms N] [--max-queued N] \
  [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] \
  [--per-task-time-ms N] [--per-task-tokens N] [--per-task-cost N] \
  [--worktree-root <dir>] [--base <b>] [--gate-task <t>] [--actor <a>] [--json]
regulus execute status --id <misión> [--json]
regulus execute approve --id <misión> --task <t> [--actor <a>]
regulus execute cancel --id <misión> --reason <r> [--actor <a>]
regulus execute bench --id <misión> [--version N] [--parallel N] [--work-ms N] [--json]
```

Salidas: `0` éxito (dry-run siempre 0: nada rehúsa, solo planifica),
`1` rechazo o tarea bloqueada/fallida/cancelada, `2` uso.
`run` en ola vacía (todo sellado/bloqueado) no toca el scheduler:
reanudación sin duplicados por construcción.

`execute bench` corre la ola secuencial (1 worker) y en paralelo
sobre trabajo simulado y worktrees en memoria: reporta walls,
speedup y equivalencia de calidad (mismos outcomes; si divergen,
falla nombrando en vez de aprobar).

## Evidencia

- Unitarias e integración con backend en memoria (sin repo, sin git,
  deterministas): `internal/execute/execute_test.go` — transiciones
  del ledger, ledger sellado y tamper-evident, paralelo real
  (concurrencia ≥ 2), espera de dependientes, serialización de
  colisiones, timeout aislado, routing bloqueante con propagación,
  cancelación, budgets (rehúsa antes de despachar), ciclo (nombra
  el bucle), reanudación sin duplicar (conteo de ejecuciones +
  clave de intento), re-claveo prohibido, supervisado con approve,
  dry-run sin sellos, verificación (conflicto/ownership/gate),
  rechazos de plan, benchmark equivalente, cancel con ledger
  preservado.
- CLI: `internal/cmdx/execute_test.go` — dry-run, rechazos, ciclo
  approve/cancel, bench, subcomando desconocido.
- Benchmark secuencial vs paralelo con calidad equivalente:
  `regulus execute bench --id <m> [--parallel N] [--work-ms N]`.
