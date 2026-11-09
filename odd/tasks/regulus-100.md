# regulus-100 — tercera barrida: Regulus al 100

Header: 10 gaps del tercer scan, cada uno con RED/GREEN observado. Branch: develop (merge a main solo cuando lo pida el dueño; RDD afuera).
Read until `## Log` before edits.

## Specs

- S1: "revisa nuevamente que otros puntos de mejora tenemos"
- S2: "procede papi. hay que dejar Regulus al 100"
- S3: Reglas por item: H1/H2 con test de error exacto; H3 sin tocar el mapeo exit-1→200 pineado; H4 solo en main (server-level, no por-endpoint); H5 medir en fixture 300 receipts antes de dimensionar, sin bajar frecuencia de poll; H6 sin sacar conteos; H7 solo skills_list/evolve_tick/evolve_propose (probe_* afuera a propósito); H8 sin romper hermeticidad (shape file, no regulus.json real); H9 refusal nombra el path ancestro, sin auto-chdir; H10 golden de rutas + point-in-time a audit/superseded. Cada cambio con test; suite verde al cierre; commits work-unit en develop.

## Tasks

- [x] H1 (S2,S3) — serveMCP: sc.Err al salir + techo 1MB documentado — route: inline — commit: a801bc3
- [x] H2 (S2,S3) — mcpInt: rehusar floats no enteros — route: inline — commit: 39f3388
- [ ] H3 (S2,S3) — HTTP 500 en store-errors (6 sitios, envelope igual) — route: inline — commit: —
- [ ] H4 (S2,S3) — serve timeouts + graceful shutdown en main — route: inline — commit: —
- [ ] H5 (S2,S3) — dashboard cache headers/memo (medir primero) — route: worker — commit: —
- [ ] H6 (S2,S3) — traces sin N+1 (glob o cap) — route: worker — commit: —
- [ ] H7 (S2,S3) — portar skills_list/evolve_tick/evolve_propose a MCP — route: worker — commit: —
- [ ] H8 (S2,S3) — bench gauntlet-shape.json + B5 con 6 quick — route: worker — commit: —
- [ ] H9 (S2,S3) — init: refusal con ancestro + --force + validación — route: worker — commit: —
- [ ] H10 (S2,S3) — golden de rutas + point-in-time a audit — route: inline — commit: —

## Log

- L1: "revisa nuevamente que otros puntos de mejora tenemos"
- L2: "procede papi. hay que dejar Regulus al 100"
- L5: H2 done — RED 3 casos tabla (2.9/-1.5/3.0) -> GREEN con math.Trunc; strings enteros e ints intactos; suite verde, vet/fmt limpios.
- L4: H1 done — RED build-fail (serveMCP void) -> GREEN; sc.Err sale exit 1 + stderr, techo en usage; suite verde, vet/fmt limpios.
- L3: scan mw8q46dm-h-gz93 + spot-checks del parent (serveMCP loop, mcpInt, ListenAndServe pelado, init cwd, 200-vs-400) — 10 items verificados como reales antes de implementar.
