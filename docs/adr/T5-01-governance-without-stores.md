# ADR T5-01 — Governance without new stores

Date: 2026-10-31 · Status: accepted · Phase 5 (Almanac)

## Context

Knowledge needs lifecycle states (proposed, verified, rejected), but the
Almanac's core invariant is an append-only log as the single source of
truth. In-place state edits would break that invariant and every derived
index built on top of it.

## Decision

States live in one append-only sidecar, `status.jsonl` (id, from, to, by,
note, at), folded at read time; SUPERSEDED stays computed from
`supersedes` edges. Defaults need no entries (lessons ACCEPTED, imports
PROPOSED, everything else OBSERVED). New fields on observations
(source, origin, expires_at) ride the existing JSONL lines. Relations
gain four validated kinds; exclusion sets (superseded, invalidated,
rejected, expired) are computed per query, never stored.

## Consequences

- The log stays append-only and human-readable; history is the audit trail.
- Terminal states (REJECTED, ACCEPTED) never move; agents cannot
  self-promote (verify demands cited evidence).
- Imported knowledge is PROPOSED by construction until a human or evidence
  moves it.
