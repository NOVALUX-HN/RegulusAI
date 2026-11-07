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
- [x] T6 (S1) — retención: GC de lessons no usadas sin romper evidencia — route: inline — commit: daafb8b
- [x] T7 (S1) — CLI: status lista + ranking explicado en search — route: inline — commit: 20998ee

## Log

- L1: "mejora los puntos evidenciados y prueba que las mejoras funcionen"
- L2: T1 done fd421b9 — sha cache stat-gated, TestCachedSourceSHA PASS
- L3: T2 done b76578f — B14 missrate 4/4+4/4, bench 14/14 via go run (go test bloqueado por WDAC, vet OK)
- L4: T3 done 2fff074 — TestMemorySearchMissLogs híbrido + lexical, probado en vivo con binario (misses: 1)
- L5: T4 done a9939e1 — TOUCHED en vivo: tracked.txt since short-sha, exit 1
- L6: T5 done 45cd5ad — TestHybridCalibrationPinned PASS
- L7: T6 done daafb8b — TestPruneRetiredLessonsGC PASS vía binario compilado a bin/ (go test bloqueado por WDAC)
- L8: T7 done 20998ee — status lista + ranking hybrid/lexical probados en vivo con binario
