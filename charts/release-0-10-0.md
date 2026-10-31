# release-0-10-0 — bump + gauntlet + merge + tag

User authorization: "crea el release nuevo con todos los cambios" 2026-10-31. Contiene `50249aa` win-hardening, `5ee7d31` crap-paydown, `d355993` shell-LF, `172d5b6` retention-gc, `145365e` memory-hybrid, `9953490` toolchain-gates. Convención (de `release-0-9-0.md` L1): tags `vMAJOR.MINOR` en main al cerrar chart; bump en rama + rebuild + gauntlet, tag en main al merge. Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: 0.10.0 consistente, verde y tagueado en main con CI verde.
- Scope: `package.json` 0.9.0→0.10.0, `README.md` status, `CHANGELOG.md` entrada `## [0.10.0]`, rebuild stampleado (`npm run build`), `binaries/`+`bin/` (gitignored, artefactos), merge develop→main + tag `v0.10` + push. Sin cambios de código.
- Constraints: sin tocar gates/código; versión estampada vía ldflags; tag solo en main tras merge verde local.
- Acceptance: S1, S2, S3
- Artifacts: `bin/regulus` + `binaries/` stampleados 0.10.0, receipts `release-0-10-0/V1`.
- Dependencies: V1 → V2 → V3 en serie.
- Risk: CRITICAL (delivery: merge+tag+push a main) — requiere verifier + CI watch.
- Verification: `spec check` + `gate run release-0-10-0/V1` 12/12 + V2 independiente + CI verde en V3.
- Completion: V1–V3 CLOSED, `spec OK: 3`, CI main verde.

## Acceptance

### Scenario: S1 versión 0.10.0 consistente

- Given `package.json` en 0.9.0 y `develop` con los 6 commits de features
- When se sube a 0.10.0
- Then `package.json`, `README.md` status, `CHANGELOG.md` entrada 0.10.0, `bin/regulus` + `binaries/` stampleados 0.10.0 y `doctor` reporta `matches package.json`

### Scenario: S2 release gauntlet verde

- Given el árbol con versión 0.10.0
- When corre `bin/regulus gate run release-0-10-0/V1`
- Then 12/12 pass con receipts append-only

### Scenario: S3 merge a main + tag + CI

- Given `develop` verde y `origin/main` en su HEAD actual
- When se mergea a `main` y se taguea `v0.10`
- Then `main` avanza fast-forward o merge limpio, el push incluye tag, y el workflow gauntlet en main queda verde

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| V1 | S1–S2 | bump 0.10.0 (package.json, README, CHANGELOG) + rebuild stampleado + release gauntlet | @regulus-worker |
| V2 | S1–S2 | verificación independiente: versión match, binarios stampleados, receipts V1 existen y pasan | @regulus-verifier |
| V3 | S3 | merge a main + tag v0.10 + push (+CI watch) | lead |

## Log

- L1: Chart opened 2026-10-31 desde "crea el release nuevo". HEAD develop `9953490`, main `38e5d07` (35 commits behind). Precedente `release-0-9-0.md` (V1 worker bump+gauntlet, V2 lead merge+tag).
