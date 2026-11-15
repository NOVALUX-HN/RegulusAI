# Instalación, versionado y distribución (NOV-36 / T22)

Canales soportados:
- `scripts/build.bash` + `scripts/build.ps1`: cross 6 targets a `binaries/regulus-*` + `bin/regulus(.exe)`.
- `scripts/install-devin.ps1` / `.bash`: build stamped `package.json -> -X main.version`, copia a `~/bin|~/.local/bin`, verifica S1 `contract==regulus.contract/v1`, S2 `mcp initialize serverInfo.contract`, S3/S4 `devin skills/mcp add`.
- `bin/regulus.js:12-` launcher npm; `scripts/sign-local.ps1` autofirmado CurrentUser (solo SmartScreen local, no distribución).
- `scripts/install-dashboard-autostart.*`: autostart dashboard aparte.

Versionado:
- Fuente: `package.json:3` `0.13.0`; `versioninfo.json` Fixed 0.13.0 + String 0.13.0.0; `cmd/regulus/regulus.manifest:5` `0.13.0.0 asInvoker`.
- Pines: `internal/contract/contract.go:7` `regulus.contract/v1`, `internal/mcp/version.go:7` `regulus.mcp/v1`.
- `internal/doctor/version.go:12-` `VersionLine` compara stamped vs `package.json`, `dev` advierte rebuild.

Limpia / update / rollback / diagnóstico:
- Limpia: build + install-devin + `regulus init --stack/--harness-from` (E5/F9 CHANGELOG v0.6/v0.7).
- Update: re-run build + install-devin; `doctor version` verifica stamp.
- Rollback: reinstalar tag previo desde `binaries/`; `regulus.json` gates/allowlist sin versión, no migra.
- Diagnóstico: `regulus doctor`, `GET /v1/git` branch/HEAD, `GET /v1/metrics` receipts/pass.

Smoke: `go test ./internal/doctor ./internal/contract ./internal/mcp -count=1` + `regulus mcp` initialize/tools/list + install-devin S1-S4.

## Releases verificables (NOV-70 / T51)

- Tag `v*` en main dispara `.github/workflows/release.yml`: matriz 6 targets (linux/darwin/windows × amd64/arm64) + `checksums.txt` SHA-256 + attestación de procedencia SLSA + Release con notas generadas de `CHANGELOG.md`.
- `verify-published` descarga el Release y verifica: checksums, `regulus contract`, `regulus doctor`, journey mínimo (init → spec → gate --yes → close). Windows queda explícitamente `NotSigned` (NOV-44: firma opcional, sin autofirmados como confianza).
- Launcher npm (`bin/regulus.js` + `bin/verify-hash.js`): verifica el binario resuelto contra `checksums.txt` empaquetado y **falla cerrado** ante hash alterado. Binarios dev (`go build` local, sin pin) corren sin verificación. `npm run test:launcher` (`node --test bin/`) prueba: pin válido, tamper rechazado, dev sin pin, sin checksums.
- Instalación en limpio Linux + Windows siguiendo solo la guía, verificada en CI.

## Política de versión y tags

- Fuente única: `package.json` (`version` X.Y.Z). `versioninfo.json`, `regulus.manifest` y el head de `CHANGELOG.md` la siguen (`regulus manifest check` lo impone + gate docs-drift).
- Tags `vX.Y.Z` se cortan en `main` al hacer merge; nunca en `develop`. El gate de deriva documental falla si las cifras no coinciden.
