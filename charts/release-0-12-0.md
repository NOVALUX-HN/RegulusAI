# release-0-12-0 — bump + gauntlet + merge + tag

User authorization: "sube los cambios y actualiza la versión de Regulus para probarlo en opencode" 2026-11-02. Contiene: win-publisher (VersionInfo+manifest), win-localtrust (sign-local), dashboard-ux (U1/U2), dashboard-vision (V1–V4 + endpoints), learn-backfill (2 lecciones), README con 6 imágenes + sección Dashboard. Convención (`release-0-11-0.md` L1/L2): bump en rama + rebuild + gauntlet, tag en main al merge, CI verde. Read down to `## Log` before working.

Class: CRITICAL

Domain: sextant

## Contract

- Objective: 0.12.0 consistente, verde y tagueado en main con CI verde, listo para probar en opencode (plugin usa el binario nuevo sin reload).
- Scope: `package.json` 0.11.0→0.12.0, `README.md` status, `CHANGELOG.md` entrada `## [0.12.0]`, `versioninfo.json` + `cmd/regulus/regulus.manifest` 0.11.0.0→0.12.0.0 (+ regenerar `.syso`), rebuild stampleado (`scripts/build.ps1`), commit de todo el árbol pendiente, merge develop→main + tag `v0.12.0` + push. Sin cambios de conducta.
- Constraints: sin tocar gates/código salvo el bump; `task close` inexistente (cierre manual como en 0.11.0); tag solo en main tras merge verde local.
- Acceptance: S1, S2, S3
- Artifacts: `bin/regulus.exe` + `binaries/` stampleados 0.12.0, receipts `release-0-12-0/V1`.
- Dependencies: V1 → V2 → V3 en serie.
- Risk: critical
- Verifier: V2
- Verification: `spec check` + `gate run release-0-12-0/V1` 12/12 + V2 independiente + CI verde en V3 + `doctor` matches + smoke del dashboard nuevo.
- Completion: V1–V3 CLOSED, `spec OK: 3`, CI main verde.

## Acceptance

### Scenario: S1

- Given `package.json` en 0.11.0 y el árbol con todo el batch sin commitear
- When se sube a 0.12.0 (versión + stamp + rebuild)
- Then `package.json`, `versioninfo.json`, `regulus.manifest`, `README.md`, `CHANGELOG.md` en 0.12.0, binarios stampleados y `doctor` reporta `matches package.json`

### Scenario: S2

- Given el árbol con versión 0.12.0 (release gauntlet verde)
- When corre `gate run release-0-12-0/V1`
- Then 12/12 pass con receipts append-only

### Scenario: S3

- Given `develop` verde y V2 PASS
- When se mergea a `main`, se taguea `v0.12.0` y se pushea (+CI watch)
- Then `main` avanza limpio, el tag viaja y el workflow gauntlet en main queda verde

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| V1 | S1–S2 | bump 0.12.0 + versioninfo/manifest + rebuild + commit batch + release gauntlet | @regulus-worker | open |
| V2 | S1–S2 | verificación independiente: versión match, receipts V1 pasan, endpoints nuevos vivos | @regulus-verifier | done |
| V3 | S3 | merge develop→main + tag v0.12.0 + push + CI watch — ejecuta lead inline | inline | open |

## Log

- L1: Chart opened 2026-11-02 (batch: win-publisher, win-localtrust, dashboard-ux, dashboard-vision V1-V4, learn-backfill, README imágenes; `develop==main==3af4fba`, árbol dirty). Deuda a saldar: version hardcodeada 0.11.0.0 en versioninfo/manifest.
