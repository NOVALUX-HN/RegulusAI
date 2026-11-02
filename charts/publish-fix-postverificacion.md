# publish-fix-postverificacion — develop + merge a main

User authorization: "sube todos los cambios a develop y luego haz un merge a main" 2026-11-01. Publica F1/F2 (docs/charts, 5M+1U, 2×12/12 verdes) a `develop` y luego merge a `main` con push. Precedente `release-0-11-0.md` (V1 worker, V2 verifier, V3 lead). `develop==main==82f03ff` al abrir, árbol con F1/F2 sin commitear. Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: F1/F2 en `origin/develop` y `origin/main` con merge limpio y CI verde, sin tocar código prod.
- Scope: commit de `docs/dashboard.md`, `design.md`, `charts/release-0-11-0.md`, `charts/dashboard-pro.md`, `charts/dashboard-projects.md`, `charts/fix-postverificacion.md`. Solo `git` (checkout/commit/push/merge); ningún cambio de código.
- Constraints: no re-escribir historia; merge `develop→main` fast-forward o merge limpio; `task close` inexistente (cierre manual como en 0.11.0); no tag (no pedido).
- Acceptance: S1, S2, S3
- Artifacts: commits en develop + merge a main, receipts F1/F2 ya existentes (`run_34202881e61275b0`, `run_e878b5a22ae632f1`).
- Dependencies: S1 → S2 → S3 en serie.
- Risk: critical
- Verifier: S2
- Verification: `spec check` + `git status` limpio + `develop==origin/develop`, `main==origin/main` tras push + CI verde en S3.
- Completion: S1–S3 CLOSED, push verificado.

## Acceptance

### Scenario: S1

- Given árbol en `main 82f03ff` con F1/F2 sin commitear (5M+1U) y `develop==main`
- When se commitea todo a `develop` y se pushea a `origin/develop`
- Then `develop` avanza 1 commit limpio, `git status` limpio y `develop==origin/develop`

### Scenario: S2

- Given S1 pusheado a `origin/develop`
- When el verificador comprueba diff solo-docs, `spec OK` y F1/F2 12/12 vigentes
- Then da PASS independiente sin tocar código

### Scenario: S3

- Given S2 PASS y `develop` verde
- When se mergea `develop→main` y se pushea `main` (+CI watch)
- Then `main==develop==origin/main`, árbol limpio y workflow gauntlet en main verde

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| V1 | S1 | commit F1/F2 a develop + push origin/develop | @regulus-worker | open |
| V2 | S2 | verificación independiente: diff solo-docs + spec + receipts | @regulus-verifier | open |
| V3 | S3 | merge develop→main + push + CI watch — ejecuta lead inline | inline | open |

## Log

- L1: Chart opened 2026-11-01 (F1/F2 done 2×12/12, 5M+1U en `main`, `develop==main==82f03ff`). Sin tag.
