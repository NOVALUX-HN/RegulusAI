# Tokens reales por host — medición y atribución (NOV-73)

Documento vivo (no H10): el estado de hoy de cada host, cómo se mide
y con qué evidencia. Traslada aquí la sección viva que estaba al final
de `docs/token-savings.md` (ese archivo queda como registro H10
congelado con un puntero a este documento).

## Estado verificado por host (2026-11-15, esta máquina)

| Host | Fuente | Versión verificada | Estado |
|------|--------|-------------------|--------|
| opencode | `opencode session export <id>` → `info.tokens.{input,output,reasoning,cache.{read,write}}` + `info.model{id,providerID}` | opencode v2.2.5, 303 sesiones en `~/.local/share/opencode/opencode.db` (`session_v2`) | **measured-host** (prueba E2E abajo) |
| claude | `~/.claude/projects/*/*.jsonl`: `message.usage` (`input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, `model`) | claude v2.1.325; formato verificado en 1 transcript real; `~/.claude/sessions/` vacío | **unknown** (formato verificado, sin sesiones vivas que atribuir) |
| codex | `~/.codex/sessions/*/*/*/rollout-*.jsonl`: líneas `token_usage_record` (`input_tokens`, `cached_input_tokens`, `cache_write_input_tokens`, `output_tokens`, `reasoning_output_tokens`) | formato verificado en 1 rollout real (model `gpt-6-luna`); binario no instalado en esta máquina | **unknown** (formato verificado + reader listo, sin tarea ejecutada que atribuir) |
| devin | MCP + skills, sin contadores locales | sin cambios desde F8 | **unknown** (`host exposes none`: MCP no expone conteos) |

Lectores (`internal/trace/hostreaders.go`, solo lectura, fallan
cerrado ante deriva de esquema): `ParseOpencodeExport`,
`ParseClaudeTranscript`, `ParseCodexRollout` — todos validados por
`ParseHostUsage` (64KB, sin negativos, cotas anti-absurdo 1e12,
single-line ≤512, source del enum). Fixtures: transcripts reales
anonimizados + malformados + hostiles (`hostreaders_test.go`).

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
