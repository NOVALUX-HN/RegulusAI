# Regulus AI — build the release binaries with version stamping.
#
# Usage: powershell -File scripts/build.ps1 [-Version <version>]
#   Version defaults to the package.json version.
#
# Windows mirror of scripts/build.bash: cross-compiles the regulus core
# for the six supported platforms into binaries/ (distributed by the npm
# package; the launcher resolves the right one). The version is stamped
# via ldflags. Pure PowerShell — no bash, no node dependency.

param(
  [string]$Version = ""
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

if ([string]::IsNullOrWhiteSpace($Version)) {
  $pkg = Get-Content (Join-Path $root "package.json") -Raw | ConvertFrom-Json
  $Version = $pkg.version
}
$stamp = "main.version=$Version"

$targets = @(
  "darwin/arm64",
  "darwin/amd64",
  "linux/arm64",
  "linux/amd64",
  "windows/arm64",
  "windows/amd64"
)

New-Item -ItemType Directory -Force -Path (Join-Path $root "binaries") | Out-Null
$env:CGO_ENABLED = "0"

# win-publisher T1: stamp Windows EXEs with Company/Product metadata.
# goversioninfo runs via `go run` pinned (@v1.4.0) — no go.mod dependency,
# no blobs in git (*.syso is git-ignored and regenerated every build).
# The .syso files only affect GOOS=windows builds; other targets ignore them.
# If `go run <pkg>@<version>` cannot resolve (offline), fallback:
#   go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0
# then `goversioninfo -64 ...` / `goversioninfo -arm -64 ...` with the same args.
# NOTE: keep versioninfo.json FileVersion/ProductVersion in sync with package.json
# (0.11.0 -> 0.11.0.0) on every version bump.
Write-Host "generating Windows version resources (goversioninfo v1.4.0)"
& go run "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0" -64 -o (Join-Path $root "cmd/regulus/resource_windows_amd64.syso") (Join-Path $root "versioninfo.json")
if ($LASTEXITCODE -ne 0) {
  throw "goversioninfo failed for windows/amd64 (exit ${LASTEXITCODE})"
}
& go run "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.4.0" -arm -64 -o (Join-Path $root "cmd/regulus/resource_windows_arm64.syso") (Join-Path $root "versioninfo.json")
if ($LASTEXITCODE -ne 0) {
  throw "goversioninfo failed for windows/arm64 (exit ${LASTEXITCODE})"
}
foreach ($target in $targets) {
  $os, $arch = $target.Split("/")
  $ext = ""
  if ($os -eq "windows") {
    $ext = ".exe"
  }
  $name = "regulus-${os}-${arch}${ext}"
  Write-Host "building binaries/${name} (${Version})"
  $env:GOOS = $os
  $env:GOARCH = $arch
  & go build -trimpath -ldflags "-s -w -X ${stamp}" -o "binaries/${name}" ./cmd/regulus
  if ($LASTEXITCODE -ne 0) {
    throw "go build failed for ${target} (exit ${LASTEXITCODE})"
  }
}

$count = (Get-ChildItem (Join-Path $root "binaries")).Count
Write-Host "done: ${count} binaries in binaries/ (version ${Version})"

# H3 (S4): the documented flow also refreshes the local binary, stamped
# with the same version. A raw `go build` stays unstamped by design
# (doctor warns); `npm run build` never is. Clear the cross-compile
# GOOS/GOARCH first so the local binary matches the host.
Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Write-Host "building bin/regulus.exe (${Version})"
& go build -trimpath -ldflags "-s -w -X ${stamp}" -o (Join-Path $root "bin/regulus.exe") ./cmd/regulus
if ($LASTEXITCODE -ne 0) {
  throw "go build failed for the local binary (exit ${LASTEXITCODE})"
}
Write-Host "local: bin/regulus.exe (version ${Version})"
