# Idempotent read-only Windows diagnostics (F14.1/NOV-112).
# Repeats the inventory without changing the system: no policy,
# Defender, registry, ACL, certificate or audit writes; no secrets.
# Safe to re-run: pure reads with try/catch, JSON to stdout.
# Usage: powershell -NoProfile -ExecutionPolicy Bypass -File scripts/diag-windows.ps1
$ErrorActionPreference = "Continue"
$rows = @()
function Add-Row($name, $value, $source, $command, $exit, $status) {
  $script:rows += [pscustomobject]@{name=$name; value=$value; source=$source; command=$command; exit=$exit; status=$status}
}
function Try-Value([scriptblock]$sb) {
  try { $v = & $sb; return @($v, 0, "observed") } catch { return @($null, 1, "unavailable") }
}
$os = Get-CimInstance Win32_OperatingSystem -ErrorAction SilentlyContinue
if ($os) {
  Add-Row "os.caption" $os.Caption "Win32_OperatingSystem" "Get-CimInstance" 0 "observed"
  Add-Row "os.version" $os.Version "Win32_OperatingSystem" "Get-CimInstance" 0 "observed"
  Add-Row "os.build" $os.BuildNumber "Win32_OperatingSystem" "Get-CimInstance" 0 "observed"
  Add-Row "os.arch" $os.OSArchitecture "Win32_OperatingSystem" "Get-CimInstance" 0 "observed"
} else { Add-Row "os" $null "Win32_OperatingSystem" "Get-CimInstance" 1 "unavailable" }
try { $gv = (go version) 2>$null; Add-Row "go.version" $gv "go version" "go version" 0 "observed" } catch { Add-Row "go.version" $null "go version" "go version" 1 "unavailable" }
foreach ($k in @("GOROOT","GOTOOLCHAIN","GOOS","GOARCH","CGO_ENABLED","GOCACHE","GOTMPDIR")) {
  try { $v = (go env $k) 2>$null; Add-Row "go.$($k.ToLower())" $v "go env" "go env $k" 0 "observed" } catch { Add-Row "go.$($k.ToLower())" $null "go env" "go env $k" 1 "unavailable" }
}
try { Add-Row "identity.elevated" ([System.Security.Principal.WindowsPrincipal][System.Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator) "WindowsPrincipal" "IsInRole" 0 "observed" } catch { Add-Row "identity.elevated" $null "WindowsPrincipal" "IsInRole" 1 "unavailable" }
try { $mp = Get-MpComputerStatus -ErrorAction Stop; Add-Row "defender.av" "AV=$($mp.AntivirusEnabled) RT=$($mp.RealTimeProtectionEnabled)" "Get-MpComputerStatus" "Get-MpComputerStatus" 0 "observed" } catch { Add-Row "defender.av" $null "Get-MpComputerStatus" "Get-MpComputerStatus" 1 "unavailable" }
try { $ep = (Get-ExecutionPolicy -List | Out-String).Trim(); Add-Row "execpolicy" $ep "Get-ExecutionPolicy" "Get-ExecutionPolicy -List" 0 "observed" } catch { Add-Row "execpolicy" $null "Get-ExecutionPolicy" "Get-ExecutionPolicy -List" 1 "unavailable" }
try { $ci = Get-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy" -ErrorAction Stop; Add-Row "ci.policy.key" "Emode=$($ci.EmodePolicyRequired) Sku=$($ci.SkuPolicyRequired) Verified=$($ci.VerifiedAndReputablePolicyState) (no enforcement inferred)" "HKLM CI Policy" "Get-ItemProperty" 0 "observed" } catch { Add-Row "ci.policy.key" $null "HKLM CI Policy" "Get-ItemProperty" 1 "unavailable" }
try { $al = Get-AppLockerPolicy -Effective -ErrorAction Stop | Select-Object -ExpandProperty RuleCollections; Add-Row "applocker.effective" "$al" "Get-AppLockerPolicy" "Get-AppLockerPolicy -Effective" 0 "observed" } catch { Add-Row "applocker.effective" $null "Get-AppLockerPolicy" "Get-AppLockerPolicy -Effective" 1 "unavailable" }
Add-Row "deviceguard.effective" $null "needs event correlation (T2)" "pending" $null "unknown" | Out-Null
$rows | ConvertTo-Json -Depth 4
