# Orquestación adaptativa (F12.1 / NOV-96)

Decidir **qué agente/host** ejecuta cada tarea según riesgo, capabilities
probadas, rendimiento histórico, coste y disponibilidad — con políticas
deterministas, scoring explicable y comparación contra routing fijo.

> La decisión **propone**, nunca ejecuta: `orchestrate decide` no lanza
> ningún agente. La ejecución sigue haciéndose por los comandos explícitos
> existentes (`bench lab run`, …). `evaluate` mide sobre resultados ya
> observados.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Agentes | `bench.LabRegistry` (F11.2: responsibility, tools, host, inputs) |
| Capabilities | Matriz F11.7 (`bench.CapabilityIDs`, `DefaultHostManifests`, `DegradationFor`): solo **probada** cuenta como verificada |
| Latencia histórica | `bench.LabResult.DurationMS` + `bench.SummarizeLatencies` (mínimos F11.4: la mediana exige n≥5) |
| Coste histórico | Tokens `bench.LabResult` + `trace.RateCard` / `trace.CostOfUsage` (F11.5; desconocido nunca es cero) |
| Fallback con permisos | `mcp.Principal.Delegate` (F11.8: delegar **solo restringe**) |
| Traza | `bench.TraceHintFor` + store F11.1 (`observability.RecordExec`) |

## Comandos

```sh
# Inventario: agentes, capabilities verificadas con evidencia y límites
regulus orchestrate agents [--json]

# Decidir (propone; exit 1 si nada es elegible, exit 2 uso)
regulus orchestrate decide --domain <d> --lang <l> \
  --complexity <low|medium|high> --risk <low|medium|high|critical> \
  [--cap <a,b>] [--constraint <a,b>] [--output <a,b>] [--criteria <a,b>] \
  [--history <lab-results.json>] [--rates <tarifas.json>] \
  [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] \
  [--max-delegations N] [--unavailable <a,b>] [--fixed <agent>] \
  [--dry-run] [--out <dir>] [--json]

# Evaluar dinámica vs fija sobre resultados observados (exit 0 siempre:
# KEEP_FIXED es una conclusión, no un fallo)
regulus orchestrate evaluate --dynamic <a.json> --fixed <b.json> \
  [--rates <tarifas.json>] [--json]
```

`--history`, `--dynamic` y `--fixed` aceptan arrays JSON de
`bench.LabResult` (estricto, acotado). `--rates` acepta un array de
`trace.RateCard`. Sin historial, latencia/coste puntúan neutro con nota;
sin tarifas, el dinero queda desconocido y no limita ni compara.

## Cómo se puntúa y justifica una decisión

1. **Gates deterministas (en orden)**: disponibilidad → capabilities
   (cada exigida debe estar declarada **con evidencia** y en estado
   **probada** en la matriz del host; si no, el agente se excluye con el
   motivo nombrado) → techo de riesgo solo → budgets (tiempo/tokens;
   coste solo si el dinero es conocido).
2. **Scoring explicable** con pesos fijos y visibles (siempre impresos):
   `capability 0.30, risk 0.25, quality 0.15, latency 0.15, cost 0.15`
   (suman 1; sin auto-ajuste). Latencia y coste son **relativos** (el mejor
   estimado elegible = 1.0); calidad es la tasa de éxito absoluta (0.5
   neutro sin historial); capability es la amplitud verificada del host
   (x/12 probadas); riesgo es el margen entre techo del agente y riesgo
   de la tarea. Cada factor lleva su `why`; el desempate es el id
   alfabético. Misma entrada → mismos bytes (la traza es metadato de
   registro; en `--dry-run` no se registra nada).
3. **Uno o dos agentes**: riesgo crítico ⇒ par primario+revisor (con un
   solo elegible se **rehúsa** en vez de ir solo); en otro caso el mejor.
4. **Fallback o no-delegación justificados**: el fallback es el mejor
   no elegido con scopes **restringidos a la intersección** con el
   primario (F11.8); con `--max-delegations 0` o sin candidatos se
   justifica la no-delegación en vez de delegar a ciegas.
5. **Contraste con routing fijo**: cada decisión dice si coincide con el
   fijo (`--fixed`, por defecto `baseline`, el control del Lab), si
   difiere y por cuánto, o si el fijo ni era elegible. `evaluate`
   confirma empíricamente con calidad, latencia y coste — y puede
   concluir `KEEP_FIXED` (empates, regresiones o evidencia insuficiente
   conservan el fijo).

## Budgets y límites

`--budget-time-ms`, `--budget-tokens` (proxy), `--budget-cost` (moneda de
la tarifa) excluyen al agente que los excede (coste solo con dinero
conocido); `--max-delegations 0..4` acota la cadena (0 la prohíbe).

---

# Ingeniería paralela (F12.2 / NOV-97)

Paralelizar trabajo **independiente** reduciendo el tiempo de pared
sin conflictos ni pérdida de control — extendiendo `orchestrate`
(DAG + scheduler + worktrees), nunca un sistema paralelo aparte.

> `parallel` ejecuta comandos reales en worktrees reales. El plan
> (`--dry-run`) no crea nada ni registra nada; la ejecución registra
> cada fase en el exec log F11.1 con el mismo `run_id`.

