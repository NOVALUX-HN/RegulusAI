# NOV-43 — Diagnóstico reproducible Windows (T23)

Fecha: 2026-11-14. Sin desactivar protecciones.

Bloqueo reproducido:
- `go test ./internal/gates` → `fork/exec .../go-build.../gates.test.exe: Una directiva de Control de aplicaciones bloqueó este archivo.`
- Mismo toolchain/compilador: `go test ./internal/httpapi` PASS (1.6s). No todos los avisos tienen misma causa.
- Compilado estable: `go test -c -o C:/Temp/gates.test.exe ./internal/gates` COMPILE 0, SHA256 `7ACEAD8B165ECA6CF6497E7A4C2D69F0E05FDDDD31E54DC4612EEA11A1C8D725`, `NotSigned`.
- Directo `C:/Temp/gates.test.exe` → PASS (4/4 structured). Bloqueo específico a `...\AppData\Local\Temp\gpK0aAYu\go-build...\gates.test.exe`.
- CodeIntegrity Operational: 3077 + 3089 + 3033 + 3118, `go.exe` intentó cargar `gates.test.exe` que no cumplió Enterprise signing (Policy ID `0283ac0f-fff1-49ae-ada1-8a933130cad6`).

Árbol: `go.exe` → `gates.test.exe` (Temp go-build). Padre: `go test`. `go build`, `go vet` no ejecutan → pasan.

Inventario EXE/DLL:
- Helpers propios: `cli-check.exe` inexistente (find 0); nada que eliminar.
- `bin/regulus.exe`, `binaries/regulus-windows-*.exe` (generados, gitignored); `cmd/regulus/regulus.manifest` asInvoker.
- Temporales Go: `go-build...\*.test.exe` por paquete; reputación por hash, sin firma.

Clasificación:
- Shell innecesario: `internal/gates/gates.go:204-290` ShRunner `sh -c/pwsh/cmd /C` → resoluble con StructuredRunner (NOV-45).
- Helper eliminable: ninguno (cli-check no existe).
- Test temporal Go: `gates.test.exe` en Temp → no resoluble solo con código; mitigación: `go test -c -o C:/Temp/` + ejecutar, o WSL2 para unit/core.
- Binario distribución: `regulus.exe` reputación/firma → NOV-44, no bloquea dev.
- Política empresarial: Policy 0283ac0f Enterprise signing → depende de Windows, no de Regulus.

Hipótesis abiertas: por qué httpapi Temp pasa y gates Temp no (misma raíz Temp, distinto hash/contenido heurístico). Experimento: compilar ambos a C:/Temp y comparar 3077/3118.
