# ci-green — GitHub Actions vuelve a verde

User authorization: CI log 2026-10-30 (gauntlet en `main`): `Restore cache failed: Dependencies file is not found ... Supported file pattern: go.sum` (warning) + `layer 4 unit-tests fail (15019ms)` en ubuntu-latest mientras en Windows el mismo árbol da 9/9. Read down to `## Log` before working.

## Acceptance

### Scenario: setup-go no pide go.sum

- Given un checkout fresco sin `go.sum` (invariante cero-deps, `TestZeroDepsGoMod`)
- When el workflow instala Go
- Then no hay warning de caché: el step desactiva la caché o la ancla a `go.mod`

### Scenario: el fallo de unit-tests en Linux es reproducible y localizado

- Given el mismo árbol que falla en CI
- When se corre `go test ./...` en Linux (WSL Ubuntu local con Go 1.27)
- Then el paquete/test que falla queda nombrado con su salida, igual que en CI

### Scenario: el gauntlet CI vuelve a verde

- Given el fix del fallo localizado
- When corre `bin/regulus gate run ci/push` en Linux
- Then 9/9 gates pass con receipts; el log CI lo muestra

### Scenario: un fallo futuro deja evidencia

- Given un gate rojo en CI
- When termina el job
- Then los receipts de `.regulus/evidence/ci__push/` se suben como artifact (siempre, incluso en fallo) para diagnóstico sin adivinar

## Tasks

| ID | Spec | Work | Route |
|---|---|---|---|
| C1 | S1 | `cache: false` (o ancla a `go.mod`) en `setup-go` de `gauntlet.yml` + quitar warning | @regulus-worker |
| C2 | S2–S3 | repro en WSL + localizar test que falla en Linux + fix portable | @regulus-worker |
| C3 | S4 | subir receipts como artifact en CI (always, en fallo también) | @regulus-worker |

## Log

- **L1**: Chart opened 2026-10-30 desde el log CI pegado por el usuario. WSL Ubuntu disponible local con Go 1.27 instalándose para repro honesto (sin adivinar).
