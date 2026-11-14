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
