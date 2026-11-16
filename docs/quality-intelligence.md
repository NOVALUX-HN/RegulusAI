# Inteligencia continua de calidad (F12.4 / NOV-99)

Detectar **degradación de calidad, rendimiento, coste y
compatibilidad** a partir de ejecuciones reales: baselines por
agente/modelo/host/tarea/versión, ingesta de
results/traces/gates/evals con procedencia, detección de cambios en
éxito, retries, latencia, tokens, errores y cobertura con
umbrales/métodos adecuados al N, segmentación anti falsos positivos,
diagnóstico con casos/evidencia, y reproducción desde evidencia
versionada.

> El módulo **propone investigación, nunca actúa**: no hay path
> `--apply`/`--fix`, ninguna función muta producción, y los
> `next:` del diagnóstico lo dicen explícito.

## Reutilización (sin duplicar)

| Necesidad | Fuente reutilizada |
|---|---|
| Traza versionada de ejecuciones | Log global F11.1 (`observability.LoadExec` / `QueryExec`, `.regulus/observability/exec.jsonl`) |
| Evals con veredicto y tiempos | Resultados Lab F11.2 (`bench.ReadLabEvidence`: `LabResult` con `Verdict`, `DurationMS`, `Retries`, `Errors`, tokens con procedencia) |
| Manifiestos + sha y reportes | Forma F11.3 (`WriteTierEvidence`/`ReadTierEvidence`): `baseline.json` + `manifest.json` con sha256, fallo cerrado ante edición |
| Cuantiles y N mínimo | Disciplina F11.4 (`bench`): mediana necesita 5, p90 10, p95 20; bajo el mínimo se describe, no se afirma |
| Tokens con procedencia | Vocabulario F11.5 (`trace.SourceMeasuredHost/...`): desconocido nunca es cero; sin medición la regla de coste no corre |
| Contexto con fuentes | F12.3 no se consume aquí; la ventana JSON acepta observaciones con `evidence` libre para citarlo |

CodeGraph **no** se usa: reservado a F12.8 con benchmark.

## Comandos

```sh
# Congelar una baseline versionada sobre la ventana (exit 0 guarda,
# 1 rehúsa —sin observaciones en el segmento, entrada mala—, 2 uso)
regulus quality baseline --window <ventana.json> [--lab-dir <dir>] \
  [--agent A] [--model M] [--host H] [--task T] [--version V] \
  [--thresholds <t.json>] --out <dir> [--json]

# Comparar baseline contra ventana: diagnóstico, nunca gate
# (exit 0 diagnostica —haya o no alertas—, 1 rehúsa, 2 uso)
regulus quality check --baseline <dir> --window <ventana.json> \
  [--thresholds <t.json>] [--out <dir-diag>] [--json]

# Ver solo las alertas de un diagnóstico ya guardado
regulus quality alerts --dir <dir-diag> [--json]
```

`--lab-dir` funde resultados Lab verificados (sha del manifiesto)
con la ventana, etiquetando cada observación (`exec|lab|gate|eval|
window`) con su run y su evidencia. `check --out` guarda el
diagnóstico renderizado **verbatim** (redacción previa al store).
`check` sale 0 aunque haya regresiones —diagnosticar no es
gatear (precedente `bench diag`)—; `alerts` re-imprime lo guardado
sin recalcular.

## Cómo detecta y por qué

1. **Segmento**: la baseline fija `agente/modelo/host/tarea/versión`
   (vacío = `(any)`). `check` solo compara observaciones del mismo
   segmento; el resto se excluye y **se cuenta** (`window n=N
   matched + M excluded`). Ventana mezclada ⇒ limitación + mezcla
   por host/modelo para exponer el confound.
2. **N adecuado al método**: con `n<5` no hay reglas —solo
   descripción y `insufficient sample (n=X, need >=5)`—. Tasas
   (éxito, errores), media de retries y mediana corren con `n>=5`;
   p90 exige `n>=10`; p95 exige `n>=20` (mínimos F11.4).
3. **Reglas** (defectos en `DefaultThresholds`, eco verbatim en
   baseline y diagnóstico): éxito cae ≥0.10 absoluto; errores
   suben ≥0.10; retries medios suben ≥1.0; mediana/p90/p95 de
   latencia suben ≥20%; tokens medidos suben ≥25%; cobertura cae
   ≥0.05 puntos. `--thresholds` acepta JSON estricto (campos
   desconocidos fallan; lo omitido conserva el defecto).
4. **Dimensiones honestas**: tokens o cobertura desconocidos en un
   lado **saltan su regla** (limitación explícita), nunca leen
   como cero. Mediana con baseline cero no afirma cambio relativo.
5. **Alerta** = baseline, ventana, N, magnitud (`a -> b (Δ, %)`),
   casos (hasta 5 run ids peores), evidencia por caso, límite y
   paso de investigación. Sin casos no hay alerta.
6. **Anti falsos positivos**: si la lectura ingenua (sin
   segmentar) afirmaría lo contrario que la segmentada, el
   diagnóstico lo declara (`segmentation reversed the naive read`).
7. **Reproducción**: cada diagnóstico trae `reproduce: regulus
   quality check --baseline <dir> --window <file>`; el manifiesto
   del diagnóstico fija los shas de baseline y ventana, y el de la
   baseline el suyo —mismos bytes, mismo veredicto; lo editado
   falla cerrado.

## Límites

- Misma entrada ⇒ mismos bytes (salvo el `reproduce:` con rutas,
  que es metadato de invocación).
- `--thresholds` nunca debilita los mínimos de N: son constantes
  de metodología, no configuración.
- Una baseline vacía en el segmento es un error, no una baseline.
- El pin de dispatch (`run_split_test.go`) lo actualiza el parent:
  `quality` se registra por `init()` sin tocar `cli.go`.
