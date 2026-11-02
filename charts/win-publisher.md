# win-publisher — que Windows identifique los EXEs de Regulus

User authorization: "el sistema de seguridad de windows esta detectando algunos ejecutables de go porque no puede confirmar quien publico esos exe. puedes revisar y corregir para que windows no detecte nada de regulus como amenaza" 2026-11-02. Evidencia: `bin/regulus.exe`, `binaries/regulus-windows-amd64.exe`, `binaries/regulus-windows-arm64.exe` → `Get-AuthenticodeSignature: NotSigned` (3/3); cero `signtool/pfx/VersionInfo/manifest/rsrc` en el repo; builds en `scripts/build.ps1:47,63`, `scripts/build.bash:55,64-66`, `regulus.json:9`; EXEs no trackeados (solo `bin/regulus.js`), se construyen local. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: los EXEs muestran identidad NOVALUX-HN / Regulus AI en Propiedades (ya no blanks "desconocido") y hay guía para permitirlos; no se promete firma Authenticode (imposible sin certificado público, se declara).
- Scope: `versioninfo.json` nuevo, `cmd/regulus/regulus.manifest` nuevo, `scripts/build.ps1` + `scripts/build.bash` (generar `.syso` al compilar), `.gitignore` (`*.syso`), `docs/troubleshooting.md` (sección Windows). Sin cambiar conducta de gates ni del plugin.
- Constraints: sin certificado no hay `Signed` — `Get-AuthenticodeSignature` seguirá `NotSigned`, se documenta; sin blobs binarios en git (`.syso` se genera, no se commitea); sin nuevas dependencias en `go.mod` (goversioninfo vía `go run`, solo en builds Windows); CI ubuntu intacto (syso solo afecta `GOOS=windows`).
- Acceptance: T1, T2
- Artifacts: receipts `win-publisher/T*` (12/12 por tarea).
- Dependencies: T1 y T2 independientes.
- Risk: standard
- Verification: `spec check` + `gate run win-publisher/T*` 12/12 + `Get-AuthenticodeSignature` (NotSigned esperado) + Propiedades muestra Company/Product + `doctor` matches.
- Completion: T1–T2 CLOSED vía `task close`.

## Acceptance

### Scenario: T1

- Given `go build` emite EXEs sin metadata (`NotSigned` 3/3, Propiedades sin Company/Product) desde `scripts/build.ps1:47,63` y `scripts/build.bash:55,64-66`
- When el build genera `resource_windows_*.syso` desde `versioninfo.json` (CompanyName NOVALUX-HN, ProductName Regulus AI) + manifest `asInvoker`
- Then `bin/regulus.exe` y `binaries/regulus-windows-*.exe` muestran Company/Product en Propiedades, `go vet`+gates verdes y ningún `.syso` queda trackeado

### Scenario: T2

- Given Windows avisa por publisher no firmado y no hay guía en `docs/` (cero-hits `Defender|SmartScreen|Unblock-File`)
- When `docs/troubleshooting.md` explica causa (sin cert público), qué ver en Propiedades y cómo permitir (Unblock-File si hay MOTW, exclusión Defender, `go run` como alternativa)
- Then un usuario con el aviso resuelve en <5 min siguiendo la guía y el doc declara el límite honesto (firma completa = certificado público, no incluido)

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| T1 | T1 | VersionInfo+manifest: json + manifest + wiring build scripts + gitignore | @regulus-worker | done |
| T2 | T2 | docs: sección Windows en troubleshooting (causa, permitir, límite) | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-02 (scout ses_e66167feeffeB1676ETZKyz1ep + shell: 3/3 NotSigned, git ls-files solo `bin/regulus.js`, build.ps1 leído). Límite: sin cert público no hay `Signed`; se fija identidad, no confianza.
- CLOSED T1 STANDARD spec OK + 12 receipt(s) PASS (run_7680d19fa6bfa476). VersionInfo NOVALUX-HN/Regulus AI 0.11.0.0 en 3 EXEs, manifest asInvoker, `*.syso` ignorados, doctor matches. Nota: 1er intento cayó en unit-tests por timeout infra (exit −1, cero fallos) → re-run 311s PASS.
- CLOSED T2 STANDARD spec OK + 12 receipt(s) PASS (run_82c2664e30049d53). Sección Windows en troubleshooting +41 líneas. Mismo patrón timeout→re-run en unit-tests.
- L2: verifier 2/5 directo + lead completa 3/5 con shell (VersionInfo CompanyName/ProductName CONFIRMADO, NotSigned 3/3 esperado, syso no trackeados). Deuda: `versioninfo.json`/`regulus.manifest` hardcodean `0.11.0.0` — sincronizar en cada bump (aviso en scripts).
