# Recuperación autónoma (F12.5 / NOV-100)

Recuperar **ejecuciones interrumpidas sin reiniciar todo ni
duplicar efectos secundarios**: estados durables y checkpoints por
tarea y agente, reanudación idempotente, retries acotados con
backoff/timeout y circuit breakers, clasificación de errores,
reasignación solo cuando capabilities y estado lo permiten,
idempotency keys o compensaciones, y cancelación con cleanup y
rollback parcial.

> El módulo **registra intención durable, no ejecuta trabajo**:
> `checkpoint`/`resume`/`retry` persisten estado; el executor
> externo realiza los efectos a través de claves idempotentes y
> confirma las compensaciones, que el módulo anota en orden
> inverso. Proponer, nunca actuar (precedentes `orchestrate
> decide`, `quality check`).

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Traza versionada de ejecuciones | Log global F11.1 (`observability.RecordExec`, `.regulus/observability/exec.jsonl`): cada comando emite un evento bajo el **mismo run_id**; el store de recovery solo lo cita (`TraceRunID`), nunca lo duplica |
| Permisos que solo restringen | `mcp.PrincipalFor` + `Delegate` (F11.8): `checkpoint --scope` debe ser subconjunto del grant del caller; `resume --new-agent --scope` debe estrechar lo guardado, nunca ensanchar |
| Capabilities verificadas | Registro F12.1 (`orchestrate.RegistryFromManifests` sobre la matriz F11.7): el target de una reasignación debe anunciar cada capability requerida **y** estar verificada; agentes fuera del registro solo toman checkpoints sin capabilities requeridas |
| Aislamiento y merge | F12.2 (`parallel`, worktrees): recovery anota *dónde* vive el trabajo aislado (artifacts), nunca lo mueve; el cleanup de worktrees sigue siendo `orchestrate orphans` |
| Cableado CLI | Patrón F12.4: lógica en `internal/cmdx/recovery*.go`, `internal/cli/recovery*.go` solo forwarder delgado + `init` (el pin de dispatch lo actualiza el parent) |

## Comandos

```sh
# Registrar un checkpoint durable (crea la run al primer uso;
# exit 1 ante transición ilegal o scope que ensancha, exit 2 uso)
regulus recovery checkpoint --run <id> --task <t> --agent <a> \
  --state <pending|running|checkpointed|succeeded|failed|timed_out> \
  [--host <h>] [--version <v>] [--scope <a,b>] [--cap <a,b>] \
  [--require-cap <a,b>] [--input <k=v,...>] [--output <k=v,...>] \
  [--artifact <k=v,...>] [--compensate <k=desc,...>]

# Reanudar: reasignación opcional, rotación de stalls a timed_out,
# y plan idempotente (qué se salta por aplicado, qué falta)
regulus recovery resume --run <id> --task <t> --agent <a> \
  [--new-agent <b>] [--agent-caps <a,b>] [--to-host <h>] \
  [--scope <a,b>] [--reason <r>] [--effect <e,...>] \
  [--max-retries N] [--timeout-ms N]

# Gastar un retry del budget sobre un fallo clasificado
# (exit 1 ante clase no reintentable o budget agotado —que deja
# el checkpoint en failed de forma durable—)
regulus recovery retry --run <id> --task <t> --agent <a> \
  [--error <texto>] [--class <c>] [--max-retries N] [--timeout-ms N]

# Estado auditable (humano o JSON): checkpoints, claves,
# retries, efectos aplicados, compensaciones y circuitos
regulus recovery status --run <id> [--task <t>] [--json]

# Compensar en LIFO lo pendiente (el operador ejecuta cada undo;
# el comando lo anota; un fallo deja rolled_back_partial)
regulus recovery rollback --run <id> --task <t> --agent <a>

# Cancelar (terminal y auditada; exige --reason)
regulus recovery cancel --run <id> [--task <t>] [--agent <a>] --reason <r>
```

`--effect` por defecto vale los efectos ya aplicados (el plan
muestra al menos los skips). `status --task` filtra sin tocar el
store. Sin `--scope` en `resume --new-agent`, el target hereda los
scopes guardados (estrechar a igual = permitido).

## Cómo se garantiza la idempotencia en reanudación

1. **Clave por intento**: `run/tarea/agente#intento`. Reanudar
   conserva intento y clave; reintentable avanza el intento con
   clave nueva. Un ack caído re-juega contra la misma clave y
   devuelve lo guardado en vez de re-ejecutar.
2. **Ledger de efectos**: `ApplyEffect(clave)` ejecuta `fn` solo
   si la clave no tiene resultado; si lo tiene, devuelve el output
   sin llamar a `fn` (el test de crash lo pinta: 1 ejecución tras
   crash + resume + re-apply).
3. **Escritura atómica + sha**: `run.json` se escribe por
   temp+rename con `manifest.json` (sha256). Un store editado no
   es evidencia: `resume` rehúsa en vez de adivinar (fallo
   cerrado, disciplina F11.3).
4. **Vacío no borra**: reescribir un checkpoint con mapas/host
   vacíos conserva contexto, evidencia y permisos (el circuito no
   se queda ciego).

## Reglas de reintento, timeout, circuito y errores

- **Budget**: 3 retries por defecto (`--max-retries` 0..10).
- **Backoff**: determinista, `base 1s × 2^(n-1)` con techo 30s
  (sin jitter: dos operadores replican el mismo delay y la
  auditoría lo cita).
- **Timeout**: 60s por intento (`--timeout-ms`); un
  `running` cuyo `last_confirmed` lo excede rota a `timed_out`
  antes de reanudar (clasificado, no silencioso).
- **Circuito**: 3 fallos consecutivos abren el host 5 min;
  `resume` en host abierto rehúsa (reasignar o esperar); un éxito
  lo cierra.
- **Clases**: `crash|network|timeout|unknown` reintentan (acotado);
  `host_offline` exige cambiar de host; `partial` nunca
  re-reintenta a ciegas —resume por ledger—; `permanent|auth`
  nunca reintentan; `cancelled` es terminal.

## Reasignación, cancelación y rollback

- Solo en checkpoints vivos, con capabilities requeridas ⊆
  anunciadas ∩ verificadas, scopes ⊆ guardados, y circuito del
  host destino cerrado. Mueve (no copia): contexto, evidencia,
  ledger y compensaciones viajan; la historia anota el motivo.
- `cancel` es terminal y exige motivo; `rollback` solo corre
  sobre `failed|timed_out|succeeded`, en LIFO; el primer fallo de
  compensación deja `rolled_back_partial` nombrando lo que resta.
- Limpieza de worktrees/branches: la cita el checkpoint, la
  ejecuta el operador con F12.2 (`orphans --prune`).

## Límites conocidos

- El executor de efectos y de undos vive fuera: el CLI registra;
  la idempotencia real exige que el trabajo externo use las
  claves que `resume` imprime.
- Agentes fuera del registro F12.1 solo heredan checkpoints sin
  capabilities requeridas.
- El circuito no se reinicia por `retry` con éxito (solo por
  efecto aplicado o `checkpoint succeeded`): ante flapping, manda
  el cooldown.
