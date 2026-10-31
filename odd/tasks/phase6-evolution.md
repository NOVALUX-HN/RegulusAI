# Phase 6 — Evolution Controlled Self-Improvement

Header: Controlled self-improvement for Evolution without unrestricted
auto-evolution: explicit pipeline OBSERVE → HYPOTHESIZE → PROPOSE → PROBE →
EVALUATE → ADOPT/REJECT, isolated experiments, baselines with mandatory
guarantees, INV/DOM separation, human-approved adoption, full audit trail.
No auto-approval, ever.

## Specs

S1: "## 1. EVOLUTION PIPELINE Implementa: OBSERVE → HYPOTHESIZE → PROPOSE →
PROBE → EVALUATE → ADOPT / REJECT. Cada etapa debe tener contratos explícitos."
S2: "## 2. OBSERVATION Identifica oportunidades de mejora a partir de: Failed
executions. Repeated failures. Guardian findings. Gauntlet results. Execution
costs. Memory insights. Task outcomes. No conviertas una observación aislada en
una regla general."
S3: "## 3. HYPOTHESIS Cada hipótesis debe explicar: Problem. Root cause.
Proposed change. Expected benefit. Potential regressions. Evaluation criteria."
S4: "## 4. EXPERIMENT MANAGEMENT Registra: Hypothesis. Baseline. Candidate.
Evaluation dataset. Model. Runtime. Parameters. Budget. Results. Regression
analysis. Decision."
S5: "## 5. OPTIMIZATION STRATEGIES Diseña soporte para: Sequential
optimization. Parallel exploration. Hybrid optimization. Implementa
progresivamente."
S6: "## 6. EVALUATION METRICS Evalúa: Task success rate. Acceptance criteria
satisfaction. Regression rate. Defect detection. False positives. False
negatives. Cost per successful task. Token consumption. Execution duration.
Policy compliance. Evidence completeness."
S7: "## 7. BASELINE AND REGRESSION Conserva una versión estable. Compara
candidatos contra ella. Una mejora no puede adoptarse si deteriora garantías
obligatorias."
S8: "## 8. ROLLBACK Implementa: Versioning. Comparison. Rollback. History.
Audit trail."
S9: "## 9. EVOLUTION DOMAINS Mantén separados: INV: evolución del harness,
instrucciones, herramientas y workflows. DOM: evolución del conocimiento y
políticas del dominio. Una mejora de dominio no puede modificar silenciosamente
las garantías fundamentales."
S10: "## 10. SECURITY Toda propuesta debe tratarse como una modificación no
confiable. Debe: 1. Estar aislada. 2. Tener procedencia. 3. Superar
validaciones. 4. Superar regresiones. 5. Cumplir políticas. 6. Poder revertirse.
No permitas autoaprobación."
S11: "## 11. TESTING Prueba: Experiment reproducibility. Baseline comparison.
Candidate evaluation. Regression detection. Budget enforcement. Adoption.
Rejection. Rollback. Concurrent experiments."
S12: "## 12. ACCEPTANCE CRITERIA Evolution debe demostrar que: 1. Puede formular
hipótesis. 2. Puede ejecutar experimentos aislados. 3. Puede comparar
resultados. 4. Puede detectar regresiones. 5. Puede rechazar mejoras
perjudiciales. 6. Puede adoptar mejoras verificadas. 7. Puede revertir
modificaciones. 8. Mantiene las garantías del harness. Actualiza documentación
y ADRs."

## Tasks

- T1 (S1,S2): pipeline stage contracts + observation collectors (7 sources) + single-observation guard — route: inline — commit: ed1e2fe
- T2 (S3,S4): hypothesis validation + experiment record + store with states — route: inline — commit: 54c2170
- T3 (S5): sequential + parallel + hybrid runners, progressive — route: inline — commit: fbe9cca
- T4 (S6,S7,S8): metrics + baseline comparison + mandatory-guarantee gate + versioning/rollback/history — route: inline — commit: a6f2261
- T5 (S9,S10): INV/DOM tagging + DOM-cannot-touch-fundamentals enforcement + untrusted-proposal pipeline + human-approved adopt — route: inline — commit: 817feff
- T6 (S11,S12): test matrix + acceptance live proof — route: inline — commit: 52413f2
- T7 (docs): docs/evolution.md + ADRs + CHANGELOG + full verification + final report — route: inline — commit: adde8ad

