# F14.2 — Timeline correlacionada del bloqueo

Repro: `go test -c -o .tmp-gotest/t2-probe.test.exe ./internal/mbench` (compile exit 0,
15:34:58–59Z) + ejecución directa (15:35:02–03Z → `Permission denied`). Artefacto
compila bien; el fallo ocurre en la **carga del ejecutable**, no en compilación ni en test.
XML completo: `events/codeintegrity-2026-11-17.xml` (8 eventos, sin datos sensibles).

## Correlación (ActivityID `{f58cb9c9-7560-000a-5895-b4f56075dd01}`, usuario S-1-5-21-…-1001)

| Hora (Z) | EventRecordID | ID | Nivel | Hecho |
|---|---|---|---|---|
| 15:35:03.1179 | 9691 | 3077 | Error | `bash.exe` intentó cargar `t2-probe.test.exe`: Requested level 2 vs Validated 1, Status `0xc0e90002`, Policy `VerifiedAndReputableDesktop` (ID 27555.1000.240208, GUID `{0283ac0f-fff1-49ae-ada1-8a933130cad6}`), SHA256 `D6AA…038` |
| 15:35:03.1180 | 9692 | 3089 | Info | Firma del mismo artefacto: 0 firmas, Publisher `Unknown`, SHA1 `0FA4…1A4` — dato complementario del 3077 |
| 15:35:03.1180 | 9693 | 3118 | Info | **Smart App Control Block Details** para SHA256-flat `2CB3…10C`: DefenderTrust `-16777216` (no confiable), sin amenaza, sin llamada cloud, `DefenderDisabled=false` |
| 09:01:24 (mismo día) | — | 3077+3118 | Error/Info | Mismo patrón con `go.exe` cargando `Temp\...\go-build…\exe\regulus.exe` (temp de Go) — prueba que el temp de Go recorre el mismo mecanismo |

## Tabla evento → regla → artefacto → efecto

| Evento | Regla/política | Artefacto (SHA256) | Efecto |
|---|---|---|---|
| 3077 Error | VerifiedAndReputableDesktop `{0283ac0f…}` (Smart App Control) | t2-probe.test.exe `D6AA…038` (flat `2CB3…10C`), sin firma | Carga denegada al proceso `bash.exe` |
| 3033 Error | Enterprise signing level (compañero del 3077) | mismo artefacto | Denegación redundante, misma causa |
| 3118 Info | Smart App Control | mismo hash flat | Detalle del bloqueo: no confiable, sin amenaza |
| Defender/Operational 1150/1151/2010 | — (informativos de rutina) | — | Sin bloqueo atribuible a Defender |
| AppLocker EXE/DLL 8001 (15/11) | "directiva aplicada correctamente" | — | Solo carga de política, sin denegaciones; AppLocker **no** es el mecanismo |

## Discriminador observado (hecho, no inferencia)

- `govern.test.exe` (SHA256 `5B66…`, sin firma, 6.1 MB, 16/11) **ejecuta** hoy; `t2-probe.test.exe`
  (SHA256 `2CB3…`, sin firma, 6.2 MB, 17/11) **bloqueado** en el mismo directorio.
- Conclusión soportada: la firma **no** es el discriminador; lo es la reputación/antigüedad
  del hash (política *VerifiedAndReputable* = firmado **o** reputado). Binarios frescos
  (hash nuevo en cada `go build`/`go test`) → no reputados → bloqueados.

## Separación hechos / inferencias / desconocidos

- Hechos: 3077+3089+3118 con timestamps, hashes, PolicyID/GUID y firmante Unknown; Defender
  sin eventos de bloqueo; AppLocker sin denegaciones; compilación exit 0; ambos .test.exe sin firma.
- Inferencias: el mecanismo es Smart App Control bajo política VerifiedAndReputableDesktop;
  el discriminador es reputación del hash (respaldado por el par govern/t2-probe).
- Desconocidos: estado on/off de SAC en el centro de seguridad (sin lectura disponible);
  umbral exacto de reputación; por qué `go run ./cmd/regulus` ejecuta (hipótesis: caché de
  build reutiliza binario ya evaluado — verificar en T3).
