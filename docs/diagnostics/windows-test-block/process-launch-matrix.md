# F14.5 — Matriz de lanzamientos y trazabilidad causal

Binario único: `.tmp-gotest/t5-probe.test.exe` = `go test -c ./internal/mbench`,
SHA256 `2CB3…10C`, NotSigned, bytes idénticos en todos los intentos directos
(verificado por hash antes de cada serie). Sin secretos en líneas de comando.

## Matriz (3 repeticiones por camino, timestamps Z)

| Camino | Lanzador (padre) | R1 | R2 | R3 | Eventos CI |
|---|---|---|---|---|---|
| A. `go test -count=1` | `go.exe` (temp go-build) | 15:56:05 ok 0.19s | 15:56:09 ok | 15:56:12 ok | ninguno |
| B. `& exe` (PowerShell) | `powershell.exe` | 15:56:21 bloqueado | 15:56:25 bloqueado | 15:56:29 bloqueado | 3077+3089+3033+3118 ×3 (09:56:26/29) |
| C. `./exe` (Git Bash) | `bash.exe` | 15:55:55 bloqueado | 15:55:58 bloqueado | 15:56:02 bloqueado | 3077 rec 9706/9711/9716 + séquito |
| D. `go run ./cmd/regulus` | `go.exe` | 15:56:21 ok | 15:56:24 ok | 15:56:28 ok | ninguno |
| E. `Start-Process` (PS) | `powershell.exe` | 15:56:37 start-fail | 15:56:40 start-fail | 15:56:44 start-fail | 3077+… ×3 (09:56:37/41/44) |
| CTRL. mismo bytes en `%TEMP%`, lanzado por bash | `bash.exe` | 15:57:20 bloqueado | — | — | (misma firma que T2) |
| CTRL2. copia creada por PowerShell, mismos bytes, lanzada por bash | `bash.exe` | bloqueado | — | — | — |

Árboles por escenario:

```
A: pwsh/bash(pid N) -> go.exe -> go-build.../*.test.exe (corre, sin eventos)
B/E: powershell.exe(pid M) -> t5-probe.test.exe (3077: powershell.exe intentó cargar)
C: bash.exe(pid 78) -> t5-probe.test.exe (3077: bash.exe intentó cargar)
D: bash -> go.exe -> exe\regulus.exe (corre, sin eventos)
```

## Conclusión (confianza media-alta)

- La variable que cambia con el resultado es el **camino de lanzamiento**:
  ejecución gestionada por el toolchain Go (A, D) corre; ejecución directa desde
  shell (B, C, E) es denegada con 3077/3118. 15/15 intentos consistentes hoy.
- **Causalidad vs correlación**: CTRL elimina la ruta (mismos bytes en Temp,
  bloqueado igual); CTRL2 elimina el creador (copia de PowerShell, bloqueada
  igual). Queda el lanzador/contexto como variable discriminante.
- **Limitaciones**: (1) el exec gestionado por go es no-determinístico entre días
  (falló otras jornadas: CLI full, mbench, harness T8) — la regla es asimétrica:
  shell-directo siempre bloquea (9/9 hoy + historial), go-gestionado a veces corre;
  (2) el mecanismo interno exacto por el que go-gestionado evita el 3077 es
  desconocido (sin ETW/ProcMon disponible en el host: no instalado);
  (3) integridad Medium en todos los intentos (constante, no explica diferencias);
  (4) sin Mark-of-the-Web medido (streams no legibles en este FS: registrar como
  unknown, ver F14.7).
- Experimento que refutaría: un binario lanzado por bash que corra, o un `go test`
  que genere 3077 para el mismo hash el mismo día. Ninguno observado.
