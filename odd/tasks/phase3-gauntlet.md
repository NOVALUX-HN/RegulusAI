# Phase 3 — Gauntlet Independent Verification

Header: Evolve Gauntlet into an independent verification engine. No new gates invented (§1),
no Decision Intelligence. Deterministic facts stay in receipts; semantic evaluations are
recorded, never authoritative (verdict.Resolve). Mandatory gates can never be omitted:
`required` defaults true, profiles only add optional gates. CLI exit codes 0/1/2 preserved;
`gate run` default (no profile) runs everything exactly as today. No commits.

## Specs

S1: "Gates con ID, categoría, condiciones de ejecución (when/paths), inputs declarados, comando,
timeout, resultado, evidencia y failure policy (halt|advisory). Ejecución progresiva por capas;
fallos halt detienen, advisory registra y continúa. Sin inventar gates."
S2: "Verificación semántica: guardianes especializados con responsabilidad delimitada; registry
machine-readable (identity, responsibility, scope, permissions, inputs, required evidence,
output schema, acceptance/rejection/abstention criteria) espejando los 6 guardians."
S3: "Verificación operacional por perfiles: `gate run --profile NAME` ejecuta required +
miembros del perfil; gates costosos (mutación, vuln) solo en perfiles que los incluyan.
Sin daemon disponible no hay sandbox nuevo — perfiles, no infraestructura."
S4: "Guardian governance: solo IDs del registry evalúan (crew rechazado); validación de inputs;
veredicto parseable; evidencia citada debe existir; modo read-only por construcción (evaluate
solo lee + append a evaluations.jsonl; test fija que receipts/charts no cambian)."
S5: "Independencia: el implementador no controla su evaluación (crew no evalúa); el Core
consolida vía verdict centralizado (receipts deciden, evaluations se registran con disputa)."
S6: "Risk-based: perfiles + required; TRIVIAL→quick (solo required), CRITICAL→full + verifier;
mandatory nunca omitido aunque el perfil o `when` diga lo contrario."
S7: "FP/FN: `gate verdict` reporta disagreement rate, inconclusive, evidence completeness
(receipts con/sin manifest) y separa determinista (receipts) de semántico (evaluations)."
S8: "Tests §8: ejecución, orden, propagación (halt vs advisory + exits), aislamiento guardian,
outputs inválidos, evidencia faltante, desacuerdo, generación de veredictos, regresión
(comportamiento default idéntico)."
S9: "Aceptación 1-7: resultados verificables, reproducibilidad, permisos delimitados,
independencia, Core controla veredictos, mandatory ineludible, evidencia vinculada."

## Tasks

- T1 (S1,S6): gate metadata (id/category/when/on_fail/required/profiles) + skip/advisory + `gate run --profile` + tests — route: inline — commit: —
- T2 (S2,S4,S5): `internal/guardians` registry + `gate evaluate` (validación + append) + aislamiento + tests — route: inline — commit: —
- T3 (S5,S7): consolidación (`gate verdict`: determinista vs semántico, stats, dispute) + tests — route: inline — commit: —
- T4 (docs): `docs/gauntlet.md` + ADRs + regulus.json metadata (categorías, sin gates nuevos) + CHANGELOG + tests — route: inline — commit: —
- T5: verificación total (build/vet/gofmt/tests/bun) + prueba viva + reporte — route: inline — commit: —

## Log

L1 (user original verbatim): "PHASE 3 -- GAUNTLET INDEPENDENT VERIFICATION MISSION [full spec S1-S9 as above] No desarrolles Decision Intelligence todavía. Actualiza documentación y contratos."
L2 (decision): No se inventan gates ni resultados; los 12 layers existentes no cambian (solo metadata aditiva con defaults = comportamiento actual). Evaluations en evaluations.jsonl (no colisiona con globs *.json de evidence). Sin commits.
L3 (2026-10-30, Phase 3 completed): T1-T5 done. Full `go test ./...` green, bun 56/56, vet+gofmt clean. Live fixture: dry-run plan + run (skip *.go, advisory exit 0, manifest) + 2 evaluations + `gate verdict` FAIL with DISPUTE flagged and rates. Nota: herramienta edit rechaza payloads con secuencias <...> (workaround: ediciones simples o vía codemode). Sin Decision Intelligence.
