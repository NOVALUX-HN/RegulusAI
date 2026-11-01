# release-0-11-0 — bump + gauntlet + merge + tag

User authorization: "ayúdame a actualizar la rama y crea un nuevo release" 2026-11-01. Contiene `940e2be..61490c2`: programa macizo 16/16 (memory/flow/tokens/dash), `7f770f3` regulus-health-fix H1–H7, `04acbd6` close-sobre-último-manifest C1, `ab8f681` design.md canónico, `7e21082` dashboard-pro D1–D3, `61490c2` dashboard-projects P1–P4. Convención (`release-0-10-0.md` L1/L2): bump en rama + rebuild + gauntlet, tag `v0.11.0` en main al merge, CI verde. Rama ya al día (`develop == origin/develop == 61490c2`, árbol limpio). Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: 0.11.0 consistente, verde y tagueado en main con CI verde.
- Scope: `package.json` 0.10.0→0.11.0, `README.md` status, `CHANGELOG.md` entrada `## [0.11.0]`, rebuild stampleado (`npm run build` / `scripts/build.ps1`), `binaries/`+`bin/` (gitignored, artefactos), merge develop→main + tag `v0.11.0` + push. Sin cambios de código.
- Constraints: sin tocar gates/código; versión estampada; tag solo en main tras merge verde local; `task close` inexistente en este flujo (cierre manual como en 0.10.0).
- Acceptance: S1, S2, S3
- Artifacts: `bin/regulus.exe` + `binaries/` stampleados 0.11.0, receipts `release-0-11-0/V1`.
- Dependencies: V1 → V2 → V3 en serie.
- Risk: critical
- Verifier: V2
- Verification: `spec check` + `gate run release-0-11-0/V1` 12/12 + V2 independiente + CI verde en V3.
- Completion: V1–V3 CLOSED, `spec OK: 3`, CI main verde.

## Acceptance

### Scenario: S1

- Given `package.json` en 0.10.0 y `develop` en `61490c2` con el batch (versión 0.11.0 consistente)

- Given `package.json` en 0.10.0 y `develop` en `61490c2` con el batch de arriba
- When se sube a 0.11.0
- Then `package.json`, `README.md` status, `CHANGELOG.md` entrada 0.11.0, `bin/regulus.exe` + `binaries/` stampleados 0.11.0 y `doctor` reporta `matches package.json`

### Scenario: S2

- Given el árbol con versión 0.11.0 (release gauntlet verde)

- Given el árbol con versión 0.11.0
- When corre `gate run release-0-11-0/V1`
- Then 12/12 pass con receipts append-only

### Scenario: S3

- Given `develop` verde y `origin/main` en `1ca0a0a` (merge a main + tag + CI)

- Given `develop` verde y `origin/main` en `1ca0a0a`
- When se mergea a `main` y se taguea `v0.11.0`
- Then `main` avanza con merge limpio, el push incluye tag, y el workflow gauntlet en main queda verde

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| V1 | S1–S2 | bump 0.11.0 (package.json, README, CHANGELOG) + rebuild stampleado + release gauntlet | @regulus-worker | done |
| V2 | S1–S2 | verificación independiente: versión match, binarios stampleados, receipts V1 existen y pasan | @regulus-verifier | done |
| V3 | S3 | merge a main + tag v0.11.0 + push (+CI watch) — ejecuta lead inline | inline | done |

## Log

- L1: Chart opened 2026-11-01 desde "actualiza la rama y crea un nuevo release". develop == origin/develop == 61490c2, main == origin/main == 1ca0a0a, árbol limpio. Precedente release-0-10-0.md (V1 worker, V2 verifier, V3 lead; tag práctica v0.10.0).
- CLOSED V1 CRITICAL spec OK + 12 receipt(s) PASS.
- CLOSED V2 CRITICAL spec OK + 0 receipt(s) PASS.
- CLOSED V3 CRITICAL spec OK + 0 receipt(s) PASS.
- L2: CIERRE MANUAL (precedente 0.9.0/0.10.0): V1 12/12 run_6b0280b140040874 + rebuild 7/7 sellado; V2 verifier PASS + bytes 0.11.0 verificados; V3 ff 1ca0a0a..6e733d7 + tag v0.11.0 + push; CI 41345305422 rojo (exit 99, node filtrado al PATH) -> fix 6de0b26 -> CI 41348359270 SUCCESS. Tag en 6e733d7; reintento local BLOCKED host-side, no regresion.
