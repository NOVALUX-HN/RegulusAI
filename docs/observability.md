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
never NaN. Honest gap, pinned by tests: guardian disagreement had no
backing store until the guardian decision log landed (now measured
from it when present). `BudgetUsage` marks every item measured or estimated with a
reason: execution time (receipt durations) and tool usage (trace counts)
are measured; tokens and model costs are unmeasured unless the caller
supplies reliable figures — no cost is ever invented.

## 3b. Per-task cost accounting (F1)

Every trace entry optionally carries its proxy cost: the task it
served plus `duration_ms` (wall time), `output_bytes` (output size),
and `input_tokens`/`output_tokens` — token fields only when the host
exposes counts (OpenCode exposes none, so they stay 0 = unknown,
never estimated). Zero means unknown, not zero: lines written before
these fields existed decode as unattributed/unknown and keep reading.

`trace.CostByTask` folds every session log into per-task totals
(unattributed legacy lines bucket together); `regulus trace record`
accepts `--task/--duration-ms/--output-bytes/--input-tokens/--output-tokens`
and `regulus trace cost [--task <t>]` reports the fold. The report is
auditable: it recomputes from the same JSONL lines `trace show`
reads, so re-listing any session reproduces every number. `Summarize`
adds the proxy sums (`trace_duration_ms`, `trace_output_bytes`,
`tokens_total` only when traces carry counts); `BudgetUsage` extends
the tool-usage line with the proxy sums and measures tokens from
traces when present (caller figures take precedence). Token counts
never become money: model costs stay honest zero.

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
