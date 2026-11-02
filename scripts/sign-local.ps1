# Regulus AI — firma local autofirmada para los EXEs (win-localtrust L1).
#
# Usage: powershell -File scripts/sign-local.ps1
#
# Genera (o reutiliza) un cert code-signing autofirmado CN "NOVALUX-HN Local"
# en Cert:\CurrentUser\My, lo instala en CurrentUser\Root y
# CurrentUser\TrustedPublisher, firma con signtool los EXEs que existan y
# verifica con Get-AuthenticodeSignature (Status Valid = firmado). Idempotente:
# re-correr con todo firmado es no-op (omite lo ya firmado con este cert).
#
# Solo CurrentUser — nunca toca LocalMachine. Si signtool.exe no esta
# instalado falla ANTES de crear nada (exit 1, sin cambios a medias).
# Alcance honesto: el Signed solo vale en las PCs donde este cert este
# instalado (ver docs/troubleshooting.md, L2).

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

$subject = "CN=NOVALUX-HN Local"
$thumbFile = Join-Path $root ".regulus/localtrust-thumbprint.txt"
$targets = @(
  "bin/regulus.exe",
  "binaries/regulus-windows-amd64.exe",
  "binaries/regulus-windows-arm64.exe"
)

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

# 0. Rutas objetivo (solo lectura): se firma lo que exista, se avisa lo ausente.
$existing = @()
foreach ($t in $targets) {
  if (Test-Path (Join-Path $root $t)) { $existing += $t }
  else { Write-Warning "no existe ${t}; se omite" }
}

# 1. signtool PRIMERO: si falta, error claro y exit 1 sin crear nada.
$signtool = Find-SignTool
if (-not $signtool) {
  Write-Error "signtool.exe no encontrado: instala Windows SDK o VS Build Tools (componente 'Windows SDK') y vuelve a correr este script. Sin cambios: no se creo ni instalo ningun certificado."
  exit 1
}
Write-Host "signtool: ${signtool}"

# 2. Cert code-signing CN NOVALUX-HN Local en CurrentUser\My (reutiliza si existe).
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

# 3. Confianza solo-CurrentUser: Root + TrustedPublisher (si falta).
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

# 4. Firma (signtool) + 5. verificacion (Get-AuthenticodeSignature -> Valid).
$rows = @()
$failed = @()
foreach ($t in $existing) {
  $full = Join-Path $root $t
  $before = Get-AuthenticodeSignature $full
  if ($before.Status -eq "Valid" -and $before.SignerCertificate -and $before.SignerCertificate.Thumbprint -eq $cert.Thumbprint) {
    Write-Host "no-op: ${t} ya firmado con este cert"
  }
  else {
    Write-Host "firmando ${t} ..."
    # Sin /tr (timestamp): funciona offline; vale para confianza local.
    & $signtool sign /sha1 $cert.Thumbprint /fd SHA256 $full | Out-Null
    if ($LASTEXITCODE -ne 0) {
      throw "signtool fallo para ${t} (exit ${LASTEXITCODE})"
    }
  }
  $after = Get-AuthenticodeSignature $full
  $cn = ""
  if ($after.SignerCertificate) { $cn = $after.SignerCertificate.Subject }
  $rows += [pscustomobject]@{ File = $t; Status = $after.Status.ToString(); Subject = $cn }
  if ($after.Status -ne "Valid") { $failed += $t }
}

Write-Host ""
$rows | Format-Table -AutoSize | Out-String | Write-Host -NoNewline
if ($failed.Count -gt 0) {
  Write-Error ("verificacion fallo en: " + ($failed -join ", "))
  exit 1
}
Write-Host "done: $($rows.Count) firmado(s), 0 pendiente(s)"
