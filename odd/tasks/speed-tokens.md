# speed-tokens — harness más rápido y con menos tokens sin perder enfoque

Header: ejecutar el plan 1-por-1 con verificación real de que cada punto funcionó. Branch: develop (no subir a main hasta cerrar todo).
Read until `## Log` before edits.

## Specs

- S1: "esta bien que siga adelante, hasta no terminar los cambios no subiremos todo a main."
- S2: "ayudame a verificar que tenemos en regulus, que puntos de mejora encuentras por hacer para que el harness funcione de forma mas rapida sin perder su enfoque y que consuma menos tokens"
- S3: "ejecuta el plan uno por uno y verifica que efectivamente cada punto si haya funcionado"

## Tasks

- [x] T1 (S2,S3) — context single-spawn: un solo `memory context --resolve-domain` en Go (files+domains+lessons en 1 spawn) — route: inline — commit: 8d1234a
- [x] T2 (S2,S3) — compaction sin bloqueo: `tick+note` a background / `--check-only` por mtime — route: inline — commit: 617e3d1
- [x] T3 (S2,S3) — HEAD cacheado + `--no-staleness`: cache HEAD por proceso, anotación solo con `--staleness` — route: delegated (worker mw8b13bv-2-k1jc, parent verified) — commit: f4fd943
- [x] T4 (S2,S3) — reindex diferido: debounced/async o `REGULUS_DEFER_REINDEX=1`, default lexical en racha de writes — route: delegated (worker mw8b4nr8-3-72fr, parent verified) — commit: 6785515
- [x] T5 (S2,S3) — `gate run --quick` (layers 1-4+spec+gofmt), full solo pre-close, cache coverage.out por sha — route: delegated (worker mw8bvnkk-4-5wve, parent verified) — commit: aaa4209
- [ ] T6 (S2,S3) — toolset lazy + tabla única: `TE_TOOLSET=auto` lazy, generar MCP+TS desde una tabla, sacar search-default a skill — route: inline — commit: —
- [ ] T7 (S2,S3) — `trace record-batch --ndjson` (1 spawn por flush con duration/output-bytes) — route: inline — commit: —
- [ ] T8 (S2,S3) — fallback barato: si domains>1 sin pin, inyectar 1 lesson + hint `memory_search --domain` — route: inline — commit: —
- [ ] T9 (S2,S3) — minimal recortado: ocultar `evolve_*/probe_*/trace_*/audit` en minimal, Crew a 1 línea — route: inline — commit: —
- [ ] T10 (S2,S3) — bench como gate de presupuesto: B10+B11 pinean guidance/context bytes, `CostByTask` en `evolve_report` — route: inline — commit: —

## Log

- L1: "esta bien que siga adelante, hasta no terminar los cambios no subiremos todo a main."
- L2: "ayudame a verificar que tenemos en regulus, que puntos de mejora encuentras por hacer para que el harness funcione de forma mas rapida sin perder su enfoque y que consuma menos tokens"
- L3: "ejecuta el plan uno por uno y verifica que efectivamente cada punto si haya funcionado"
- L4: explorer handoff mw8arrgn-1-6fd5 con 10 puntos y evidencia path:line (context multi-spawn, compaction triple-spawn, N gits por search, reindex sincrónico, gauntlet sin quick, superficie 19vs14, trace flush, fallback frágil, guardians siempre, bench como gate).
- L5: T1 done 8d1234a — Go 4/4 (chart-pin, single-library, file-hit, unfiltered), bun domain 82/82, lib 149/149, tsc clean, live probe `domain: go (reason: chart:charts/app.md)` en 1 spawn. Slice note: develop lleva 8858 authored lines en 60 commits vs origin/main (pre-existing); T1 slice propio 282 líneas.
- L6: T2 done 617e3d1 — tick+note concurrentes vía Promise.allSettled tras el resume (wall max, no suma); bun domain 83/83, tsc clean, go cli verde.
- L7: T3 done f4fd943 — cache de staleness en proceso (N hits 1 probe, negativos pegados, HEAD dir-keyed tras regresión cazada por suite), --no-staleness en search/context, default anotado intacto; --staleness omitido a propósito (default ya anota, sería no-op). Worker RED->GREEN + parent re-verificó: focused 3/3, cli suite ok, vet/fmt limpios, docs/memory.md al día.
- L8: T4 done 6785515 — REGULUS_DEFER_REINDEX=1 saltea Reindex sincrónico (search degrada a lexical con warning honesto, repara con reindex explícito); default intacto. Worker RED->GREEN + parent re-verificó: burst test, cli+almanac verdes, vet/fmt limpios. Nota: el scratch del worker (/tmp/gpETl75d) era el TMPDIR de la sesión — recreado vacío tras limpiar.
- L9: T5 done aaa4209 — --quick (6 gates lean por RunTask con config angostada, excluyente con --profile) + cache coverage.out por sha256(config+HEAD) con receipt honesto; --profile intacto. Worker RED->GREEN + parent re-verificó: focused 4/4, gates+cli verdes, vet/fmt limpios, dry-run vivo quick=6/full=12.
