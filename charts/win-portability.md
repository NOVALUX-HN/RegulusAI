# win-portability — gauntlet y build portables a Windows

User authorization: "aplica las mejoras" (2026-10-30) tras el análisis completo del proyecto. Alcance: P0 portabilidad Windows — el gauntlet y el build deben funcionar en PowerShell sin `sh` en PATH. Read down to `## Log` before working.

## Acceptance

### Scenario: mutation gate runs without POSIX shell

- Given a Windows machine with Go and gremlins on PATH but without `sh` on PATH
- When `regulus gate run` executes the mutation gate
- Then the gate produces a pass/fail verdict on efficacy (not on missing shell) and writes its receipt

### Scenario: vulncheck gate resolves without Unix home path

- Given a Windows machine with govulncheck on PATH
- When `regulus gate run` executes the vulncheck gate
- Then the gate runs from PATH resolution (no hardcoded `$HOME/go/bin`) and writes its receipt

### Scenario: release build works from PowerShell and bash

- Given a Windows machine with Go and Node but without bash
- When the release build runs via the portable entrypoint
- Then all six platform binaries land in `binaries/` with version stamping

### Scenario: CI installs are reproducible

- Given the CI workflow on `main`
- When the gauntlet workflow installs gremlins and govulncheck
- Then both installs are pinned to explicit versions (no `@latest` floating)

## Tasks

| ID | Spec | Work | Route |
|---|---|---|---|
| W1 | S1–S2 | portable gate runner fallback (`sh` → `cmd`/PowerShell) + `regulus.json` mutation/vulncheck via PATH | @regulus-worker |
| W2 | S3 | portable build entrypoint (`scripts/build.ps1` + npm script fallback) | @regulus-worker |
| W3 | S4 | pin CI installs (`gremlins`, `govulncheck`, actions) + verify | @regulus-worker |
| W4 | S1 | mutation portable-fast (clean-export sin POSIX: sin `node_modules`/bloat por mutante, sin timeouts Win) | @regulus-worker |

## Log

- **L1**: Chart opened 2026-10-30 from full-repo analysis. P0 first under evolution budget (one candidate at a time).
- **L2**: W1 CLOSED — `internal/gates/gates.go` ShRunner fallback (`sh` → `pwsh/powershell/cmd`, 127 mapping), `regulus.json` mutation/vulncheck via PATH. Receipts: build/vet 0, `gates` tests ok.
- **L3**: W1b CLOSED — fixtures portables (`f2_test.go` sin `tail/mktemp/tar`, `init.go:189` smoke `echo`), `go test ./...` 19 pkgs ok.
- **L4**: W2 CLOSED — `regulus.json` coverage sin `tail`, crap via `go run`, `scripts/build.ps1` (6 targets) + `package.json` build fallback. Lead fix PS5.1 `-flag=val` → `-flag val`.
- **L5**: W3 CLOSED — `gauntlet.yml` gremlins@v0.6.0, govulncheck@v1.9.0, cero `@latest`. tsc 0, spec OK 4.
- **L6**: PROVE 2026-10-30 — `go test ./...` 19 pkgs ok; `cover` 83.5% ≥80; `crap` max 28.85 <30 (mcpInt 30.05→7 con +26 líneas test); `govulncheck` 0 vulns; `spec` 6+4 OK; `gate_run` MCP timeout en unit-tests/mutation (largo), evidencia parcial layers 1-3 + shell full; mutation S1 holds (corre sin sh, veredicto eficacia, 0% por timeouts Win en `receipts`, fixture F2 7/7 pasa).
- **L7**: W4 CLOSED + GAUNTLET 9/9 — mutation `--workers 1 --timeout-coefficient 10`, run real 1m01s eficacia 88.89% exit 0. Shell `bin/regulus.exe gate run win-portability/final`: 9/9 pass (typecheck/lint/build/unit-tests 79s/coverage/mutation 43s/crap/vulncheck/spec), receipts en `.regulus/evidence/win-portability__final/`.
