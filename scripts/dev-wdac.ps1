# Regulus AI — dev-wdac.ps1: unico punto WDAC/firma dev (reemplaza 7 scripts viejos).
#
# Reemplaza (borrados):
#   apply-dev-supplemental.ps1 (BBB, base de apply aqui)
#   check-supplemental-status.ps1 (status aqui, corregido AAA+BBB)
#   new-dev-supplemental.ps1 (solo-generaba ID aleatorio, sin firma/deploy)
#   activate-dev-supplemental.ps1 (copia cruda AAA a Active, sin firmar)
#   repair-dev-supplemental.ps1 (duplicaba apply con AAA 0.0.0.4 fijo)
#   fix-supplemental-remove-audit.ps1 (bump puntual 0.0.0.2, absorbido)
#   sign-supplemental.ps1 (solo AAA, sin regen/refresh)
# Mantiene separado (dominio distinto, con refs vivas en docs/CI):
#   sign-dev.ps1, sign-local.ps1 (firma; dev-wdac sign los invoca si existen),
#   build.ps1/.bash, test-windows.ps1, install-devin.*, install-dashboard-autostart.*, sync-plugin-defaults.ts
#
# Uso (desde la raiz del repo):
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-wdac.ps1 status
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-wdac.ps1 apply [-Version "10.0.0.3"] [-SkipRegen] [-PolicyId "DBC82BBB-..."]
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-wdac.ps1 verify
#   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/dev-wdac.ps1 sign [-Paths "bin/regulus.exe"] [-Release]
#
# status/verify: NO requieren admin, NO mutan nada (solo citool + lectura XML/cert/disco).
# apply/sign: REQUIEREN ADMIN para CiTool (sign puede correr sin admin para firmar, pero CiTool exige admin).
param(
  [string]$Command = "status",
  [string]$Version = "",
  [string]$PolicyId = "DBC82BBB-A3D2-4AC1-8CAE-1B368D49A3EF",
  [string[]]$StalePolicyIds = @("D04FA7AE-2CED-4AC0-8847-F9E01BD72D19","74182097-788a-4c16-8d1c-705094444ed7","9734dc9a-144f-403d-b0c0-9d08ee9e229a","DBC82AAA-A3D2-4AC1-8CAE-1B368D49A3EF"),
  [switch]$SkipRegen,
  [switch]$NoRefreshFallback,
  [string[]]$Paths = @(),
  [switch]$Release
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

$basePolicyId = "0283ac0f-fff1-49ae-ada1-8a933130cad6"
$aaaId = "DBC82AAA-A3D2-4AC1-8CAE-1B368D49A3EF"
$policyId = $PolicyId.Trim().Trim('{}')
$subject = "NOVALUX-HN Local"
$subjectFull = "CN=NOVALUX-HN Local"
$thumbFile = Join-Path $root ".regulus/localtrust-thumbprint.txt"
$outDir = Join-Path $root ".regulus/dev-supplemental"
$scanDir = Join-Path $outDir "scan"
$xmlPath = Join-Path $outDir "RegulusDevSupplemental.xml"
$cipPath = Join-Path $outDir ("{" + $policyId + "}.cip")
$cerPath = Join-Path $outDir "dev-update.cer"

function Find-SignTool {
  $cmd = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $kitRoots = @()
  if ($Env:ProgramFiles) { $kitRoots += (Join-Path $Env:ProgramFiles "Windows Kits") }
  if (${Env:ProgramFiles(x86)}) { $kitRoots += (Join-Path ${Env:ProgramFiles(x86)} "Windows Kits") }
  foreach ($kr in $kitRoots) {
    if (Test-Path $kr) {
      $found = Get-ChildItem -Path $kr -Filter signtool.exe -Recurse -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match "\\x64\\" } |
        Sort-Object FullName -Descending |
        Select-Object -First 1
      if ($found) { return $found.FullName }
    }
  }
  return $null
}

