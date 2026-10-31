# ADR T1-01 — Centralized verdict authority

Date: 2026-10-30 · Status: accepted · Phase 1 (trust foundation)

## Context

Gate outcomes, probe contrasts, guardian assessments and human reads all
produced verdict-like language (`pass`/`fail` strings, free-text verdicts)
with no single authority defining the states or who may set them. An agent
that writes its own verdict can launder an unverified claim into "done".

## Decision

`internal/verdict` owns the contract: four states
(PASS, FAIL, BLOCKED, INCONCLUSIVE) and six reasons (ok,
deterministic_failure, policy_violation, insufficient_evidence,
infrastructure_failure, evaluator_disagreement). Only core-observed
execution facts map to a verdict (`Adjudicate`). Agents and guardians
produce `Evaluation`s; `Resolve` records them and flags disagreement but
returns the core verdict untouched. Legacy `pass`/`fail` strings stay as a
derived read-only projection.

## Consequences

- Every new execution path must map its facts through `Adjudicate`; new
  states require an ADR.
- `evaluator_disagreement` is recorded, never adjudicated — a guardian can
  dispute, never override.
- Exit codes follow the verdict (0/1); usage errors (2) stay outside.
