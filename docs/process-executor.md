# Process Executor (F15.2 / NOV-117)

Única API de ejecución de Regulus: `internal/pxexec`. Ningún módulo decide
cómo lanzar procesos; todos describen QUÉ (argv separado, sin shell) y el
executor decide CÓMO, observa, cancela y clasifica.

## Contrato

```go
res := pxexec.Run(ctx, pxexec.Spec{
    Name: "go", Args: []string{"version"},
    Dir:  root,                 // working directory explícito ("" = hereda)
    Env:  []string{"FOO=1"},    // se suma al mínimo (SystemRoot/TEMP/TMP/PATH)
    Stdin:   nil,               // opcional
    Timeout: 30 * time.Second,  // 0 = 120s; negativo = rechazado
    Secrets: []string{"token"}, // claves redactadas en Render/diagnóstico
    SkipReason: "",             // si no vacío: SKIPPED, no se lanza
})
// res: Outcome PASS/FAIL/BLOCKED/SKIPPED/NOT_RUN, ErrClass
// launch/policy/timeout/cancel/process-fail, Exit real, Duration,
// PID, Mechanism, Stdout/Stderr (cap 4MB, truncamiento declarado),
// BlockHint (ruta + SHA-256 + query de eventos) solo en BLOCKED.
```

Shell solo explícito y justificado: `Spec{Shell: "powershell", Justify: "..."}`.
Sin `Justify` se rechaza antes de lanzar. Prohibido: `Start-Process`-fallback
automático, ocultar errores con shell, tocar SAC/WDAC/Defender/AppLocker,
elevar privilegios.

## Reglas de selección del mecanismo

1. Binario toolchain/reputado + argv separado → `go-native` (directo).
2. Comando que exige glob/pipe/redirección → shell explícito con `Justify`
   archivado en el caller (F15.3 lo audita).
3. Test Go con binario temporal → vía runner F15.4, nunca EXE directo
   (excepción validada + documentada o nada).
4. Todo lo demás: directo o rediseño sin subproceso.

## Clasificación (nunca BLOCKED→PASS)

- `PASS`: exit 0.
- `FAIL`: exit ≠ 0 del proceso (código real preservado).
- `BLOCKED`: señal de política (App Control/Device Guard/SAC/`0xc0e90002`)
  o `EACCES` sobre ejecutable existente + `BlockHint` (ruta, SHA-256 si legible,
  query `CodeIntegrity/Operational` 3077/3089/3118). Diagnóstico, no bypass.
- `NOT_RUN`: fallo de lanzamiento, timeout (hijo señalizado; en Windows los
  nietos pueden sobrevivir sin job object — declarado, no oculto), cancelación.
- `SKIPPED`: `SkipReason` del caller (precondición no cumplida).

## Límites conocidos

- Sin job objects: en timeout/cancel se señaliza el hijo directo; nietos en
  Windows pueden quedar huérfanos (declarado en `Detail`).
- Entorno mínimo: `SystemRoot/TEMP/TMP/PATH/PATHEXT`; el resto lo pone el caller.
- Salida capada a 4 MB con aviso explícito.
- `Render()` redacta secretos (`Secrets` + patrones de `observability`).

## Tests

`internal/pxexec/pxexec_test.go`: contrato (Validate), skip, PASS real
(`go version`), launch-vs-fail, timeout (`ping -n 6`, 500 ms), cancel,
clasificador BLOCKED (señal sintética), argv con espacios, redacción.
Solo binarios inbox/toolchain: determinista bajo SAC.

`internal/pxexec/guard_test.go`: auditoría de lanzadores. Recorre el
repo (salta `.git`), parsea cada `.go` no-test fuera de `internal/pxexec`
y falla si llama `exec.Command`/`exec.CommandContext` o importa
`os/exec` sin excepción documentada. Excepciones: `internal/gates`
(ShRunner/StructuredRunner), `*_test.go`, `scripts/` (sin Go). La
allowlist cubre además los usos import-only sin lanzamiento
(`LookPath`, clasificación `exec.Error`, `HideWindow`) y los callers
pendientes documentados abajo.