function Get-VersionParts([string]$v) {
  $m = [regex]::Match($v, '(\d+)\.(\d+)\.(\d+)\.(\d+)')
  if (-not $m.Success) { return $null }
  return @([int]$m.Groups[1].Value, [int]$m.Groups[2].Value, [int]$m.Groups[3].Value, [int]$m.Groups[4].Value)
}

function Compare-Versions([string]$a, [string]$b) {
  $pa = Get-VersionParts $a; $pb = Get-VersionParts $b
  if (-not $pa -or -not $pb) { return 0 }
  for ($i = 0; $i -lt 4; $i++) {
    if ($pa[$i] -lt $pb[$i]) { return -1 }
    if ($pa[$i] -gt $pb[$i]) { return 1 }
  }
  return 0
}

function Invoke-Status {
  Write-Host "=== 1. citool -lp (RegulusDevSupplemental) ==="
  try { citool -lp | Select-String -Pattern "RegulusDevSupplemental" -Context 0,6 }
  catch { Write-Warning "citool -lp fallo: $($_.Exception.Message)" }

  Write-Host ""
  Write-Host "=== 2. citool --list-policies (RegulusDev + base) ==="
  try { citool --list-policies | Select-String -Pattern "RegulusDevSupplemental|dbc82aaa|dbc82bbb|0283ac0f" -Context 2,6 }
  catch { Write-Warning "citool --list-policies fallo: $($_.Exception.Message)" }

  Write-Host ""
  Write-Host "=== 3. Active cip (AAA viejo + BBB vigente) ==="
  foreach ($id in @($aaaId, $policyId)) {
    $active = "C:\Windows\System32\CodeIntegrity\CiPolicies\Active\{$id}.cip"
    if (Test-Path -LiteralPath $active) {
      Get-Item -LiteralPath $active | Format-Table FullName,Length,LastWriteTime -AutoSize
    } else {
      Write-Host "NO existe en Active: $active"
    }
  }

  Write-Host ""
  Write-Host "=== 4. XML local vs cert actual ==="
  if (Test-Path $xmlPath) {
    Select-String -Path $xmlPath -Pattern 'VersionEx|CertRoot|CertPublisher|PolicyID|BasePolicyID|UpdatePolicySigner' | ForEach-Object { $_.Line.Trim() }
  } else { Write-Host "NO existe XML: $xmlPath" }
  Write-Host ""
  Write-Host "thumb file:"
  if (Test-Path $thumbFile) { Get-Content $thumbFile -Raw } else { Write-Host "(sin thumb file)" }
  Write-Host ""
  Write-Host "certs NOVALUX-HN en CurrentUser\My:"
  Get-ChildItem Cert:\CurrentUser\My -ErrorAction SilentlyContinue | Where-Object { $_.Subject -match 'NOVALUX' } | Format-Table Subject,Thumbprint,NotAfter -AutoSize
  if (Test-Path $cerPath) {
    Write-Host "dev-update.cer:"
    Get-Item $cerPath | Format-Table FullName,Length,LastWriteTime -AutoSize
  }

  Write-Host ""
  Write-Host "=== 5. CIP locales (AAA vs BBB) ==="
  foreach ($id in @($aaaId, $policyId)) {
    $local = Join-Path $outDir ("{" + $id + "}.cip")
    if (Test-Path -LiteralPath $local) {
      Get-Item -LiteralPath $local | Format-Table FullName,Length,LastWriteTime -AutoSize
    } else {
      Write-Host "NO existe local: $local"
    }
  }

  Write-Host ""
  Write-Host "Diagnostico:"
  Write-Host "  firmada=False/aplica=False/autorizado=False -> el .cip desplegado quedo sin firmar o con firmante viejo."
  Write-Host "  OJO: CertRoot TBS (XML) NUNCA es igual al thumbprint: TBS es hash del TBS del cert, thumb es hash del cert completo."
  Write-Host "  Si rotaste cert (thumb nuevo) y la politica vieja tenia UpdatePolicySigner del cert viejo,"
  Write-Host "  el update necesita el firmante viejo o remover + reboot. Ver 'verify' y CodeIntegrity log 3077/3099."
}

