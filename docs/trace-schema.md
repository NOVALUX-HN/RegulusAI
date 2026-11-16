# Trace schema — ejecución estructurada F11.1 (NOV-87)

Versioned execution events: rebuild any run by agent, task and
delegation, from start to terminal result, from persisted events —
after a crash included. Log libre ≠ trazabilidad: este schema es la
fuente estructurada; los spans OTel-compatibles (`internal/
observability` trace/project) siguen proyectándose read-only desde
sus stores y no cambian.

- Store: `.regulus/observability/exec.jsonl` (global, append-only
  JSONL, una línea por evento; gitignored, nunca committed).
- Código: `internal/observability/exec.go` (schema, validación,
  redacción, persistencia, query, reconstrucción).
- CLI: `regulus trace emit|timeline|prune-exec`
  (`internal/cli/timeline.go`, dispatch en `internal/cli/trace.go`).

## IDs (propagación)

Mint una vez (`MintRunID`, `MintAgentExecutionID`,
`MintDelegationID`, `MintEventID`: prefijo + 64 bits crypto/rand,
process-safe) y propaga por flags de `trace emit`, plugin y
adaptadores; cada evento posterior cita los mismos IDs.

| Campo | Alcance |
|---|---|
| `run_id` | una ejecución integrada (requerido) |
| `task_id` | la tarea Sextant servida |
| `agent_execution_id` | una invocación de agente (origen del evento) |
| `delegation_id` | una arista de delegación: request y result lo comparten |
| `session_id` | la sesión del host |
| `event_id` | el evento (idempotencia: reenviarlo es no-op) |
| `parent_event_id` | enlace padre-hijo opcional |

## Evento

| Campo | Tipo | Notas |
|---|---|---|
| `schema_version` | int | hoy `1`; lectores rechazan versiones desconocidas |
| `event_id`, `run_id` | string | requeridos, una línea |
| `task_id`, `agent_execution_id`, `delegation_id`, `session_id`, `parent_event_id` | string | opcionales, una línea |
| `event_type` | enum | vocabulario cerrado abajo |
| `status` | enum | `started\|running\|succeeded\|failed\|timed_out\|cancelled` |
| `host`, `model`, `host_version` | string | solo host-expuesto, nunca estimado |
| `at` | RFC3339 UTC | canónico para orden |
| `monotonic_ms` | int | marca del proceso escritor (solo intra-proceso; se rellena si es 0) |
| `duration_ms` | int | duración medida, `>= 0` |
| `payload` | objeto | metadata; strings truncados, secretos redactados |
| `error_details` | string | texto libre, redactado pre-disco |

Tipos: `run.start|run.end|task.start|task.end|agent.start|
agent.end|context|tool.call|tool.result|delegation.request|
delegation.result|retry|timeout|gate|artifact|decision|error`.

Estados terminales inequívocos: `succeeded|failed|timed_out|
cancelled` (`IsTerminal`). Veredictos derivados — nunca
almacenados — de `ReconstructRun`: `crashed` (hay start sin
`run.end` terminal: se reconstruye tras el crash) e `incomplete`
(eventos sin start).

Atribución de delegación: `delegation.request` fija origen
(`agent_execution_id`) y destino (`payload.to`); `delegation.result`
con el mismo `delegation_id` la cierra (`pending` hasta entonces).

## Límites, retención, redacción

- Sin transcripts completos: todo string de `payload` más allá de
  `MaxExecStringValue` (2048 runas) se trunca con marcador y
  `payload.truncated=true`; payloads de más de `MaxExecPayloadKeys`
  (64) o `MaxExecPayloadBytes` (8192) se rechazan.
- Escritura incremental con lock inter-proceso, idempotente por
  `event_id` (duplicado = no-op, reporta `dup=true`).
- Lectura tolerante: líneas malformadas y `schema_version`
  desconocido se cuentan y saltan (`malformed`), duplicados
  conservan la primera ocurrencia (`duplicates`); orden de lectura
  por `at` (desorden y concurrencia leen una timeline coherente).
- Redacción previa al disco en todos los campos de texto y
  `payload` (deny-list compartida con `redact.go`); no hay writer
  sin redactar.
- Retención: `trace prune-exec [--keep N]` (defecto
  `DefaultExecKeep` = 5000) reescribe quedándose con los últimos N;
  ficheros nunca se borran salvo rewrite temp+rename.

## Comandos

