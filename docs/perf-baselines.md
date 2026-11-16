# Perf baselines (NOV-65, automatizado NOV-79)

Fuente de verdad: `bench/perf-baseline.json` (versionado, con entorno,
repeticiones y umbral por benchmark). Generado por comando y comparado
por comando: `bash scripts/perf-check.sh [--report FILE]`; regenerar
solo ante cambios intencionales con `bash scripts/perf-check.sh --update`
y commitear el JSON como cambio propio. El workflow `perf.yml`
(semanal + manual) corre la comparación y publica el reporte como
artefacto (90 días). Regla: regresión sobre el umbral del benchmark
falla (repetir la medición, no editar los números).

Historial: medido 2026-11-14, linux/amd64 (Ryzen 7 5700X), Go 1.27.3.
`go test -run XXX -bench . -benchtime Nx` por paquete.

| Benchmark | N (seed) | ns/op |
|-----------|----------|-------|
| almanac BenchmarkNotes (200 obs) | 100 | 3,258,487 |
| almanac BenchmarkContext (200 obs) | 100 | 15,423,236 |
| almanac BenchmarkHybridSearch (200 obs) | 100 | 13,631,312 |
| trace BenchmarkRecord | 100 | 4,593,213 |
| trace BenchmarkCostByTask (4×50) | 100 | 7,434,709 |
| cli BenchmarkMemorySearchCached (50 obs) | 30 | 186,727,411 |
| cli BenchmarkMemorySearchNoStaleness (50 obs) | 30 | 200,259,661 |

Notas honestas:
- N pequeño y máquina concreta: son órdenes de magnitud, no SLOs.
  Los umbrales se quedan en 2x hasta que 3+ corridas programadas den
  bandas de variabilidad por benchmark; entonces se reemplaza el ratio
  fijo por comparación estadística (benchstat) — ver procedimiento abajo.
- Memory search (NOV-79): el fixture ahora construye un repo git con
historia y fija las observaciones a un head viejo, así la variante
cached ejerce probes rev-list reales (acotados por la caché de vida
de proceso) mientras `--no-staleness` los salta: el par discrimina
en vez de empatar. La caché ahorra spawns frente a probes sin caché
(O(iteraciones) en vez de O(hits × iteraciones)), no frente a saltar
el cómputo: documentar el delta medido, no asumir dirección.
- Los benchmarks no corren por defecto (`go test` solo ejecuta seeds
  de fuzz + tests); el perfil full los incluye vía `-bench`.
- Tamaño de contexto (tokens/bytes de `memory context` y compaction)
  se mide junto a tokens reales (Paso 53, `docs/host-usage.md`): el
  costo de contexto y el de cómputo se reportan por separado.
- Fuzz targets (8): gates allowlist×2, spec GWT, almanac import, MCP
  framing + mcpInt, receipts decode, manifest decode. Corpus semilla
  versionado en cada archivo; `fuzz.yml` corre 60s por target cada
  semana; cada crasher se convierte en regresión permanente (ver
  `internal/gates/testdata/fuzz/` + fix NOV-65 del fail-open en
  quote-only input).

## Procedimiento: actualizar un baseline legítimo vs regresión (NOV-79)

1. Ante un BREACH: repetir con `bash scripts/perf-check.sh` en el
   mismo runner; si persiste, verificar que el cambio de código lo
   explica (más trabajo real, no ruido).
2. Cambio intencional: correr con `--update`, revisar el diff del JSON
   (solo los benchmarks afectados deben moverse), commitear el JSON
   solo, con mensaje que cite el motivo.
3. Regresión: no tocar el JSON; arreglar el código y la comparación
   vuelve a verde. Los números del JSON nunca se editan a mano.
