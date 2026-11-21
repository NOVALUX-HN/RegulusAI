# F14.6 — Análisis de política Smart App Control y criterios de confianza

## 1. Política efectiva identificada (con evidencia)

| Campo | Valor | Evidencia |
|---|---|---|
| Nombre de política | `VerifiedAndReputableDesktop` | 3077 EventData PolicyName (events/*.xml, rec 9691) |
| PolicyID | 27555.1000.240208 | idem |
| PolicyGUID | `{0283ac0f-fff1-49ae-ada1-8a933130cad6}` | idem + `C:\Windows\System32\CodeIntegrity\CiPolicies\Active\{0283ac0f…}.cip` (7079 B, listado solo-lectura) |
| PolicyHash | `2668895A…F937A01` | idem |
| Modo | **enforcement** (no audit) | 3077/3033 son Error con denegación efectiva + ejecución impedida; en audit no habría bloqueo |
| Decisión aplicada | Requested signing level 2 vs Validated 1, Status `0xc0e90002` | idem |

Otras políticas activas en el directorio (solo presencia, sin inferir función):
`{0939ed82…}`, `{1283ac0f…}` (79 907 B — por nomenclatura MS suele ser la variante
de evaluación), `{1678656c…}`, `{1939ed82…}`, `{2678656c…}`, `{60fd87f8…}`,
`{784c4414…}`. No se afirma cuál audita vs impone salvo la `{0283ac0f…}` que los
eventos prueban imponiendo.

Fuentes no disponibles (unavailable, sin elevación): `citool.exe -lp`
(`Error 0x80070005`), `Win32_DeviceGuard` (clase inválida en este build), modo
on/off de SAC en el Centro de seguridad.

## 2. Criterio de confianza (documentación oficial)

Microsoft Learn (consultado 2026-11-17):
- SAC permite solo código **firmado** o **predicho seguro por el servicio cloud de
  inteligencia**; si el servicio no predice con confianza → **bloqueo**
  (overview + appcontrol).
- "Over time, the code's reputation might change as new signals arrive" — la
  reputación es temporal y por hash/señales, no permanente.
- GUID `{0283AC0F…}` = inbox policy de SAC (`VerifiedAndReputableDesktop`), normal
  en dispositivos consumer con SAC activado (Q&A 2026-08-30).
- 3033/3077/3089/3118 = eventos de evaluación de firma + detalles SAC
  (event-id-explanations).

## 3. Tabla artefacto → firma/origen/MOTW → evento → decisión observada

| Artefacto (SHA256-12) | Firma | Origen/MOTW | Evento | Decisión |
|---|---|---|---|---|
| t2/t3/t5-probe `2CB3…` | NotSigned, Publisher Unknown | build local, Zone.Identifier ausente | 3077+3089+3033+3118 (DefenderTrust −16777216, sin amenaza, sin cloud call) | **bloqueado** vía bash/PS |
| mismo `2CB3…` vía `go test`/`go run` | idem | idem | sin eventos | **corre** (no-determinístico entre días) |
| govern.test.exe `5B66…` | NotSigned | build local 16/11, zone ausente | sin eventos | **corre** |
| bin/regulus.exe `6E80…` | **Valid**, CN=NOVALUX-HN Local (autofirmado, expira 2031) | firma local, zone ausente | sin eventos | **corre** |

Lectura: la firma local autofirmada NO es confianza Microsoft (no explica permisos);
el patrón observado es reputación/antigüedad del hash + servicio cloud (que en los
3118 capturados NO hizo cloud call: `DefenderMadeCloudCall=false`). Sin MOTW en
ningún artefacto local (Zone.Identifier ausente en todos los medidos).

## 4. SAC vs WDAC adicional vs otra capa

- A favor de SAC como única capa: PolicyName/GUID inbox de SAC; 3118 "Smart App
  Control Block Details"; Q&A confirma el GUID como SAC; AppLocker sin denegaciones;
  Defender sin eventos de bloqueo.
- En contra / no descartado: otras `.cip` activas sin rol identificado; el mecanismo
  exacto por el que `go.exe`-gestionado evita el 3077 es desconocido.
- Conclusión: SAC-VerifiedAndReputable es la capa que deniega (probado); no hay
  evidencia de segunda capa participando.

## 5. Incógnitas que no se pueden resolver desde el host

1. Estado on/off/evaluation exacto de SAC (requiere `citool`/UI elevada).
2. Umbral y señales de la reputación por hash (caja negra del servicio cloud).
3. Por qué `go.exe`-gestionado evita a veces la evaluación (sin ETW/ProcMon).
4. Rol de las demás `.cip` activas.
5. Si `bin/regulus.exe` corre por firma local, por reputación acumulada o por otra vía.