function Invoke-Verify {
  [string]$txt = ""
  try { $txt = (citool -lp --json | Out-String) } catch { $txt = "" }
  if ([string]::IsNullOrWhiteSpace($txt)) { $txt = (citool -lp | Out-String) }
  $txt | Select-String -Pattern "RegulusDevSupplemental" -Context 0,6
  $okSigned = ($txt -match "(?i)(Policy is Signed|firmada)\s*[:=]\s*true") -and ($txt -match [regex]::Escape($policyId))
  $okAuth = ($txt -match "(?i)(Is Authorized|autorizado)\s*[:=]\s*true") -and ($txt -match [regex]::Escape($policyId))
  # Fallback JSON exacto por PolicyID (citool -lp --json usa IsSignedPolicy/IsAuthorized).
  try {
    $j = (citool -lp --json | Out-String) | ConvertFrom-Json
    $mine = @($j.Policies | Where-Object { $_.PolicyID -ieq $policyId -or $_.PolicyID -ieq "{$policyId}" })
    if ($mine.Count -gt 0) {
      $p = $mine[0]
      $okSigned = [bool]$p.IsSignedPolicy
      $okAuth = [bool]$p.IsAuthorized
      Write-Host ("BBB {{{0}}} Version={1} Signed={2} Enforced={3} Authorized={4}" -f $policyId, $p.VersionString, $p.IsSignedPolicy, $p.IsEnforced, $p.IsAuthorized)
    }
  } catch { }
  if ($okSigned -and $okAuth) {
    Write-Host ""
    Write-Host "OK: suplementaria BBB firmada y autorizada."
    return $true
  }
  Write-Host ""
  Write-Warning "BBB {$policyId} sigue sin autorizar. Corre 'status' y revisa CodeIntegrity Operational 3077/3099."
  return $false
}

