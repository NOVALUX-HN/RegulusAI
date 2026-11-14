# F7 — Protocolo experimental (NOV-56)

Versión: `f7-protocol-v1` · Fecha: 2026-11-14 · Estado: mínimo viable (NOV-56/57/58)

F7 explora mejoras candidatas sin tocar la línea base y sin filtrar la
respuesta. Este documento es normativo para NOV-58 (motor) y NOV-57
(evidencia). No redefine lo que ya existe: enlaza.

## 1. Entrada canónica: bench NOV-35

- La única entrada de calidad es `bench/baseline.json`, suite
  `macizo-bench-v3` (14 tareas B1–B14), generada por `internal/bench`
  (NOV-35). No se copia la rúbrica ni los checks en propuestas.
- Un reporte candidato solo es comparable si `suite` y `suite_version`
  coinciden con el baseline. `bench.Compare` decide ADOPT/REJECT por
  contraste tarea a tarea; la duración es informativa, nunca veredicto.
- Costo separado de calidad (NOV-39): `proxy_tokens`, `duration_ms` y
  `quality_per_kproxy` se reportan, no se juzgan. Una mejora que paga su
  costo es válida; un ahorro con regresión de calidad es REJECT.

## 2. Splits versionados: exploración vs holdout

- Definición máquina: `bench/f7-splits.json`, split actual
  `f7-split-v1` (versión 1, seed `20261114`):
  - exploración: B1–B10 (cover, split, prose, veracity, gates, security,
    cost, retrieval, gates-real, token-proxy).
  - holdout: B11–B14 (lesson-context, decision, gate-profile, missrate).
- La exploración solo ejecuta IDs de exploración. El holdout nunca se
  ejecuta durante la búsqueda; solo en validación final E2E (NOV-61,
  fuera de este mínimo).
- Versionado: cambiar cualquier ID o la suite exige bump de versión del
  split y re-baseline. El manifest pinnea `split.name`, `split.version`
  y `split.sha256`; un drift de `bench/f7-splits.json` invalida la
  evidencia (`VerifyExperimentManifest`).

## 3. Anti-leakage

- Las propuestas (`ExplorationCandidate`) llevan solo `ID`, `domain`
  (INV/DOM) y `parent_id`. Nunca llevan rúbricas, respuestas, checks,
  prompts del holdout ni contenido del holdout.
- El `ExperimentManifest` (NOV-57) guarda referencias + SHA-256, nunca
  contenido: `candidate.manifest_path`, `config_ref`, `split.file`,
  `receipt_id`s. La recuperación es bajo demanda (`Load`, `Resolve`).
- El evaluador ve exploración; el holdout solo existe como digest hasta
  NOV-61. Una propuesta que nombre un ID del holdout se rechaza.

## 4. Seeds reproducibles

- `RunOptions.Seed != 0` es obligatorio; `ExperimentManifest.seed`
  pinnea el mismo valor. El orden de candidatos es estable (orden de
  entrada) y la probe debe ser determinista en `(seed, candidate, attempt)`.
- Nada depende de wall-clock dentro de la probe. El tiempo solo vive en
  `RunBudget.MaxWallTime` como límite duro, nunca como insumo.

## 5. Taxonomía de resultados y métricas separadas

Estados por tarea/candidato (mismo vocabulario en motor y manifest):

| estado | significado |
|---|---|
| `pass` | criterio cumplido, evidencia verificable |
| `fail` | criterio no cumplido (incluye `probe error:`) |
| `timeout` | la probe agotó su plazo |
| `infra` | fallo de entorno (IO, runner, ledger corrupto) |
| `non-evaluable` | sin criterio aplicable o detalle truncado más allá de lo útil |
| `missing` | no se ejecutó (cancelado, presupuesto agotado, resume pendiente) |

Métricas separadas (nunca mezcladas):

