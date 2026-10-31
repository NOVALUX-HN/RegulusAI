# Phase 2 — Sextant Engineering Discipline

Header: Audit + evolve Sextant ONLY (+necessary integrations). No Evolution, no Decision Intelligence.
Machine-checkable discipline in core Go; prose (skill/agents/discipline.md) updated boundedly to match.
Contracts: `spec.Check` behavior unchanged (new chart sections are additive); `chart init` scaffold extended
backwards-compatibly; CLI exit codes 0/1/2; no plugin/MCP changes (no new host tools needed).
No commits (dirty tree on main; report lists files).

## Specs

S1: "Clasificación TRIVIAL/STANDARD/COMPLEX/CRITICAL con planning/spec/execution/verification/documentation/completion requirements por nivel. Profundidad proporcional al riesgo. Trivial jamás evade políticas de seguridad (destructive/security ⇒ mínimo CRITICAL, testeado)."
S2: "Contratos estructurados: objective, scope, constraints, acceptance, artifacts, dependencies, risk, verification, completion. Validables automáticamente (`task check`). Trazabilidad contrato↔escenarios (acceptance nombra escenarios que deben existir)."
S3: "Spec management: chart init genera Contract+Class; persistencia=chart file; versionado=git; trazabilidad Spec↔Task; validator detecta inconsistencias (contrato sin escenarios, escenarios sin contrato, tasks sin spec)."
S4: "Delegación gobernada: reglas cuándo-delegar por nivel (TRIVIAL→inline siempre; STANDARD→unidades acotadas; COMPLEX→scout-then-build; CRITICAL→verificador independiente requerido). Cada delegación (fila Tasks con Route) validada: agente conocido, contexto (Spec), output esperado, aceptación."
S5: "Execution governance: `task status` — progreso (todo/doing/blocked:reason/done), dependencias (doing/done con deps sin cerrar ⇒ violación; ciclos ⇒ violación), scope (done sin close ⇒ violación), replan justificada (entradas REPLAN: en Log con formato válido)."
S6: "Completion integrity: `task close <chart> <task>`: el Core comprueba spec OK + receipts de la task con PASS + requirements del nivel (CRITICAL exige verifier task con receipts PASS). Ningún self-report cierra."
S7: "Tests §7: clasificación, generación de contratos (template), validación acceptance, delegaciones, dependencias (incl. ciclos), replanificación, completion verification, fallos y recuperación (blocked con razón, close con lista accionable)."
S8: "Aceptación Sextant 1-6: contratos estructurados; workflow adaptado a complejidad; delegaciones justificadas/delimitadas; bloqueos identificables; aceptación verificable; finalización depende de evidencia. Docs + ADRs actualizados."

## Tasks

- T1 (S1): `internal/sextant` classify + requirements matrix + tests — route: inline — commit: —
- T2 (S2,S3): contract parse/validate + chart template + tests — route: inline — commit: —
- T3 (S4,S5): delegation rules + progress/deps/blockers/REPLAN + `task status` model + tests — route: inline — commit: —
- T4 (S6): `task close` completion verification + CLI `task` group + tests — route: inline — commit: —
- T5 (S8 docs): skill/agent/discipline bounded updates + `docs/sextant.md` + ADRs + tests — route: inline — commit: —
- T6: full verification (vet/build/tests/coverage) + live fixture proof + final report — route: inline — commit: —

## Log

L1 (user original verbatim): "PHASE 2 -- SEXTANT ENGINEERING DISCIPLINE MISSION [full spec S1-S8 as above] No implementes Evolution ni Decision Intelligence en esta fase. Actualiza documentación, pruebas y ADRs."
L2 (decision): Sextant hoy = Charts markdown + spec.Check + prosa en skill/agentes; nada machine-checkable para S1-S6. Se implementa en core Go con tracción en charts existentes (20 charts deben seguir pasando `chart list`/`spec check`). Sin commits.
L3 (2026-10-30, Phase 2 completed): T1-T6 done. Full `go test ./...` green, vet+gofmt clean, sextant coverage 89.2%, bun 56/56 (plugin sync + token budgets green). Live: scaffold `task check` OK, legacy yolo-fix `task check` OK (sin Contract no hay violaciones en check; close lo exigiría — migración documentada). Prosa sincronizada (.devin espejado, literales regenerados). Sin Evolution/Decision Intelligence.