```sh
# Registrar (IDs con prefijo; --event-id ausente = se mintea)
regulus trace emit --run <r> --type run.start --status started \
  --task <t> --agent <a> [--delegation <d>] [--session <s>] \
  [--parent <p>] [--host <h>] [--model <m>] [--host-version <v>] \
  [--duration-ms N] [--event-id <e>] [--payload '<json>'] [--error <msg>]

# Timeline por run, tarea, agente o delegación (filtros AND)
regulus trace timeline --run <r> [--task <t>] [--agent <a>] \
  [--delegation <d>] [--session <s>] [--type <t>] [--status <s>] [--json]

# Reconstrucción post-crash: veredicto, duración, delegaciones, evidencia
regulus trace timeline --run <r> --rebuild

# Rotación del log global
regulus trace prune-exec [--keep N]
```

Salidas: `exec recorded <event_id>` (dup: `exec event <id> already
recorded (duplicate ignored)`); timeline una línea por evento
`<at> <type> <status> run=<r>[ task=<t>][ agent=<a>]`;
`--rebuild` añade `run <r>: status <s>, <#e> event(s), <#d>
delegation(s), <#m> malformed, <#dup> duplicate(s)`, línea de crash
y `delegation <d>: <from> -> <to> (<status>)` por arista. Vacío:
`(no execution events match)`, exit 0. Exits: 0 ok, 1 fallo
(validación/IO), 2 uso.

## Migraciones

`ExecSchemaVersion = 1`. Migraciones aditivas: nuevos tipos,
status o campos opcionales suben la versión menor documentada
aquí; cambiar o reordenar campos existentes exige versión mayor y
lector dual. Hoy no hay v0: el log anterior (`events.jsonl` S1,
spans, session traces) convive sin migrar — la proyección
correlacionada (`ProjectTimeline`) los une por
`task/session/execution_id`.

## Ejemplo anonimizado

`exec.jsonl` de mentira (7 líneas en disco, 6 eventos únicos: la
línea 5 repite `event_id` y el writer la ignoraría; el lector la
contaría como duplicada). Reconstruye: `run_0000000000000001`
`succeeded`, 9000 ms, 1 delegación
`agent_0000000000000001 -> agent_0000000000000002 (succeeded)`.

```jsonl
{"schema_version":1,"event_id":"evt_0000000000000001","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","event_type":"run.start","status":"started","host":"opencode","model":"example-model","host_version":"v0.0.0-example","at":"2026-11-16T12:00:00Z","monotonic_ms":1}
{"schema_version":1,"event_id":"evt_0000000000000002","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","agent_execution_id":"agent_0000000000000001","event_type":"agent.start","status":"started","host":"opencode","at":"2026-11-16T12:00:01Z","monotonic_ms":1001}
{"schema_version":1,"event_id":"evt_0000000000000003","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","agent_execution_id":"agent_0000000000000001","delegation_id":"dlg_0000000000000001","event_type":"delegation.request","status":"running","payload":{"to":"agent_0000000000000002","reason":"example"},"at":"2026-11-16T12:00:02Z","monotonic_ms":2001}
{"schema_version":1,"event_id":"evt_0000000000000004","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","agent_execution_id":"agent_0000000000000002","session_id":"sess_example","event_type":"tool.call","status":"running","payload":{"tool":"bash"},"at":"2026-11-16T12:00:03Z","monotonic_ms":3001}
{"schema_version":1,"event_id":"evt_0000000000000004","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","agent_execution_id":"agent_0000000000000002","session_id":"sess_example","event_type":"tool.call","status":"running","payload":{"tool":"bash"},"at":"2026-11-16T12:00:03Z","monotonic_ms":3001}
{"schema_version":1,"event_id":"evt_0000000000000005","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","agent_execution_id":"agent_0000000000000002","delegation_id":"dlg_0000000000000001","event_type":"delegation.result","status":"succeeded","duration_ms":4000,"at":"2026-11-16T12:00:07Z","monotonic_ms":7001}
{"schema_version":1,"event_id":"evt_0000000000000006","run_id":"run_0000000000000001","task_id":"TASK-EXAMPLE","event_type":"run.end","status":"succeeded","duration_ms":9000,"at":"2026-11-16T12:00:09Z","monotonic_ms":9001}
```

Hashes (SHA-256, contenido exacto de arriba con `\n` final por línea):

- fichero completo: `31d246ae95b60f2a1f3878af03bd8effdbc99d5220da76beb9c6ef51758547af`
- primera línea: `0785531d4ca7fa87932501b8276102ffaf62be3dda00b6fcefc72e64ba25f3c0`
