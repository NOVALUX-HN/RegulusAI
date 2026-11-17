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
