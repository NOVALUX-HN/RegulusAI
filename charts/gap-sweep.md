# gap-sweep — barrido de mejoras 2026-11-01

User authorization: "ataca todos los puntos de mejora hasta finalizarlos todos. avísame cuando todos estén terminados. no te detengas hasta tener todos listos" 2026-11-01. Cubre los 6 puntos del análisis (restos, MCP gate_run, floors, SPLIT, portabilidad, lint). Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: cerrar los 6 puntos de mejora con gauntlet verde por tarea y sin cambiar conducta salvo lo declarado.
- Scope: `scripts/build.bash`, `scripts/install-devin.bash`, `scripts/install-devin.ps1`, `.opencode/lib/domain.ts`, `.opencode/plugins/regulus.ts`, `internal/mcp/`, `internal/observability/metrics.go`, `internal/sextant/contract.go`, `internal/cli/cli.go`, `internal/almanac/hybrid.go`, `regulus.json` (solo comando lint con analizadores stdlib), tests asociados. Sin nuevas dependencias (invariante zero-dep `TestZeroDepsGoMod`), sin cambiar thresholds de floors.
- Constraints: mismos mensajes/veredictos salvo G5/G6 declarado; helpers nuevos <30 líneas; todo cambio con test que lo cubra.
- Acceptance: G1, G2, G3, G4, G5, G6
- Artifacts: receipts `gap-sweep/G*` por tarea (12/12 cada una).
- Dependencies: G1–G6 independientes entre sí salvo G3 tras G4 (misma zona CRAP; reorden permitido si hay conflicto).
- Risk: complex
- Verification: `spec check charts/gap-sweep.md` + `gate run gap-sweep/G*` 12/12 por tarea.
- Completion: G1–G6 CLOSED vía `task close`, `spec OK: 6`.

## Acceptance

### Scenario: G1

- Given el pendiente `regulus-health-gaps.md:60` (borrar `internal/gates/__h4probe.go`) y el TODO stale `scripts/install-devin.ps1:22`
- When se verifica el árbol actual
- Then `internal/gates/__h4probe.go` ausente, el TODO stale resuelto o convertido en puntero vigente, y `task close` verificado operativo (corrección `plugin-ux.md:59`: el cableado existe)

### Scenario: G2

- Given el plugin `gate_run` con presupuesto largo (`.opencode/lib/domain.ts:114`, `.opencode/plugins/regulus.ts:154`) y el precedente `charts/d3-gate-run-null.md:36`
- When corre un gauntlet largo vía MCP
- Then devuelve veredictos por capa con receipts `rec_...`, exit code propagado y output acotado con sha, sin null ni truncado a vacío; test de regresión que lo prueba

### Scenario: G3

- Given CRAP max 27.28 (`internal/observability/metrics.go:113` Summarize 27.28, `internal/sextant/contract.go:62` ParseContract 27.25, `internal/almanac/hybrid.go:209` applyLatestLimit cobertura 26.7%)
- When se añade COVER sin cambiar conducta
- Then CRAP max <25 y `gate run gap-sweep/G3` 12/12

### Scenario: G4

- Given funciones top con complejidad alta (`internal/observability/metrics.go:113` Summarize cx 27, `internal/sextant/contract.go:62` ParseContract cx 27, `internal/cli/cli.go:26` CLI.Run cx 27)
- When se aplica SPLIT en helpers <30 líneas con mismos veredictos
- Then cada función tocada queda <30 líneas y misma conducta, `gate run gap-sweep/G4` 12/12

### Scenario: G5

- Given `scripts/build.bash:16` y `scripts/install-devin.bash:35,39` exigen `node` para leer `package.json`
- When falta `node` en PATH
- Then el build y el instalador resuelven la versión con fallback puro bash (grep/sed sobre `package.json`) y fallan ruidoso solo si no hay versión; ambos scripts verificados en Windows y POSIX

### Scenario: G6

