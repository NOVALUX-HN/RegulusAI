# retention-gc — podar .regulus sin romper auditoría

User authorization: "sube los cambios y continúa con el siguiente punto" 2026-10-31, siguiente = retención `.regulus` (segundo de la lista). Push `04ea86d..2e07199` hecho. Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: podar `.regulus/evidence/` + `.regulus/traces/` con comandos auditables, sin borrar evidencia de tasks CLOSED sin autorización explícita.
- Scope: `internal/evidence/` + `internal/cli/evidence.go` (nuevo `prune`), `internal/trace/` + `internal/cli/trace.go` (`--all`), `.github/workflows/gauntlet.yml` (retention-days), `docs/troubleshooting.md` (política). Sin tocar gates, memoria, library.
- Constraints: dry-run sin `--yes`; manifests (`manifest-*.json`) nunca se borran; tasks CLOSED en `charts/` rehusadas sin `--force`; `corrupt/` intacto; `go test ./internal/evidence/ ./internal/trace/ ./internal/cli/` verde.
- Acceptance: S1, S2, S3
- Artifacts: `bin/regulus`, receipts `retention-gc/R*`.
- Dependencies: R1 primero (guard compartido); R2–R3 en paralelo tras R1.
- Risk: CRITICAL (destructivo: borra receipts/traces) — requiere verifier, dry-run por defecto, guard CLOSED.
- Verification: `spec check charts/retention-gc.md` + `gate run retention-gc/R*` + verifier independiente con prueba de borrado en tmp.
- Completion: 3 tasks CLOSED + V1 PASS, `spec OK: 3`, capa 7 pass.

## Acceptance

### Scenario: S1 evidence prune con guard

- Given un task-dir con receipts + manifests y el chart marcando CLOSED o no
- When `bin/regulus evidence prune --task <slug> [--keep N]` corre sin `--yes`
- Then lista lo que borraría sin borrar nada; con `--yes` borra receipts viejos (respeta `--keep`, nunca manifests); si CLOSED y sin `--force`, rehúsa con mensaje que cita la autorización requerida

### Scenario: S2 trace prune bulk

- Given ≥79 sesiones en `.regulus/traces/`
- When `bin/regulus trace prune --all [--keep N]` corre
- Then rota cada sesión a sus últimos N registros (default 200 como hoy), sin borrar ficheros; reporte por sesión

### Scenario: S3 trail CI + docs

- Given `gauntlet.yml` con `retention-days: 7` solo para `ci__push/`
- When se sube a 30 días y se actualiza la política en `docs/troubleshooting.md`
- Then CI conserva receipts 30 días y el doc describe los dos comandos + guards

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| R1 | S1 | `evidence prune --task --keep --yes [--force]` + guard CLOSED (lee `charts/` Log) + tests (dry-run, keep, manifests intactos, refuse CLOSED, force borra) | @regulus-worker |
| R2 | S2 | `trace prune --all [--keep]` + tests (bulk rota, default 200, reporte) | @regulus-worker |
| R3 | S3 | bump `retention-days: 7→30` + `docs/troubleshooting.md` política + sizes | @regulus-worker |
| V1 | S1-S3 | verificación independiente: reproduce prune en copia tmp (dry-run no borra, guard CLOSED rehúsa, --force borra, manifests intactos) + `gate run` capa 7 pass | @regulus-verifier |

## Log

- L1: Chart opened 2026-10-31. Scout `ses_e6c8d1ee0ffeAGGDGRRJjeiJNo`: ~60 task-dirs, ≥79 traces (vs 39/49 el 2026-10-30); `trace prune` solo `--session` + `--keep 200` sin `--all` (`trace.go:93-107`); `evidence` solo `list/show/verify` sin prune (`evidence.go:15-33`); `verify` cuarentena move-only; `memory prune` ya es dry-run sin `--yes`; CI `retention-days: 7` (`gauntlet.yml:77`); política `troubleshooting.md:207-231` (sin gc por diseño, borrado manual).
- L2: R1/R2/R3 CLOSED-code 2026-10-31 — R1 `evidence prune` fail-closed (`ses_e6c8c9a1cfferdhaIh38oubk1b`, gate 12/12), R2 `trace prune --all` (`ses_e6c879412ffeKSpN3bOeVBlxiB`, 12/12), R3 retention 30d + docs (`ses_e6c824aa4ffej7vKYsB2nHtYjy`, 12/12, 62 dirs/82 traces/821 receipts). V1-subagent REFUTED por falta de shell (`ses_e6c7dafa6ffelVfVIE7vABTZ8k`, solo gate 12/12 + estático); lead reprodujo en tmp: dry-run lista 3/1 sin borrar, `--yes --keep 1` deja newest+manifest, guard CLOSED rehúsa exit 1 y `--force` borra con warning, `trace prune --all --keep 2` deja 2×2 + reporte 85 sesiones (nota: rotó sesiones reales a últimas 2 — 168 kept — traces son estado local gitignored, receipts intactos). Tmp limpio. `spec OK: 3`. Nota: `task close` inexistente en 0.9.0, cierre manual.
