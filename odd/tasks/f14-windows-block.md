# F14 — Investigación forense de bloqueos de tests en Windows + F15 Resolución

Header: milestone F14 (4 issues NOV-112..115, todas Urgent/Backlog) + F15 (2 issues NOV-116..117, Urgent/Backlog). Reglas transversales: solo lectura en F14 (no cambiar políticas, Defender, registro, ACL, certificados, logs ni auditoría); no inferir estado activo por una clave aislada; no declarar causa raíz sin correlación temporal y del artefacto; BLOCKED/SKIPPED/UNKNOWN nunca son PASS; cada campo indica valor, fuente, comando, timestamp y estado observed/unavailable/unknown; no volcar secretos. F15.1 exige NOV-115 completada Y revisada; F15.2 exige NOV-115 revisada + F15.1. Gate: pausa para revisión humana entre F14 y F15. Solo commits (uno por tarea), ningún push (pushea el usuario).

## Specs

- S1 (NOV-112 F14.1 · Inventario forense del entorno Windows y toolchain Go):
  - "## Objetivo — Obtener un inventario reproducible del equipo Windows donde OpenCode V2 ejecuta Regulus. Debe poder realizarse de forma desatendida."
  - "## Instrucciones — 1. Identificar edición, versión, build, arquitectura y estado de actualización de Windows. 2. Registrar versión y rutas reales de Go (go version, go env, GOROOT, GOTOOLCHAIN, GOOS, GOARCH, CGO_ENABLED), Git, PowerShell y OpenCode. 3. Registrar identidad efectiva del proceso (usuario, grupos, integridad/elevación) sin revelar credenciales ni datos personales. 4. Detectar controles de ejecución: Microsoft Defender, Smart App Control, SmartScreen, App Control for Business/WDAC, AppLocker, políticas corporativas y otros proveedores visibles. 5. Registrar políticas efectivas y su origen (local, GPO, MDM/Intune), distinguiendo Enabled/Audit/Disabled/Not configured. No inferir estado activo por una clave de registro aislada. 6. Inventariar configuración Go para temporales/caché, rutas de trabajo y ACL. 7. Identificar si OpenCode corre en Windows nativo, WSL, contenedor, SSH o sesión remota."
  - "## Límites — Solo lectura. No cambiar políticas, Defender, registro, ACL, certificados ni configuración de seguridad. No volcar secretos. Si algo requiere elevación, registrar la limitación y continuar."
  - "## Entregables — * Informe Markdown bajo docs/diagnostics/windows-test-block/. * JSON estructurado con inventario y procedencia. * Script PowerShell idempotente para repetirlo. * Cada campo indica valor, fuente, comando, timestamp y estado observed/unavailable/unknown."
  - "## Aceptación — Identifica entorno e identidad del runner; distingue políticas activas de configuradas; no contiene secretos ni cambia el sistema; adjunta comandos, exit codes y artefactos reales."

- S2 (NOV-113 F14.2 · Correlación de eventos Code Integrity, Defender y AppLocker):
  - "## Objetivo — Determinar qué mecanismo bloquea realmente los ejecutables de prueba y obtener evidencia correlacionada con hora, ruta, hash y proceso."
  - "## Instrucciones — 1. Consultar en modo lectura los logs disponibles: Microsoft-Windows-CodeIntegrity/Operational, AppLocker (EXE and DLL / MSI and Script), Microsoft-Windows-Windows Defender/Operational y fuentes relacionadas existentes. 2. Buscar eventos alrededor de intentos de go test, go test -c, runner actual y ejecución directa. Registrar IDs relevantes, XML completo y explicación basada en contenido observado; no asumir que 3077/3089 siempre son la causa. 3. Correlacionar timestamp, EventRecordID, usuario, proceso padre/hijo, ruta, SHA-256, firmante/certificado y regla/policy ID si están disponibles. 4. Distinguir WDAC/App Control, Smart App Control, AppLocker, Defender/EDR, ACL, Mark-of-the-Web, bloqueo de descarga u otra causa. 5. Separar bloqueo preventivo, evento de auditoría, advertencia informativa y fallo ajeno a seguridad. 6. Si un log no es accesible, documentar permiso faltante y fuentes alternativas no invasivas."
  - "## Límites — No limpiar logs, cambiar auditoría/enforcement, crear exclusiones ni alterar políticas. No declarar causa raíz sin correlación temporal y del artefacto."
  - "## Entregables — Timeline correlacionada y exportación JSON/XML/EVTX sin datos sensibles; tabla evento → regla/política → artefacto → efecto; hashes SHA-256; fuentes inaccesibles documentadas."
  - "## Aceptación — La causa de cada bloqueo reproducido está respaldada por eventos o evidencia equivalente. Las conclusiones separan hechos, inferencias y desconocidos."