## Migrated callers (executor-first, cero cambio de conducta)

Cada sitio conserva argv, cwd, streams, puntos de truncado y forma de
error; solo cambia el mecanismo (pxexec):

- `internal/doctor/doctor.go` — `ExecRunner.Run`: `Combined` +
  error mapeado (nil si PASS; si no, error con `Detail`).
- `internal/cmdx/helpers.go` — `probeOpenCodeVersion` (15s): `Stdout`.
- `internal/cmdx/verify.go` — `probeGoToolchain` (15s): `Stdout`,
  `"missing-go"` si falla.
- `internal/bench/hostjourney.go` — `toolVersion` (30s): `Combined`,
  primera línea ≤120 chars, `"%s probe failed: exit status N"`.
- `internal/orchestrate/worktree.go` — `gitRun`: `Dir` + `Combined`,
  error con forma `exit status N`.
- `internal/planning/planner.go` — `workState` (2×10s): `Stdout`;
  misma lógica de fallbacks.
- `internal/impact/graph.go` — `DefaultGitLookup`: `Dir` + `Stdout`,
  timeout 10s vía contexto (reemplaza goroutine+select manual con la
  MISMA semántica: timeout→`fallbackStamp`); `headOf` igual.
- `internal/sextant/traceability.go` — `git()`: `Stdout`; error con
  forma `"git ARGS: exit status N (STDERR)"` usando `res.Stderr`.
- `internal/bench/agenthost.go` — `OpenCodeRunner.Run`:
  `Dir=workdir`, `MaxOutput` 256K (truncado idéntico), deadline del
  ctx y quota/auth intactos, mismo raw+usage.
- `internal/cmdx/bench_hostcap.go` — journey sub: `Dir=workdir`,
  `Stdout`/`Stderr` separados, error `"%s: exit status N (STDERR)"`.

## Migrated (segunda oleada, misma regla)

- `internal/bench/agenttasks.go` — check: `Dir=fix`, entorno heredado
  (igual que antes), `Combined` + corte 2000 idéntico, `AgentInfra`
  ante BLOCKED (clasificador + fallback textual).
- `internal/bench/agentlab_run.go` — check: `Dir=work`, `EnvMode exact`
  con `labChildEnv(...)` byte-idéntico, corte `labDetailCap`, veredictos
  `LabInfraError`/`LabFail`/`LabPass` intactos.
- `internal/manifest/branch.go` — `runGit`: `Dir` + `Stdout`, `""`
  ante no-PASS (ciclos rotos: pxexec solo depende de hojas).
- `internal/almanac/export.go` — `ProjectName`: timeout 30s donde antes
  no había (delta documentado: un git colgado fallaba colgando; ahora
  falla cerrado como el resto de probes).
- `internal/gates/mutation_export.go` — `git archive`: 60s, captura
  capada a 64MB (delta documentado: antes sin cap; el tar mayor falla
  cerrado en vez de truncarse en silencio).
- `internal/cli/bench_agent.go` — `probeOpenCodeVersion`: `Stdout`.
- `internal/cli/bench_host.go` — journey sub: `Dir`, `Stdout`/`Stderr`,
  error `"%s: exit status N (STDERR)"`.
- `internal/cli/root.go` — `gitTopLevel`: `Dir` + `Stdout`.

## Exceptions (las audita el guard test)

- `internal/gates/gates_exec.go` (`ShRunner`/`runShell`/`StructuredRunner`):
  env-policy NOV-81 + kill-gate + cpu stats son security features,
  nunca lanzamientos planos. Alcance F15.4.
- `*_test.go`: orquestación de tests (runners falsos, probes de fixtures,
  helpers multiproceso, probes de portabilidad).
- `scripts/`: sin fuentes Go.
- Usos import-only de `os/exec` (sin lanzamiento, con motivo):
  probes `exec.LookPath` (`agenthost.go`, `cli/gate_run.go`,
  `cli/status.go`, `testenv.go`), clasificación `exec.Error` de
  runners inyectados (`doctor.go`, exigido por `doctor_test.go`),
  infra `HideWindow` (`executil`, usada por el propio pxexec).
