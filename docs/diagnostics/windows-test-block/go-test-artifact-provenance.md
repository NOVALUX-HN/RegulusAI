# F14.7 — Procedencia de artefactos Go y auditoría de rutas

## 1. Inventario de rutas (observed)

| Variable | Valor | Fuente |
|---|---|---|
| GOTMPDIR | unset (Temp del SO por sesión, `Temp\gptrl20H`) | `go env` |
| TMP / TEMP | `C:\Users\…\AppData\Local\Temp` (sesión `gptrl20H`) | `$env:TEMP` |
| GOCACHE | `AppData\Local\go-build` (usuario FullControl) | `go env` + ACL (F14.1) |
| GOMODCACHE | `C:\Users\Andragon\go\pkg\mod` | `go env` |
| Volumen | C: NTFS (580 GB libres); FS directo no legible sin elevación (`Error 5`, registrado) | Get-PSDrive / fsutil parcial |
| ACL/propietario binarios | `DESKTOP-R0IUAOR\Andragon` en las 3 ubicaciones | `(Get-Acl).Owner` |

## 2. Dónde genera/linkea/almacena/elimina Go (observado)

- `go test`: compila en `GOCACHE`, linkea a `Temp\go-build<rand>\b001\*.test.exe`,
  lo ejecuta el propio `go.exe` y lo elimina al salir (temporales no sobreviven).
- `go test -c -o <ruta>`: linkea a la ruta explícita y lo conserva.
- `go run`: mismo esquema temp que `go test` (build→link→exec→limpieza).
- Caché caliente en todos los experimentos; **caché fría no probada**: vaciar
  GOCACHE destruiría datos del usuario y el spec lo prohíbe (registrado, no omitido).

## 3. Experimento controlado (mismos bytes, 3 rutas, mismo lanzador)

Build fresco del mismo paquete a 3 ubicaciones: hashes idénticos
(`2CB3…10C` en sidecar, repo y Temp) → exec vía `bash.exe`:
**los 3 bloqueados** (`Permission denied`). Propietario idéntico, Zone.Identifier
ausente en los 3 (sin MOTW local en ningún caso).

## 4. Conclusión

**Ruta, caché caliente, procedencia, ACL, propietario y MOTW no cambian el
resultado**: bytes idénticos se bloquean igual en sidecar, repo y Temp cuando el
lanzador es el shell. La decisión vive en la evaluación de carga (SAC), no en la
ubicación. AV/EDR: Defender es consultado durante la evaluación (3118
`DefenderCalled=true`, sin cloud, sin amenaza) pero no hay eventos de bloqueo
propios de Defender — no interviene antes ni después con denegación.

## 5. Recomendación de ruta/caché (basada en evidencia)

Ninguna ruta hace confiable el binario fresco; la ruta solo importa para el
diagnóstico (sidecar conserva el artefacto + eventos correlacionables) y para no
contaminar el checkout. Recomendación: compilar a directorio controlado del
proyecto (conserva evidencia), ejecutar, clasificar blocked vs fail (F15.1).
