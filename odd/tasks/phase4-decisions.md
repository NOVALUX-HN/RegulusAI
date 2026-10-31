# Phase 4 — Decision Intelligence

Header: Regulus evaluates technical decisions; it never makes them and never
auto-approves. Pipeline PERCEPTION → EVIDENCE → POLICY → ASSESSMENT →
DECISION with clear contracts per stage. Storage: `.regulus/decisions/`
(one JSON per decision: immutable contract + transition history + last
assessment). Assessments are advisory; POLICY gates + human approval govern;
ACCEPT requires zero blocking findings (+ human approval if critical).
Abstention is a first-class outcome (INCONCLUSIVE). No autonomous approval,
no new gates, no plugin/MCP changes. No commits.

## Specs

S1: "Contrato: id, problem, context, proposal, alternatives[] (name+tradeoff),
trade-offs, constraints[] (name: handling), risks[] (risk → mitigation),
consequences, evidence[] (refs), confidence (low|medium|high), reversibility
(easy|moderate|hard + rollback note when hard), artifacts[], commits[],
task, related[] (dec: ids). Validación de schema al proponer."
S2: "Pipeline con contratos por etapa: PERCEPTION (el problema existe),
EVIDENCE (refs resueltos o abstención), POLICY (gates duros + governance),
ASSESSMENT (10 criterios pass|fail|abstain + nota), DECISION (recomendación
+ rationale; abstención ante insuficiencia)."
S3: "10 criterios: problema real, correspondencia problema↔solución,
alternativas, restricciones, riesgos, evidencia, consistencia arquitectónica
(invariante zero-dep como contradicción detectable), complejidad nombrada en
trade-offs, impacto (artifacts), reversibilidad."
S4: "Detecciones: UNSUPPORTED_ASSUMPTION (absolutos sin evidencia),
CONTRADICTION (zero-dep + decisiones aceptadas en scope), UNJUSTIFIED_COMPLEXITY,
HIDDEN_DEPENDENCY (artifact inexistente), MISSING_ALTERNATIVES, UNMITIGATED_RISK,
OBSOLETE (scope aceptado vigente), INSUFFICIENT_EVIDENCE."
S5: "Estados PROPOSED→UNDER_REVIEW→ACCEPTED|REJECTED|INCONCLUSIVE;
ACCEPTED→SUPERSEDED (solo por otra ACCEPTED). Transiciones validadas;
assess auto-avanza PROPOSED→UNDER_REVIEW; abstención ⇒ INCONCLUSIVE."
S6: "Trazabilidad: task, artifacts (existencia chequeada), commits (formato),
specs (charts), evidence (refs resueltos), verification (veredicto del task
vinculado si hay task), related decisions; historial de transiciones con
actor y timestamp; show revalida integridad (estado vs historial)."
S7: "Governance: assessment ≠ policy ≠ human approval. ACCEPT bloqueado ante
cualquier finding bloqueante; critical exige --approve-by (quién: porqué)
registrado; sin políticas cumplidas no hay accept, ni con humano."
S8: "Tests §8: contratos válidos, evidencia faltante, contradicciones,
alternativas, riesgos, obsolescencia, transiciones, integridad."
S9: "Aceptación 1-7: registrar, evaluar justificación, contradicciones,
supuestos no demostrados, evidencia vinculada, abstención, historial."

## Tasks

- T1 (S1,S6 store): contract schema + store + transitions + integrity + tests — route: inline — commit: —
- T2 (S2,S3,S4): pipeline + 10 criteria + detections + evidence resolver + tests — route: inline — commit: —
- T3 (S5,S7): resolve governance (critical/human/blocking/obsolete) + supersede + CLI decide + tests — route: inline — commit: —
- T4 (docs): `docs/decisions.md` + ADRs T4-01/T4-02 + CHANGELOG + tests — route: inline — commit: —
- T5: verificación total (build/vet/gofmt/tests/bun) + prueba viva + reporte — route: inline — commit: —

## Log

L1 (user original verbatim): "PHASE 4 -- DECISION INTELLIGENCE MISSION [full spec S1-S9 as above] No conviertas Decision Intelligence en un sistema autónomo de aprobación. Actualiza documentación y ADRs."
L2 (decision): Evaluación con heurísticas documentadas como proxies (no NLP fingido): correspondencia y complejidad por estructura verificable; cero-dep como contradicción concreta; FP/FN con oráculo fuera de scope (como en Phase 3). Sin commits.
L3 (2026-10-31, Phase 4 completed): T1-T5 done. Full `go test ./...` green, bun 56/56, vet+gofmt clean. Live: propose→assess (10/10 ACCEPT)→resolve ACCEPTED with auto PROPOSED→UNDER_REVIEW history→show revalidates; re-resolve refused (terminal). Fixture cleaned. Sin aprobación autónoma en ningún path.
