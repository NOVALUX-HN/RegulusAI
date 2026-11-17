# Runbook Windows: ejecutar, diagnosticar, recuperarse (F15.7 / NOV-120)

Para OpenCode V2 y humanos, sin guía externa. Todo es no destructivo:
nada aquí cambia políticas, limpia cachés/logs ajenos ni desactiva protecciones.

## 1. Comandos reproducibles (mismo contexto: PowerShell o Git-Bash nativo)

```powershell
# Smoke (<2 min): núcleo + checks
powershell -NoProfile -File scripts/test-windows.ps1 -Packages "regulus-ai/internal/pxexec regulus-ai/internal/testflow"
# Focused: paquetes explícitos (espacios o comas)
powershell -NoProfile -File scripts/test-windows.ps1 -Packages "regulus-ai/internal/mission regulus-ai/internal/govern"
# Full: todo ./internal/... (30+ min en este host; ver §5)
powershell -NoProfile -File scripts/test-windows.ps1 -Strict -JsonReport .regulus/evidence/test-windows-report.json
# Vía executor (JSON versionado testflow-report-v1):
go run ./cmd/regulus testflow run --mode smoke
go run ./cmd/regulus testflow run --mode focused --packages regulus-ai/internal/mbench
go run ./cmd/regulus testflow run --mode full --save out.json [--skip a,b]
```

`--skip` solo admite paquetes nombrados explícitamente; cualquier otro
ausente es NOT_RUN y falla el veredicto (nunca PASS silencioso).

## 2. Diagnóstico no destructivo ante fallo de lanzamiento

1. Reproducir con 1 paquete y guardar salida + exit code.
2. `Get-WinEvent -LogName Microsoft-Windows-CodeIntegrity/Operational -MaxEvents 20`
   y buscar 3077 (quién intentó cargar qué) + 3089 (firma) + 3118 (detalle SAC)
   + 3033, correlacionando timestamp + ruta + SHA-256.
3. Si 3077 con Policy `VerifiedAndReputableDesktop {0283ac0f…}` y firmante
   Unknown → bloqueo SAC por reputación, no bug funcional. Ver
   `docs/diagnostics/windows-test-block/` (F14.1–F14.8).
4. `Get-AuthenticodeSignature` al binario + `Get-FileHash -Algorithm SHA256`.
   `Zone.Identifier` ausente = build local (esperado, no es causa).
5. NO hacer: exclusiones, certificados autofirmados como bypass, renombrar
   binarios, desactivar Defender/SAC/WDAC/AppLocker, limpiar logs o cachés.

## 3. Recuperación, retención, redacción

- Reintentar 1 vez el paquete aislado (el bloqueo por hash fresco es
  no-determinístico); si persiste, clasificar BLOCKED con evento, no reintentar
  en bucle.
- Retener: JSON del reporte + primera/última pantalla del fallo + EventRecordID.
  Limpiar solo `.regulus-test/test-*.exe` propios (`-KeepBinaries` solo para
  diagnóstico).
- Redactar secretos de líneas de comando y entorno antes de pegar evidencia
  (`pxexec.Render` lo hace; en manual, borrar tokens/keys).

## 4. Cuándo escalar (requiere humano/admin, F15 no lo hace solo)

- El gate de CI falla por BLOCKED sistemático → evaluar host limpio (fallback C).
- Firma/publisher o exclusión como única salida → justificación + aprobación
  administrativa explícita; hasta entonces el veredicto queda BLOCKED.
- Política del PC distinta a la documentada → re-correr F14.1–F14.2 y anexar.

## 5. Límites conocidos (no universales)

Suite completa en este host: 37 PASS / 12 BLOCKED (SAC) en la última corrida
(F15.6). CI limpia ≠ host afectado. WSL2 solo portable. Binario fresco siempre
puede bloquearse; binario reputado corre en cualquier ruta.

## 6. Reconciliación con NOV-43/45/46/47/66

- NOV-43 (diagnóstico reproducible): cerrado por F14.1–F14.8 con eventos + hash
  + discriminante probado (era hipótesis Temp+reputación; hoy es mecanismo +
  reputación por contenido + contexto de lanzamiento).
- NOV-45 (runner sin shell): cerrado por F15.2 (pxexec, shell solo justificado)
  + F15.3 (18 callers migrados; ShRunner queda como excepción auditada).
- NOV-46 (CI Windows): cerrado por F15.5 (gates estrictos + artefactos;
  CI limpia ≠ host afectado, explícito en el gate).
- NOV-47 (docs sin degradar): este runbook + docs/test-runner.md +
  docs/process-executor.md, sin pedir jamás bajar defensas.
- NOV-66 (suite completa + aserciones): cerrado por F15.4/F15.5 (veredicto
  estricto, BLOCKED nunca PASS, guard anti-regresión); queda vigente como
  procedimiento (`scripts/test-windows.ps1 -Strict`).
- Brecha que F15 cierra: lanzar procesos por vías dispersas sin clasificar el
  resultado. Lo que queda vigente: todo lo anterior como referencia histórica.
