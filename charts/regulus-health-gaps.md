# regulus-health-gaps — coherencia gauntlet + higiene riesgos

User authorization: "Ambos packs" — respuesta a "¿Qué autorizas a chartificar/implementar?" tras el análisis de 7 brechas (2 altas). Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: CI nombra las 9 capas reales

- Given `regulus.json` con 9 gates configurados
- When se lee `.github/workflows/gauntlet.yml`
- Then el step del gauntlet dice `9 layers` (no `8 layers`) y el comentario coincide con el número real de gates

### Scenario: el gate spec cubre todos los charts

- Given `charts/*.md` con 14 charts activos
- When corre `bin/regulus gate run <task>` con el gate `spec`
- Then el gate valida todos los `charts/*.md` (o falla nombrando el chart roto), no solo `charts/regulus-ai-v1.md`

### Scenario: los checks TS viven en el gauntlet versionado

- Given `.github/workflows/gauntlet.yml` corre `bunx tsc --noEmit` y `bun test ./lib/` solo en CI
- When corre `bin/regulus gate run <task>` en local sobre un cambio TS roto
- Then el gauntlet local lo caza (gates TS presentes en `regulus.json`) o documenta por qué son CI-only con razón explícita

### Scenario: el binario local sale stampled por defecto

- Given `cmd/regulus/main.go` con `version = "dev"` por defecto
- When se construye con el flujo documentado (`scripts/build.bash` o `npm run build`)
- Then `regulus doctor` reporta `matches package.json` (no `WARNING — unstamped dev build`)

### Scenario: el índice semántico no va a stale en silencio

- Given `internal/cli/memory.go` degrada a léxico con `warning: semantic index stale`
- When se hace `save` tras el último `reindex`
- Then hay `reindex` automático o `status` recuerda el stale con el comando que lo repara, sin sorprender al usuario

### Scenario: la paridad del contrato está probada

- Given `internal/contract/contract.go` como barrera plugin↔core (`regulus.contract/v1`)
- When corre el gauntlet
- Then existe `contract_test.go` que pinnea la versión y la paridad con `.opencode/lib/domain.ts`, y la deriva falla en rojo

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| H1 | S1–S2 | `gauntlet.yml` 8→9 layers + gate `spec` a todos los `charts/*.md` | @regulus-worker |
| H2 | S3 | gates TS (`tsc --noEmit`, `bun test ./lib/`) en `regulus.json` o razón CI-only documentada | @regulus-worker |
| H3 | S4–S5 | stamp por defecto + `reindex`/aviso stale en `status` | @regulus-worker |
| H4 | S6 | `contract_test.go` paridad + espejo `install-devin.bash` + `gofmt` en gauntlet o razón | @regulus-worker |

## Log

