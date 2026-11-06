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
- A4 (S2,S3): portability Git-Bash (bashPosixPath detecta /c/) — ruta: inline — commit: f3cafba
- A5 (dashboard): endpoint GET /v1/evolution (admitted/proxy/rot/pending/sessions/tick_at, solo conteos) + TickPosition exportado — ruta: inline — commit: 3f2a992

## Log

- L1 (origen, verbatim): "recuerda que Almanac es nuestro engram en este proyecto. quiero que también mejoremos almanac para hacerlo mejor que engram pero de forma lógica y que se adapte a Regulus. también implementa las mejoras pendientes"
- L2: Rama feat/almanac-prime desde el tip de feat/pro-harness (T0–T8 adentro). Engram leído (command reference: TUI, projects-consolidate, sync, cloud) + MCP tools conocidos (juicios con confianza, review, dedup, pins, timeline). Decisión: no clonar TUI/cloud (fuera de la lógica Regulus); superar por evidencia.
- L3 (A1 evidencia): `almanac.SupportOf` (supported/rot/unchecked, fail-open salvo fallo probado) + `memory search` anota cada hit y hunde rot estable. Fixture con sha256 real (el fallo honesto del hash inventado validó el verificador). CLI+almanac verdes.
- L4 (A2 evidencia): `writeContextConflicts` — pares conflicts_with con ambos lados listados rinden una línea con títulos; unilaterales en silencio; fallo de lectura enmudece la sección, nunca el bloque. Search ya lo tenía (descubierto, no duplicado). Almanac verde.
- L5 (A3 evidencia): miss log (`misses.jsonl`, best-effort) + `MissStats` + línea en doctor (conteo + top). Cero-hits que antes se encogían de hombros ahora enseñan qué guardar. CLI+almanac verdes (A3 commit 43095db).
- L6 (A4 evidencia): dialecto del bash en uso (`--version`: msys/mingw/cygwin→/c/, linux→/mnt/c) + fallback wrapper en vez de copiar binarios (msys sin su DLL no arranca). Paquete portability entero verde incl. los 2 preexistentes.
- L7 (Sextant-as-Engram): hook de compaction anexa exactamente una nota `[auto]` (conteo + ISO) best-effort + línea en SKILL LEARN. Verificado con harness aislado fuera del repo (1 bloque + 1 nota exacta); 5 notas basura de mis probes rechazadas por gobernanza (`memory reject`). Bun 148/148; tsc limpio.
- L8 (CitedRot preciso): solo lo CITADO pudre (tarea con mugre ajena ya no acusa); cuarentena cuenta como fallo incluso con Verify limpio (bug cazado por dogfood: early return la salteaba). Validate real: 2 drop genuinos + 1 exonerado que la versión gruesa acusaba mal.
- L9 (coverpkg, REVERSADO con evidencia): `-coverpkg=./...` multi-paquete DUPLICA bloques en un solo profile (265k líneas, triplicados) y el lector toma ceros ajenos — el 0% de SaveReport era artefacto del merge, no deuda. Revierto a same-package (los tests unitarios directos ya cubren la API: 55.6%/100%/96.6%) + snapshot roundtrip como pin. Doctrina: cada unidad merece su propio test; lo cruzado no se mide, se testea directo.
- L10 (cierre pendientes): gates U2+V1 12/12 PASS con evidencia fresca; validate 2 keep/2 drop firmes + packs listos; candidatos ruido descartados (reversibles); loops 6/227 sin juez; snapshots + compare verificados; suite final solo cae por bloques WDAC a binarios (todo lo que ejecuta pasa).
- L9 (coverpkg): `-coverprofile` sin `-coverpkg` solo instrumenta el paquete bajo test — el 0% de SaveReport era artefacto, no deuda (con coverpkg: 55.6%/71.4%, CRAP ~13 < 30). Gate unit-tests + scaffold init + agents actualizados; test roundtrip de snapshots.
- L7 (cierre): oleada WDAC intermitente toda la noche (Temp y .gotmp, binarios de test recién compilados; todo lo que ejecuta pasa en re-run). Diagnóstico de yolo extendido a los pasos de run (edición solo-test en yolo-permissions.test.ts, SIN commitear — respeta L2 de pro-harness: el archivo previo sigue intacto en git). Suite final: 30 pkgs ok; bun 147/147 con un flake; tsc limpio.
- L8 (P6 paridad): plugin 16→19 tools (audit, evolve_report, evolve_tick con dry-run default true); MCP 12→14 (solo read-only: audit + evolve_report; evolve_tick se une al gap deliberado con evolve_propose). Budget GUIDANCE_CAP 1500→1760 (decisión consciente). Pines actualizados deliberadamente (registry, dispatch, MCP gap, devin.md). Bun 147/147; tsc limpio; cli verde.
- L9 (P7 validate): `evolve validate [--domain] [--lesson]` — veredictos keep/drop/unreviewable desde evidencia (CitedRot unificado: solo lo citado pudre; cuarentena cuenta como fallo). Costo reportado, nunca juzga. fixtures corregidos a la semántica precisa.