- S3 (NOV-114 F14.3 · Reproducción controlada de go test y trazado de ejecutables temporales):
  - "## Objetivo — Reproducir exactamente el fallo y determinar si ocurre durante compilación, creación del archivo, carga del ejecutable, inicio del proceso o ejecución del test."
  - "## Instrucciones — 1. Leer scripts existentes (scripts/test-windows.ps1), workflows Windows y NOV-43, NOV-45, NOV-46, NOV-47, NOV-66 antes de probar. Identificar qué se implementó y qué se ejecutó realmente. 2. Ejecutar una matriz controlada: go test de paquete pequeño, go test -c con salida explícita, ejecución directa del binario, runner actual y suite completa si es seguro. 3. Registrar comando exacto, cwd, usuario, rutas temporales, exit code, stdout/stderr, duración, árbol de procesos y resultado real. 4. Capturar ruta, ACL, tamaño, hash, firma, zona/Mark-of-the-Web y origen de cada ejecutable. 5. Comparar rutas temporales de Go, directorio controlado del proyecto y otras ubicaciones relevantes sin mover artefactos a ubicaciones de confianza ni crear excepciones. 6. Verificar si go run, go test -c, go test y ejecución directa recorren mecanismos distintos. Build, vet o go run nunca sustituyen evidencia de tests ejecutados. 7. Determinar variación por paquete, test, CGO, arquitectura, ruta o política."
  - "## Límites — No ejecutar comandos que reduzcan seguridad. Limitar repeticiones y preservar evidencia."
  - "## Entregables — Matriz de reproducción por escenario; logs y artefactos mínimos; diagrama del ciclo de vida del ejecutable; lista exacta de paquetes/tests ejecutados y bloqueados."
  - "## Aceptación — Se identifica la etapa precisa del fallo y existe procedimiento determinista. Se demuestra ejecución real de tests o se documenta exactamente por qué Windows la impidió."

- S4 (NOV-115 F14.4 · Auditoría de soluciones viables y recomendación técnica basada en evidencia):
  - "## Objetivo — Convertir F14.1–F14.3 en una decisión técnica concreta para F15, sin repetir intentos fallidos ni depender innecesariamente de terceros."
  - "## Instrucciones — 1. Auditar runner, gates, scripts PowerShell, configuración Go, workflows y documentación; enlazar hallazgos con archivos/líneas y pruebas existentes. 2. Separar lo corregible en Regulus de lo externo. 3. Evaluar alternativas solo según política confirmada. 4. Para cada alternativa documentar eficacia, requisitos, privilegios, coste, riesgos, reversibilidad, seguridad, limitaciones y prueba de éxito. 5. Descartar soluciones universales falsas: certificado autofirmado como bypass de Smart App Control, exclusiones amplias, desactivar Defender/WDAC/AppLocker, renombrar binarios o cambiar extensiones. 6. Proponer plan F15 con tareas, orden, dependencias, rollback y aceptación. No implementar todavía."
  - "## Entregables — recommendation.md bajo docs/diagnostics/windows-test-block/; matriz comparativa; recomendación principal y fallback; plan detallado F15; prerrequisitos humanos/admin; referencias a evidencias F14.1–F14.3."
  - "## Aceptación — La recomendación se basa en la política y el fallo observados en el equipo real. Explica qué puede hacer OpenCode sin presencia del usuario y qué requiere intervención autorizada. F14 queda lista para revisión antes de iniciar F15."

- S5 (NOV-116 F15.1 · Implementar la corrección de causa raíz identificada en F14): prerrequisito NOV-115 completada y revisada + recommendation.md; cambio mínimo sin tocar política del host; si la salida requiere aprobación admin, detenerse y preparar el cambio exacto; tests + rollback; sin debilitar seguridad. (Gateado a revisión humana de F14.)

- S6 (NOV-117 F15.2 · Runner de tests Windows desatendido, nativo y observable): prerrequisitos NOV-115 revisada + F15.1; invocación nativa, procesos reales, resultado estructurado passed/failed/blocked/skipped/not-run (nunca blocked→passed), exit no-cero ante fallos, reproducible. (Gateado a revisión humana de F14 + F15.1.)

## Tasks

| ID | Spec | NOV | Deps | Route | Commit |
|----|------|-----|------|-------|--------|
| T1 | S1 | NOV-112 F14.1 | — | inline | 0d22201 |
| T2 | S2 | NOV-113 F14.2 | T1 | inline | — |
| T3 | S3 | NOV-114 F14.3 | T1 (+NOV-43/45/46/47/66 leídos) | inline | — |
| T4 | S4 | NOV-115 F14.4 | T1-T3 | inline | — |
| T5 | S5 | NOV-116 F15.1 | T4 + revisión humana | bloqueado | — |
| T6 | S6 | NOV-117 F15.2 | T4 + T5 | bloqueado | — |

## Log

- L1 (usuario, verbatim): "en linear tengo una nueva fase 14. trae y revisa todos los issues y ejecutarlos por favor y siempre actualiza linear con la información detallada"
- L2: F14 (NOV-112..115) + F15 (NOV-116..117) traídas de Linear con cuerpo completo. F15 gateada a revisión humana de F14 por spec (NOV-115: "F14 queda lista para revisión antes de iniciar F15"; NOV-116: prerrequisito NOV-115 completada y revisada).
- L3 (T1 done): NOV-112 F14.1 implementado (Win11 Pro 26300, go1.27.3, PS5.1, OpenCode v2.2.5, sesión consola nativa; identidad medium no-elevada; Defender on; execpolicy RemoteSigned solo CurrentUser; AppLocker vacía; clave CI presente sin inferir enforcement; SAC/SmartScreen/GPO unknown declarados; inventory.md + inventory.json + scripts/diag-windows.ps1 idempotente verificado exit 0) commit 0d22201; Linear con evidencia y Done.