function Invoke-Sign {
  # Delega a sign-dev.ps1 / sign-local.ps1 si existen (no duplicar logica de cert),
  # sino firma minima inline con el mismo cert CurrentUser.
  $signDev = Join-Path $root "scripts/sign-dev.ps1"
  $signLocal = Join-Path $root "scripts/sign-local.ps1"
  if ((-not $Release) -and (Test-Path $signDev) -and ($Paths.Count -eq 0)) {
    Write-Host "delegando a scripts/sign-dev.ps1 ..."
    powershell -NoProfile -ExecutionPolicy Bypass -File $signDev
    if ($LASTEXITCODE -ne 0) { throw "sign-dev.ps1 fallo (exit $LASTEXITCODE)." }
    return
  }
  if ($Release -and (Test-Path $signLocal) -and ($Paths.Count -eq 0)) {
    Write-Host "delegando a scripts/sign-local.ps1 ..."
    powershell -NoProfile -ExecutionPolicy Bypass -File $signLocal
    if ($LASTEXITCODE -ne 0) { throw "sign-local.ps1 fallo (exit $LASTEXITCODE)." }
    return
  }
  # Inline minimo (mismo cert CN + thumb file + Root/TrustedPublisher CurrentUser).
  $signtool = Find-SignTool
  if (-not $signtool) { Write-Error "signtool.exe no encontrado: instala Windows SDK. Sin cambios."; exit 1 }
  $savedThumb = $null
  if (Test-Path $thumbFile) { $savedThumb = ((Get-Content $thumbFile -Raw) | Out-String).Trim() }
  $cert = $null
  if ($savedThumb) { $cert = Get-ChildItem "Cert:\CurrentUser\My\$savedThumb" -ErrorAction SilentlyContinue }
  if (-not $cert -or $cert.NotAfter -le (Get-Date)) {
    $cert = Get-ChildItem Cert:\CurrentUser\My -ErrorAction SilentlyContinue |
      Where-Object { $_.Subject -eq $subjectFull -and $_.NotAfter -gt (Get-Date) } |
      Sort-Object NotAfter -Descending | Select-Object -First 1
  }
  if (-not $cert) {
    $cert = New-SelfSignedCertificate -Type CodeSigningCert -Subject $subjectFull `
      -CertStoreLocation "Cert:\CurrentUser\My" -NotAfter (Get-Date).AddYears(5) `
      -FriendlyName "NOVALUX-HN Local (regulus dev signing)"
    Write-Host "cert creado: $($cert.Subject) ($($cert.Thumbprint))"
  } else { Write-Host "cert reutilizado: $($cert.Subject) ($($cert.Thumbprint))" }
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $thumbFile) | Out-Null
  Set-Content -Path $thumbFile -Value $cert.Thumbprint -NoNewline
  foreach ($storeName in @("Root","TrustedPublisher")) {
    $present = Get-ChildItem "Cert:\CurrentUser\$storeName\$($cert.Thumbprint)" -ErrorAction SilentlyContinue
    if (-not $present) {
      $store = New-Object System.Security.Cryptography.X509Certificates.X509Store($storeName,"CurrentUser")
      $store.Open("ReadWrite"); try { $store.Add($cert) } finally { $store.Close() }
      Write-Host "confianza instalada: CurrentUser\$storeName"
    }
  }
  $targets = @()
  if ($Paths.Count -gt 0) { $targets = $Paths }
  elseif ($Release) { $targets = @("bin/regulus.exe","binaries/regulus-windows-amd64.exe","binaries/regulus-windows-arm64.exe") }
  else {
    $targets += @("bin/regulus.exe")
    foreach ($g in @(".regulus-test/test-*.exe",".regulus/*.test.exe")) {
      foreach ($m in (Get-ChildItem $g -ErrorAction SilentlyContinue)) { $targets += $m.FullName }
    }
  }
  foreach ($t in $targets) {
    $full = $t; if (-not [System.IO.Path]::IsPathRooted($full)) { $full = Join-Path $root $t }
    if (-not (Test-Path $full)) { Write-Warning "no existe ${t}; se omite"; continue }
    $before = Get-AuthenticodeSignature $full
    if ($before.Status -eq "Valid" -and $before.SignerCertificate.Thumbprint -eq $cert.Thumbprint) {
      Write-Host "no-op: ${t} ya firmado con este cert"
    } else {
      Write-Host "firmando ${t} ..."
      & $signtool sign /sha1 $cert.Thumbprint /fd SHA256 $full | Out-Null
      if ($LASTEXITCODE -ne 0) { throw "signtool fallo para ${t} (exit $LASTEXITCODE)" }
    }
  }
  Write-Host "done sign (cert $($cert.Thumbprint)). La firma sola no desbloquea WDAC; falta 'apply'."
}

