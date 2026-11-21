# Mission Engine (F13.1 / NOV-104)

Motor durable que representa y gobierna una misión de ingeniería de
principio a fin. Es la **fuente de verdad del estado global**; no
reemplaza los estados especializados de F12.2 (`orchestrate`,
worktrees) ni F12.5 (`recovery`, checkpoints): solo los cita por id.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Disciplina durable (temp+rename+manifest sha, fail-closed, budgets) | Patrón de `internal/recovery/store.go` (F12.5); forma propia del mission |
| Lock inter-proceso | `internal/fsx` (`LockDir`), el mismo que serializa el log de trazas |
| Trazabilidad versionada | Log global F11.1 (`observability.RecordExec`); cada comando emite bajo el primer `run_id` enlazado (o `mission/<id>`) con `TaskID = mission_id` |
| Cableado CLI | Patrón F12.4: lógica en `internal/cmdx/mission.go`, forwarder delgado + `init` en `internal/cli/mission.go` |

## Modelo

`internal/mission.Mission` (`mission-v1`):

- Identidad: `mission_id` estable (alfanumérico, `msn_` + 16 hex si se omite `--id`), `objective` (requerido), `description`, `repo`/`workspace`, `constraints`, `acceptance_criteria`, `risk` (`low|medium|high|critical`, default `medium`), `budget` (time/tokens/cost, 0 = unset), `creator`, `created_at`/`updated_at` (UTC).
- Estado global `state` + `generation` (concurrencia optimista: cada escritura sella generación+1; una escritura con generación vieja rehúsa con `generation conflict`).
- `links`: sets deduplicados y ordenados de `run_ids`, `task_ids`, `agent_ids`, `worktrees`, `commits`, `traces`, `receipts` (cada evento/artefacto se correlaciona con `mission_id`).
- `effects`: ledger exactamente-una-vez (`ApplyEffect(key, fn)`): re-aplicar devuelve lo guardado sin re-ejecutar, así reanudar no duplica efectos.
- `paused_from`: bookmark de `pause`/`resume`. `history`: eventos auditables (`from → to`, actor, reason, `idem_key`, timestamp). `cancel_reason`, `reopen_count`.
- Eventos idempotentes: repetir un `idem_key` visto devuelve el evento guardado sin mutar (el CLI ni siquiera escribe).

## Estados y transiciones

13 estados. Terminales (sin salida por la vía normal): `completed`,
`partially_completed`, `failed`, `cancelled`. Solo `Reopen`
(explícita, con motivo auditado, cuenta cada reapertura) sale de un
terminal, y solo hacia `recovering`.

| Desde | Hacia (vía normal) |
|---|---|
| `created` | `analyzing`, `cancelled` |
| `analyzing` | `planning`, `blocked`, `failed`, `cancelled` |
| `planning` | `awaiting_approval`, `blocked`, `failed`, `cancelled` |
| `awaiting_approval` | `executing`, `planning` (rework), `blocked`, `failed`, `cancelled` |
| `executing` | `integrating`, `validating`, `recovering`, `blocked`, `failed`, `cancelled` |
| `integrating` | `validating`, `executing` (rework), `recovering`, `blocked`, `failed`, `cancelled` |
| `validating` | `completed`, `partially_completed`, `executing` (rework), `recovering`, `blocked`, `failed`, `cancelled` |
| `recovering` | `executing`, `integrating`, `validating`, `blocked`, `failed`, `cancelled` |
| `blocked` | `analyzing`, `planning`, `awaiting_approval`, `executing`, `integrating`, `validating`, `recovering`, `failed`, `cancelled` (nunca directo a `completed`/`partially_completed`: solo se sale por `validating`) |

Invariantes: transiciones ilegales y estados desconocidos rehúsan
antes de registrar nada; mismo-estado es no-op idempotente;
`pause` solo aparca misiones vivas en `blocked` (con bookmark);
`resume` solo vuelve al estado aparcado (sin bookmark rehúsa:
avanzar explícitamente); `cancel` exige motivo; `reopen` exige
motivo y solo acepta terminales.

## Persistencia y concurrencia

- Store: `.regulus/missions/<id>/mission.json` + `manifest.json`
  (sha256). Escritura atómica temp+rename bajo lock; lectura
  verifica sha (editado = `sha mismatch`, no evidencia) y deriva
  de versión (`mission-v0` migra explícitamente a `v1` con evento
  en la auditoría; versión desconocida = `unsupported`, nunca
  adivinada).
