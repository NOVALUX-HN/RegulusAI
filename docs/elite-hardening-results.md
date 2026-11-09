# Elite-hardening — tabla comparativa probada (T6)

Fecha: 2026-11-06 · Branch: `feat/elite-hardening` · Superficie: este repo (`tree`).
Cómo reproducir cada cifra: comandos en `Verificación`. Todo lo que sigue fue observado, no estimado.

## Antes → Después

| Área | Antes | Después | Mejora | Verificación |
|------|-------|---------|--------|--------------|
| Bench | 11/11 PASS, 67ms wall, 571B detail | 13/13 PASS, 49ms wall, 675B detail | +2 tasks de decisión (B12/B13), -18ms wall | `go run ./cmd/regulus bench run` |
| Gate profiles | quick=12, full=12 (quick inútil, todo required) | quick=10, full=12 (mutation+vulncheck full-only) | quick -2 gates (~-17% gates en iteración rápida) | `gate run --dry-run --profile quick` → 10, sin profile → 12 |
| Mutación | `--workers 1`, timeout 600s, required (siempre corre) | `--workers 4`, required:false + `profiles:[full]` | ~4x paralelismo en mutación + fuera del quick | `regulus.json: mutation` + `SelectGates` |
| CRAP max | 29.91 (`envfacts/admit.go:41 loadLiveCache`, cx13 cov53.6%) | 27.45 (`cli/bench.go:146 benchBaselineCmd`), envfacts fuera del top-5 | -2.46 (-8.2%), margen bajo el umbral 30 | `go test ./... -coverprofile` + `regulus crap --max 30 --top 5` |
| CRAP envfacts | `loadLiveCache` 53.6%, monolito cx13 | `loadLiveCache` 81.8%, helpers 100% (index/apply/load/parse), pkg 85.7% | +28.2pp en función crítica, +4.7pp en paquete | `go test ./internal/envfacts -coverprofile` |
| Cobertura total | 85.3% | 85.2% | -0.1pp (ruido por +294 líneas nuevas con tests) | `go tool cover -func` |
| Evidencia aislamiento | fallback docker→host marcado `host-contained` (ambiguo) | `host-contained:degraded-docker-unavailable` + nota `ran host-contained:degraded` | fallback ya no pasa como limpio | `TestDockerFallbackNote` + 4 `TestRunWithStatsDocker*` PASS |
| Decisiones | sin scoring, `confidence` string libre, bench sin decisión | `Score` puro 0-100 + grado A/B/C/D/F cableado a `Pipeline`, B12 golden 94/A·10/F·75/B, B13 quick=10 vs full=12 | decisión medible y pineada | `go test ./internal/decisions`, `./internal/bench -run TestB12\|TestB13` |
| Tokens T1 | bounded 16608 vs full 55711 (-70.2%) | idéntico 16608 vs 55711 (-70.2%) | se mantiene el ahorro, sin regresión | `memory context --limit 20 --budget 20` vs `--limit 0 --budget 0` |
| Tokens T2/T3 | full 16 defs / minimal 3 defs, loopguard ES, fallback dominio silencioso | mismo subset + nudge EN + warning `no pin found (N domains...) — using unfiltered` | i18n unificado, fallback explícito, 0 regresión | `bun test domain+loopguard` 105 pass, full lib 148 pass 2.16s |
| TS suite | 148 pass 3.03s | 148 pass 2.16s | mismo verde, -0.87s wall (varianza de máquina, no claim) | `bun test ./.opencode/lib/` |
| Trace | 7796 calls, 86 errors, 229 sesiones, host unknown | idéntico (histórico unattributed por diseño) | sin regresión; CostByTask nuevo sí atribuye | `trace cost` |

## Lo que NO cambió (honesto)

- `go test ./...` completo en Windows paralelo falla por Device Guard (`probe.test.exe` bloqueado) y por pines viejos ya corregidos en `cli/bench_test.go` (11→13). Aislados, `journey` 28s PASS, `runtime/sextant/surface/portability` PASS. No se vende como verde total en este runner.
- Cobertura -0.1pp: el código nuevo trae sus tests (helpers 100%, Score goldens, B12/B13), pero el denominador creció. No es regresión de calidad.
- CRAP 27.45 sigue cerca de 30: el próximo paydown es `cli/bench.go:146 benchBaselineCmd` (cx10 cov44.1%).

## Commits (slices)

- T1 `63cec17` perf(gates): quick 10, mutation workers 4 full-only
- T2 `46919c0` refactor(envfacts): split loadLiveCache, helpers 100%
- T3 `cdcca88` fix(gates): degraded isolation explícito
- T4 `04b5fb0` feat(decisions): Score + B12/B13
- T5 `1723ebb` fix(i18n): loop-guard EN + domain fallback warning
- T6 (este doc) + pines cli 11→13