function Invoke-Apply {
  # 0. Admin obligatorio.
  $p = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
  if (-not $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error "corre 'apply' como administrador. Sin cambios."
    exit 1
  }
  if (-not (Get-Module -ListAvailable -Name ConfigCI)) {
    Write-Error "modulo ConfigCI no disponible. Sin cambios."
    exit 1
  }
  $signtool = Find-SignTool
  if (-not $signtool) { Write-Error "signtool.exe no encontrado: instala Windows SDK. Sin cambios."; exit 1 }
  Write-Host "signtool: $signtool"
  if (-not (Test-Path $thumbFile)) {
    Write-Error "no hay $thumbFile. Corre 'sign' primero (no-admin). Sin cambios."
    exit 1
  }
  $thumb = ((Get-Content $thumbFile -Raw) | Out-String).Trim()
  $cert = Get-ChildItem "Cert:\CurrentUser\My\$thumb" -ErrorAction SilentlyContinue
  if (-not $cert) { Write-Error "cert $thumb no esta en CurrentUser\My. Corre 'sign' primero. Sin cambios."; exit 1 }
  Write-Host "cert actual: $($cert.Subject) ($thumb)"

  # 1. Limpieza stales (best-effort).
  Write-Host ""
  Write-Host "=== 1/6 limpiando duplicados viejos ==="
  try { $lp = (citool -lp --json | Out-String); if ([string]::IsNullOrWhiteSpace($lp)) { $lp = (citool -lp | Out-String) } }
  catch { $lp = "" }
  foreach ($stale in $StalePolicyIds) {
    if ([string]::IsNullOrWhiteSpace($stale)) { continue }
    $g = $stale.Trim().Trim('{}')
    if ($g -ieq $policyId) { continue }
    if ($lp -match [regex]::Escape($g)) {
      Write-Host "removiendo stale {$g} ..."
      CiTool --remove-policy "{$g}" --json
      if ($LASTEXITCODE -ne 0) { Write-Warning "remove-policy {$g} exit $LASTEXITCODE (sigo igual)." }
    } else {
      Write-Host "stale {$g} no aparece en citool -lp, lo omito."
    }
  }

  # 2-3. Regen XML con cert actual (salvo -SkipRegen).
  if (-not $SkipRegen) {
    Write-Host ""
    Write-Host "=== 2/6 firmando binarios dev ==="
    Invoke-Sign
    $candidates = @()
    foreach ($g in @("bin/*.exe", "binaries/*.exe", ".regulus-test/test-*.exe", ".regulus/*.test.exe")) {
      foreach ($m in (Get-ChildItem $g -ErrorAction SilentlyContinue)) { $candidates += $m.FullName }
    }
    $signed = @()
    foreach ($f in $candidates) {
      $sig = Get-AuthenticodeSignature $f
      if ($sig.Status -eq "Valid" -and $sig.SignerCertificate -and $sig.SignerCertificate.Thumbprint -eq $thumb) { $signed += $f }
    }
    if ($signed.Count -eq 0) { Write-Error "ningun EXE firmado con el cert actual. Sin cambios."; exit 1 }
    Write-Host "firmados con cert actual: $($signed.Count)"
    New-Item -ItemType Directory -Force -Path $scanDir | Out-Null
    foreach ($f in $signed) { Copy-Item $f (Join-Path $scanDir (Split-Path $f -Leaf)) -Force }

    Write-Host ""
    Write-Host "=== 3/6 regenerando XML (PolicyID fijo BBB) ==="
    if (Test-Path $xmlPath) {
      $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
      Copy-Item $xmlPath "$xmlPath.bak-$stamp" -Force
      Write-Host "backup: $xmlPath.bak-$stamp"
    }
    $drivers = Get-SystemDriver -ScanPath $scanDir -UserPEs
    New-CIPolicy -Level Publisher -Fallback Hash -DriverFiles $drivers -FilePath $xmlPath -MultiplePolicyFormat | Out-Null
    Set-CIPolicyIdInfo -FilePath $xmlPath -PolicyId "{$policyId}" -PolicyName "RegulusDevSupplemental" -SupplementsBasePolicyID "{$basePolicyId}" | Out-Null
    [xml]$fixXml = Get-Content $xmlPath
    $pidNode = $fixXml.SelectSingleNode("//*[local-name()='PolicyID']")
    if ($pidNode -and $pidNode.InnerText.Trim('{}') -ine $policyId) {
      Write-Host ("PolicyID interno era {0}, lo fuerzo a {{{1}}}..." -f $pidNode.InnerText, $policyId)
      $pidNode.InnerText = "{$policyId}"
      $idSetting = $fixXml.SelectSingleNode("//*[local-name()='Setting' and @ValueName='Id']/*[local-name()='String']")
      if ($idSetting) { $idSetting.InnerText = "{$policyId}" }
      $fixXml.Save($xmlPath)
    }
    Write-Host "PolicyID fijado a {$policyId}, base {$basePolicyId}"
    if (Select-String -Path $xmlPath -Pattern "Audit Mode" -Quiet) {
      Set-RuleOption -FilePath $xmlPath -Option 3 -Delete
      Write-Host "Audit Mode eliminado (Option 3)."
    }
    if (-not (Select-String -Path $xmlPath -Pattern "Update Policy No Reboot" -Quiet)) {
      Set-RuleOption -FilePath $xmlPath -Option 16
      Write-Host "Update Policy No Reboot habilitado (Option 16) para aplicar sin reboot."
    }
    [string]$target = $Version.Trim()
    if ([string]::IsNullOrWhiteSpace($target)) {
      [xml]$tmpXml = Get-Content $xmlPath
      $tmpVer = $tmpXml.SelectSingleNode("//*[local-name()='VersionEx']").InnerText
      $maxVer = $tmpVer
      $deployed = [regex]::Matches($lp, 'RegulusDevSupplemental[\s\S]{0,200}?(\d+\.\d+\.\d+\.\d+)')
      foreach ($m in $deployed) {
        if ((Compare-Versions $m.Groups[1].Value $maxVer) -gt 0) { $maxVer = $m.Groups[1].Value }
      }
      $parts = Get-VersionParts $maxVer
      $parts[3] = $parts[3] + 1
      $target = "$($parts[0]).$($parts[1]).$($parts[2]).$($parts[3])"
      Write-Host "version auto: $tmpVer / desplegada max $maxVer -> $target"
    }
    [xml]$xml = Get-Content $xmlPath
    $verNode = $xml.SelectSingleNode("//*[local-name()='VersionEx']")
    if ($verNode) { $verNode.InnerText = $target; $xml.Save($xmlPath) }
    Write-Host "VersionEx -> $target"
    Export-Certificate -Cert $cert -FilePath $cerPath -Force | Out-Null
    Add-SignerRule -FilePath $xmlPath -CertificatePath $cerPath -User -Update | Out-Null
    Write-Host "UpdatePolicySigner agregado (User + Update, sin Supplemental)."
  } else {
    Write-Host ""
    Write-Host "=== 2-3/6 SkipRegen: uso XML existente ==="
    if (-not (Test-Path $xmlPath)) { Write-Error "no existe $xmlPath y pediste -SkipRegen. Sin cambios."; exit 1 }
    if (-not [string]::IsNullOrWhiteSpace($Version)) {
      [xml]$xmlS = Get-Content $xmlPath
      $vn = $xmlS.SelectSingleNode("//*[local-name()='VersionEx']")
      if ($vn) { $vn.InnerText = $Version.Trim(); $xmlS.Save($xmlPath) }
      Write-Host "VersionEx -> $($Version.Trim()) (forzada con -SkipRegen)"
    }
  }

  $raw = Get-Content $xmlPath -Raw
  if ($raw -match "SupplementalPolicySigner") { Write-Error "el XML tiene SupplementalPolicySigners (solo para bases). Sin firmar."; exit 1 }
  if ($raw -match "Audit Mode") { Write-Error "el XML todavia tiene Audit Mode. Sin firmar."; exit 1 }
  [xml]$xml2 = Get-Content $xmlPath
  $upd = $xml2.SelectNodes("//*[local-name()='UpdatePolicySigner']")
  if (-not $upd -or $upd.Count -eq 0) { Write-Error "sin UpdatePolicySigner en el XML. Sin firmar."; exit 1 }
  Write-Host "XML OK: sin SupplementalSigners, sin Audit, con UpdatePolicySigner."

  Write-Host ""
  Write-Host "=== 4/6 generando .cip ==="
  ConvertFrom-CIPolicy -XmlFilePath $xmlPath -BinaryFilePath $cipPath | Out-Null
  Write-Host "cip: $cipPath ($((Get-Item -LiteralPath $cipPath).Length) bytes)"

  Write-Host ""
  Write-Host "=== 5/6 firmando .cip ==="
  $unsignedLen = (Get-Item -LiteralPath $cipPath).Length
  $p7out = Join-Path $outDir ((Split-Path $cipPath -Leaf) + ".p7")
  $p7root = Join-Path $root ((Split-Path $cipPath -Leaf) + ".p7")
  foreach ($f in @($p7out, $p7root)) { if (Test-Path -LiteralPath $f) { Remove-Item -LiteralPath $f -Force } }
  & $signtool sign -v /sha1 $thumb -p7 "$outDir" -p7co 1.3.6.1.4.1.311.79.1 -fd sha256 $cipPath
  if ($LASTEXITCODE -ne 0) { throw "signtool fallo (exit $LASTEXITCODE). Sin desplegar." }
  $signedP7 = $null
  foreach ($f in @($p7out, $p7root)) { if (Test-Path -LiteralPath $f) { $signedP7 = $f; break } }
  if (-not $signedP7) { throw "signtool no produjo .p7. Sin desplegar: revisa que el cert tenga clave privada en CurrentUser\My." }
  Copy-Item -LiteralPath $signedP7 -Destination $cipPath -Force
  foreach ($f in @($p7out, $p7root)) { if (Test-Path -LiteralPath $f) { Remove-Item -LiteralPath $f -Force } }
  $newLen = (Get-Item -LiteralPath $cipPath).Length
  Write-Host ("cip firmado: {0} ({1} bytes, antes {2})" -f $cipPath, $newLen, $unsignedLen)
  if ($newLen -le $unsignedLen) { throw ("el .cip firmado no crecio ({0} -> {1}): quedo sin firmar. Sin desplegar." -f $unsignedLen, $newLen) }

  Write-Host ""
  Write-Host "=== 6/6 CiTool update + refresh (sin reboot) ==="
  CiTool --update-policy "$cipPath" --json
  if ($LASTEXITCODE -ne 0) { throw "CiTool --update-policy fallo (exit $LASTEXITCODE)." }
  Write-Host "desplegado con CiTool."
  CiTool --refresh --json
  if ($LASTEXITCODE -ne 0) { Write-Warning "CiTool --refresh exit $LASTEXITCODE (sigo a verificar)." }
  else { Write-Host "refresh OK." }

  Write-Host ""
  Write-Host "=== verificacion (sin reboot) ==="
  if (Invoke-Verify) { exit 0 }

  if (-not $NoRefreshFallback) {
    Write-Warning "sigue sin autorizar; intento fallback WMI sin reboot..."
    try {
      $dst = "C:\Windows\System32\CodeIntegrity\CiPolicies\Active\{$policyId}.cip"
      Copy-Item -LiteralPath $cipPath -Destination $dst -Force
      Invoke-CimMethod -Namespace root/Microsoft/Windows/CI -ClassName PS_UpdateAndCompareCIPolicy -MethodName Update -Arguments @{FilePath = $dst} | Out-String | Write-Host
      Invoke-Verify | Out-Null
    } catch {
      Write-Warning "fallback WMI fallo: $($_.Exception.Message)"
    }
  }

  Write-Host ""
  Write-Host "Si aun dice autorizado=False:"
  Write-Host "  1. Revisa CodeIntegrity Operational 3077/3099 para el motivo real."
  Write-Host "  2. Si rotaste cert, el update firmado con cert nuevo no lo autoriza el UpdatePolicySigner viejo: hay que remover + reboot."
  Write-Host "  3. Solo como ultimo recurso: Restart-Computer"
}

switch ($Command.ToLower()) {
  "status" { Invoke-Status }
  "verify" { if (-not (Invoke-Verify)) { exit 1 } }
  "sign" { Invoke-Sign }
  "apply" { Invoke-Apply }
  default {
    Write-Host "Uso: scripts/dev-wdac.ps1 <status|apply|verify|sign> [-Version x.x.x.x] [-SkipRegen] [-Paths ...] [-Release]"
    exit 1
  }
}
