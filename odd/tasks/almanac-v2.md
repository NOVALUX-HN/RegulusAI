# almanac-v2 — semantic retrieval for the Almanac (spike v2.0)

Authorizedowner order: implement the 7 recommendations, tests always
green before and after every unit. No release bump in this feature
(spike lands unreleased; release is a later owner decision). Read down
to `## Log`.

## Specs

- S1: "haz lo que recomiendas entonces siempre ejecutando test para verificar que todo funcione" — full authorization for the 7 items below; every unit lands with its tests green (go + bun + tsc + vet + gofmt).
- S2 (rec-1, eval): real-data eval — paraphrase/typo fixture pairs mined from Almanac-shaped observations; the 0.92 dedup threshold comes from the data, not fiat.
- S3 (rec-2, domain scope): vector search scopes by detected domain first, global fallback second.
- S4 (rec-3, decay): decay per observation type — notes decay (14d half-life), decisions slower, admitted lessons never decay.
- S5 (rec-4, composition): vector dedup composes with topic-key chains and supersede-hiding — one decider, no double-hiding; header mismatch never trusted.
- S6 (rec-5, stale): stale/missing index degrades to lexical + warning, never blocks or lies.
- S7 (rec-6, no-regression): hybrid never ranks below lexical-only on the existing journey fixtures (pinned test).
- S8 (rec-7, ladder): ONNX deferred — v2.0 ships hash embedder only; MiniLM waits for live miss-rate evidence.
- S9 (design, docs/memory-v2.md): hash char-ngram embedder (384 dims, int8, deterministic) + flat scan + RRF fusion (k=60) + `memory reindex` (pure function of the JSONL, byte-identical rebuilds, sha-pinned receipt); zero new go.mod deps; perf budget 10k obs (reindex < 2s, p95 < 50ms); `memory search` hybrid-on-when-fresh with `--lexical` escape; `memory relate` vector candidates; save-time dedup promotion.

## Tasks

- [x] T1 (S9) — hash embedder + retrieval.bin + `memory reindex` + determinism/header tests — route: inline — commit: b8911be
- [x] T2 (S3,S4,S6,S7,S9) — hybrid search (RRF + type decay + domain scope + stale degrade + `--lexical`) + no-regression tests — route: inline — commit: 9f00896
- [x] T3 (S2,S5) — save-time dedup promotion + composition tests + eval fixture — route: inline — commit: 8981221
- L2 (decision, T3 wiring): CheckDuplicate enforced at the CLI save/note boundary, NOT inside almanac.Save. Save-level refusal would break `memory import` round-trips (re-importing identical content must succeed) and every programmatic caller; the decider is exported and shared so future callers (e.g. reflect admission) opt in deliberately. T2 fixtures saving identical texts stay valid as-is.
- [x] T4 (S9) — `relate --suggest` vector candidates (top-5, floor 0.5, suggestion-only) + tests — route: inline — commit: ef328ac
- [ ] T5 (S8,S9) — perf budget test + zero-deps check + docs (memory-v2 status, troubleshooting) — route: inline — commit: —
- [ ] T6 (S1) — full verification (go + bun + tsc + vet + gofmt) + reviewable commits — route: inline — commit: —

## Log

- L1: Origin — "acabo de realizar unos cambios usando el harness de regulus desde opencode para mejorar regulus mismos. revisa los ultimos cambios y verifica que esten correctos. otra cosa que necesito es analizar la mejora que vamos a implementar de Almanac. haz una revision y dime como potenciarlo para que regulus funcione de mejor forma" → review done (0.8.1 verified, binaries restamped), 7 recommendations approved by "eso que recomiendas nos ayudaria a mejorar regulus?" → "haz lo que recomiendas entonces siempre ejecutando test para verificar que todo funcione".
