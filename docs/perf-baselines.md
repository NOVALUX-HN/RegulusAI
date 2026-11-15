# Perf baselines (NOV-65)

Medido 2026-11-14, linux/amd64 (Ryzen 7 5700X), Go 1.27.3.
`go test -run XXX -bench . -benchtime Nx` por paquete.
Regla: regresión >2x frente a esta tabla falla la review
(repetir la medición, no editar los números).

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
- Memory search domina por spawns git de staleness (caché vs
  `--no-staleness` difieren poco aquí porque el fixture temporal no
  tiene remotos; en repos reales con behind-counts la brecha crece).
- Los benchmarks no corren por defecto (`go test` solo ejecuta seeds
  de fuzz + tests); el perfil full los incluye vía `-bench`.
- Fuzz targets (8): gates allowlist×2, spec GWT, almanac import, MCP
  framing + mcpInt, receipts decode, manifest decode. Corpus semilla
  versionado en cada archivo; `fuzz.yml` corre 60s por target cada
  semana; cada crasher se convierte en regresión permanente (ver
  `internal/gates/testdata/fuzz/` + fix NOV-65 del fail-open en
  quote-only input).
