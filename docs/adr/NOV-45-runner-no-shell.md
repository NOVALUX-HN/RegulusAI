# NOV-45 — Runner nativo sin shell (T24)

Implementado: `StructuredRunner` + `StructuredCmd` en `internal/gates/gates.go`.

- `exec.CommandContext` directo, sin `sh -c/pwsh/cmd /C`; nunca usa `shellName/shellArgs/runShell`.
- `executil.HideWindow`, captura acotada `runShellCap`, timeout por ctx, exit code preservado, `EnvUnset` y `Env` sin concatenar, metacaracteres literales.
- `Executable` vacío → error no-verdict (-1).

Inventario helpers:
- Eliminados: ninguno (cli-check.exe no existe en repo).
- Conservados: `cmd/regulus/regulus.manifest` asInvoker, `executil.HideWindow`, `internal/runtime/` sin shell (0 matches), `ShRunner` legacy para comandos que sí pidan shell explícito.

Matriz WSL2 vs nativo:
- WSL2/Ubuntu (sin Go instalado, repo en /mnt/c): unit/core portables, sin semántica Windows.
- Nativo Windows: filesystem, rutas, permisos, procesos, señales, integración, empaquetado. WSL2 no reemplaza.
- Temporales `go test`: no resolubles solo con código bajo Enterprise signing; mitigación `go test -c -o C:/Temp/` + run directo (ver NOV-43).

Tests: `internal/gates/structured_test.go` 4/4 PASS vía `C:/Temp/gates.test.exe` (Temp go-build bloqueado por policy 0283ac0f, ver NOV-43). `go vet` + `go build` limpios. Full gates: 2 fallos preexistentes ambientales (receipt dup + cwd regulus.json), no del cambio.
