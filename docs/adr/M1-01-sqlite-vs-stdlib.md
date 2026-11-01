# M1-01 — SQLite vs stdlib index for Almanac: stdlib wins, SQLite deferred

## Status

Proposed (worker spike + measurements done 2026-11-01; guardian verdict
PENDING — orchestrator to invoke `@regulus-reviewer` read-only on this file).

## Context

M1 (`charts/macizo-memory.md` M1) asks whether the Almanac JSONL store —
full-scan per query (`internal/almanac/almanac.go:262-290`), no locks
(`internal/budget/budget.go:100` single-writer convention), zero-deps
(`go.mod:1-3`, `TestZeroDepsGoMod`) — should move to SQLite
(`modernc.org/sqlite`, pure Go, no cgo) or stay on a stdlib-built index.

Real sizes measured 2026-11-01 (this tree, Windows):

| Artifact | Size | Notes |
|---|---|---|
| `.regulus/almanac/observations.jsonl` | 28,795 B, 57 lines | the whole truth |
| `.regulus/almanac/pins.jsonl` | 80 B | 1 pin |
| `.regulus/almanac/relations.jsonl` | 117 B | 1 relation |
| `.regulus/almanac/retrieval.bin` | 23,205 B | derived, rebuildable |
| `.regulus/traces/` | 124 files | sessions, values excluded |
| `.regulus/library/` | ~2 KB (4 files) | 2 lessons + 2 INDEX.md |
| `memory context` (built binary, warm) | ~57 ms | dominated by startup, not scan |
| `memory search` (built binary, warm) | ~960–975 ms lexical and hybrid | startup+work; scan is NOT the bottleneck at 57 rows |
| 10k-row scale (repo perf tests, just re-run) | reindex 179 ms; hybrid p50 32 ms / p95 48 ms / max 54 ms | inside §5 50 ms design budget, verdict ceiling 250 ms |

Spike (in `/tmp/m1spike`, OUTSIDE the tree, never committed):
(a) `modernc.org/sqlite v1.61.1`, schema `obs(id,at,type,topic,body)` +
`obs_fts FTS5`, real + synthetic (10k rows, 3.4 MB) data;
(b) stdlib only: sorted byte-offset index + trigram postings, same data.
Spike LOC: sqlite ~80, stdlib ~106 — both trivial; the cost is not code,
it is the dependency.

| Measure (10k rows) | SQLite (modernc) | stdlib |
|---|---|---|
| Load / build index | 124 ms | 266 ms (in-memory; persisted offsets ~120 KB) |
| Typical query (top-5, avg) | ~503 µs (unprepared, per-query round-trips; pessimistic) | ~3 µs |
| Store size for 3.4 MB source | 9.4 MB (~2.7×, FTS5 overhead) | ~120 KB offsets + postings (~7 MB naive in-memory; prunable) |
| Query quality | FTS5 ranking, prefix, snippets | trigram recall + exact-verify; ranking stays in `hybrid.go` |
| cgo | none (pure Go transpilation) | n/a |
| Cross-compile (6 targets) | VERIFIED: `linux/arm64`, `darwin/arm64`, `windows/amd64` build clean | trivially yes |
| Binary size delta | +7.3 MB (spike: 2.8 MB stdlib vs 10.1 MB sqlite; base `regulus` 12.8 MB → ~+57%) | ~0 |
| Supply chain | 26 modules in graph, 20 `go.sum` lines (from zero), ~239 MB module cache (`modernc.org/*`) | zero |

## Decision

**Stay stdlib for M2: extend the `retrieval.bin` pattern (sorted offsets +
trigram/term postings, `flock` file lock where writers appear) instead of
adopting SQLite.** Revisit SQLite only if one of these trips: store >100k
observations, a second concurrent writer becomes real (not conventional),
or M4 envfacts mining needs ad-hoc aggregation SQL cannot replace with a
scan. None holds today: 57 rows, single writer, scans already inside budget.

Why not SQLite now: at this scale it is slower per query in the spike,
~2.7× larger on disk, +57% binary, and its real price is the invariant —
it ends zero-deps (first `require` in `go.mod` history), forces
`TestZeroDepsGoMod` to be amended, adds a `docs/audit/dependency-map.md`
§2 row + CI `go.sum` handling + `govulncheck` surface for 26 modules, all
to solve a problem (full-scan latency) the measurements say we do not have
(search p95 48 ms at 10k rows vs 250 ms verdict ceiling; real store 57 rows).

## Consequences

- `go.mod` keeps zero `require`; `TestZeroDepsGoMod` stays green untouched.
- `docs/audit/dependency-map.md` needs NO change under this decision
  (§2 `core runtime | zero modules` row stands as-is).
- If a later ADR flips to SQLite, that same change MUST in one commit:
  amend/replace `TestZeroDepsGoMod`, add the §2 dependency-map row
  (modules + pins + enforcement), un-ignore `go.sum` in CI (`gauntlet.yml`
  "no go.sum" path), and record the `govulncheck` surface — per the chart
  constraint "`docs/audit/dependency-map.md` + CI + test se actualizan en
  el mismo cambio".
- M2 scope is therefore: WAL-compatible append + ordered/trigram index in
  `retrieval.bin` + file locks + legacy JSONL read path. No migration of
  module surface.
- Guardian verdict: APRUEBA CON CONDICIONES (`ses_e6adf2f88ffeKx8NCyEHvzA6Qf`, 2026-11-01) — 2 must-fix: (a) `flock` no existe en stdlib portable/Windows: M2 usará lockfile `O_CREATE|O_EXCL` + `tmp+Rename` o diferirá locks (sin `x/sys`); (b) reindex atómico (`tmp+Rename`), plan `IndexVersion=2` con `ErrHeaderMismatch` amable, y coste-por-escritura medido a 10k. 2 should-fix: tripwires medibles + aclarar 960ms CLI-wall vs 32ms in-process. Sin (a)(b), M2 no arranca.
