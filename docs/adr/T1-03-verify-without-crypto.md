# ADR T1-03 — Receipt verification without cryptography

Date: 2026-10-30 · Status: accepted · Phase 1 (trust foundation)

## Context

Receipts are hash-pinned (SHA-256 of full stdout/stderr stored beside the
captured text), but the hash lives inside the file it describes: rewriting
both is undetectable from the file alone. Phase 0 (AUD-P1-03) proposed
opt-in HMAC signing.

## Decision

No cryptography is introduced — there is no demonstrable need (personal
harness; reviewers audit via git history and CI artifacts). Instead,
`evidence verify` recomputes every content hash, checks schema versions,
and cross-checks each receipt against its candidate manifest (file
presence, manifest membership, post-run file hash). Findings:

- corruption (malformed bytes, hash mismatch) → file quarantined to
  `corrupt/` (moved, never deleted) and reported;
- sound receipt with a broken link (missing manifest, unlisted) → reported,
  file stays;
- unknown future schema → fails closed, file stays (a newer writer's data
  must survive an older verifier).

Recovery is re-running the gate, not repairing the file.

## Consequences

- `evidence verify [--task]` is the integrity read path; CI can gate on it.
- Signing stays deferred until receipts cross trust boundaries (revisit
  AUD-P1-03 then, with a key-management story first).
