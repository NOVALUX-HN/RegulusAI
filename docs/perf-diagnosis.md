# Diagnóstico de rendimiento (F11.4 / NOV-90)

Objetivo: identificar qué agente, etapa, herramienta o delegación
causa lentitud, reintentos y trabajo improductivo — con evidencia
enlazada y sin afirmar anomalías sin muestra suficiente.

## Fuentes (consume, no duplica)

| Dato | Origen | Qué aporta |
|------|--------|-----------|
| Eventos versionados | F11.1 `internal/observability/exec.go` (`exec.jsonl`) | tipos, estados, `DurationMS`, delegaciones, conteos de reintentos/timeouts/cancelaciones/fallos |
| Resultados aislados | F11.2 Agent Lab (`LabResult`, `AgentOutcome`) | duración, tokens con procedencia, veredicto, delegaciones observadas |
| Etapas | F11.3 `TierPhases` (`tiers_run.go`) | vocabulario `prep/queue/inference/tools/eval/teardown` reutilizado tal cual |

El diagnóstico nunca crea un store paralelo ni redefine fixtures,
splits o outcomes.

## Regla de N mínimo (metodología, no configurable)

Los cuantiles usan nearest-rank (`rank = ceil(q·n)`, determinista,
sin interpolación) y solo se afirman con N suficiente:

| Cuantil | N mínimo | Con menos muestra |
|---------|----------|-------------------|
| mediana | 5 | `insufficient sample (n=X, need 5)`, sin afirmación |
| p90 | 10 | `insufficient sample (n=X, need 10)`, sin outliers |
| p95 | 20 | `insufficient sample (n=X, need 20)`, sin afirmación |

Los mínimos son constantes del motor (`DiagMinMedianN/P90/P95`),
a propósito fuera de los umbrales configurables: ninguna
configuración puede debilitar la guarda. La detección de outliers
(exceso sobre p90) exige `n >= 10`; con menos, el informe anota la
muestra insuficiente y no reclama ningún outlier.

## Atribución por etapa

Cada evento aporta su `DurationMS` a una etapa según su tipo:

| Tipo(s) F11.1 | Etapa |
|---------------|-------|
| `run.start`, `task.start`, `agent.start`, `context` | `prep` |
| `delegation.request`, `delegation.result`, `retry` | `queue` (espera de delegación + backoff) |
| `tool.call`, `tool.result` | `tools` |
| `gate`, `artifact`, `decision` | `eval` |
| residual `agent.start→agent.end` menos tools del agente | `inference` |

Reglas honestas del modelo:

- Los `run/task/agent.end` terminales **no entran en ninguna etapa**:
  su `DurationMS` es la pared del alcance (el Lab emite la duración
  completa en `run.end`), nunca un costo de etapa.
- `timeout`/`error` son resultados: se cuentan, no se etapifican.
- Tipos desconocidos se anotan, no se atribuyen.
- `teardown` queda en 0: F11.1 no emite teardown (la limpieza del
  workspace no deja traza). El resto de la pared que ninguna etapa
  explica se reporta como `unattributed`, nunca como teardown
  inventado.
- La latencia de una herramienta se lee en `tool.result` (el
  resultado cierra la llamada); `tool.call` cuenta llamadas. Nada
  cuenta doble.
- Pared (`wall`, vano `run.start→run.end` o fallback documentado)
  y agregado (`Σ` duraciones Lab) se reportan siempre juntos. Si el
  agregado supera la pared, el informe lo anota: hay trabajo
  paralelo o solapado; la pared no es una suma.

## Umbrales configurables (sin autoajuste)

```json
{
  "slow_stage_ms": 10000,
  "slow_tool_ms": 2000,
  "loop_repeats": 5,
  "dup_repeats": 3,
  "retry_budget": 3
}
```

`regulus bench diag --thresholds <file>`: JSON estricto (campos
desconocidos fallan), claves omitidas conservan el valor por
defecto, valores inválidos se rechazan. Cada informe repite los
umbrales con los que corrió (`rules: ... (configured, never
auto-tuned)`): el diagnóstico toma los umbrales por valor y los
devuelve verbatim; ningún hallazgo los modifica.

## Detectores

- **Loops**: `loop_repeats` llamadas consecutivas a la misma
  herramienta (ordenadas por pared). Alerta con la racha, primer y
  último `event_id` y total de la herramienta.
- **Duplicación**: `dup_repeats` llamadas idénticas (herramienta +
  argumentos canónicos) no consecutivas. Una clave ya reportada como
  loop no se duplica como duplicación.
- **Herramientas lentas**: costo agregado por herramienta ≥
  `slow_tool_ms` (total, llamadas y máximo en la muestra).
- **Delegaciones sin contribución**: `request` sin `result`
  (pending) o resultado no-`succeeded`. Muestra el borde
  `origen -> destino` con su `delegation_id`.
