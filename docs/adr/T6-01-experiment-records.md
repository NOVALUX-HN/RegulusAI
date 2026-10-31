# T6-01 — Experiment records mirror the decisions pattern

## Status

Accepted.

## Context

Phase 6 needs stored experiments with states, transitions and integrity.
`internal/decisions` already solves exactly that for decision contracts:
validated record, exclusive ids, appended history, legal-successor map,
re-checkable integrity.

## Decision

Mirror the pattern instead of inventing a second one: `Experiment` lives
under `.regulus/evolution/<id>.json` with schema version, state machine
(PROPOSED → RUNNING → EVALUATED → ADOPTED | REJECTED), `O_EXCL` writes
and `CheckExperimentIntegrity`. Same shapes, different store, no shared
code — the two domains must evolve independently.

## Consequences

One mental model for both stores. Experiment-specific rules (trust
attestation, domain check, baseline gate) compose inside `Adopt` without
touching the decisions package.
