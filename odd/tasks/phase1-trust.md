# Phase 1 — Trust Foundation

Header: Strengthen evidence integrity, receipt verifiability, centralized verdicts, execution policy.
Contracts preserved: `regulus.contract/v1` unchanged, `Receipt.Verdict` pass/fail derived values unchanged,
CLI exit codes unchanged (0/1/2), plugin output parsing unaffected (pass-through + additive lines only).
No commits by the agent (pre-existing dirty tree on main; report only).

## Specs

S1: "Evidencia: SHA-256, artifact fingerprinting, Git HEAD, diff hashing, modified/untracked inventory, config fingerprint, tool versions, command args, timestamps, unique execution IDs, task-artifact links. Un Git HEAD no identifica el contenido exacto: manifiesto reproducible del candidato."
S2: "Receipts verificables: integridad, inmutabilidad lógica, procedencia, versionado, relaciones entre evidencias, detección de alteraciones, recuperación ante corrupción. Sin cripto innecesaria (ADR: sin HMAC — hash-pinned + manifest index + verify es suficiente para el threat model personal)."
S3: "Contrato centralizado de veredictos: PASS/FAIL/BLOCKED/INCONCLUSIVE; deterministic_failure, policy_violation, insufficient_evidence, infrastructure_failure, evaluator_disagreement. Core autoridad final; agentes/guardianes producen evaluaciones (registradas, nunca override)."
S4: "Seguridad de ejecución: timeouts mandatorios con caps, resource limits (output cap + CPU registrado; cgroups fuera de scope — ADR), cwd pin, env protection (sólo nombres, unset opcional), clasificación standard/privileged/destructive, allowlist opcional, destructive blocklist catastrófica, controles destructivos, restricciones fs (cwd + docker opt-in), aislamiento por proceso, docker opt-in con degradación segura registrada."
S5: "Reproducibilidad: artifact identity, config, env (nombres), tool versions, inputs, commands, outputs (hash), errors, duration, CPU; reproducibilidad exacta vs estadística como anotación por gate (default unknown) + conteo en manifiesto."
S6: "Tests: hash consistency, artifact changes, untracked, tamper, verdict consistency, timeout enforcement, resource restrictions, concurrent executions, failure recovery."
S7: "Aceptación: (1) artefactos con identidad verificable; (2) receipts comprobables; (3) Core centraliza veredictos; (4) comandos bajo política; (5) evidencias reconstruyen condiciones; (6) tests automatizados."

## Tasks

- T1 (S3,S7.3): `internal/verdict` — Status/Reason/Adjudicate/Evaluation + tests — route: inline — commit: —
- T2 (S1,S5,S7.1,S7.5): `internal/manifest` — candidate manifest + provenance + tests — route: inline — commit: —
- T3 (S2,S7.2): receipts additive fields + `evidence verify` + quarantine + `evidence show` ext + tests — route: inline — commit: —
- T4 (S4,S7.4): gates execution policy (Config.Execution, classification, cwd, caps, env, docker opt-in/fallback, dry-run, verdict wiring, RunTask) + `gate run` ext + tests — route: inline — commit: —
- T5 (S6): concurrency + recovery + hash/artifact/untracked/tamper/verdict/timeout/resource tests (folded per slice, consolidated here) — route: inline — commit: —
- T6 (docs): `docs/trust.md`, `docs/adr/T1-*.md`, CHANGELOG Unreleased, repo `regulus.json` execution block, full verification (vet/build/tests/coverage), final report — route: inline — commit: —

## Log

L1 (user original verbatim): "PHASE 1 -- TRUST FOUNDATION MISSION [full spec S1-S7 as above] EXECUTION RULES Preserva los contratos existentes. Implementa cambios incrementales. Actualiza la documentación y registra ADRs cuando corresponda. No avances a Sextant ni a Gauntlet. Entrega un informe con archivos modificados, pruebas y evidencias."
L2 (decision): dirty tree pre-existente en main (modificaciones bajo internal/almanac, cli, etc. anteriores a esta fase). No se crea rama ni commits: el informe lista archivos modificados. Contratos preservados por diseño aditivo (ver specs header).
L3 (2026-10-30, Phase 1 completed): T1-T6 done. Full `go test ./...` green, total coverage 83.2% (>= 80 floor), vet clean. Live fixture proof: gate run PASS + manifest + `evidence verify` 2/2 [1 manifest]. T4 delegated to gentle-ai-worker (task mvvigq12-1-3fx); parent corrected manifest placement (task dir, stamped receipts) + added output-cap/ResourceExhausted. No Sextant/Gauntlet work.
