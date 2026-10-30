# Almanac v2 — semantic retrieval for the Almanac (investigation + spec)

Status: implemented — v2.0 spike landed 2026-10-29 (T1–T4, see §7) ·
Origin: design candidate (2026-10-28, chart `self-audit-fixes.md` S10) ·
Author: lead session, from live upstream research · v2.1 ONNX stays
deferred per S8.

## 1. Where Engram is today (verified 2026-10-28)

Engram (`Gentleman-Programming/engram`, 7.6k★) is a Go binary over
**SQLite + FTS5** (`~/.engram/engram.db`, lexical full-text), exposed
through CLI, MCP, HTTP API and TUI. Gentle-AI only installs and injects
it (`internal/components/engram/` is installer plumbing; the store is
upstream). Upstream adds relevance ranking (`candidate_relevance.go`)
and consolidation, **but there is no vector schema and no embedding
model anywhere in the tree** — "vectorize like Engram" is not a thing
Engram does. Engram's real strengths: durable sessions/observations/
relations model, provenance discipline, consolidation, and a clean
entity split. Regulus already matches most of the entity model
(observations, pins, relations, revision chains); what Regulus lacks,
and Engram lacks, is **semantic recall** — finding the observation when
the wording changed.

## 2. What a vectorized local memory must respect here (our invariants)

1. **Zero Go dependencies** — `go.mod` has none today; the 6-platform
   single-binary build must stay trivial (CGO would complicate two of
   the six).
2. **Deterministic, receipt-provable** — every retrieval channel a gate
   or journey can test must be reproducible; a nondeterministic encoder
   cannot earn a receipt.
3. **Single writer, source of truth on disk** — the index is a *derived
   artifact*, never edited by hand, never the truth itself.
4. **Personal scale first** — thousands of observations, not millions
   (Premature ANN is over-engineering on the Necessity Ladder).

## 3. Local vector-store options, compared

| Option | Fit | Why / why not now |
|---|---|---|
| **Flat quantized scan (pure Go)** | ✅ **chosen, v2.0** | At 10k obs × 384 int8 bytes ≈ 3.8 MB, an exact brute-force cosine scan is milliseconds on one core. Zero deps, zero runtime, byte-deterministic, trivially CRAP-clean. The honest ladder answer. |
| sqlite-vec (SQLite ext, C) | ⛔ for now | Needs SQLite + extension loading; with pure-Go `modernc.org/sqlite` loadable extensions are fragile cross-platform, and mattn's port reintroduces CGO + toolchains. A dependency we do not need under ~50k observations. |
| LanceDB (embedded columnar) | ⛔ | Pulls Arrow + native dylib; a warehouse for a personal notebook. |
| Qdrant (local/server) | ⛔ | Second runtime to babysit; fights the single-writer, file-based state model. |
| usearch / FAISS (ANN libs, CGO) | ⛔→later | Fast ANN, but CGO for savings at scales we won't reach; revisit past 50k obs. |
| Pure-Go HNSW | ⛔ | No mature zero-dep implementation; unnecessary below the flat-scan wall. |

**Trigger to revisit**: > 50k observations or p95 search > 50 ms.
Until then flat scan, and only then sqlite-vec / usearch behind the same
retrieval interface.

## 4. The embedding question (the real hard part)

- v2.0 embedder = **hashed char-ngram features → 384 dims, int8
  quantized** (hashing trick, signed buckets, L2-normalized before
  quantize). Pure Go, ~80 lines, fully deterministic, trivially tested.
  It gives topic-affinity and typo-tolerance recall; it is NOT a
  MiniLM-class paraphrase model — an honest limitation, documented.
- v2.1 (opt-in) = **local ONNX MiniLM** via `onnxruntime` dynamic load
  (model file under `.regulus/almanac/models/`, gitignored). Pluggable
  `--embedder`, falls back to the hash embedder. Index header records
  `{embedder, dim, version}`; a mismatch forces a reindex — receipts
  pin which encoder produced the retrieval.
- Rejected: cloud/LLM-provider embeddings — nondeterministic across
  runs, network-bound, and cannot earn a deterministic receipt (invariant
  2). Optional non-gate curiosity only.
- LLM providers cannot silently embed: retrieval that a gate proves must
  come from a local, versioned encoder (padlock discipline).

## 5. Design — Almanac v2

