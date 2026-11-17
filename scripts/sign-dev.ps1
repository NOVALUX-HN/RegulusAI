# Regulus AI — firma de binarios de dev con el cert local (paso previo a la
# suplementaria WDAC por publisher).
#
# Usage:
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/sign-dev.ps1
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/sign-dev.ps1 -Paths ".regulus-test/test-bench.exe"
#
# Reutiliza el mismo cert que scripts/sign-local.ps1 (CN "NOVALUX-HN Local",
# thumbprint en .regulus/localtrust-thumbprint.txt); si no existe lo crea e
# instala la confianza solo-CurrentUser (Root + TrustedPublisher). Si
# signtool.exe no esta instalado falla ANTES de crear nada (exit 1).
#
# Por defecto firma lo que exista de: bin/regulus.exe,
# .regulus-test/test-*.exe, .regulus/*.test.exe. Los binarios de test son
# efimeros (hash nuevo por build): hay que re-firmar despues de cada
# `go test -c`. La firma sola NO desbloquea Device Guard/WDAC (ver
# docs/troubleshooting.md L2): el desbloqueo real es la politica
# suplementaria por publisher (tarea siguiente), que confia en este cert.
param(
  [string[]]$Paths = @()
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

$subject = "CN=NOVALUX-HN Local"
$thumbFile = Join-Path $root ".regulus/localtrust-thumbprint.txt"

function Find-SignTool {
  $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $kitRoots = @()
  if ($Env:ProgramFiles) { $kitRoots += (Join-Path $Env:ProgramFiles "Windows Kits") }
  if (${Env:ProgramFiles(x86)}) { $kitRoots += (Join-Path ${Env:ProgramFiles(x86)} "Windows Kits") }
  foreach ($kr in $kitRoots) {
    foreach ($kit in @("10", "11")) {
      $binDir = Join-Path $kr "$kit/bin"
      if (Test-Path $binDir) {
        $found = Get-ChildItem -Path $binDir -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
          Where-Object { $_.FullName -match "\\x64\\" } |
          Sort-Object FullName -Descending |
          Select-Object -First 1
        if ($found) { return $found.FullName }
      }
    }
  }
  return $null
}

function Find-LocalCert {
  param([string]$SavedThumbprint)
  if ($SavedThumbprint) {
    $saved = Get-ChildItem "Cert:\CurrentUser\My\$SavedThumbprint" -ErrorAction SilentlyContinue
    if ($saved -and $saved.NotAfter -gt (Get-Date)) { return $saved }
  }
  $bySubject = Get-ChildItem Cert:\CurrentUser\My -ErrorAction SilentlyContinue |
    Where-Object { $_.Subject -eq $subject -and $_.NotAfter -gt (Get-Date) } |
    Sort-Object NotAfter -Descending |
    Select-Object -First 1
  return $bySubject
}

# 0. Objetivos: explícitos o globs de dev por defecto (solo lo que exista).
$targets = @()
if ($Paths -and $Paths.Count -gt 0) {
  foreach ($p in $Paths) { $targets += $p }
}
else {
  $targets += @("bin/regulus.exe")
  foreach ($g in @(".regulus-test/test-*.exe", ".regulus/*.test.exe")) {
    foreach ($m in (Get-ChildItem $g -ErrorAction SilentlyContinue)) { $targets += $m.FullName }
  }
}
$existing = @()
foreach ($t in $targets) {
  $full = $t
  if (-not [System.IO.Path]::IsPathRooted($full)) { $full = Join-Path $root $t }
  if (Test-Path $full) { $existing += $full }
  else { Write-Warning "no existe ${t}; se omite" }
}
if ($existing.Count -eq 0) {
  Write-Host "nada para firmar (sin binarios existentes). Compila primero, p. ej. go test -c -o .regulus-test/test-bench.exe ./internal/bench"
  exit 0
}

# 1. signtool PRIMERO: si falta, exit 1 sin crear nada.
$signtool = Find-SignTool
if (-not $signtool) {
  Write-Error "signtool.exe no encontrado: instala Windows SDK o VS Build Tools (componente 'Windows SDK') y vuelve a correr este script. Sin cambios: no se creo ni instalo ningun certificado."
  exit 1
}
Write-Host "signtool: ${signtool}"

# 2. Cert (reutiliza sign-local) + 3. confianza solo-CurrentUser.
$savedThumb = $null
if (Test-Path $thumbFile) {
  $savedThumb = ((Get-Content $thumbFile -Raw -ErrorAction SilentlyContinue) | Out-String).Trim()
}
$cert = Find-LocalCert -SavedThumbprint $savedThumb
if ($cert) {
  Write-Host "cert reutilizado: $($cert.Subject) ($($cert.Thumbprint))"
}
else {
  $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject $subject `
    -CertStoreLocation "Cert:\CurrentUser\My" `
    -NotAfter (Get-Date).AddYears(5) `
    -FriendlyName "NOVALUX-HN Local (regulus dev signing)"
  Write-Host "cert creado: $($cert.Subject) ($($cert.Thumbprint))"
}
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $thumbFile) | Out-Null
Set-Content -Path $thumbFile -Value $cert.Thumbprint -NoNewline

foreach ($storeName in @("Root", "TrustedPublisher")) {
  $present = Get-ChildItem "Cert:\CurrentUser\$storeName\$($cert.Thumbprint)" -ErrorAction SilentlyContinue
  if ($present) {
    Write-Host "confianza OK: CurrentUser\$storeName ya contiene el cert"
  }
  else {
    $store = New-Object System.Security.Cryptography.X509Certificates.X509Store($storeName, "CurrentUser")
    $store.Open("ReadWrite")
    try {
      $store.Add($cert)
    }
    finally {
      $store.Close()
    }
    Write-Host "confianza instalada: CurrentUser\$storeName"
  }
}

# 4. Firma + 5. verificacion (Get-AuthenticodeSignature -> Valid).
$rows = @()
$failed = @()
foreach ($full in $existing) {
  $before = Get-AuthenticodeSignature $full
  if ($before.Status -eq "Valid" -and $before.SignerCertificate -and $before.SignerCertificate.Thumbprint -eq $cert.Thumbprint) {
    Write-Host "no-op: ${full} ya firmado con este cert"
  }
  else {
    Write-Host "firmando ${full} ..."
    & $signtool sign /sha1 $cert.Thumbprint /fd SHA256 $full | Out-Null
    if ($LASTEXITCODE -ne 0) {
      throw "signtool fallo para ${full} (exit ${LASTEXITCODE})"
    }
  }
  $after = Get-AuthenticodeSignature $full
  $cn = ""
  if ($after.SignerCertificate) { $cn = $after.SignerCertificate.Subject }
  $rel = $full
  if ($full.StartsWith($root)) { $rel = $full.Substring($root.Length).TrimStart('\', '/') }
  $rows += [pscustomobject]@{ File = $rel; Status = $after.Status.ToString(); Subject = $cn }
  if ($after.Status -ne "Valid") { $failed += $full }
}

Write-Host ""
$rows | Format-Table -AutoSize | Out-String | Write-Host -NoNewline
if ($failed.Count -gt 0) {
  Write-Error ("verificacion fallo en: " + ($failed -join ", "))
  exit 1
}
Write-Host "done: $($rows.Count) firmado(s), 0 pendiente(s)"
Write-Host "recorda: la firma sola no desbloquea WDAC; falta la suplementaria por publisher que confie en $($cert.Thumbprint)."
