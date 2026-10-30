# Regulus AI -- install regulus on PATH for Devin Desktop/CLI (D1: S1-S2).
#
# Usage: powershell -NoProfile -File scripts/install-devin.ps1
#
# What it does:
#   1. Builds bin/regulus from ./cmd/regulus, stamping main.version from
#      package.json:
#        go build -trimpath -ldflags "-X main.version=<package.json>" -o bin/regulus ./cmd/regulus
#   2. Installs regulus.exe into a directory already on PATH -- ~/bin when
#      ~/bin is on PATH (preferred), otherwise the first existing PATH
#      directory we can write to (fallback, reported on stdout). When no
#      PATH directory is writable, the binary is still left in ~/bin and
#      the script exits 1 telling the user to add ~/bin to PATH.
#   3. Verifies S1: `regulus contract` prints regulus.contract/v1 and
#      `regulus --version` reports the stamped version.
#   4. Verifies S2: `regulus mcp` answers an initialize request over stdio
#      with serverInfo.contract = regulus.contract/v1 (30s timeout).
#
# Pure PowerShell (no bash, no node). Exit 0 on success, 1 with a message
# on stderr otherwise. Requires Go on PATH.
#
# TODO (honest, no bash mirror): there is no scripts/install-devin.bash.
# A correct mirror is NOT trivial (more than 10 lines): the stdio
# initialize smoke with a 30s timeout plus .exe/PATH handling needs
# non-portable helpers (timeout/coproc availability differs between
# Git-Bash, Cloud and minimal containers), so a thin mirror would lie
# about covering "Git-Bash/Cloud". Git-Bash on Windows can run this file
# directly:
#   powershell.exe -NoProfile -File scripts/install-devin.ps1
# Linux/Cloud users without PowerShell can reproduce it with:
#   go build -trimpath -ldflags "-X main.version=$(node -p "require('./package.json').version")"
#     -o bin/regulus ./cmd/regulus
#   install -m755 bin/regulus ~/.local/bin/regulus   # ~/.local/bin on PATH
#   regulus contract          # want: regulus.contract/v1
#   printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
#     | timeout 30 regulus mcp | grep -q regulus.contract/v1

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

function Fail([string]$msg) {
  [Console]::Error.WriteLine("install-devin: $msg")
  exit 1
}

# 0. Go is required; fail loud instead of producing a stale binary.
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  Fail "Go toolchain not found on PATH. Install Go (https://go.dev/dl/) and re-run this script."
}

# 1. Build with the package.json version stamped in.
$pkg = Get-Content (Join-Path $root "package.json") -Raw | ConvertFrom-Json
$Version = $pkg.version
if ([string]::IsNullOrWhiteSpace($Version)) {
  Fail "could not read a version from package.json."
}
Write-Host "building bin/regulus (version ${Version})"
& go build -trimpath -ldflags "-X main.version=${Version}" -o "bin/regulus" ./cmd/regulus
if ($LASTEXITCODE -ne 0) {
  Fail "go build failed (exit ${LASTEXITCODE})."
}
# The -o target above is literal: on Windows `go build -o bin/regulus`
# writes extensionless bin/regulus (it does NOT append .exe), so prefer
# exactly that and only then fall back to bin/regulus.exe.
$built = $null
foreach ($candidate in @("bin/regulus", "bin/regulus.exe")) {
  if (Test-Path -LiteralPath (Join-Path $root $candidate) -PathType Leaf) {
    $built = Join-Path $root $candidate
    break
  }
}
if (-not $built) {
  Fail "go build succeeded but neither bin/regulus.exe nor bin/regulus exists."
}