- **Storage split**: `almanac/observations.jsonl` stays the append-only
  truth (single writer, provenance intact). New derived artifact
  `.regulus/almanac/retrieval.bin`: header `{magic, version, embedder,
  dim, count, sha256-of-source-prefix}` + count × (id + 384 × int8).
  Rebuilt by `regulus memory reindex` — a pure function of the JSONL.
- **Hybrid retrieval**: the existing deterministic bucket-then-match
  (lexical channel) stays; the vector scan becomes the second channel;
  fusion via reciprocal rank fusion (RRF, k=60) with (a) recency decay
  (half-life default 14 days, configurable) and (b) confidence boost
  from the observation's type/provenance — replacing today's plain
  age annotations. Deterministic end-to-end: same input → same order,
  every time, receipt-proven.
- **Dedup promotion (borrowed from Engram's consolidation, made cheap)**:
  a save that cosines > 0.92 against an existing observation is written
  as a *revision-linked* observation (or refused with guidance) instead
  of a silent near-duplicate — matching the Almanac's existing
  relations/pins model.
- **Relation suggestions**: `memory relate` gains candidates from the
  vector channel (top-k above threshold), instead of being unused.

### First spike — measurable acceptance criteria (each becomes a scenario)

1. **Deterministic rebuild**: `memory reindex` twice → byte-identical
   `retrieval.bin`, sha256-pinned in a receipt.
2. **Lexical-miss recall**: on a fixed fixture, a paraphrased/typo query
   that the lexical channel misses ranks the correct observation in the
   vector channel's top-3 (journey test).
3. **Fusion stability**: hybrid + decay ranking of a fixed corpus is
   bit-stable across two processes (journey test, hash of the order).
4. **Perf budget**: 10k synthetic observations — reindex < 2 s, hybrid
   search p95 < 50 ms design budget (fixture benchmark). The CI verdict
   ceiling is 5x (p95 < 250 ms): shared runners cannot resolve 50 ms
   deterministically, so the test logs p95 against 50 ms every run and
   fails only on catastrophic collapse.
5. **Zero dependencies**: `go.mod` gain after v2.0 = 0.
6. **Header-mismatch refuses**: a `retrieval.bin` whose header embedder
   mismatches configuration is rebuilt, never read trusted.

## 6. Phasing

- **v2.0 (the spike)** — hash embedder + flat scan + RRF fusion + decay
  + reindex command + journey tests. One worker unit.
- **v2.1** — pluggable embedder, local ONNX MiniLM.
- **v2.2** — sqlite-vec/usearch only past the §3 trigger; interface move,
  no schema rewrite (JSONL truth unchanged forever).

## 7. v2.0 as built (2026-10-29)

The spike landed as specced, unreleased (no release bump per the feature
order). What shipped:

- **T1** — hash char-ngram embedder (384 dims, int8, `hash-ngram-v1`) +
  `retrieval.bin` + `memory reindex` (pure function of the JSONL,
  byte-identical rebuilds, sha-pinned receipt) + determinism/header tests.
- **T2** — hybrid search: RRF fusion (k=60) + per-type decay (notes 14d,
  decisions 60d, lessons never) + domain-scope-first + stale/missing index
  degrades to lexical with a warning + `--lexical` escape +
  no-regression tests (hybrid never ranks below lexical-only).
- **T3** — save-time dedup promotion: `CheckDuplicate` enforced at the CLI
  save/note boundary (not inside `Save`, so `memory import` round-trips
  stay intact), 0.92 bar, topic-key revision bypass, supersede-hiding
  composition + eval fixture tests.
- **T4** — `memory relate --suggest <id>`: top-5 vector candidates above
  the 0.5 floor, suggestion-only (writes nothing); a missing/stale/corrupt
  index is rebuilt automatically via `Reindex` (cheap, deterministic) with
  a stderr note — R2 (almanac-robustez), 2026-10-30. Unknown ids still
  refuse before any rebuild.

Measured at landing (2026-10-29 sandbox, `internal/almanac/perf_test.go`,
10k synthetic obs, fixed seed 42):

- reindex ≈ 180ms — inside the §5 < 2s budget with 10x headroom.
- hybrid p95 ≈ 39–44ms (p50 ≈ 30–31ms, max ≈ 41–53ms over 4 runs) —
  inside the §5 50ms design budget after the T5-followup single-load
  optimization (one HybridSearch parses the JSONL once and reads
  pins/relations once, shared by both rank paths; before: p95 ≈ 55–60ms,
  p50 ≈ 47–49ms). The perf test logs p95 against the 50ms design budget
  every run; the hard verdict is 250ms (5x), set 2026-10-30 after the
  shared-runner CI failure (healthy code measured p95 ≈ 124ms pinned to
  one contended CPU — a 50ms hard gate is not deterministic).
- dedup 0.92 bar margins (eval fixture,
  `TestDedupThresholdMatchesFixture`): weakest dup typo/pooling 0.9350
  (+0.015 above the bar), nearest clean unrelated/pool-vs-runtime 0.2565
  (−0.66 below the bar). Paraphrases sit at 0.9503–0.9603; the other
  clean pair at 0.1193.

Deferred per S8: v2.1 pluggable ONNX MiniLM waits for live miss-rate
evidence. `go.mod` gain after v2.0: zero require directives
(`TestZeroDepsGoMod` pins the S9 invariant).

## M1 addendum (2026-10-30) — miss-rate measured, embedder pluggable

Measurement only: no ONNX dependency, no model binary, `go.mod` still
zero requires (`TestZeroDepsGoMod` green).

**Miss-rate** (`internal/almanac/missrate_test.go`,
`TestMissRateParaphraseVsTypo`): 5-doc corpus (distinct fixture a-sides
+ unrelated distractors), queries = the 4 dup b-sides as AND terms.

| query | band | cosine | lexical | vector rank | hybrid top-3 |
|---|---|---|---|---|---|
| paraphrase/postgres-pool | paraphrase | 0.9603 | miss | 1 | yes |
| paraphrase/pool-leak | paraphrase | 0.9503 | miss | 1 | yes |
| typo/pooling | typo | 0.9350 | miss | 1 | yes |
| typo/cache-sizing | typo | 0.9419 | miss | 1 | yes |

Paraphrase vector top-3 recall **2/2**, typo vector top-3 recall **2/2**,
lexical-miss rate **4/4** — every fixture query carries a novel or
typo'd word the AND lexical channel cannot match, while the vector
channel ranks the target first and hybrid (fresh index) surfaces it in
its top 3 end to end.

**ONNX gap** (`TestMissRateTrueParaphraseGap`): a genuine rewording with
no shared content words ("pgbouncer style multiplexing…") best-scores
**0.2327** — inside the unrelated band (0.1193–0.2565), far below the
suggestion floor 0.5 and the dedup bar 0.92. The hash embedder covers
near-paraphrase + typo; true paraphrase stays blind. v2.1 ONNX stays
deferred until a real paraphrase eval set exists — this test documents
the gap without adding ONNX plumbing.

**Pluggable embedder** (S2): `Embedder` interface + `LookupEmbedder`
registry (`internal/almanac/embed.go`), `ReindexWith` / `LoadIndexWith`
/ `HybridSearchWith` (`vindex.go`, `hybrid.go`). Header already carried
`{embedder, dim, version}` — now pinned by `TestHeaderPinsEmbedderDim-
Version` (`hash-ngram-v1`, 384, 1); foreign headers refuse with
`ErrHeaderMismatch` (`TestHeaderMismatchForcesReindex`), unknown ids
refuse before any write. CLI: `memory reindex [--embedder <id>]` and
`memory search [--embedder <id>]` (default `hash-ngram-v1`); `reindex`
stdout names the encoder (`embedder: hash-ngram-v1`) so evidence
receipts pin it. Dedup and `relate --suggest` stay on the default
encoder (save path carries no flag — same encoder on the fly and on
disk, no skew).

## M2 verification (2026-10-30) — M1 numbers re-checked, v2.1 still deferred

Coherence check against the code (no M1 content changed): re-ran
`TestMissRateParaphraseVsTypo` — sims 0.9603 / 0.9503 (paraphrase) and
0.9350 / 0.9419 (typo), all vecRank 1, hybrid top-3, lexical-miss 4/4 —
and `TestMissRateTrueParaphraseGap` — best 0.2327, below the 0.5 floor.
Bars confirmed at their definitions: `DedupThreshold` 0.92
(`internal/almanac/dedup.go`) and `SuggestFloor` 0.5
(`internal/almanac/relate.go`). v2.1 ONNX stays deferred: the only
registered encoder is `hash-ngram-v1` (`LookupEmbedder` refuses unknown
ids, `TestZeroDepsGoMod` keeps `go.mod` at zero requires), and the index
header keeps pinning `{hash-ngram-v1, 384, 1}`.
