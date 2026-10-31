# ADR T3-01 — Failure policy, conditions, and profiles

Date: 2026-10-30 · Status: accepted · Phase 3 (Gauntlet)

## Context

Every gate failure stopped the gauntlet, every gate always ran, and there
was no vocabulary for "informational" checks, conditional execution, or
cheap-vs-full runs. Expensive gates (mutation, vuln checks) paid their
full price on every invocation with no policy to scope them.

## Decision

Gates gain additive metadata with behavior-preserving defaults: `on_fail`
(`halt` default, `advisory` records-and-continues), `when` path globs
(optional gates only; required gates always run; unevaluable conditions
fail open with a receipt note), `required` (default true — mandatory
policy can never be omitted by omission), `profiles` membership, `id` and
`category`. `gate run --profile` selects required + members; `quick` and
`full` are reserved; unknown profiles are an error, never a silent
subset. Skips and advisory outcomes are recorded (result, manifest, CLI
footer) instead of disappearing.

## Consequences

- Default runs behave exactly as before (all gates, halt on failure).
- Advisory gates must be justifiable in review: a FAIL that exits 0 is a
  policy statement, visible in every footer.
- New metadata needs policy tests incl. false-run guards (skip really
  skips, required really runs).
