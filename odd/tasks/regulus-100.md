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
- [x] H3 (S2,S3) — HTTP 500 en store-errors (9 sitios, envelope igual) — route: inline — commit: f706c39
- [x] H4 (S2,S3) — serve timeouts + graceful shutdown en main — route: inline — commit: ce855c5
- [ ] H5 (S2,S3) — dashboard cache headers/memo (medir primero) — route: worker — commit: —
- [x] H6 (S2,S3) — traces sin N+1 (glob o cap) — route: worker mw8qiduq-j-p49p — commit: 9a83063
- [ ] H7 (S2,S3) — portar skills_list/evolve_tick/evolve_propose a MCP — route: worker — commit: —
- [ ] H8 (S2,S3) — bench gauntlet-shape.json + B5 con 6 quick — route: worker — commit: —
- [ ] H9 (S2,S3) — init: refusal con ancestro + --force + validación — route: worker — commit: —
- [x] H10 (S2,S3) — golden de rutas + point-in-time a audit — route: inline — commit: e41696b

## Log

- L1: "revisa nuevamente que otros puntos de mejora tenemos"
- L2: "procede papi. hay que dejar Regulus al 100"
- L9: H6 done — RED N-List -> GREEN streaming counter (misma semántica error/task, path detalle intacto); suite httpapi verde, vet/fmt limpios, diff revisado.
- L8: H10 done — RED golden (docs sin /v1/evolution) -> GREEN; línea evolution en Rutas + 3 banners (paths estables por refs vivas en logs/charts, no moves); suite httpapi verde, vet/fmt limpios.
- L7: H4 done — RED firma vieja -> GREEN; server con ReadHeader 5s + Write 30s, SIGINT/SIGTERM graceful (vivo: 200 + exit 0); suite cmd verde, vet/fmt limpios.
- L6: H3 done — RED 200-en-error -> GREEN 500 (9 sitios collections+dashboard, envelope intacto, mapeo exit-1->200 intacto); pin viejo actualizado al contrato; suite httpapi verde, vet/fmt limpios.
- L5: H2 done — RED 3 casos tabla (2.9/-1.5/3.0) -> GREEN con math.Trunc; strings enteros e ints intactos; suite verde, vet/fmt limpios.
- L4: H1 done — RED build-fail (serveMCP void) -> GREEN; sc.Err sale exit 1 + stderr, techo en usage; suite verde, vet/fmt limpios.
- L3: scan mw8q46dm-h-gz93 + spot-checks del parent (serveMCP loop, mcpInt, ListenAndServe pelado, init cwd, 200-vs-400) — 10 items verificados como reales antes de implementar.