- Un solo coordinador: `UpdateMission` (lock + carga + mutación en
  memoria + sello con la generación cargada). Dos procesos
  concurrentes: el segundo ve `generation conflict`, recarga y
  reintenta; nada se sobrescribe en silencio. `create` nunca
  sobrescribe (doble create = `already exists`).
- Crash entre transición y sello = nunca pasó: la próxima carga ve
  el último estado confirmado y reanuda desde ahí (mismo contrato
  que F12.5).

## CLI

```sh
regulus mission create --objective <o> [--id <id>] [--description d] [--repo r] [--workspace w] \
  [--constraint a,b] [--criteria a,b] [--risk low|medium|high|critical] \
  [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] [--creator c] [--actor a] [--json]
regulus mission show --id <id> [--json]
regulus mission list [--state <s>] [--json]
regulus mission status --id <id> [--json]
regulus mission advance --id <id> --to <state> [--reason r] [--actor a] [--idem-key k]
regulus mission pause --id <id> [--reason r] [--actor a] [--idem-key k]
regulus mission resume --id <id> [--reason r] [--actor a] [--idem-key k]
regulus mission cancel --id <id> --reason <r> [--actor a] [--idem-key k]
regulus mission link --id <id> [--run a,b] [--task a,b] [--agent a,b] [--worktree a,b] \
  [--commit a,b] [--trace a,b] [--receipt a,b] [--actor a]
regulus mission reopen --id <id> --reason <r> [--actor a] [--idem-key k]
regulus mission report --id <mid> [--json]
regulus mission verify-report --id <mid>
```

`report` arma el expediente versionado (`report-v1.json` junto al
record: objetivo, criterios con evidencia o motivo, planes y DAG,
tareas con agentes/commits/evidencia, approvals, tests/gates,
fallos y recuperación, costes con calidad
measured/estimated/unknown, limitaciones) y lo sella con hash;
redacta secretos antes de sellar. Lo desconocido se registra
como unknown con su motivo, nunca se inventa. Solo misiones
terminadas o bloqueadas producen expediente (otra cosa = exit
1). `verify-report` revalida versión, outcome y hash: alterado
= INVALID exit 1.

Salidas: `0` éxito, `1` rechazo (transición ilegal, conflicto de
generación, store corrupto, terminal), `2` uso. `list` con una
entrada corrupta la reporta (`CORRUPT: <causa>`) y sale `1`:
explícito, sin pérdida silenciosa.

## Evidencia de recuperación

- Unitarias: máquina (`mission_test.go`: camino completo, ilegales,
  terminales, pause/resume, cancel, reopen, idempotencia, links,
  ledger), persistencia/migración/concurrencia/validación
  (`store_test.go`: round-trip, generaciones, conflicto stale,
  8 coordinadores, sha mismatch, manifiesto corrupto, migración
  v0→v1 y sellado, versión desconocida, crash entre cada
  transición crítica, restart, ledger tras crash, cancel/resume
  durable, lista con corrupto explícito).
- Integración multi-proceso real (`internal/cmdx/mission_test.go`:
  dos procesos OS: uno stale + uno vivo; el stale observa el
  `generation conflict` y el store queda válido).
- CLI/API (`internal/cli/mission_test.go`: help, subcomando
  desconocido, ciclo completo por dispatch, cancel sin motivo =
  exit 2, validación, lista+link, cancel/resume tras restart).

## Gate F13.8 — Full Autonomous Engineering Benchmark

Corpus versionado (`mission-corpus-v1`: 10 exploration MB1..MB10 +
3 holdout sellado MH1..MH3, 23% ≥ 20%) con splits commiteados
(`bench/mission-splits-v1.json`, seed 20261117): exploration siempre,
holdout solo con flag explícito (sin flag = exit 2, nunca parcial
silencioso). Cada caso corre su journey completo sobre los motores
reales (mission, governance, report, verify) en stores aislados; los
mocks solo cubren señales deterministas de fallo y van etiquetados
`simulated` en el resultado.

```sh
regulus bench mission [--split exploration|holdout|all] [--holdout] [--save P] [--json] [--dry-run]
```

`--dry-run` valida corpus+splits y lista casos sin ejecutar. El run
agrega con `mbench.Aggregate`: PASS exige cero violaciones, outcomes
según oráculo, expedientes válidos, todas las clases cubiertas y
baseline contrastada; `--save` escribe veredicto+resultados en JSON.
Salidas: `0` solo en PASS, `1` FAIL/INSUFFICIENT_EVIDENCE o error de
evidencia, `2` uso (incluye holdout sin `--holdout`).
