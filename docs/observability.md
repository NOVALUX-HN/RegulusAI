# Regulus Observability (Phase 8)

Telemetry without vendors: a structured event log, correlation spans and
aggregated metrics built read-only over the stores Regulus already keeps
(receipts, traces, budget, evolution). Nothing is shipped to anyone;
export is local JSONL any backend can ingest. Decisions: `docs/adr/T8-01`,
`T8-02`.

## 1. Structured events

`internal/observability.Event` carries exactly the S1 fields: timestamp,
severity (debug/info/warn/error), component, execution/task/correlation
ids, optional agent id, event type, error details. Validation refuses
incomplete events. Storage is append-only JSONL under
`.regulus/observability/` (task-scoped files, `events.jsonl` default).

## 2. Spans and correlation

`Span` uses OTel-compatible field names (`trace_id`, `span_id`,
`parent_span_id`, `service.name`) so exports ingest anywhere — without
the OTel SDK (adr/T8-01). `ChildSpan` inherits trace and correlation;
`ProjectTimeline` rebuilds one correlated timeline per task from receipts
(tool lifecycle), trace entries (tool calls), budget entries (lifecycle
markers) and evolve experiments. Kinds with no backing store (guardian
detail, memory retrieval log, external calls) project empty, honestly —
never invented.

## 3. Metrics and budgets

`Summarize` aggregates the S3 rates read-only; empty input yields zeros,
never NaN. Honest gap, pinned by tests: guardian disagreement has no
backing store, so it reports 0 with a coverage note (same honesty rule
as the T2 span projections) — a real guardian-decision log would be
needed to measure it. `BudgetUsage` marks every item measured or estimated with a
reason: execution time (receipt durations) and tool usage (trace counts)
are measured; tokens and model costs are unmeasured unless the caller
supplies reliable figures — no cost is ever invented.

## 4. Security: redaction and retention

Every write path passes through redaction: deny-listed keys
(`api_key`, `secret`, `token`, `password`, `credential`, `authorization`,
private prompt fields) become `[redacted]`, plus bearer/key-pattern
scrubbing in free text. Raw-byte tests pin zero leakage. `Prune(keep)`
caps log size; all telemetry stays local under `.regulus/` (gitignored).

## 5. Exporters

`Exporter` returns errors, never panics. `FileExporter` appends redacted
JSONL; `MemoryExporter` serves tests. A backend ships these files with
whatever agent it likes — the core depends on no provider (adr/T8-02).
Malformed lines fail with line numbers; missing stores read as empty.
