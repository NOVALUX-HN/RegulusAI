# Phase 8 — Regulus Observability

Header: Provider-agnostic observability over existing stores: structured
event log with redaction, correlation spans projected from receipts/traces
(no producer rewrites), metrics aggregated read-only, measured-vs-estimated
budgets, JSONL export anyone can ship anywhere. No vendor SDKs, no secrets
on disk.

## Specs

S1: "## 1. STRUCTURED LOGGING Implementa logs estructurados con: Timestamp.
Severity. Component. Execution ID. Task ID. Agent ID cuando corresponda.
Correlation ID. Event type. Error details. Evita registrar secretos y datos
sensibles."
S2: "## 2. DISTRIBUTED TRACING Evalúa OpenTelemetry. Permite rastrear: Task
lifecycle. Agent execution. Tool calls. Guardian execution. Gauntlet.
Memory retrieval. Evolution experiments. External integrations. Mantén
correlación entre componentes."
S3: "## 3. METRICS Implementa métricas para: Task completion rate.
Verification pass rate. Verification failure rate. Guardian disagreement
rate. Evidence completeness. Evolution acceptance rate. Regression rate.
Execution duration. Resource consumption. Cost per successful task."
S4: "## 4. MODULE TELEMETRY Cada módulo debe proporcionar eventos y métricas
coherentes. Evita acoplar el Core a un proveedor específico."
S5: "## 5. BUDGET TRACKING Registra: Token usage cuando esté disponible.
Model costs cuando exista información fiable. Execution time. Tool usage.
Resource consumption. Experiment budgets. Distingue valores medidos de
estimaciones."
S6: "## 6. SECURITY No expongas: API keys. Secrets. Credentials. Private
prompts. Sensitive project data. Define políticas de redacción y
retención."
S7: "## 7. TESTING Prueba: Log structure. Correlation. Trace propagation.
Metrics. Failure handling. Sensitive data redaction. Exporter failures."
S8: "## 8. ACCEPTANCE CRITERIA Regulus debe permitir: 1. Reconstruir
ejecuciones. 2. Identificar fallos. 3. Medir rendimiento. 4. Medir costes.
5. Correlacionar eventos. 6. Detectar problemas operacionales. 7. Exportar
telemetría sin depender de un proveedor específico. Actualiza
documentación."

## Tasks

- T1 (S1+S6): event log + redaction + retention — route: delegated (gentle-ai-worker) — commit: 0b3dcda
- T2 (S2+S4): spans + correlation + store projection, OTel evaluated — route: delegated (gentle-ai-worker) — commit: 62a68ad
- T3 (S3+S5): metrics aggregation + measured-vs-estimated budgets + exporters — route: delegated (gentle-ai-worker) — commit: b4112ac
- T4 (S7+S8): matrix + acceptance + live proof — route: delegated (gentle-ai-worker, test-only) — commit: 07b0356
- T5 (docs): docs/observability.md + ADRs + CHANGELOG + full verification + final report — route: inline — commit: ea6fa7f

## Log

L1 (user original verbatim): "# PHASE 8 — REGULUS OBSERVABILITY

## MISSION

Actúa como Principal SRE y Observability Engineer.

Evoluciona el sistema de observabilidad del proyecto actual.

## 1. STRUCTURED LOGGING

Implementa logs estructurados con:

- Timestamp.
- Severity.
- Component.
- Execution ID.
- Task ID.
- Agent ID cuando corresponda.
- Correlation ID.
- Event type.
- Error details.

Evita registrar secretos y datos sensibles.

## 2. DISTRIBUTED TRACING

Evalúa OpenTelemetry.

Permite rastrear:

- Task lifecycle.
- Agent execution.
- Tool calls.
- Guardian execution.
- Gauntlet.
- Memory retrieval.
- Evolution experiments.
- External integrations.

Mantén correlación entre componentes.

## 3. METRICS

Implementa métricas para:

- Task completion rate.
- Verification pass rate.
- Verification failure rate.
- Guardian disagreement rate.
- Evidence completeness.
- Evolution acceptance rate.
- Regression rate.
- Execution duration.
- Resource consumption.
- Cost per successful task.

## 4. MODULE TELEMETRY

Cada módulo debe proporcionar eventos y métricas coherentes.

Evita acoplar el Core a un proveedor específico.

## 5. BUDGET TRACKING

Registra:

- Token usage cuando esté disponible.
- Model costs cuando exista información fiable.
- Execution time.
- Tool usage.
- Resource consumption.
- Experiment budgets.

Distingue valores medidos de estimaciones.

## 6. SECURITY

No expongas:

- API keys.
- Secrets.
- Credentials.
- Private prompts.
- Sensitive project data.

Define políticas de redacción y retención.

## 7. TESTING

Prueba:

- Log structure.
- Correlation.
- Trace propagation.
- Metrics.
- Failure handling.
- Sensitive data redaction.
- Exporter failures.

## 8. ACCEPTANCE CRITERIA

Regulus debe permitir:

1. Reconstruir ejecuciones.
2. Identificar fallos.
3. Medir rendimiento.
4. Medir costes.
5. Correlacionar eventos.
6. Detectar problemas operacionales.
7. Exportar telemetría sin depender de un proveedor específico.

Actualiza documentación."

L3 (2026-10-31, Phase 8 completed): T1-T5 done on develop (0b3dcda, 62a68ad, b4112ac, 07b0356, ea6fa7f). Full `go vet` + `build` + `go test ./...` green, 0 FAIL. T1-T4 delegated (gentle-ai-worker, assess each: writer self-verification stands, no verifier). OTel evaluated-not-adopted; projection read-only; no invented numbers.

L2 (decision): OpenTelemetry evaluated, NOT adopted as SDK: zero-dep core
policy wins; the package uses OTel-compatible field names
(trace_id/span_id/service.name) over JSONL so any backend can ingest
without coupling. Spans are projected read-only from receipts/traces/
budget/evolution stores (no producer rewrites; trace entries already
exclude values by design). Metrics aggregate the same stores. New
telemetry lives under .regulus/observability/ (gitignored with the rest
of .regulus). Redaction is key-name + bearer-pattern based; retention
policy documented in T5.
