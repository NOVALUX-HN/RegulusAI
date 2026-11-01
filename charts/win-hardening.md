# win-hardening — fragilidad Windows restante (follow-up)

User authorization: "Fragilidad Windows" elegido 2026-10-31 como primer punto. Follow-up de `charts/win-portability.md` (W1–W4 CLOSED, gauntlet 9/9) — no reabre W1–W4. Read down to `## Log` before working.

Class: COMPLEX

Domain: sextant

## Contract

- Objective: eliminar la fragilidad Windows restante fuera de W1–W4: CRLF en defaults, ambigüedad de binario, mensaje unstamped con ruta Unix, test POSIX-only que rompe sin `sh`.
- Scope: `.gitattributes`, `scripts/sync-plugin-defaults.ts`, `.opencode/plugins/regulus.ts` (parse + dataBlock + resolve log), `internal/doctor/version.go` + test, `internal/gates/policy_test.go` + `gates.go` si hace falta helper portable. Sin tocar toolchain, retención, memoria.
- Constraints: cambios mínimos portables (LF en repo, `\r?` tolerante en runtime); sin nuevos deps; `bun test ./.opencode/lib/` y `go test ./internal/doctor/ ./internal/gates/ -run TestShRunner|TestGofmt` deben seguir verdes.
- Acceptance: S1, S2, S3, S4
- Artifacts: `bin/regulus`, receipts `win-hardening/H*`, `.gitattributes` extendido.
- Dependencies: H4 desbloquea gauntlet para H1–H3 (unit-tests); H1–H3 independientes entre sí.
- Risk: STANDARD-equivalente por unidad (sin destructive/security/contracts/concurrency/delivery); Class COMPLEX solo por multi-unidad.
- Verification: `spec check charts/win-hardening.md` + `gate run win-hardening/H*` por unidad + gauntlet completo al cierre.
- Completion: las 4 tasks CLOSED con receipts PASS y `spec OK: 4`.

## Acceptance

### Scenario: S1 CRLF-tolerant defaults render

- Given `discipline.md` / `tool-guidance.md` con CRLF (checkout Windows `core.autocrlf=true`)
- When `renderDisciplineDefault` / `renderGuidanceDefaults` / `parseToolGuidance` / `dataBlock` procesan el texto
- Then el output es idéntico al LF (sin `\r` filtrado, marcador encontrado, `sync-plugin-defaults --check` pasa)

### Scenario: S2 binary resolution logged

- Given varios binarios presentes (`bin/regulus.exe`, `bin/regulus`, `binaries/...`, PATH)
- When el plugin resuelve el binario regulus
- Then el binario elegido queda logueado (first-match-wins visible para troubleshooting, sin cambiar el orden)

### Scenario: S3 unstamped message OS-aware

- Given un build dev unstamped (`-ldflags` sin versión) en Windows
- When `regulus doctor` imprime la línea de versión
- Then el WARNING nombra `scripts/build.ps1` en Windows (y `scripts/build.bash` en Unix), ambos vía `npm run build`

### Scenario: S4 portable ShRunner env test

- Given una máquina Windows sin `sh` en PATH (fallback `pwsh`/`cmd`)
- When `go test ./internal/gates/ -run TestShRunnerEnvUnsetFilter` corre
- Then el test pasa (usa comando portable sin sintaxis POSIX `${VAR:-}`, verifica EnvUnset filtra y zero-value conserva environ)

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| H1 | S1 | CRLF hardening: `.gitattributes` (TS/MD/JSON `text eol=lf`) + normalizar `split("\n")` en `sync-plugin-defaults.ts:20`, `regulus.ts:95-99 dataBlock`, `regulus.ts:70-82` + `domain` espejo + tests | @regulus-worker |
| H2 | S2 | log binario elegido en `resolveRegulus` (`regulus.ts:170-197`) sin cambiar orden | @regulus-worker |
| H3 | S3 | `VersionLine` OS-aware en `internal/doctor/version.go:30-32` + test | @regulus-worker |
| H4 | S4 | portable `TestShRunnerEnvUnsetFilter` (`internal/gates/policy_test.go:236-255` + helper si hace falta) sin sintaxis POSIX | @regulus-worker |

## Log

- L1: Chart opened 2026-10-31 desde "Fragilidad Windows". Scout `ses_e6d03bc16ffeiVIOa50V6kUhsT`: prod flashes cubiertos, CRLF caliente ya tolerante en 4 regex, quedan `split("\n")` estrictos, resolve sin log, unstamped cita `.bash` en Win, `.gitattributes` solo Go.
- L2: REPLAN 2026-10-31 — H1 hecho (`ses_e6d02fcbfffeP1P1u7Ebg6pSVM`, 4 ficheros +63/-3, bun 62/0, sync in sync) pero gauntlet `win-hardening/H1` frena en layer 4 `TestShRunnerEnvUnsetFilter` (preexistente, reproduce con stash). Causa: comando POSIX `${VAR:-absent}` no expande en `pwsh/cmd`. Se añade S4/H4 desbloqueador.
- L3: H1-H4 CLOSED 2026-10-31 — H1 CRLF, H2 resolve-log, H3 VersionLine OS-aware, H4 portable ShRunner test. Tras `crap-paydown` C1–C7, re-run propio: `win-hardening/H1` 12/12, H2 12/12, H3 12/12, H4 12/12 (gate_run tool, receipts en `.regulus/evidence/win-hardening__H*/`). `spec OK: 4`. Nota: `bin/regulus task close` no existe en 0.9.0 (`unknown command "task"`), cierre manual en log con receipts.
- L4: REPAIR 2026-10-31 — `npm run build` mostró `build.bash: $'\r'` + `pipefail invalid` (CRLF en scripts, `.gitattributes` solo cubría go/ts/md/json). Fix `d355993`: `*.bash/*.sh/*.ps1 text eol=lf` + normalizar 4 scripts a LF; `bash -n` 0; `bash build.bash` ya pasa líneas 10–11 y solo pide `node` (ausente en WSL, fallback ps1 por diseño W2). Commits `50249aa` (H), `5ee7d31` (C), `d355993` (sh).
- L5: REPAIR 2026-11-01 — `status` decía `gremlins NOT found` en Windows aunque existe (`%USERPROFILE%\go\bin`): usaba `command -v` + `$HOME/go/bin` vía shell (inexistente en pwsh). Fix: probe puro Go (`LookPath` + fallback por OS) + hint pineado `@v0.6.0` + `status_gremlins_test.go` (3 subcasos). `status` → `gremlins available`.
