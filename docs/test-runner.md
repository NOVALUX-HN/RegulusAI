# Test Runner (F15.4 / NOV-125)

`regulus testflow run --mode smoke|focused|full [--packages a,b] [--skip a,b] [--save P] [--json]`

Verificación real de tests Go a través del executor centralizado (`internal/pxexec`):
nunca compila+lancea un EXE de test directamente salvo excepción validada (hoy:
ninguna). Modos: `smoke` (núcleo rápido curado), `focused` (paquetes explícitos),
`full` (todo lo descubierto con tests). Checks separados: `go build ./...` y
`go vet` son evidencia, no sustitutos.

Veredicto: PASS/FAIL/BLOCKED/SKIPPED/NOT_RUN por paquete; el modo full falla
(exit 1) ante FAIL, BLOCKED, NOT_RUN, skips no permitidos o cobertura incompleta
(descubierto ≠ ejecutado). Cero tests ejecutados nunca es PASS. JSON versionado
(`testflow-report-v1`) con commit, host, versiones, mecanismo, conteos y
artefactos; `--save` lo persiste.

Implementación: `internal/testflow` (descubrimiento `go list -f` con presencia
de tests, ejecución por paquete con timeout, clasificación pura y testeable,
veredicto, JSON) + CLI delgado `internal/cli/testflow.go` (exit 0 solo en verde).

Evidencia de funcionamiento en host afectado (2026-11-17, harness `go run` ×3):
descubrimiento ok, checks build/vet PASS, paquete smoke clasificado BLOCKED con
hint (binario fresco), veredicto false correcto — sin falsos PASS. Suite unitaria
del núcleo (`classifyGoTest`, `Verdict`) + ejecución E2E completa pendientes de
CI/F15.6 donde los binarios frescos no estén bloqueados.