- calidad: tasas sobre `pass`/`fail` (`evolve.Summarize`, `Adoptable`).
- costo: `tokens`, `duration_ms`, `proxy` (NOV-39, informativo).
- infra: conteos de `timeout`/`infra`/`non-evaluable`/`missing`.
- Un `timeout` no es un `fail` de calidad; un `missing` no es un cero.

## 6. Presupuestos duros (NOV-58 `RunBudget`)

`MaxWallTime` (tiempo total), `MaxTokens` (suma de `Tokens`), `MaxCandidates`
(candidatos efectivos), `MaxConcurrency` (paralelismo), `MaxRetries`
(reintentos por candidato), `MaxDiffBytes` (techo de `Detail`, se trunca).
Agotar presupuesto devuelve `ErrExplorationBudget` con resultados
parciales; el timeout devuelve `context.DeadlineExceeded`; la cancelación
`context.Canceled`. Reintento solo en `fail`/`timeout`/`infra`;
`pass`/`non-evaluable`/`missing` son terminales.

## 7. Dominios, linaje, aislamiento

- INV y DOM corren el mismo protocolo, misma probe, mismo presupuesto.
  La separación INV/DOM sigue en `CheckDomain` (S9); F7 no la relaja.
- Secuencial encadena desde el último aceptado: el llamante fija
  `parent_id = <ID aceptado previo>`; el motor preserva `parent_id`
  verbatim y nunca muta el baseline.
- Workspace aislado: `RunOptions.Workspace` (opcional) confina el
  checkpoint `checkpoint.json`. El motor nunca escribe en
  `.regulus/evolution/`, nunca llama `Adopt`/`Advance` a ADOPTED y no
  tiene permiso de promover. Sin subagentes: el fallback es inline
  (goroutines + `ExploreProbe`), apto para App Control/CI.
- Resume idempotente: `RunExplorationResume` reutiliza `done[id]` sin
  re-disparar la probe; el checkpoint en workspace permite retomar tras
  cancel/timeout con el mismo `seed` y mismos candidatos.

## 8. Evidencia (NOV-57 `ExperimentManifest`)

- Esquema `1`, protocolo `f7-protocol-v1`, fichero
  `.regulus/evolution/manifests/<experiment_id>.json`.
- Enlaza: candidate manifest (`manifest_path`+`run_id`+`sha256`),
  `baseline`+`baseline_sha256`, `suite`/`suite_version`,
  `split{file,name,version,sha256}`, `seed`, `config_ref`+`config_sha256`,
  `budget`, `evaluator`+`evaluator_version`+`evaluator_sha256`,
  `results[{task_id,status,receipt_id,detail_sha256}]`,
  `decisions[{verdict,by,at,reason_sha256,receipt_id}]`, `notes` (redactadas).
- Índice compacto `ListManifests` + recuperación bajo demanda
  (`LoadExperimentManifest`, `ResolveReceipt`). Redacción de secretos en
  `notes`/`baseline` antes de persistir. Retención delegada a
  Almanac/receipts (el manifest es puntero, no copia; sin GC propio).

## 9. Lo que F7 no duplica

- NOV-35 bench: no redefine tareas ni goldens; los consume.
- NOV-39 tokens: no redefine presupuestos de contexto; separa costo de calidad.
- NOV-41 adversarial: no redefine el corpus; `adversarialLine` sigue como
  no-regresión en el ciclo (NOV-42).
- NOV-42 ciclo: no toca `baseline`/`evaluate`/`adopt`/`reject`/`rollback`;
  la exploración nunca adopta.
- NOV-34 observabilidad: no redefine `run_id`/`execution_id`; el manifest
  reutiliza `receipt.run_id` cuando existe.
- F6 contratos: no toca `docs/contract-neutral.md` ni la matriz de
  conformidad; el protocolo es neutral al harness.

## 10. Historial de versiones

- `f7-protocol-v1` (2026-11-14): mínimo F7 — bench canónico, split
  exploración/holdout, anti-leakage, seeds, taxonomía, presupuestos,
  manifest por referencia, motor inline. Cubre S8 (NOV-56/57/58).