## Comandos

```sh
# Plan (dry-run: determinista, sin efectos; exit 0)
regulus orchestrate parallel --tasks tareas.json --dry-run

# Ejecución: worktrees + merge controlado + gate (exit 0 ok; 1 fallo;
# 2 uso). Los worktrees se eliminan solo si merge y gate pasan.
regulus orchestrate parallel --tasks tareas.json \
  [--base <rama>] [--worktree-root <dir>] [--branch-prefix <p>] \
  [--max-concurrency N] [--per-task-timeout-ms N] [--wall-budget-ms N] \
  [--max-queued N] [--gate-task <tarea>] [--out <dir>] [--json]

# Huérfanos: listar (por defecto) o podar (--prune)
regulus orchestrate orphans [--branch-prefix <p>] [--active <b,c>] [--prune] [--json]
```

`tareas.json` es un array estricto de tareas:

```json
[
  {"id": "api", "title": "Migrar API", "files": ["pkg/api/"], "command": "go build ./..."},
  {"id": "doc", "title": "Actualizar docs", "files": ["docs/"], "depends_on": ["api"]}
]
```

Campos: `id` (único, charset de task), `title`, `files`
(repo-relativos que toca — dominio de colisión), `depends_on`
(aristas explícitas), `command` (shell en su worktree; vacío =
solo plan/registro), `agent` (pista de routing F12.1), `scopes`
(petición restringida F11.8), `estimate_ms`/`estimate_tokens`
(techos del dry-run).

## Cómo se aísla cada worker

| Dimensión | Mecanismo |
|---|---|
| Checkout | `git worktree add <root>/<rama> -b <rama> <base>` real (nativo, sin reimplementar git) |
| Rama | `parallel/<run>/<tarea>` (única por run; el sweeper solo toca este prefijo) |
| Directorio | uno por worker bajo `--worktree-root` (defecto `.regulus/orchestrate/worktrees`); sin ficheros compartidos |
| Workspace | **es** el worktree (F11.8: la contención de paths ancla por worker) |
| Permisos | `Principal.Delegate` (F11.8): el worker conserva solo la **intersección** con el principal; lo no listado se pierde, nunca se amplía |
| Traza | `agent_<run>_<tarea>` + `dlg_<run>_<tarea>` propios; `task.start/end`, `delegation.request/result`, `gate` y `artifact` con el mismo `run_id` (F11.1) |

## Scheduler, merge y gates

1. **DAG**: aristas explícitas + colisiones (mismo fichero o
   contención `pkg/foo` ⊃ `pkg/foo/bar.go`) ⇒ niveles
   deterministas; las colisiones sin orden explícito se serializan
   en orden alfabético y el reporte las nombra (`serialized`).
   Ciclos y planes hostiles fallan cerrados antes de crear nada.
2. **Scheduler**: un nivel en vuelo, hasta `--max-concurrency`
   (1..8, techo F11.3) workers concurrentes; `--per-task-timeout-ms`
   (timeout ⇒ `timed_out`) y `--wall-budget-ms` (deadline del run);
   `--max-queued` es la **backpressure** (más tareas que el bound ⇒
   rechazo previo, nunca cola infinita); la cancelación deja lo no
   iniciado en `cancelled` y lo dependiente en `skipped` con motivo.
3. **Aislamiento de fallos**: un worker que falla no corrompe a sus
   hermanos (cada uno es dueño de su worktree); sus dependientes se
   saltan nombrándolo; sin todos en `passed` no hay merge (worktrees
   preservados para revisión).
4. **Merge controlado**: en orden determinista (`L1, L2, …`),
   `merge --no-ff` sobre árbol limpio. El primer conflicto **para la
   línea** para revisión del integrador (ficheros sin fusionar
   nombrados, `reviewer: integrator`, resto bloqueado y nombrado) y
   el gate no corre.
5. **Gate post-merge**: con `--gate-task`, el gauntlet existente vía
   `gates.RunTask` (mismos receipts y veredicto que `gate run`); sin
   él, la puerta ligera `git diff --check`. Si falla, la integración
   se conserva con los worktrees para revisión.
6. **Limpieza**: en éxito se elimina cada worktree + rama y se hace
   `prune`; en fallo/conflicto/gate-rojo todo se preserva y
   `orphans --prune` barre después (`--active` protege runs en
   vuelo).

## Reporte (pared vs agregado)

```
parallel parallel-v1: run run_abc base HEAD
plan: 2 task(s) in 1 level(s) L1[a,b]
- a: passed 412ms branch=parallel/run_abc/a
- b: passed 388ms branch=parallel/run_abc/b
merge a: ok (parallel/run_abc/a)
merge b: ok (parallel/run_abc/b)
gate: pass (diff-check clean)
receipt: wall 420ms aggregate 800ms speedup 1.90x
cleanup: 2 worktree(s) removed
result: ok
traza: <hint F11.1>
```

`wall` es el reloj del run, `aggregate` la suma de workers
(disciplina F11.3: techos, no predicciones; el dry-run divide el
agregado estimado por el paralelismo). `--json` vuelca el
`ParallelReport` (niveles, pares serializados, veredictos, merges,
gate, `wall_ms`/`aggregate_ms`, huérfanos, `trace_hint`).