## Log

L1 (user original verbatim): "# PHASE 6 — EVOLUTION CONTROLLED SELF-IMPROVEMENT

## MISSION

Actúa como Principal AI Systems Engineer especializado en harness optimization, experimental design y autonomous systems.

Evoluciona Evolution en el proyecto actual.

Utiliza los principios de Harness² como referencia conceptual, sin copiar su implementación.

No permitas autoevolución irrestricta.

## 1. EVOLUTION PIPELINE

Implementa:

OBSERVE → HYPOTHESIZE → PROPOSE → PROBE → EVALUATE → ADOPT / REJECT.

Cada etapa debe tener contratos explícitos.

## 2. OBSERVATION

Identifica oportunidades de mejora a partir de:

- Failed executions.
- Repeated failures.
- Guardian findings.
- Gauntlet results.
- Execution costs.
- Memory insights.
- Task outcomes.

No conviertas una observación aislada en una regla general.

## 3. HYPOTHESIS

Cada hipótesis debe explicar:

- Problem.
- Root cause.
- Proposed change.
- Expected benefit.
- Potential regressions.
- Evaluation criteria.

## 4. EXPERIMENT MANAGEMENT

Registra:

- Hypothesis.
- Baseline.
- Candidate.
- Evaluation dataset.
- Model.
- Runtime.
- Parameters.
- Budget.
- Results.
- Regression analysis.
- Decision.

## 5. OPTIMIZATION STRATEGIES

Diseña soporte para:

- Sequential optimization.
- Parallel exploration.
- Hybrid optimization.

Implementa progresivamente.

## 6. EVALUATION METRICS

Evalúa:

- Task success rate.
- Acceptance criteria satisfaction.
- Regression rate.
- Defect detection.
- False positives.
- False negatives.
- Cost per successful task.
- Token consumption.
- Execution duration.
- Policy compliance.
- Evidence completeness.

## 7. BASELINE AND REGRESSION

Conserva una versión estable.

Compara candidatos contra ella.

Una mejora no puede adoptarse si deteriora garantías obligatorias.

## 8. ROLLBACK

Implementa:

- Versioning.
- Comparison.
- Rollback.
- History.
- Audit trail.

## 9. EVOLUTION DOMAINS

Mantén separados:

INV: evolución del harness, instrucciones, herramientas y workflows.

DOM: evolución del conocimiento y políticas del dominio.

Una mejora de dominio no puede modificar silenciosamente las garantías fundamentales.

## 10. SECURITY

Toda propuesta debe tratarse como una modificación no confiable.

Debe:

1. Estar aislada.
2. Tener procedencia.
3. Superar validaciones.
4. Superar regresiones.
5. Cumplir políticas.
6. Poder revertirse.

No permitas autoaprobación.

## 11. TESTING

Prueba:

- Experiment reproducibility.
- Baseline comparison.
- Candidate evaluation.
- Regression detection.
- Budget enforcement.
- Adoption.
- Rejection.
- Rollback.
- Concurrent experiments.

## 12. ACCEPTANCE CRITERIA

Evolution debe demostrar que:

1. Puede formular hipótesis.
2. Puede ejecutar experimentos aislados.
3. Puede comparar resultados.
4. Puede detectar regresiones.
5. Puede rechazar mejoras perjudiciales.
6. Puede adoptar mejoras verificadas.
7. Puede revertir modificaciones.
8. Mantiene las garantías del harness.

Actualiza documentación y ADRs."

L3 (2026-10-31, Phase 6 completed): T1-T7 done on develop (ed1e2fe, 54c2170, fbe9cca, a6f2261, 817feff, 52413f2, adde8ad). Full `go vet` + `build` + `go test ./...` green, 0 FAIL, evolve 25/25. Live: TestEndToEndAcceptance runs all six stages on real packages. CLI wiring left as explicit future work (adr/T6-02).

L2 (decision): Mirror the decisions-package pattern for experiments (validated
record + state machine + integrity re-check), one new store dir
.regulus/evolution/ (experiments + baselines); reuse budget ledger for
experiment caps/audit and the --approve-by human-approval convention for ADOPT.
Existing Assemble (evidence pack) stays read-only and becomes the OBSERVE-side
input; no vector infra, no ranking changes.