- **Reintentos/timeouts/cancelaciones/fallos**: conteos evento +
  veredicto Lab. Los reintentos sobre `retry_budget` alertan; los
  hechos (timeout/cancel/fallo) llevan umbral `occurrence (any ...
  is reported)` — toda alerta muestra muestra **y** umbral.
- **Outliers**: latencias `tool.result` sobre el p90 de la corrida,
  solo con `n >= 10`.
- **Datos parciales**: vano sin `run.end` terminal (posible crash),
  pared por fallback (`lab-result`), `malformed/duplicates` del
  lector tolerante y `unattributed` siempre visible. Una corrida
  vacía igual reporta (pared `unknown`, cero alertas), nunca falla
  en silencio.

## Comando

```bash
regulus bench diag --run <id[,id...]> [--baseline <id>] [--evidence-dir <dir>] [--thresholds <file>] [--json]
```

- `--run`: ids de ejecución del log F11.1 (coma-separados).
- `--baseline`: id del control para el delta (debe estar en el
  alcance; si falta, se anota, no se inventa).
- `--evidence-dir`: dir de evidencia Lab (`config.json` +
  `outcomes.json` + `reports.json`): aporta duraciones, tokens y
  veredictos; sin `--run` los ids salen de aquí.
- `--json`: informe máquina.
- Salidas: `0` diagnostica (una corrida lenta no es un fallo del
  comando: diagnosticar no es gatear), `1` dato/recusa, `2` uso.

## Informe reproducible: escenario controlado con baseline

Escenario sintetizado y determinista (fijado en
`TestDiagSlowRunFindsDominantStageAndAgent` y
`TestBenchDiagSlowRunVsBaseline`; duraciones fijas, cero
reloj-de-pared): la tarea corre limpia (baseline `run_fast`,
una lectura de 100 ms) y lenta (`run_slow`: racha de 6 lecturas
de 2000 ms + una herramienta `exec` de 3000 ms + delegación
`dlg_1` sin resultado + 1 reintento).

```bash
regulus bench diag --run run_slow,run_fast --baseline run_fast
```

Claves esperadas del informe (aritmética del fixture):

```text
perf-diagnosis perf-diag-v1: 2 run(s), 6 alert(s)
rules: slow_stage_ms=10000 slow_tool_ms=2000 loop_repeats=5 dup_repeats=3 retry_budget=3 (configured, never auto-tuned)

== run run_slow ==
wall 30000ms (trace-span) vs aggregate 24000ms
  stage prep          150ms
  stage queue         500ms
  stage inference   12200ms
  stage tools       15100ms (63% of aggregate)
  stage eval          200ms
  unattributed (wall remainder) 1850ms
dominant stage: tools (15100ms) [run=run_slow events=27 (regulus trace timeline --run run_slow --rebuild)]
dominant agent: baseline (20000ms) [agent=A1 run=run_slow (regulus trace timeline --run run_slow --rebuild)]
counts: retries=2 timeouts=0 cancels=0 fails=0 errors=0 malformed=0 duplicates=0
trace: regulus trace timeline --run run_slow --rebuild

alerts:
  [slow-stage] tools run=run_slow
    sample: tools: 15100ms (63% of aggregate)
    threshold: slow_stage_ms=10000
  [slow-stage] inference run=run_slow
    sample: inference: 12200ms (51% of aggregate)
    threshold: slow_stage_ms=10000
  [slow-tool] exec run=run_slow
    sample: exec: total 3000ms over 1 call(s), max 3000ms
    threshold: slow_tool_ms=2000
  [slow-tool] read run=run_slow
    sample: read: total 12000ms over 6 call(s), max 2000ms
    threshold: slow_tool_ms=2000
  [loop] read run=run_slow
    sample: 6 consecutive read calls (evt_loop_c1..evt_loop_c6, tool total 12000ms)
    threshold: loop_repeats=5
    evidence: run=run_slow events=evt_loop_c1..evt_loop_c6 (regulus trace timeline --run run_slow --rebuild)
  [delegation] dlg_1 run=run_slow
    sample: dlg_1: A1 -> anchored (pending: request without result)
    threshold: contribution required (terminal succeeded)

baseline run_fast vs run_slow: slower (wall +25000ms +500%, aggregate +19500ms)
```

Lectura: la corrida lenta identifica la etapa `tools` y el agente
`baseline` como dominantes con evidencia enlazada (`run_id`,
`event_id`s y comando de rebuild); la pared (30000 ms) y el
agregado (24000 ms) viajan separados; cada alerta muestra muestra
y umbral; el delta contra el baseline cuantifica la regresión
(+25000 ms de pared). Con `n < 5/10/20` los cuantiles del mismo
informe dirían `insufficient sample` en lugar de afirmar.
El verde lo confirma CI (`Diag*` herméticos, sin toolchain).
```

