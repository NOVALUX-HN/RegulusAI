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
- [ ] T3 (S2,S3) — HEAD cacheado + `--no-staleness`: cache HEAD por proceso, anotación solo con `--staleness` — route: inline — commit: —
- [ ] T4 (S2,S3) — reindex diferido: debounced/async o `REGULUS_DEFER_REINDEX=1`, default lexical en racha de writes — route: inline — commit: —
- [ ] T5 (S2,S3) — `gate run --quick` (layers 1-4+spec+gofmt), full solo pre-close, cache coverage.out por sha — route: inline — commit: —
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
