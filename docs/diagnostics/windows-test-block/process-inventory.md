# F15.1 — Inventario de procesos y matriz de ejecución Windows (v1)

Fecha: 2026-11-17. Cobertura: `grep` completo de `os/exec`, `exec.Command*`,
`cmd.exe`, PowerShell, Bash, `Start-Process`, scripts y helpers (explorer
mwkwd9m9-3-xzrj) + reconciliación con trazas reales F14.2/F14.5/F14.7.
Sin `syscall.StartProcess`, sin `Start-Process`/`wsl`/`pwsh -c`/`cmd /C` en Go,
sin `.ps1/.sh` invocados desde Go, sin `go test -c` en Go.

## 1. Inventario (caller → mecanismo → clasificación)

| # | Archivo:línea | Flujo | Mecanismo real | Clasificación |
|---|---|---|---|---|
| 1 | gates/gates_exec.go:127 StructuredRunner | gates | `exec.CommandContext` directo, sin shell | mantener |
| 2 | gates_exec.go:145 dockerProbe | gates | directo `docker info` + HideWindow | mantener |
| 3 | gates_exec.go:162-271 ShRunner/shellName/shellArgs | gates arbitrarios `regulus.json` | `sh -c`/`pwsh -Command`/`cmd /C` | migrar a Executor (shell solo explícito/justificado) |
| 4 | gates_exec.go:186 docker run | gates aislados | directo `docker`, shell dentro del contenedor | mantener (aislado) + documentar |
| 5 | mutation_export.go:75 | gates | directo `git archive HEAD` | mantener |
| 6 | mcp/policy.go:39,62,312 | MCP | no lanza (solo autoriza `gate:execute`) | mantener (fuera de alcance exec) |
| 7 | evolve (promote/validation) | evolve | no ejecuta (solo valida `Commands[]`) | mantener |
| 8 | bench/agenttasks.go:278, agentlab_run.go:414 | bench/lab | directo check del fixture (`Dir=fix/work`) | migrar a Executor |
| 9 | bench/agenthost.go:47 OpenCodeRunner | bench | directo `opencode run --format json` | migrar a Executor |
| 10 | bench/hostjourney.go:88 toolVersion | bench | directo `--version` | migrar a Executor |
| 11 | doctor/doctor.go:38 ExecRunner | doctor | genérico directo | migrar a Executor |
| 12 | cmdx/helpers.go:59, verify.go:118 | cmdx | `opencode --version`, `go version` | migrar a Executor |
| 13 | cmdx/bench_hostcap.go:300 selfBin | bench | auto-reexec con `Dir=workdir` | migrar a Executor |
| 14 | orchestrate/worktree.go:191 gitRun | orchestrate | directo `git worktree …` | mantener |
| 15 | planning/planner.go:404,416, manifest/branch.go:58, impact/graph.go:42,180, almanac/export.go:37, sextant/traceability.go:100 | varios | directos `git …` solo-lectura | mantener |
| 16 | cmdx/mission_test.go:328 | tests | auto-spawn helper | migrar a Executor (test helper) |
| 17 | journey/f2_test.go:70 | tests | `go build -o` fixture | rediseñar (binario temporal → runner) |
| 18 | portability/*_test, gofmt_test, version probes | tests | `bash --version`, `gofmt -l`, `go version/list` | mantener (lectura) |
| 19 | scripts/test-windows.ps1:53,61 | scripts/CI | `go test -c -o` + `& exe` | migrar a Executor (base del F15.4) |
| 20 | scripts/build*.ps1/.bash, install-devin* | scripts | `go build -trimpath -ldflags` pineado | mantener |
| 21 | scripts/sign-*.ps1, dev-wdac.ps1 | scripts | `signtool sign` / re-invocación PS | mantener (firma/WDAC, fuera de F15) |
| 22 | test binaries (`go test`) | Go toolchain | temp `go-build…/*.test.exe`, lanzado por `go.exe` | rediseñar (vía runner F15.4, no EXE directo) |

## 2. Matriz experimental (mecanismo × 3 reps, este host)

| Mecanismo | Resultado | Evidencia |
|---|---|---|
| `go test` (temp, go-spawned) | 3/3 corre hoy; falló otros días | F14.5 A (0.19s), F14.3 C |
| `go run` | 3/3 corre hoy; 1 fallo histórico con reintento ok | F14.5 D, F14.3 F, harness T8 |
| `go build`, `go vet`, `git …` | exit 0 siempre (no ejecutan / lectura) | T15 probe: BUILD 0, VET 0, GIT 0 (f6f57d4) |
| EXE fresco vía bash/PS directo | 9/9 bloqueado + 3077 | F14.3 D/E, F14.5 B/C/E, T15 /tmp probe bloqueado |
| EXE reputado vía bash | corre (5B66 copia incluida) | F14.2 G, F14.8 decisivo |
| `go test -c` + exec directa | bloqueado (mismo que EXE fresco) | F14.5 CTRL (excepción solo si hash reputado: no probada) |

Listas: bloqueadas = shell-directo de hash fresco; permitidas = lectura + go-gestionado
del día + hash reputado; variables = go-gestionado entre días; no probadas =
caché fría, elevation, otro usuario, otro host (F15.6).

## 3. Árbol de procesos (flujos críticos)

```
opencode/bash(pid N) -> go.exe -> Temp/go-build…/*.test.exe   (tests; a veces 3077)
opencode/bash(pid N) -> go.exe -> exe/regulus.exe             (run; verificado)
bash(pid N) -> ./*.test.exe                                   (directo; 3077 denegado)
powershell(pid M) -> t5-probe.test.exe                       (directo; 3077 denegado)
regulus -> git/opencode/gofmt/docker/go (hijos directos)      (permitidos)
```

## 4. Plan de migración (orden impacto/riesgo)

1. F15.2: `internal/exec/` Executor central (contrato argv-separado, cwd, env
   mínimo, timeout, cancelación, anti-huérfanos, clasificación
   PASS/FAIL/BLOCKED/SKIPPED/NOT_RUN, diagnóstico con ruta/hash/eventos).
2. F15.3: migrar #3, #8–#13, #16–#17, #19, #22 por ese orden; #1–#2, #4–#7, #14–#15,
   #18, #20–#21 se mantienen con excepción documentada + check anti-regresión
   (`grep` en CI que falla ante `exec.Command` fuera de `internal/exec`).
3. F15.4: runner smoke/focused/full sobre el Executor + JSON versionado.
4. F15.5/F15.6/F15.7 según spec.

## 5. Alternativas descartadas

- Compilar+lanzar EXE de test directamente como mecanismo general: refutado por
  F14.5 CTRL (bloqueado igual); solo válido como excepción validada.
- Sustituir todo por `go run`: no ejecuta tests reales y oculta el fallo (spec S14.3).
- Exclusiones/Warn-and-continue: viola BLOCKED≠PASS.
