# almanac-elite — endurecer el Almanac punto por punto con prueba

Header: mejoras puntuales sobre los 7 puntos evidenciados en la auditoría del Almanac, cada una con RED/GREEN observado. Branch: feat/almanac-elite.
Read until `## Log` before edits.

## Specs

- S1: "mejora los puntos evidenciados y prueba que las mejoras funcionen"

## Tasks

- [x] T1 (S1) — sha cache: evitar re-hash del log en cada query — route: inline — commit: fd421b9
- [x] T2 (S1) — missrate como gate: pinnear recall paraphrase/typo en bench — route: inline — commit: b76578f
- [x] T3 (S1) — RecordMiss probado: cero-hit registra y MissStats lo muestra — route: inline — commit: 2fff074
- [x] T4 (S1) — check accionable: paths citados vs git_head — route: inline — commit: a9939e1
- [x] T5 (S1) — calibración documentada: RRF/decay/topN pineados con test — route: inline — commit: 45cd5ad
- [ ] T6 (S1) — retención: GC de lessons no usadas sin romper evidencia — route: inline — commit: —
- [ ] T7 (S1) — CLI: status lista + ranking explicado en search — route: inline — commit: —

## Log

- L1: "mejora los puntos evidenciados y prueba que las mejoras funcionen"
