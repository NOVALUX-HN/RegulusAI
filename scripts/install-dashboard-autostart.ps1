#Requires -Version 5.1
<#
.SYNOPSIS
  Regulus dashboard — arranque automatico al encender la PC (T3, S2).
.DESCRIPTION
  Crea un acceso directo en shell:startup que lanza `regulus serve`
  en el puerto fijo, loopback-only. Idempotente: re-ejecutar actualiza.
.PARAMETER Port
  Puerto fijo del visual. Default 17817.
.PARAMETER Repo
  Raiz del repo (donde corre `regulus serve`). Default: esta repo.
.PARAMETER Uninstall
  Quita el acceso directo en vez de crearlo.
.EXAMPLE
  powershell -NoProfile -File scripts/install-dashboard-autostart.ps1
  powershell -NoProfile -File scripts/install-dashboard-autostart.ps1 -Uninstall
#>
param([int]$Port = 17817, [string]$Repo = "", [switch]$Uninstall)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($Repo)) { $Repo = (Split-Path -Parent $PSScriptRoot) }
$Repo = (Resolve-Path $Repo).Path
$Startup = [IO.Path]::Combine([Environment]::GetFolderPath("Startup"), "Regulus Dashboard.lnk")

if ($Uninstall) {
  if (Test-Path $Startup) { Remove-Item $Startup -Force; Write-Host "removed: $Startup" }
  else { Write-Host "nothing to remove" }
  exit 0
}

$Binary = Join-Path $Repo "bin\regulus.exe"
if (-not (Test-Path $Binary)) { $Binary = Join-Path $Repo "bin\regulus" }
$Wsh = New-Object -ComObject WScript.Shell
$Link = $Wsh.CreateShortcut($Startup)
$Link.TargetPath = $Binary
$Link.Arguments = "serve --port=$Port"
$Link.WorkingDirectory = $Repo
$Link.Description = "Regulus harness vivo (read-only, 127.0.0.1:$Port)"
$Link.Save()
Write-Host "installed: $Startup -> $Binary serve --port=$Port (repo $Repo)"
Write-Host "abre http://localhost:$Port/ tras reiniciar (o doble clic al acceso directo ahora)"
