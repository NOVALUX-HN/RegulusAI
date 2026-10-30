# almanac-g1-g4 — semántica medible + docs alineados

User authorization: "procede" — tras "quedan G1 semántica real (ONNX MiniLM) y G4 doc-drift para otro Chart". Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: miss-rate semántico medido

- Given el embedder hash-ngram-v1 (`internal/almanac/embed.go:19`) y el corpus de eval (`TestDedupThresholdMatchesFixture`)
- When corre la medición de miss-rate en paráfrasis vs typo
- Then hay un reporte con números (paráfrasis recall top-3, lexical-miss rate) que justifica o difiere v2.1 ONNX, sin añadir dependencias a `go.mod`

### Scenario: interfaz embedder pluggable sin romper zero-dep

- Given `EmbedderID` pineado en el header (`internal/almanac/vindex.go`) y `TestZeroDepsGoMod`
- When se introduce la interfaz pluggable (`--embedder`, header `{embedder,dim,version}`, mismatch fuerza reindex)
- Then el hash sigue por defecto, el header registra el encoder, y `go.mod` sigue con cero requires

### Scenario: docs describen el layout real

- Given `docs/memory.md:46-48` declara `almanac/views/<domain>/` y `internal/almanac/almanac.go:1-12` "una substrate"
- When se corrige la doc
- Then `docs/memory.md` describe `.regulus/almanac/observations.jsonl` (truth) + `.regulus/library/<domain>/` (views) + `.regulus-export/` (bundle) + `retrieval.bin` (derivado), sin contradecir el código

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| M1 | S1–S2 | miss-rate fixture + interfaz pluggable header (sin ONNX binario) | @regulus-worker |
| M2 | S3 | docs/memory.md layout real + memory-v2 estado | @regulus-worker |

## Log

- L1: Chart opened 2026-10-30 desde "procede" (G1 `embed.go:1-5` + `memory-v2.md:54-62,158`, G4 `memory.md:46-48` vs `library.go:61-69` + `export.go:62-76`). v2.1 ONNX binario diferido per S8; este chart solo mide y deja la interfaz lista, sin romper zero-dep.
- L2: M1 CLOSED 2026-10-30 — S1 miss-rate medido (`internal/almanac/missrate_test.go`, corpus 5 docs + 4 queries b-side AND): paráfrasis recall top-3 2/2 (sims 0.9603, 0.9503, rank 1), typo recall top-3 2/2 (sims 0.9350, 0.9419, rank 1), lexical-miss 4/4, hybrid top-3 e2e 4/4; gap rewording-real mejor sim 0.2327 (banda unrelated 0.1193–0.2565, bajo floor 0.5) → ONNX sigue diferido, gap documentado en `docs/memory-v2.md` addendum M1. S2 pluggable (`embed.go` `Embedder`+`LookupEmbedder`, `vindex.go` `ReindexWith`/`LoadIndexWith`, `hybrid.go` `HybridSearchWith`, CLI `reindex`/`search --embedder` default hash-ngram-v1, header `{embedder,dim,version}`={hash-ngram-v1,384,1} pineado, mismatch→`ErrHeaderMismatch`+reindex, stdout `embedder:` pinea encoder en receipts; dedup/`relate --suggest` quedan en default sin flag — sin skew). `go.mod` cero requires intacto (`TestZeroDepsGoMod` verde). `bin/regulus gate run almanac-g1-g4/M1` 12/12 pass. Receipts en `.regulus/evidence/almanac-g1-g4__M1/`: `typecheck-1793384058126396100`, `lint-1793384059196914100`, `build-1793384059758421200`, `unit-tests-1793384060410337900`, `coverage-floor-1793384089429072500`, `mutation-1793384090131129500`, `crap-1793384133213668200`, `vulncheck-1793384133592584300`, `spec-1793384136417501300`, `ts-typecheck-1793384136778981300`, `ts-unit-tests-1793384137200138200`, `gofmt-1793384138586055700` (primer intento 11/12: `gofmt` rojo por alineación en `missrate_test.go:54-77`, `gofmt -w`, re-run 12/12). Sin tocar M2.
- L3: M2 CLOSED 2026-10-30 — S3 docs alineados al layout real. `docs/memory.md` §3 reescrito: truth `.regulus/almanac/observations.jsonl` (`almanac.go: File`) + `pins.jsonl` + `relations.jsonl` + derivado `retrieval.bin` (header `{magic,version,embedder,dim,count,sha256}`, `vindex.go: IndexPath`, mismatch→reindex) + views `.regulus/library/<domain>/` (`library.go: Dir`: `INDEX.md`, `lessons/<id>/LESSON.md`, `candidates/`, `harnesses/<task-slug>/tree/` + `EXPERIENCE.md`) + bundle `.regulus-export/almanac.jsonl` + `INDEX.md` (`export.go`); `almanac/views/<domain>/` + `lessons/<id>.md` eliminados. §5: fila search reescrita a híbrido (RRF k=60 + decay + confidence, `--lexical`/`--embedder`/`--limit`/`--any`/`--latest`/`--include-superseded`), `history` a `--topic-key`, `export` con rutas del bundle, filas nuevas `import`, `pin|unpin`, `relate`/`--suggest`, `doctor`, `reindex`; fila F4 `reflect` intacta. `docs/memory-v2.md` §7 + `## M2 verification`: M1 re-chequeado contra código (sims 0.9603/0.9503/0.9350/0.9419 rank 1, gap 0.2327 < floor 0.5, `DedupThreshold` 0.92, `SuggestFloor` 0.5) sin tocar la tabla M1; v2.1 ONNX sigue diferido (solo `hash-ngram-v1`, `go.mod` cero requires). `bin/regulus gate run almanac-g1-g4/M2` 12/12 pass. Receipts en `.regulus/evidence/almanac-g1-g4__M2/`: `typecheck-1793384352761565700`, `lint-1793384353845412100`, `build-1793384354354103900`, `unit-tests-1793384355010115000`, `coverage-floor-1793384384164107900`, `mutation-1793384384854535900`, `crap-1793384430017470900`, `vulncheck-1793384430395280900`, `spec-1793384433266476600`, `ts-typecheck-1793384433628678200`, `ts-unit-tests-1793384434067686900`, `gofmt-1793384435427922000` (primer intento 12/12, sin re-runs). Sin tocar M1 (interfaz `Embedder`/`ReindexWith`/`LookupEmbedder` intacta, `TestZeroDepsGoMod` verde).
