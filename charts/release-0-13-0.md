# release-0-13-0 — bump + gauntlet + merge + tag (merge/tag pendientes del dueño)

User authorization: "aplica el C3 con aumento en 0.13.0". Contiene: speed-tokens (T1–T10), fresh-fixes (F1–F10), regulus-100 (H1–H10), merge-ready (C1 paydown + C2 flake), bench gate exit, fix R3-001. Convención (release-0-12-0.md): bump en develop + rebuild + gauntlet, tag en main al merge, CI verde. Read down to `## Log` before working.

Class: STANDARD

Domain: sextant

## Contract

- Objective: 0.13.0 consistente y verde en develop, listo para merge + tag en main cuando el dueño lo pida.
- Scope: `package.json` 0.12.0→0.13.0, `README.md` status, `CHANGELOG.md` entrada `## [0.13.0]`, `versioninfo.json` + `cmd/regulus/regulus.manifest` 0.12.0.0→0.13.0.0, rebuild stampleado (`scripts/build.bash`), commit en develop. Sin cambios de conducta.
- Constraints: sin tocar gates/código salvo el bump; tag solo en main tras merge verde (pendiente del dueño, igual que el merge mismo).
- Acceptance: S1, S2 (S3 pendiente del dueño).
- Artifacts: `bin/regulus` + `binaries/` stampleados 0.13.0 (git-ignorados, no commiteados).
- Risk: standard
- Verification: `doctor` matches package.json + suite verde + CI en develop.

## Acceptance

### Scenario: S1

- Given `package.json` en 0.12.0 y el árbol con todo el batch en develop
- When se sube a 0.13.0 (versión + stamp + rebuild)
- Then `package.json`, `versioninfo.json`, `regulus.manifest`, `README.md`, `CHANGELOG.md` en 0.13.0 y `doctor` reporta `matches package.json`

### Scenario: S2

- Given el árbol con versión 0.13.0
- When corre la suite (`go ./...` + `bun lib/`) y el gauntlet en CI develop
- Then todo verde

### Scenario: S3 (pendiente del dueño)

- Given `develop` verde en 0.13.0
- When se mergea a `main`, se taguea `v0.13.0` y se pushea (+CI watch)
- Then `main` avanza limpio, el tag viaja y el workflow gauntlet en main queda verde

## Log

- L1: "aplica el C3 con aumento en 0.13.0"
- L2: merge develop->main 664a232 + tag v0.13.0 pusheados.
- L3: CI main verde (run 42569292270 success, serves 0.13.0). V1 bump consistente, V2 suite+gauntlet verdes, V3 CI main verde. Release CLOSED.
