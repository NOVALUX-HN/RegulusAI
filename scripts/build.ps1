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