# 2. Pick the install directory: prefer ~/bin when it is on PATH.
$preferred = Join-Path $HOME "bin"
$normPreferred = $preferred.TrimEnd('\', '/')
$pathDirs = ($env:PATH -split ';') | ForEach-Object { $_.Trim().TrimEnd('\', '/') } | Where-Object { $_ -ne '' }
$destDir = $null
if ($pathDirs -contains $normPreferred) {
  $destDir = $preferred
  Write-Host "install dir: ${destDir} (preferred ~/bin, on PATH)"
} else {
  # Documented fallback: first existing PATH directory we can write to.
  foreach ($d in $pathDirs) {
    if (-not (Test-Path -LiteralPath $d -PathType Container)) { continue }
    $probe = Join-Path $d ".regulus-write-test"
    try {
      New-Item -ItemType File -Path $probe -Force | Out-Null
      Remove-Item -LiteralPath $probe -Force
      $destDir = $d
      break
    } catch {
      continue
    }
  }
  if ($destDir) {
    Write-Host "install dir: ${destDir} (fallback: ~/bin not on PATH, first writable PATH dir)"
  } else {
    New-Item -ItemType Directory -Force -Path $preferred | Out-Null
    Copy-Item -LiteralPath $built -Destination (Join-Path $preferred "regulus.exe") -Force
    Fail "no writable directory on PATH. Binary left at $(Join-Path $preferred 'regulus.exe') - add ${preferred} to PATH and re-run."
  }
}
New-Item -ItemType Directory -Force -Path $destDir | Out-Null
$installed = Join-Path $destDir "regulus.exe"
Copy-Item -LiteralPath $built -Destination $installed -Force
Write-Host "installed: ${installed}"

# 3. S1: contract + stamped version, resolved via PATH like Devin would.
$onPath = Get-Command regulus -ErrorAction SilentlyContinue
if ($onPath -and $onPath.Source -ne $installed) {
  Write-Host "warning: PATH resolves regulus to $($onPath.Source), not ${installed} (shadowing install)"
}
$contractOut = (& $installed contract 2>&1) -join "`n"
$contractOut = $contractOut.Trim()
if ($contractOut -ne "regulus.contract/v1") {
  Fail "S1 contract check failed: regulus contract printed '$contractOut', want 'regulus.contract/v1'."
}
Write-Host "contract: ${contractOut}"
$versionOut = (& $installed --version 2>&1) -join "`n"
$versionOut = $versionOut.Trim()
if ($versionOut -ne "regulus ${Version}") {
  Fail "S1 version check failed: regulus --version printed '$versionOut', want 'regulus ${Version}'."
}
Write-Host "version: ${versionOut}"

# 4. S2: stdio initialize smoke - serverInfo.contract must be v1 (30s cap).
$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $installed
$psi.Arguments = "mcp"
$psi.RedirectStandardInput = $true
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.UseShellExecute = $false
$psi.CreateNoWindow = $true
$proc = New-Object System.Diagnostics.Process
$proc.StartInfo = $psi
[void]$proc.Start()
try {
  $init = '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"install-devin-smoke","version":"0"}}}'
  $proc.StandardInput.WriteLine($init)
  $proc.StandardInput.Flush()
  $read = $proc.StandardOutput.ReadLineAsync()
  if (-not $read.Wait(30000)) {
    Fail "S2 MCP smoke failed: no initialize response within 30s."
  }
  $line = $read.Result
  if ([string]::IsNullOrWhiteSpace($line)) {
    Fail "S2 MCP smoke failed: empty initialize response."
  }
  $gotContract = $null
  try {
    $gotContract = ($line | ConvertFrom-Json).result.serverInfo.contract
  } catch {
    $gotContract = $null
  }
  if ($gotContract -ne "regulus.contract/v1") {
    Fail "S2 MCP smoke failed: serverInfo.contract = '$gotContract', want 'regulus.contract/v1'. Raw: $line"
  }
  Write-Host "mcp initialize: serverInfo.contract = ${gotContract}"
} finally {
  try {
    if (-not $proc.HasExited) { $proc.Kill() }
  } catch {
  }
  $proc.Dispose()
}

Write-Host "done: regulus ${Version} on PATH (${installed}), contract + MCP initialize verified."
