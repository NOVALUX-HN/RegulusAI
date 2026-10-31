# ADR T5-02 — Filter retrieval, don't rerank it; no new vector infra

Date: 2026-10-31 · Status: accepted · Phase 5 (Almanac)

## Context

The hybrid channel (lexical + flat quantized-cosine, RRF x decay +
confidence) already balances precision and cost at personal scale.
Requests arrived for governance-aware and scoped retrieval; the tempting
answer is a new ranker or new vector infrastructure.

## Decision

Retrieval gains filters, not weights: governance (rejected hidden),
validity (expired hidden), relations (invalidated hidden, superseded as
before), and system scoping — applied identically on the lexical and
hybrid paths via the shared matcher, with `--include-stale` /
`--include-superseded` escape hatches. Scoped `memory context` filters by
task/path/system with a per-list budget. Precision and cost are
documented, not rebuilt: zero new dependencies, zero new indexes.

## Consequences

- Stale knowledge stops surfacing as current without changing what
  "relevant" means; rankings stay byte-stable for fresh renders.
- `memory check` owns contradiction/staleness reporting; search owns
  hiding it. Separate jobs, separate tests.
- A same-instant double save can no longer share an identity: Save
  advances the timestamp until the id is fresh (append-only preserved).
