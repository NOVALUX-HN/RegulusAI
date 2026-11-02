# win-localtrust — firma local autofirmada para los EXEs

User authorization: "mejor resolveremos de forma local ese problema de la seguridad, al menos mientras encontramos otra solucion totalmente gratuita y open source" 2026-11-02. Supera a win-publisher (T1/T2 CLOSED): sin cert público no hay `Signed` global; esto deja `Signed` real en las PCs del equipo. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: en las PCs del equipo, `bin/regulus.exe` y `binaries/regulus-windows-*.exe` pasan de `NotSigned` a `Valid` (firmado y confiable) con cert local NOVALUX-HN, sin comprar nada y sin tocar el build por defecto.
- Scope: `scripts/sign-local.ps1` nuevo (genera cert code-signing autofirmado en CurrentUser\My, lo instala en Root+TrustedPublisher, firma con signtool, verifica), `docs/troubleshooting.md` (sección firma local: uso + límite), `.gitignore` si hiciera falta. Sin cambiar `build.ps1/b.bash`, gates, plugin ni CI.
- Constraints: solo CurrentUser (nunca LocalMachine sin pedirlo); script idempotente (re-ejecutable); si no hay SDK de Windows (signtool ausente) falla con mensaje claro, no a medias; límite honesto: vale solo donde el cert esté instalado.
- Acceptance: L1, L2
- Artifacts: receipts `win-localtrust/L*` (12/12 por tarea).
- Dependencies: L1 antes que L2 (la doc describe el script).
- Risk: standard
- Verification: `spec check` + `gate run win-localtrust/L*` 12/12 + `Get-AuthenticodeSignature → Signed` en los 3 EXEs tras correr el script + `doctor` matches.
- Completion: L1–L2 CLOSED vía `task close`.

## Acceptance

### Scenario: L1

- Given los 3 EXEs en `NotSigned` y ningún cert local
- When se corre `powershell -File scripts/sign-local.ps1`
- Then el cert NOVALUX-HN existe en CurrentUser, los 3 EXEs quedan `Signed` con ese CN y re-correr es no-op

### Scenario: L2

- Given L1 funciona en esta PC
- When se lee `docs/troubleshooting.md`
- Then la sección firma local explica uso (un comando), alcance (solo PCs con el cert) y límite (no equivale a cert público), sin mentir

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| L1 | L1 | script sign-local.ps1 idempotente + firmado verificado | @regulus-worker | done |
| L2 | L2 | docs troubleshooting: firma local (uso, alcance, límite) | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-02 (win-publisher dejó identidad sin firma; SignPath aparcado por el usuario; vía local mientras aparece solución gratis+open).
- CLOSED L1 STANDARD spec OK + 12 receipt(s) PASS: sign-local.ps1 idempotente (CurrentUser, sin signtool aquí → fail claro verificado + happy path simulado Valid).
- CLOSED L2 STANDARD spec OK + 12 receipt(s) PASS: sección firma local en troubleshooting (uso, alcance, límite honesto).
