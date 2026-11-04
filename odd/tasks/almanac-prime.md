# almanac-prime — Almanac mejor que Engram, con lógica Regulus

Engram opina (vectores + juicios de modelo). Almanac verifica
(receipts + provenance): ese es el filo. Esta feature lo convierte en
conducta — ranking por soporte, contradicciones visibles, miss-rate
productizado — más el portability pendiente de pro-harness.

## Specs

- S1: "Almanac es nuestro engram en este proyecto. Mejorarlo para hacerlo mejor que engram de forma lógica y adaptado a Regulus"
- S2: "implementa las mejoras pendientes" (portability WSL-vs-GitBash registrado en pro-harness/L3; propuestas de retiro vía tick, nunca admisión directa)
- S3: Cada cambio con test nuevo o modificado (RED→GREEN); suite verde al cierre de cada unidad; commits work-unit en feature branch; lo no verificado se reporta, no se afirma
- S4: Engram mirror no disponible (provider caído) — este documento es la continuidad

## Tasks

- A1 (S1,S3): support-status en retrieval — `SupportOf` en almanac + anotación en `memory search` (rot hunde con motivo) — ruta: inline — commit: 4dfda3c
- A2 (S1,S3): contradicciones visibles — notas de conflicto en `memory context` — ruta: inline — commit: 42b1b20
- A3 (S1,S3): miss log + miss-rate en doctor — ruta: inline — commit: 43095db
- A4 (S2,S3): portability Git-Bash (bashPosixPath detecta /c/) — ruta: inline — commit: A4-pendiente-hash

## Log

- L1 (origen, verbatim): "recuerda que Almanac es nuestro engram en este proyecto. quiero que también mejoremos almanac para hacerlo mejor que engram pero de forma lógica y que se adapte a Regulus. también implementa las mejoras pendientes"
- L2: Rama feat/almanac-prime desde el tip de feat/pro-harness (T0–T8 adentro). Engram leído (command reference: TUI, projects-consolidate, sync, cloud) + MCP tools conocidos (juicios con confianza, review, dedup, pins, timeline). Decisión: no clonar TUI/cloud (fuera de la lógica Regulus); superar por evidencia.
- L3 (A1 evidencia): `almanac.SupportOf` (supported/rot/unchecked, fail-open salvo fallo probado) + `memory search` anota cada hit y hunde rot estable. Fixture con sha256 real (el fallo honesto del hash inventado validó el verificador). CLI+almanac verdes.
- L4 (A2 evidencia): `writeContextConflicts` — pares conflicts_with con ambos lados listados rinden una línea con títulos; unilaterales en silencio; fallo de lectura enmudece la sección, nunca el bloque. Search ya lo tenía (descubierto, no duplicado). Almanac verde.
- L5 (A3 evidencia): miss log (`misses.jsonl`, best-effort) + `MissStats` + línea en doctor (conteo + top). Cero-hits que antes se encogían de hombros ahora enseñan qué guardar. CLI+almanac verdes (A3 commit 43095db).
- L6 (A4 evidencia): dialecto del bash en uso (`--version`: msys/mingw/cygwin→/c/, linux→/mnt/c) + fallback wrapper en vez de copiar binarios (msys sin su DLL no arranca). Paquete portability entero verde incl. los 2 preexistentes.
