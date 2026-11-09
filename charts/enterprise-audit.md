# enterprise-audit — re-auditoría total post-cambios nivel empresarial

User authorization: "es correcto. procede con el plan y avisame hasta que finalices todo" 2026-11-03. Fase final tras B+O+H. Solo lectura + reporte, no aplica mejoras (las propone como backlog). Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: auditar todo Regulus tras B+O+H y dictaminar si precisión/velocidad/tokens pueden seguir mejorando con bar empresarial (Google/Meta/Nvidia): gates verdes, receipts trazables, bench sin regresiones, guardians sin must-fix, presupuestos medidos.
- Scope: lectura de receipts/manifests, `Metrics Summarize`, `Budget Usage`, `bench Compare` vs `baseline 7/7 14ms/254B`, guardian sweep (reviewer, evidence-auditor, crap-analyst, mutation-auditor, spec-keeper, gate-verifier) + verifier en claims de riesgo. Sin tocar código, permisos, allowlist, gates ni superficie.
- Constraints: read-only (cero edits prod/superficie); cada afirmación cita `rec_…`/receipt path; fallos/skips/pendientes reportados con next step; `spec check` OK.
- Acceptance: E1, E2
- Artifacts: reporte `docs/enterprise-audit-*.md` + receipts `enterprise-audit/E*` (verificación, no cambio).
- Dependencies: corre tras B2+O2+T1→P1 CLOSED; E1 antes que E2.
- Risk: standard
- Verification: `spec check` + `gate run enterprise-audit/E*` 12/12 (gates deben pasar sin cambios) + verifier PASS en muestreo de riesgo.
- Completion: E1–E2 CLOSED vía `task close`.

## Acceptance

### Scenario: E1

- Given B+O+H CLOSED con receipts 12/12 y baseline `bench 7/7 14ms/254B`
- When se re-miden precisión (spec rechazos, bench golden), velocidad (wall-time por Oi, gates) y tokens (medidos T1 vs full) contra baseline
- Then tabla antes/después con veredicto ADOPT/sin-regresiones y lista de gaps que aún pueden mejorar

### Scenario: E2

- Given tabla E1 + guardian sweep + budgets medidos
- When se dictamina bar empresarial (hermético, portable Win/Linux, CI verde, releases versionados, multi-proyecto, denies intactos) y se proponen próximos 3 Charts sin aplicarlos
- Then reporte cierra con next steps concretos y nada pendiente a medias

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| E1 | E1 | re-medir precisión/velocidad/tokens vs baseline | @regulus-verifier | done |
| E2 | E2 | dictamen empresarial + backlog 3 Charts | @regulus-scout | done |

## Log

- L1: Chart opened 2026-11-03 como fase final del plan noche. Corre tras H. O1 layer-4 en diagnóstico (background go test en curso).

- CLOSED E1 STANDARD spec OK + 12 receipt(s) PASS (run_a2515cd5e40685ab).
- CLOSED E2 STANDARD spec OK + 12 receipt(s) PASS (run_65b20054fff3f122).