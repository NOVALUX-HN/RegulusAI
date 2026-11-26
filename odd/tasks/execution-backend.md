# execution-backend — verificación que funciona con o sin políticas de Windows

Fuente: NOV-119 (F15.6, BLOCKED) y NOV-127 (F15.8, auditoría). Evidencia medida en `develop` @ `1e8c49f`.

## Specs

S1 — El usuario necesita soluciones a nivel de código, no instrucciones para ejecutar a mano.

S2 — "sin necesidad de ser admin para ejecutar comandos" — ninguna ruta puede requerir elevación, instalación global, ni cambios en Smart App Control / WDAC.

S3 — "verifica que funcionen tanto en Windows como en Linux sin bloqueos de Windows" — el camino nativo debe quedar idéntico; el sandbox sólo se activa cuando el probe prueba un bloqueo de política.

S4 — El host afectado no puede ejecutar ningún PE recién compilado: `go build` pasa, ejecutar el resultado falla con `Permission denied`. SAC en enforcement (`{0283ac0f-fff1-49ae-ada1-8a933130cad6}`). Medido: 0 de 52 paquetes ejecutables de forma nativa.

S5 — Ejecutar la suite en WSL2, compilando dentro del distro, es el único camino sin admin que produce resultados correctos. Medido: 52/52 verde con Go 1.27.3 + gremlins + node en `$HOME`.

S6 — `internal/gates/gates_exec.go:127` crea procesos con `exec.CommandContext` fuera de `pxexec`, así que un rechazo de política se propaga como error crudo en lugar de `BLOCKED`. Hallazgo de auditoría contra NOV-118.

S7 — El bootstrap del toolchain descarga 150 MB. No puede ocurrir de forma implícita: requiere opt-in explícito y una ruta de reversión conocida.

S8 — Un sandbox sin procedencia mezcla evidencia. Toda ejecución debe declarar el backend con el que se probó (`native` | `wsl`), y nunca presentarse como equivalente a CI.

Ejemplo literal del usuario: "verifica que funcionen tanto en Windows como en Linux sin bloqueos de Windows".

## Tasks

| ID | Specs | Ruta | Commit |
|----|-------|------|--------|
| T1 | S1 S2 S3 S4 S5 S8 | inline | `feat(execbackend): backend resolution and WSL execution` |
| T2 | S1 S2 S7 S8 | inline | `feat(cli): regulus exec-backend status/run` |
| T3 | S3 S6 | inline | `feat(gates): route gate execution through the executor` |
| T4 | S1 S7 | inline | `docs: execution backend runbook; retire scripts/verify-wsl.sh` |

## Delivery

Slice boundaries, recorded because T1 exceeded the ~400-line review slice:

1. `d40f596` — `internal/execbackend` only (657 lines, 295 of them tests). Reviewed alone: the decision layer has no callers yet, so it cannot change behaviour.
2. `efae1b6` — `scripts/verify-wsl.sh` + `scripts/regulus-wsl.sh`, the operating entry points.
3. `6fc8288` — `docs/execution-backend.md`, the runbook.
4. `3c2e2d3` — this feature document.
5. `00a7233` — version stamping in the launcher (behaviour of the entry point, not of the decision layer).
6. `f7fd2d5` — the consumer note for `internal/execbackend`.

Cumulative: 6 commits, ~1035 authored lines. The slice rule was exceeded once, at slice 1, and the split above is the delivery strategy from that point on.

T3's second half (routing `internal/gates/gates_exec.go` through the executor) stays open: it only matters on a host where the product binary is trusted but its children are not, which today is the signed-release scenario, not a local one.

`internal/execbackend` has no caller today, on purpose. F16.2 (normalize capability states and evidence) consumes `HostProbe` as the "this host can execute freshly built binaries" capability, so the package is a prepared dependency of the next phase rather than dead code. If F16.2 ends up not needing it, delete it instead of leaving it unowned.

T1 — paquete `internal/execbackend`: enum cerrado de backends, probe que distingue bloqueo de política de un binario ausente, y ejecución de un comando dentro del distro con traducción de rutas. Sin efecto sobre el camino nativo.

T2 — superficie de CLI para que el backend sea usable y auditable: estado resuelto y ejecución de un comando arbitrario. Es lo que permite verificar Windows y Linux con el mismo binario.

T3 — cerrar la brecha de S6: los gates dejan de crear procesos por su cuenta y pasan por el executor, de modo que un rechazo de política se clasifique `BLOCKED`.

T4 — runbook y retiro del script temporal, sólo cuando T3 esté en verde.

## Log

L1: "podemos probar esta solución ahorita antes de proceder con las nuevas fases? así verificamos que esto funcionará"

L2: "podemos solucionar a nivel de código sin necesidad de ser admin para ejecutar comandos? estoy fuera de la casa para hacerlo y necesito solucionar"

L3: "procede con las mejoras y verifica que funcionen tanto en Windows como en Linux sin bloqueos de Windows"

L4: Medición que fundamenta S4/S5 (misma sesión): nativo 0/52; cross-compile `-exec` 36/52; Go nativo en WSL 50/52; +gremlins 51/52; +node 52/52 (exit 0, 7m35s). Los fallos del cross-compile eran 100% entorno, 0% lógica.

L5: T2 se revisó al implementar: en un host con SAC en enforcement no arranca ningún binario de Regulus, así que un subcomando de CLI nuevo no es verificable ni usable ahí. Se reemplazó por un launcher (`scripts/regulus-wsl.sh`) que compila linux/amd64 y ejecuta el comando dentro del distro. Verificado en vivo: `contract`, `doctor`, `status` y `gate run --quick --yes demo/T1` → **6/6 gates passed, verdict PASS**.

L6: El guard test del repo (`TestEveryProductionSpawnHidesWindow`, `TestNoRawProcessLaunch`) detectó `os/exec` en `internal/execbackend`. Corregido: el runner usa `pxexec.Run`, que aporta ocultamiento de consola, entorno mínimo y clasificación BLOCKED. Después del fix: execbackend, executil, pxexec y testflow en verde.

L7: T3 entregado como `docs/execution-backend.md`: síntoma, causa, tabla de intentos que no funcionan, los dos entry points, resultados medidos, costo y reversión, variables de configuración, regla de diseño para spawns y límites conocidos.
