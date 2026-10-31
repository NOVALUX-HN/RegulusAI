# T8-02 — Project, don't instrument: read-only timeline projection

## Status

Accepted.

## Context

S2/S4 want per-module events and metrics with no provider coupling. The
expensive design rewrites every producer (gates, guardians, almanac,
evolve) to emit telemetry. The cheap design reads the artifacts they
already write.

## Decision

Project read-only: receipts, trace entries, budget entries and evolve
experiments already contain who ran what, in which order, with what
outcome and duration — the projection (`ProjectTimeline`) correlates
them by task/session without touching a single producer. Kinds with no
backing store project empty and say so. Direct emission (`Record`,
`StartSpan`) exists for future producers, not as a retrofit mandate.

## Consequences

Zero producer churn, zero behavior risk. If a module later needs
finer-grained spans, it emits them through the same redacted paths —
the projection keeps working unchanged.
