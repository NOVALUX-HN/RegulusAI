# ADR T2-02 — Completion is core-verified, never self-reported

Date: 2026-10-30 · Status: accepted · Phase 2 (Sextant)

## Context

Progress lived in markdown tables an agent edits itself: marking `done`
was indistinguishable from being done. No authority checked acceptance,
evidence, dependencies, or the level's requirements.

## Decision

`regulus task close <chart> <id>` (backed by `internal/sextant`) is the
sole completion authority. It verifies the contract, the spec, the task's
hash-verified all-PASS receipts, closed dependencies, and — for CRITICAL —
passing verifier evidence. Success flips Status and appends a CLOSED log
record; failure lists every unmet condition and changes nothing. A `done`
status without a CLOSED record is itself a `task status` violation.
TRIVIAL work closes on stated acceptance (no receipts demanded); anything
else needs evidence. Verifier evidence gates CRITICAL closure, not the
verifier's own closure (no close-order deadlock).

## Consequences

- Agents request closure; the core grants it. Reviews audit CLOSED records
  + receipts, never claims.
- Legacy charts close once they add a Contract section (documented
  migration, no forced rewrite).
