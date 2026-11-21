# Tokens reales por host — medición y atribución (NOV-73, F11.5)

Documento vivo (no H10): el estado de hoy de cada host, cómo se mide
y con qué evidencia. Traslada aquí la sección viva que estaba al final
de `docs/token-savings.md` (ese archivo queda como registro H10
congelado con un puntero a este documento).

> F11.5 (NOV-91) extiende este documento sin duplicar NOV-73: los
> lectores, el E2E de opencode y el hook siguen siendo los de NOV-73
> (reutilizados tal cual; la reconciliación está al final). F11.5
> añade etiquetas (`estimated`, `reasoning_tokens`, claves
> task/agent/reason), redacción, tarifas explícitas, correlación
> run/task/agent/session/host y reportes con procedencia.

## Estado verificado por host (2026-11-16, esta máquina — re-auditoría F11.5)

| Host | Fuente | Versión verificada | Estado |
|------|--------|-------------------|--------|
| opencode | `opencode session export <id>` → `info.tokens.{input,output,reasoning,cache.{read,write}}` + `info.model{id,providerID}` | opencode v2.2.5, 303 sesiones en `~/.local/share/opencode/opencode.db` (`session_v2`) | **measured-host** (E2E NOV-73 reutilizado; re-exportado hoy sin llamadas: `ses_e1a0…` → input 557, output 593, reasoning 0, cache read 42537 write 11696) |
| claude | `~/.claude/projects/*/*.jsonl`: `message.usage` (`input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, `model`) | claude v2.1.325; formato re-verificado hoy en 1 transcript real (21 líneas, 1 bloque usage) | **unknown** (deriva tolerada: claves extra `server_tool_use`/`service_tier` ignoradas, líneas usage a cero suman 0; sin sesión viva que atribuir) |
| codex | `~/.codex/sessions/*/*/*/rollout-*.jsonl`: líneas `token_usage_record` (`input_tokens`, `cached_input_tokens`, `cache_write_input_tokens`, `output_tokens`, `reasoning_output_tokens`) | formato re-verificado hoy en 1 rollout real (model `gpt-6-luna`, `cli_version` 0.169.0, 1 `token_usage_record`); binario no instalado en esta máquina | **unknown** (formato verificado + reader listo, sin tarea ejecutada que atribuir) |
| devin | MCP + skills, sin contadores locales | sin cambios desde F8 | **unknown** (`host exposes none`: MCP no expone conteos) |

Lectores (`internal/trace/hostreaders.go`, solo lectura, fallan
cerrado ante deriva de esquema): `ParseOpencodeExport`,
`ParseClaudeTranscript`, `ParseCodexRollout` — todos validados por
`ParseHostUsage` (64KB por documento de uso / 64MB por documento de
host, sin negativos, cotas anti-absurdo 1e12 por figura y 1e6 por
tarifa, sumas anti-overflow, single-line ≤512, source del enum con
`estimated`). `DevinUnknown()` declara el estado verificado de Devin
en vez de inventar cifras. Fixtures: transcripts reales
anonimizados + malformados + hostiles + deriva (`hostreaders*.go`,
`f115_test.go`). Deriva 2026-11-16 tolerada sin cambiar el contrato:
`model` nulo en exports opencode (unknown, no error), claves extra
en usage de claude (ignoradas), líneas usage a cero (suman 0).

## Prueba de punta a punta (opencode, 2026-11-15)

Tarea real mínima ejecutada headless (sin modelo de juguete):

```bash
opencode run --model opencode-go/beam-free --format json "Reply with exactly: OK"
# → session ses_e1f2e6878ffemDdkn5TiPM9rl0, step-finish tokens
#   {input:3865, output:4, reasoning:22, cache:{read:1600, write:0}}
opencode session export ses_e1f2e6878ffemDdkn5TiPM9rl0
# → info.tokens {input:4307, output:11, reasoning:22,
#    cache:{read:1600, write:0}}, model beam-free
regulus trace record --session ses_e1f2e6878ffemDdkn5TiPM9rl0 \
  --message msg_probe --call c1 --tool opencode --status completed \
  --task probe/NOV-73 --execution-id exec-probe-1 \
  --input-tokens 4307 --output-tokens 33 \
  --cache-read-tokens 1600 --model beam-free --host opencode \
  --host-version 2.2.5
# → trace recorded (output 11 + reasoning 22 = 33, documentado en el lector)
regulus trace cost --task probe/NOV-73
# → task probe/NOV-73: 1 call(s), 0 error(s),
#   tokens 4307 in + 33 out (measured-host),
#   cache 1600 read + 0 write (measured-host),
#   local ~8 proxy tokens (chars/4, measured-local) across 1 session(s)
```

Primera vez que un total deja de ser `unknown`: el costo distingue
`measured-host` de `measured-local` y nunca mezcla fuentes.

## Hook de OpenCode: pasa lo que el evento exponga

El SDK instalado (`@opencode/plugin`) no expone uso en
`execute.after` (tool/session/agent/message/id/input/status) —
verificado en `node_modules/@opencode/plugin/dist/effect/tool.d.ts`.
El hook (`plugins/regulus.ts`) envía el uso **solo si el evento lo
trae**, vía `usageFlags()` (`lib/domain-trace.ts`, testeado en
`lib/trace-usage-flags.test.ts`): sin `usage` no agrega flags y el
registro sigue sin estimar. Los flags viajan por el batcher (mapeo
extendido en `TRACE_RECORD_*_FLAGS`), no por spawns sueltos.

## Atribución: run, ejecución, tarea

- `run_id` = id de sesión del host (`session_v2.id`, `sessionId` del
  transcript, `session_id` del rollout); en CLI equivale a `--session`.
- `execution_id` = flag nuevo `trace record --execution-id` (antes no
  existía forma de fijarlo por CLI; `record-batch` ya lo aceptaba por
  ser campo de `trace.Entry`).
- `task` = `--task` (per-task cost attribution, verificado en
  `TestTraceRecordExecutionIDRoundTrip`).

## Re-medición T1/T2/T3 con tokens reales: estado honesto

- **T3 (loop guard): NO MEDIBLE** — 0 incidentes reales; el guard es
  advisory. Sin cambios: se mantiene por opcionalidad, no por ahorro.
- **T1 (compaction acotada) y T2 (toolset minimal): PENDIENTE con
  protocolo** — requieren el harness de evals con repeticiones
  (tareas agentes idénticas con/sin mecanismo, leyendo el uso de la
  sesión runner vía estos lectores). No se fabrica ningún número: la
  tabla en chars/4 de `token-savings.md` (H10, 2026-11-01) sigue
  siendo la referencia medida hasta que NOV-80 ejecute ese protocolo
  a escala.
- Las cifras de este documento son todas de la corrida E2E de arriba
  (1 tarea, 1 runner, 2026-11-15); los rangos y el signo se sostienen,
  los absolutos dependen del contenido.

## F11.5 · Normalización sin perder originales (NOV-91)

- `reasoning_tokens` preserva el desglose original: opencode pliega
  reasoning en output (contabilidad OpenAI) pero guarda el split;
  codex guarda `reasoning_output_tokens` junto a `output_tokens`.
  Los totales del reporte concilian con la fuente primaria campo a
  campo (`ReconcileTotals`: input/output/reasoning/cache exactos o
  el reporte no sale).
- Etiquetas: `measured-host` (el host lo expuso), `measured-local`
  (proxy chars/4), `estimated` (fallback explícito, nunca
  silencioso), `proxy`, `unknown`. `unknown` jamás suma cero:
  `TokensKnown`/`Known` distinguen lo medido de lo no medido y el
  dinero unknown no se tasa.
- Correlación: `HostUsage` lleva `run_id` (= sesión del host =
  `--session`), `execution_id` (run F11.1), `task`, `agent`, `host`,
  `model`, `host_version` + `reason` (motivo verificado de los
  estados declarados). `CorrelateUsage` une un reporte del host con
  su identidad de traza y rechaza el mismatch sesión/run y los
  conteos con procedencia unknown. `DedupEntries` evita el doble
  conteo en re-ingestas.
- Redacción previa al store: `RedactSecrets` enmascara
  `sk-*`/`xox*`/`ghp_*`/AWS/`PRIVATE KEY`/`bearer`/api-keys en todo
  texto libre; del transcript solo salen conteos, modelo e
  identidad — el contenido nunca se registra.

## F11.5 · Coste solo con tarifa explícita (NOV-91)

- `RateCard`: JSON estricto con `model` + `host_version` +
  `effective_date` (YYYY-MM-DD) + `currency` + tarifas por 1K
  (input/output/reasoning/cache r/w, ≥0, ≤1e6). Sin tarifa no hay
  coste: mismatch modelo/versión o versión ausente fallan cerrado.
- Tarifa cero explícita = coste cero conocido (medido gratis) —
  distinto de unknown (sin precio). `CostOfUsage` tasa un reporte;
  `CostMoneyByTask` pliega por (task, model, versión) con una línea
  por tarifa: sin conteos, sin modelo o sin tarifa → unknown con el
  motivo (`no explicit rate for M@V`), nunca cero.
- Ejemplo (tarifa `rates.json`, 2026-11-01, USD): 557 in + 593 out
  + 0 reasoning + 42537/11696 cache →
  `money task probe/NOV-91 (beam-free@2.2.5, measured-host): 0.9522 USD (rate 2026-11-01)`.

## F11.5 · CLI y reportes con procedencia (NOV-91)

- `trace record` acepta `--agent` y `--reasoning-tokens` (además de
  `--task`/`--execution-id`/tokens/modelo/host/versión); auto-etiqueta
  `measured-host` cuando hay conteos o identidad del host.
- `trace cost [--task T] [--agent A] [--run R] [--by task|agent|host] [--rate F]`: los buckets muestran `tokens … (measured-host)` + `reasoning N (…)` + `local ~N proxy tokens (chars/4, measured-local)`; `unknown (host exposes none)` donde el host no midió. `--rate` añade la sección `money` con tarifa citada o unknown con motivo.
- `trace host-status [--json]`: tabla declarada por host (opencode
  measured-host; claude/codex/devin unknown con motivo verificado).
- `trace cost --by bogus` → exit 2 nombrando la regla; rate
  ausente/malformado → exit 1 nombrando el archivo.

## F11.5 · Reconciliación con NOV-73 (sin duplicar trabajo)

Reutilizado tal cual: `ParseOpencodeExport`/`ParseClaudeTranscript`/
`ParseCodexRollout`, `ParseHostUsage`, E2E opencode, hook `usageFlags`,
flag `--execution-id`, E2E real measured-host. F11.5 solo extiende:
re-auditoría 2026-11-16 (formatos + deriva documentada arriba),
`reasoning_tokens`/`estimated`/`task`/`agent`/`reason`, redacción,
tarifas, correlación, `--by`/`--rate`/`host-status` y tests de
límites/overflow/duplicados. Evidencia F11.5 (2026-11-16, sin
llamadas a modelos — re-export read-only + probes locales
revertidas del store):

```bash
opencode --version  # v2.2.5
opencode session export ses_e1a0ebbcbffepMSm0tI22h4wUg  # input 557 … (solo lectura)
regulus trace record --session ses_e1a0… --task probe/NOV-91 --agent worker-a --execution-id run-e2e-1 --input-tokens 557 --output-tokens 593 --reasoning-tokens 0 --cache-read-tokens 42537 --cache-write-tokens 11696 --model beam-free --host opencode --host-version 2.2.5
regulus trace cost --task probe/NOV-91  # 557 in + 593 out (measured-host), cache 42537/11696 — concilia con el export
regulus trace cost --task probe/NOV-91 --rate rates.json  # money … 0.9522 USD (rate 2026-11-01)
regulus trace host-status  # 1 measured-host + 3 unknown con motivo
```
