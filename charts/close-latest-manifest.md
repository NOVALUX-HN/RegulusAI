# close-latest-manifest — el cierre verifica el último manifest verde

User authorization: "resuelve lo pendiente hasta que funcione" 2026-11-01 (H2 `task close` bloqueado por 1 receipt FAIL histórico legítimo; precedente documentado exige mejora: close sobre último manifest verde). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: `task close` verifica el último manifest del task (sus artefactos listados, hash-verificados, todo PASS) en vez de todo el historial; el historial fail→fix→pass sigue en disco y auditable.
- Scope: `internal/sextant/close.go` (`checkEvidence`/`taskEvidence`), `internal/evidence/*` solo lectura (nuevo helper si hace falta), `internal/sextant/close*test.go`, docs de cierre si existen.
- Constraints: append-only se preserva (ningún receipt se borra/mueve; cuarentena intacta); manifests viejos siguen verificables por `evidence verify`; un task sin manifest verde nunca cierra; el cambio no toca gauntlet, probes ni Almanac.
- Acceptance: C1
- Artifacts: receipts `close-latest-manifest/C1` (12/12).
- Dependencies: ninguna (H2 ya verde en árbol final; su cierre formal es la prueba de aceptación).
- Risk: complex
- Verification: `spec check` + `gate run close-latest-manifest/C1` 12/12 + @regulus-verifier reproduce el cierre formal de H2 + @regulus-reviewer valida la candidata.
- Completion: C1 CLOSED vía `task close`.

## Acceptance

### Scenario: C1

- Given `checkEvidence` en `internal/sextant/close.go:103-124` lista TODO el dir del task y falla si cualquier receipt histórico tiene `Verdict != pass` (H2 bloqueado por `crap-1793515190150000100.json` exit 1 aunque los 2 runs posteriores son 12/12 PASS con manifests `run_aac463bc45eff6db` y `run_1796cdacddb07ede`)
- When corre `task close charts/regulus-health-fix.md H2` tras el cambio
- Then el cierre verifica solo el último manifest (el de `run_1796cdacddb07ede`, 12 artefactos PASS hash-verificados), CLOSED formal con 12 receipt(s) PASS, y el receipt FAIL histórico sigue en disco y visible en `evidence list`

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|--------|
| C1 | C1 | close sobre último manifest + H2 formal | @regulus-worker | done |
| V1 | C1 | reproducción independiente del cierre H2 | @regulus-verifier | done |

## Log

- L1: Chart opened 2026-11-01 (pendiente: H2 `task close` exit 1 por `crap-1793515190150000100.json` histórico; `evidence verify --task regulus-health-fix/H2` 31/31 passed en 3 manifests; precedente M3/gap-sweep pide close-sobre-último-manifest).

- CLOSED C1 COMPLEX spec OK + 12 receipt(s) PASS (run_5fbf2d9eb48583d9).