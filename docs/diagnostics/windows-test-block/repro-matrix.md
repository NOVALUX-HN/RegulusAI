# F14.3 — Matriz de reproducción y ciclo de vida del ejecutable

Fecha: 2026-11-17. Referencias previas leídas: `scripts/test-windows.ps1`,
`docs/windows-diagnosis.md` (NOV-43, hipótesis Temp-específico + reputación por hash),
issues NOV-43/45/46/47/66 (todos Done). Lo nuevo de esta matriz: el discriminador
padre/contexto y la no-determinismo por binario fresco.

## Matriz (comando exacto, cwd = repo, usuario no elevado)

| # | Escenario | Compilación | Archivo | Carga/Ejecución | Etapa del fallo |
|---|---|---|---|---|---|
| A | `go vet ./internal/mbench/` | exit 0 | n/a (sin exec) | n/a | — (no ejecuta) |
| B | `go build ./...` | exit 0 | n/a (sin exec) | n/a | — (no ejecuta) |
| C | `go test -count=1 -run TestCorpusPinned ./internal/mbench/` (temp SO) | exit 0 | creado en Temp go-build | **corre** (1.2s y 0.2s, dos corridas) | ninguna |
| D | `go test -c -o .regulus-test/t3-d.test.exe` + exec directa vía bash | exit 0 | `.regulus-test/t3-d.test.exe` 6.2 MB, SHA256 `2CB3…10C`, NotSigned | **bloqueado** (`Permission denied`), 3077 09:37:19 | carga del ejecutable |
| E | `go test -c -o .tmp-gotest/t3-e.test.exe` + exec directa vía bash | exit 0 | `.tmp-gotest/t3-e.test.exe`, mismo bytes que D | **bloqueado**, 3077 09:37:20 | carga del ejecutable |
| F | `touch` + `go run ./cmd/regulus bench mission --dry-run` (relink forzado) | exit 0 | temp go-build | **corre**, lista 10 casos | ninguna |
| G | `.tmp-gotest/govern.test.exe` (16/11, sin firma) exec directa | n/a (preexistente) | SHA256 `5B66…` | **corre** | ninguna |
| T2 | `go test -c` mbench + exec directa (repro T2) | exit 0 | `t2-probe.test.exe` = mismos bytes que D/E | **bloqueado**, 3077+3089+3118 09:35:03 | carga del ejecutable |

## Hallazgos

1. **La compilación y la creación del archivo nunca fallan.** El fallo es siempre en la
   carga: el proceso padre (`bash.exe` o `go.exe`) intenta cargar el `.test.exe` y Code
   Integrity lo deniega (3077, Requested 2 vs Validated 1, `0xc0e90002`).
2. **Mismos bytes, distinto resultado según quién lo lanza**: `2CB3…10C` (D/E/T2, lanzado
   por `bash.exe`) → bloqueado; binario equivalente del mismo paquete lanzado por `go.exe`
   vía `go test` (C, dos corridas) → corre; `go run` con relink forzado (F) → corre;
   `5B66…` lanzado por bash (G) → corre.
3. **No-determinismo real**: binarios frescos a veces corren (`go run` del harness T8 falló
   una vez y pasó al reintentar; `go test` de `internal/cli` y `internal/mbench` fallaron
   otros días). Cada link fresco se evalúa de nuevo; el resultado varía. Por eso **ningún
   flujo puede depender de ejecutar el binario recién linkeado sin verificación**.
4. **Firma descartada como discriminador** (ambos .test.exe `NotSigned`, uno corre y otro no).
   Reputación/antigüedad del hash + contexto del padre explican los casos mejor que la
   ubicación (Temp vs repo vs sidecar: los tres se bloquearon alguna vez).
5. `go vet`, `go build` y `go run --dry-run`-style nunca sustituyen evidencia de tests
   ejecutados: solo prueban compilación o paths que no ejecutan binarios frescos.

## Ciclo de vida del ejecutable (diagrama)

```
go build/test -c --(exit 0)--> archivo .exe (hash, sin firma, sin MoTW observado)
        |
        +-- lanzado por go.exe (go test / go run) --> a veces corre, a veces 3077
        +-- lanzado por bash.exe (exec directa)   --> 3077 Error + 3033 + 3089 + 3118
                                                     (Policy VerifiedAndReputableDesktop
                                                      {0283ac0f…}, Publisher Unknown)
                                                     --> Permission denied / Device Guard
```

## Paquetes y estado

- Ejecutados realmente en esta sesión: `internal/mbench` (C, PASS 2 veces), `internal/cli`
  subset previo, `internal/mission`, `internal/cmdx`, `internal/govern` (días previos).
- Bloqueados al ejecutar (no al compilar): `internal/cli` full suite, `internal/mbench`
  (una corrida), harness T8 (una corrida, pasó al reintentar).
- Artefactos preservados: `docs/diagnostics/windows-test-block/events/` (XML),
  `.tmp-gotest/t2-probe.test.exe` + `.regulus-test/t3-d.test.exe` (hash `2CB3…10C`).
  Limpieza: `.regulus-test/t3-*.exe` y `.tmp-gotest/t3-e.test.exe` se eliminan tras la matriz
  (son repro, no entregables).
