# almanac-robustez — G2+G3+G5

User authorization: "Robustez (Recomendado)" — respuesta a "Almanac no está al 100% (5 gaps). ¿Qué autorizas a chartificar?" (G2 All() tolerante + G3 reindex auto + G5 resume completo). Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: substrate tolera 1 línea corrupta

- Given `.regulus/almanac/observations.jsonl` con 1 línea corrupta entre N válidas
- When corre `search`, `context`, `history` o `doctor`
- Then ningún comando cae: `All()` salta la línea con aviso, `doctor` reporta `malformed line(s)`, el resto funciona

### Scenario: índice no va a stale en silencio

- Given un `save` tras el último `reindex`
- When corre `search` o `relate --suggest`
- Then hay reindex automático o aviso que repara sin rehusar en silencio; `relate --suggest` no rehúsa si puede degradar a lexical con aviso

### Scenario: resume trae contexto completo

- Given notas, decisions, discoveries, preferences y lessons guardadas
- When corre `regulus memory context` tras restart frío
- Then el contexto lista más de 5 notes (o paginado explícito), lessons con contenido (no solo conteo), y decisions/discoveries/preferences visibles

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| R1 | S1 | `All()` tolerante + `doctor` coherente (G2) | @regulus-worker |
| R2 | S2 | reindex auto/aviso + `relate` degrada (G3) | @regulus-worker |
| R3 | S3 | `context` completo + identity (G5) | @regulus-worker |

## Log

- L1: Chart opened 2026-10-30 desde auditoría Almanac (G2 `almanac.go:184-185` vs `health.go:28-30`, G3 `hybrid.go:42-53` + `memory.go:335-339`, G5 `context.go:78-84,105-136`). Autorización "Robustez (Recomendado)". G1 semántica real y G4 doc-drift fuera de alcance.
- L2: R1 CLOSED 2026-10-30 — S1 `All()` tolerante + `doctor` coherente (G2). `internal/almanac/almanac.go:All` salta líneas corruptas con aviso `log.Printf` a stderr sin abortar; `Doctor` intacto como fuente de `malformed line(s)`; `Search/Context/History` heredan tolerancia vía `All`. Tests: `TestAllMissingIsEmptyAndMalformedSkipped` + `TestAllTolerantSkipsCorruptBetweenValids` (1 corrupta entre 2 válidas → search/history/context verdes + doctor malformed línea 2). `bin/regulus gate run almanac-robustez/R1` 12/12 pass. Receipts en `.regulus/evidence/almanac-robustez__R1/`: rec_18e35f048cc4beb8 typecheck, rec_18e35f04cbe95d4c lint, rec_18e35f04f51f8c2c build, rec_18e35f051bc17494 unit-tests, rec_18e35f0f6768f660 coverage-floor, rec_18e35f0f91580d94 mutation, rec_18e35f197dae9e5c crap, rec_18e35f1a0a3ca4cc vulncheck, rec_18e35f1ab681e83c spec, rec_18e35f1ad64f79a4 ts-typecheck, rec_18e35f1af0566704 ts-unit-tests, rec_18e35f1b4105d9f0 gofmt. Sin tocar R2/R3.
- L3: R2 CLOSED 2026-10-30 — S2 reindex auto + `relate` no rehúsa (G3). `search` ya avisaba con comando repara (`memory.go:335-339`, probado por `TestMemorySearchStaleWarnsLexicalOnly`) — intacto. `SuggestRelated` auto-repara en vez de rehusar: índice missing/stale/corrupto/header-mismatch se reconstruye una vez vía `Reindex` (función pura del JSONL, byte-determinista, barata, zero-dep) y reporta `rebuilt=true` (patrón `HybridSearch degraded`); la CLI anuncia `note: semantic index was stale or missing — rebuilt automatically` en stderr. Degradar a lexical se descartó: fabricaría cosenos y mentiría (las sugerencias SON el canal vectorial). Id desconocido sigue rehusando antes de reconstruir (bad input nunca escribe); rebuild fallido conserva el puntero reindex, nunca silencio. Tests: `TestSuggestMissingIndexAutoRebuilds` + `TestSuggestStaleIndexAutoRebuilds` (el save post-reindex aparece — lectura stale lo perdería) + `TestSuggestCorruptIndexAutoRebuilds` + `TestSuggestUnknownIDWritesNothing` + CLI `TestMemoryRelateSuggest{Missing,Stale}IndexAutoRebuilds` (exit 0 + aviso + paraphrase). `bin/regulus gate run almanac-robustez/R2` 12/12 pass. Receipts en `.regulus/evidence/almanac-robustez__R2/`: rec_18e35f653e69de68 typecheck, rec_18e35f65890c4e74 lint, rec_18e35f65addde8c0 build, rec_18e35f65f0308a70 unit-tests, rec_18e35f70090e1134 coverage-floor, rec_18e35f7032c411f4 mutation, rec_18e35f7a53912f70 crap, rec_18e35f7ac31055ec vulncheck, rec_18e35f7b6f206d40 spec, rec_18e35f7b8f0be5bc ts-typecheck, rec_18e35f7ba8a06a70 ts-unit-tests, rec_18e35f7c333fdfe4 gofmt. Sin tocar R1/R3 (R1 tolerancia intacta, suite verde).
- L4: R3 CLOSED 2026-10-30 — S3 `context` completo + identity (G5). `Context` delega en `ContextWithLimit(root, now, limit)`: encabezado con `ProjectName` (identity), `Session notes` completa por defecto (las 7 del fixture visibles, no solo 5) + `--limit N` con linea explicita `showing latest N of M`, lessons por dominio con cue + lesson (no solo conteo, con guarda nil), seccion `Decisions / Discoveries / Preferences` cronologica con contenido, `Pinned` intacto. CLI `memory context [--limit N]` (`--limit < 0` rehusa con usage). Tests: `TestContextShowsFullResumeWithIdentity` (7 notes + decision/discovery/preference + lesson cue+contenido + identity) + `TestContextLimitPaginatesNotesExplicitly` (5/7 oculta las 2 viejas + aviso; full sin aviso) + CLI `TestMemoryContextLimitAndIdentity` (full + `--limit 5` + identity). Docs: fila F3 `docs/memory.md` + celda `README.md`. `bin/regulus gate run almanac-robustez/R3` 12/12 pass. Receipts en `.regulus/evidence/almanac-robustez__R3/`: rec_18e35fba121c7cd0 typecheck, rec_18e35fba53d65600 lint, rec_18e35fba7db649d0 build, rec_18e35fbac21a998c unit-tests, rec_18e35fc520279714 coverage-floor, rec_18e35fc549c38b78 mutation, rec_18e35fcf45f2a358 crap, rec_18e35fcfb8b8a25c vulncheck, rec_18e35fd061c00ad4 spec, rec_18e35fd081a8935c ts-typecheck, rec_18e35fd09b9f51c4 ts-unit-tests, rec_18e35fd0ed0216f0 gofmt. Sin tocar R1/R2 (tolerancia, reindex auto y pinned intactos, suite verde).