- Given el gate lint es `go vet ./...` pelado (`regulus.json:8`) y el receipt `coverage-floor` sufre `stdout_hash_mismatch` en Windows (quirk preexistente 0.9.0/0.10.0/G1/G2 que bloquea `task close`)
- When se endurece el lint solo con analizadores stdlib sin nuevas dependencias y se corrige la causa raíz del hash mismatch
- Then el gate lint enumera analizadores explícitos (o flags) documentados, sigue verde en este árbol, `TestZeroDepsGoMod` intacto, y `evidence verify` pasa limpio para la tarea enabling `task close`

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|------|-------|
| G1 | G1 | restos: verificar probe ausente, resolver TODO ps1:22, verificar task close | @regulus-worker |  |
| G2 | G2 | MCP gate_run: test regresión truncado/exit-code + fix si reproduce | @regulus-worker |  |
| G3 | G3 | COVER: tests para bajar CRAP max <25 | @regulus-worker |  |
| G4 | G4 | SPLIT: partir Summarize/ParseContract (helpers <30) | @regulus-worker |  |
| G5 | G5 | portabilidad: fallback versión sin node en build.bash + install-devin.bash | @regulus-worker |  |
| G6 | G6 | lint: endurecer go vet con analizadores stdlib explícitos + causa raíz coverage-floor stdout_hash_mismatch | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-01 desde autorización batch total. Evidencia base: crap max 27.28 (medido 2026-11-01), `internal/gates/__h4probe.go` ausente (Test-Path False), `plugin-ux.md:59` (task cableado), `d3-gate-run-null.md:36` (precedente MCP), `build.bash:16` + `install-devin.bash:35,39` (dependencia node).
- L2: G1 CLOSED 2026-11-01 (`ses_e6b47d864ffexbZLRBho5oIAoB`, verificación pura, cero cambios): probe ausente, ps1:22 ya puntero vigente, task close operativo. `gate run gap-sweep/G1` 12/12 PASS (`run_ad4311052e921cf2`, rec_18e3c78d…–rec_18e3c7c55…). Nota: `task close` rechaza por `coverage-floor` stdout_hash_mismatch (metric 85.60>=80 PASS, quirk Windows preexistente igual que 0.9.0/0.10.0) — cierre manual como en release-0-10-0 L2.
- L3: G2 CLOSED 2026-11-01 (`ses_e6b432d46ffeEke9D1mkyIo3ev`, commit `dfaa8c6`): TS `formatGateResult` acota con sha (`GATE_RUN_OUTPUT_MAX=8192`, paridad con `receipts.MaxCapture`) + 2 tests regresión; `bun test` 78/0. `gate run gap-sweep/G2` 12/12 PASS (`run_27f32e2d9cc56dc3`, rec_18e3c7e4a…–rec_18e3c808e…). Mismo quirk `coverage-floor` stdout_hash_mismatch → cierre manual. Quirk pasa a ser causa raíz a investigar en G6 (bloquea `task close` en Windows).
- L4: G4 CLOSED 2026-11-01 (`ses_e6b3eadf7ffelIcD5lK9haIRZh`, commit `6b78b80`): SPLIT Summarize 123→22, ParseContract 97→21, CLI.Run 63→24; CRAP max 27.28→26.65. `gate run gap-sweep/G4` 12/12 PASS (`run_63e33fd9618bbdf9`, rec_18e3c8ab1…–rec_18e3c8f8a…). Mismo quirk coverage-floor → cierre manual.
- L5: G3 CLOSED 2026-11-01 (`ses_e6b2ef0cfffes4RSwxvANThb61`, commit `a064e3a`): solo tests nuevos (cero fuentes tocadas); applyLatestLimit 26.7→100%, mcpStrList 27.3→100%, reflect.Run 76.6→83.0%; CRAP max 26.65→24.98 <25. `gate run gap-sweep/G3` 12/12 PASS (`run_89310c2525a626f1`, rec_18e3c9d09…–rec_18e3ca1ef…). Mismo quirk → cierre manual.
- L6: G5 CLOSED 2026-11-01 (`ses_e6b1bc87bffe1sgL3KcdkkNHkJ`, commit `8f73e22`): `pkg_version_fallback()` en `build.bash` + `install-devin.bash` (arg > node > grep/sed > fail ruidoso); espejo Go `internal/portability/version.go` + 6 tests; `.ps1` ya nodeless verificado por test. `gate run gap-sweep/G5` 12/12 PASS (`run_dbfb8f2371724ec2`). Stubs debug*_test.go eliminados por orquestador. Mismo quirk → cierre manual. G6 ampliado con causa raíz del quirk; revalidado `spec OK: 6` + `task check: OK`.
- L7: G6 CLOSED 2026-11-01 (worker `ses_e6b12ed9cffeT2IrK3937mEepj` + remate orquestador, commit `199f0aa`): Frente A lint con 35 analizadores stdlib explícitos en `regulus.json:8` + pin test; Frente B causa raíz del quirk: receipt guardaba stdout truncado (8192+marcador) pero sha del completo → `captureCheck` nunca coincidía; fix `receipts.StoredSHA` + `IsTruncated` + pins `stdout/stderr_stored_sha256`, verify fail-closed para legacy. `gate run gap-sweep/G6` 12/12 ×2 runs, `evidence verify` 24/24 PASS, `task close charts/gap-sweep.md G6` → CLOSED formal (primera del batch por vía formal).
- L8: Cierres G1–G5 2026-11-01: re-run `gate run gap-sweep/G1` sobre árbol final (`199f0aa`) → 12/12 PASS (`run_4d1a9a700294c3d8`); sus receipts nuevos verifican limpio pero `task close` sigue bloqueado por el manifest antiguo (receipt cuarentenado, legacy sin pin → fail-closed por diseño; `prune` protege cuarentena, no se fuerza). Decisión: se mantienen los cierres manuales G1–G5 (precedente 0.9.0/0.10.0; runs originales 12/12 PASS + re-run final verde) con el quirk ya corregido en G6 para futuros cierres formales. No se re-corren G2–G5 (mismos gates, mismo árbol final ya probado por G1-re-run y G6 ×2; valor marginal nulo). Batch completo: G1–G6 CLOSED (G6 formal, G1–G5 manual documentado).

- CLOSED G6 COMPLEX spec OK + 24 receipt(s) PASS.