- L1: Chart opened 2026-10-30 desde análisis scout (9 gates Go en `regulus.json:2-11`, TS solo CI en `gauntlet.yml:37-45`, spec solo 1 chart en `regulus.json:11`, stamp dev en `internal/doctor/version.go:29-30`, stale en `internal/cli/memory.go:325`, contract sin test en `internal/contract/contract.go:1-8`). Autorización "Ambos packs".
- L2: H1 CLOSED 2026-10-30 — S1 (`gauntlet.yml` "8 layers"→"9 layers") + S2 (gate `spec` ahora `go run ./cmd/regulus spec check charts/`; `specCmd` acepta `<file|dir> [more...]`, el fallo nombra el chart roto). Gauntlet `regulus-health-gaps/H1` 9/9 pass. Receipts: `typecheck-1793373988607634400`, `lint-1793373989675543900`, `build-1793373990300658400`, `unit-tests-1793373991455001400`, `coverage-floor-1793374035339705700`, `mutation-1793374036034500500`, `crap-1793374192259161800`, `vulncheck-1793374192648663200`, `spec-1793374195933534900` (dir `.regulus/evidence/regulus-health-gaps__H1/`).
- L3: H2 CLOSED 2026-10-30 — S3 (gates TS añadidos en `regulus.json`, capas 10–11: `ts-typecheck` = `bunx tsc --noEmit -p .opencode/tsconfig.json`, `ts-unit-tests` = `bun test ./.opencode/lib/`; sin `cd`/`&&`, portables vía ShRunner en sh/pwsh/cmd; bun 1.4.4 verificado en Windows, no hizo falta razón CI-only). Sondas negativas: `__h2probe.ts` con error de tipos → tsc exit 1 (TS2322), `__h2probe.test.ts` con 1 fallo → bun test exit 1 (56 pass/1 fail); sondas eliminadas, verde confirmado. Consistencia: `gauntlet.yml` "9 layers"→"11 layers", `docs/troubleshooting.md` "eight"→"eleven". H1 intacto (capas 1–9 sin cambios). Gauntlet `regulus-health-gaps/H2` 11/11 pass. Receipts: `typecheck-1793374361432859600`, `lint-1793374362514647600`, `build-1793374363024888200`, `unit-tests-1793374363668864800`, `coverage-floor-1793374444430196700`, `mutation-1793374445123301600`, `crap-1793374488580909100`, `vulncheck-1793374488946857600`, `spec-1793374491591771900`, `ts-typecheck-1793374491947755300`, `ts-unit-tests-1793374492392373500` (dir `.regulus/evidence/regulus-health-gaps__H2/`).
- L4: H3 CLOSED 2026-10-30 — S4 (`scripts/build.bash` + `scripts/build.ps1` ahora refrescan el binario local stampled con la versión de package.json además de `binaries/`; `npm run build` deja `bin/regulus[.exe]` stampled — `doctor` reporta `0.8.1 (matches package.json)`; `go build` crudo sigue unstamped por diseño y `doctor` avisa nombrando `npm run build`; `troubleshooting.md` + `README.md` documentan el flujo). S5 (`almanac.IndexState` fresh/stale/missing/empty; `regulus status` línea `memory:` y `regulus memory doctor` línea `index:` con `run \`regulus memory reindex\`` cuando stale/missing/corrupt; aviso de `memory search` ahora cita el comando que repara). H1/H2 intactos (capas 1–11 y conteo sin cambios; `TestZeroDepsGoMod` verde, sin go.sum). Gauntlet `regulus-health-gaps/H3` 11/11 pass. Receipts: `typecheck-1793374938604387400`, `lint-1793374939854645300`, `build-1793374940436222200`, `unit-tests-1793374941556368900`, `coverage-floor-1793374985099242700`, `mutation-1793374985795315500`, `crap-1793375027772116700`, `vulncheck-1793375029631846100`, `spec-1793375032350364800`, `ts-typecheck-1793375032892573500`, `ts-unit-tests-1793375033335587700` (dir `.regulus/evidence/regulus-health-gaps__H3/`).
- L5: H4 CLOSED 2026-10-30 — S6 (`internal/contract/contract_test.go`: `TestVersionIsPinned` fija `regulus.contract/v1` + `TestParityWithTypeScriptMirror` exige `REGULUS_CONTRACT` de `.opencode/lib/domain.ts` == `contract.Version`; deriva probada en rojo — espejo a `v999` → FAIL `contract drift: Go core speaks "regulus.contract/v1" but the TypeScript mirror speaks "regulus.contract/v999"`, restaurado → PASS. Espejo `scripts/install-devin.bash` mínimo portable: build stampleado + `~/.local/bin` (exige estar en PATH, sin fallback) + S1 (`contract`/`--version` vía PATH) + S2 (initialize por stdio con `timeout 30`, `grep -qF`, pipefail desactivado solo en ese pipeline por el SIGPIPE de `head`); `install-devin.ps1:22` TODO stale → puntero al espejo; `docs/devin.md` +1 línea bash. `gofmt` como capa 12 `go test ./internal/gates/ -run TestGofmtClean -count=1`: un gate `gofmt -l` pelado siempre sale 0 (vacuo) y `test -z "$(gofmt -l …)"` no es portable (este host no tiene sh — `where.exe sh` vacío — y los gates H2 pasan en sh/pwsh/cmd); gofmt sobre gofumpt (stdlib, invariante zero-dep `TestZeroDepsGoMod`). Higiene colateral: `internal/gates/gates.go` normalizado CRLF→LF vía `gofmt -w` (único fichero flagged; contenido idéntico salvo CR; build + suite verdes). Consistencia: `gauntlet.yml` "11 layers"→"12 layers", `docs/troubleshooting.md` "eleven"→"twelve". H1/H2/H3 intactos (capas 1–11 sin cambios). Sonda negativa del gate: `gofmt_test.go` con indentación a espacios → FAIL nombrando el fichero, restaurado → PASS. Omitido con razón: secret-scan/npm audit (fuera del mínimo H4); `install-devin.bash` no ejecutado en este host (sin shell POSIX disponible — riesgo residual bajo: espeja las 6 líneas manuales de `ps1:30-36` + estructura S1/S2 del ps1 verificado); sin `.gitattributes` `eol=lf` (mínimo; el mensaje del test dice `gofmt -w`). PENDIENTE ORQUESTADOR: borrar `internal/gates/__h4probe.go` (skeleton de sonda limpio y válido — el shell del worker bloquea deletes por diseño: `del` denegado) y re-verificar trivial (`go test ./internal/gates/`); el gauntlet de abajo se corrió con ese fichero presente pero es inerte (compila, gofmt-clean, sin statements). Gauntlet `regulus-health-gaps/H4` 12/12 pass. Receipts: `typecheck-1793375685137874900`, `lint-1793375686194924100`, `build-1793375686700395300`, `unit-tests-1793375687345287800`, `coverage-floor-1793375729380646200`, `mutation-1793375730055665300`, `crap-1793375773436382200`, `vulncheck-1793375773807790200`, `spec-1793375776721385100`, `ts-typecheck-1793375777074518000`, `ts-unit-tests-1793375777502847600`, `gofmt-1793375778859587600` (dir `.regulus/evidence/regulus-health-gaps__H4/`).
