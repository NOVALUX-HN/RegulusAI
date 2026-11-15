# Regulus AI — suite completa en Windows (NOV-66, paso 48).
#
# Compila CADA paquete de test a un .exe dentro del workspace
# (.regulus-test/, nunca %TEMP%) y lo ejecuta, porque App Control
# bloquea los binarios temporales de `go test` (ver
# docs/windows-diagnosis.md: eventos 3077/3033/3118). Si un binario
# resulta bloqueado igual, el script imprime el diagnóstico accionable
# en vez de un "permission denied" mudo y sigue con el resto.
#
# Uso: powershell -NoProfile -File scripts/test-windows.ps1 [-Packages "a b"] [-KeepBinaries]
param(
  [string]$Packages = "./internal/...",
  [switch]$KeepBinaries
)

$ErrorActionPreference = "Continue"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$outDir = Join-Path $root ".regulus-test"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$env:GOTMPDIR = Join-Path $root ".regulus-test-cache"
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null

function Test-IsAppControlBlock($text) {
  return ($text -match "Control de aplicaciones" -or $text -match "blocked this file")
}

# Paquetes con pruebas Go (los que tienen *_test.go).
$pkgs = & go list @($Packages -split '\s+' | Where-Object { $_ -ne '' }) 2>$null | Where-Object { $_ -like "regulus-ai/internal/*" }
if (-not $pkgs) { throw "go list devolvió vacío para $Packages" }

$failed = @()
$blocked = @()
$passed = 0
foreach ($pkg in $pkgs) {
  $name = ($pkg -replace "regulus-ai/internal/", "") -replace "/", "."
  if ([string]::IsNullOrEmpty($name)) { $name = "root" }
  $exe = Join-Path $outDir ("test-" + $name + ".exe")
  $build = & go test -c -o $exe $pkg 2>&1
  if ($LASTEXITCODE -ne 0) {
    # Sin tests compilables no hay nada que correr (p. ej. paquetes solo-biblioteca).
    if ($build -match "no test files") { continue }
    $failed += "${pkg}: COMPILE FAIL :: ${build}"
    continue
  }
  # Nota: los args van por splatting (@(...)): PowerShell parte
  # "-test.count=1" en "-test" + ".count=1" si se pasa directo.
  $run = & $exe @('-test.count=1') 2>&1
  $code = $LASTEXITCODE
  $text = ($run -join "`n")
  if ($code -ne 0 -and (Test-IsAppControlBlock $text)) {
    $blocked += $pkg
    Write-Host "BLOCKED (App Control): $pkg"
    Write-Host "  Diagnóstico: CodeIntegrity Operational, eventos 3033/3077/3089/3118 con ruta, hash y firmante."
    Write-Host "  Ver docs/windows-diagnosis.md. Reintentar en ventana limpia o validar en WSL: wsl go test $pkg"
    continue
  }
  if ($code -ne 0) {
    $failed += "${pkg}: FAIL :: ${text}"
    continue
  }
  $passed++
  Write-Host "PASS: $pkg"
}

if (-not $KeepBinaries) { Remove-Item (Join-Path $outDir "test-*.exe") -ErrorAction SilentlyContinue }

Write-Host ""
Write-Host "windows-suite: ${passed} paquetes PASS, $($failed.Count) FAIL, $($blocked.Count) bloqueados por App Control"
foreach ($f in $failed) { Write-Host "FAIL: $f" }
foreach ($b in $blocked) { Write-Host "BLOCKED: $b" }
# Los bloqueos de App Control no son rojos funcionales: se reportan
# aparte para el laboratorio (paso 48 exige distinguirlos). El exit
# solo falla por fallos reales.
if ($failed.Count -gt 0) { exit 1 }
exit 0
