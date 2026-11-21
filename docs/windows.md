# Windows sin degradar seguridad (NOV-47 / T27)

Soportado: Windows 11, Go 1.27 (`go.mod`), Git con o sin Git Bash en PATH. `ShRunner` usa `sh` si existe, si no `pwsh/powershell/cmd`; `StructuredRunner` nunca lanza shell.

Instalación:
- `scripts/build.ps1` → `bin/regulus.exe` + `binaries/`; `scripts/install-devin.ps1` verifica contrato/MCP.
- Verificación: `Get-FileHash -Algorithm SHA256` (integridad ≠ confianza Authenticode), `Get-AuthenticodeSignature` (hoy `NotSigned` salvo autofirmado local), `regulus doctor`, `GET /v1/git`.

Diagnóstico (sin desactivar nada):
- Smart App Control ≠ SmartScreen ≠ App Control for Business ≠ AppLocker. No son una sola tecnología.
- CodeIntegrity Operational: eventos 3077/3089/3033/3118 con ruta/SHA256/firmante/proceso/hora (ver `docs/windows-diagnosis.md`, Policy `0283ac0f-...`).
- Flujo operativo actual (F15): runbook en `docs/windows-runbook.md`, runner `regulus testflow run`, executor `docs/process-executor.md`, forense F14 en `docs/diagnostics/windows-test-block/`.
- Runner: `StructuredRunner` para invocaciones simples; shell solo opt-in explícito.
- Temporales `go test` bloqueados → `go test -c -o C:/Temp/` + ejecutar; WSL2 para unit, nativo para filesystem/procesos.
- Recuperación: reinstalar desde `binaries/`, `Unblock-File` solo archivo propio descargado (MOTW), nunca exclusiones amplias ni desactivar Defender/firewall/SAC.

Firma releases: hoy sin firma pública aprobada (ver NOV-44). Guía distingue lo que Regulus controla (runner, helpers, receipts) de restricciones del sistema (Enterprise signing, reputación).

## Permisos privados (NOV-85 Paso 65)

`internal/fsx` crea `.regulus/` con 0700 y archivos con 0600, y los
appends concurrentes usan lockfile (`O_EXCL`, atómico también en
Windows). En Windows los bits de modo no se pueden forzar (mandan las
ACLs): `memory doctor` e `init` lo advierten en vez de afirmar
privacidad — usá un perfil por usuario y disco cifrado (BitLocker/EFS);
la CLAIM de privacidad del repo vale para Unix. Los locks entre procesos
sí aplican en Windows.
