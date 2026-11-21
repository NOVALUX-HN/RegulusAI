# F14.1 — Inventario forense del entorno Windows y toolchain Go

Fecha: 2026-11-17. Método: solo lectura, sin elevación, sesión del propio runner.
Cada campo: valor observado, fuente, comando, exit code y estado observed/unavailable/unknown.
Nada aquí cambia el sistema; las ausencias se registran, no se rellenan.

## 1. Sistema operativo (observed)

| Campo | Valor | Fuente / comando (exit 0) |
|---|---|---|
| Edición | Microsoft Windows 11 Pro | `Get-CimInstance Win32_OperatingSystem` (Caption) |
| Versión / Build | 10.0.26300 / 26300 | idem (Version/BuildNumber) |
| Arquitectura | 64 bits | idem (OSArchitecture) |
| Sesión | Consola nativa, SessionId 1, no WSL (`$WSL_DISTRO_NAME` vacío), no contenedor/SSH | `$env:SESSIONNAME` → `Console`; `(Get-Process -Id $PID).SessionId` → 1 |

## 2. Toolchain (observed)

| Campo | Valor | Fuente / comando (exit 0) |
|---|---|---|
| Go | go1.27.3 windows/amd64 | `go version` |
| GOROOT | C:\Program Files\Go | `go env GOROOT` |
| GOTOOLCHAIN | auto | `go env GOTOOLCHAIN` |
| GOOS/GOARCH | windows/amd64 | `go env` |
| CGO_ENABLED | 0 | `go env CGO_ENABLED` |
| GOCACHE | C:\Users\Andragon\AppData\Local\go-build | `go env GOCACHE` |
| GOTMPDIR | unset (usa temp del SO) | `go env GOTMPDIR` (vacío) |
| GOPATH | C:\Users\Andragon\go | `go env GOPATH` |
| Temp del runner | C:\Users\...\Temp\gptrl20H (por sesión) | `$env:TEMP` |
| ACL go-build | SYSTEM/Administradores/Andragon = FullControl | `(Get-Acl $env:LOCALAPPDATA\go-build)` |
| Git | 2.56.0.windows.3 | `git --version` |
| PowerShell | 5.1.26100.9818 | `$PSVersionTable` |
| OpenCode | v2.2.5 | `opencode --version` |

## 3. Identidad efectiva (observed)

| Campo | Valor | Fuente / comando (exit 0) |
|---|---|---|
| Usuario | DESKTOP-R0IUAOR\Andragon, no elevado | `IsInRole(Administrator)` → False |
| Integridad | Medium (S-1-16-8192) | `C:\Windows\System32\whoami.exe /groups` |
| Administradores | S-1-5-32-544 deny-only (sin elevación efectiva) | idem |
| Grupos extra | docker-users (S-1-5-21-…-1002) | idem |

## 4. Controles de ejecución (observed + unknown declarado)

| Control | Estado registrado | Fuente / comando |
|---|---|---|
| Defender AV / real-time / AM / antispyware | Enabled / True (los 4) | `Get-MpComputerStatus` (exit 0) |
| ExecutionPolicy | MachinePolicy/UserPolicy/Process/LocalMachine = Undefined; CurrentUser = RemoteSigned | `Get-ExecutionPolicy -List` (exit 0) |
| AppLocker efectiva | Vacía (sin reglas listadas, exit 0) | `Get-AppLockerPolicy -Effective` → sin RuleCollections |
| Clave CI\Policy | Presente: EmodePolicyRequired=0, SkuPolicyRequired=0, VerifiedAndReputablePolicyState=1 | `Get-ItemProperty HKLM:\SYSTEM\CurrentControlSet\Control\CI\Policy` (exit 0). **No se infiere enforcement**: la política efectiva (DeviceGuard) queda unknown hasta F14.2/eventos |
| Smart App Control | unknown (sin clave legible, sin `MpPreference` visible) | claves `HKLM:...\SmartAppControl` y `Get-MpPreference.SmartAppControlEnabledState` sin salida |
| SmartScreen | unknown | `HKLM:...\Explorer SmartScreenEnabled` sin salida |
| GPOs aplicadas | No enumeradas (gpresult muestra encabezado + filtradas, sin lista capturada) | `gpresult /r` parcial → unknown, reintentar en F14.2 |
| Origen de políticas | No determinado (local vs GPO vs MDM) | unknown hasta correlación |

## 5. Comandos exactos y exit codes

Todos los comandos de §1–§4 con exit 0 salvo: `whoami /groups` vía Git-Bash (resolvió a /usr/bin/whoami, error de uso — reejecutado como `C:\Windows\System32\whoami.exe`, exit 0). Ningún comando requirió elevación; ninguno modificó el sistema. Artefactos: `inventory.json` (misma información estructurada), `scripts/diag-windows.ps1` (repite este inventario de forma idempotente).

## 6. Limitaciones

- Device Guard / WDAC efectivo: unknown (requiere correlación de eventos, T2).
- Smart App Control / SmartScreen: unknown.
- Sin secretos volcados; rutas de usuario abreviadas donde no aportan.
