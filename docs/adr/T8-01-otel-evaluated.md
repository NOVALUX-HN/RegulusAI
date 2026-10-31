# T8-01 — OpenTelemetry evaluated, not adopted

## Status

Accepted.

## Context

S2 says "Evalúa OpenTelemetry" — evaluate, not mandate. The core policy
is zero external modules (pinned by `internal/runtime` boundary tests):
an OTel SDK would be the first third-party dependency of the Go core,
with version-churn and supply-chain surface, for field names we can emit
ourselves.

## Decision

Emit OTel-compatible field names (`trace_id`, `span_id`,
`parent_span_id`, `service.name`) over plain JSONL, no SDK. Any backend
that ingests OTel-shaped JSON takes our export unchanged; the core stays
dependency-free. Revisit only if a backend requires SDK-side protocol
features (e.g. live tail sampling) that static export cannot provide.

## Consequences

`TestOTelFieldNamePin` locks the wire shape. Export stays a file copy,
not an integration project.
