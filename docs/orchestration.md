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
