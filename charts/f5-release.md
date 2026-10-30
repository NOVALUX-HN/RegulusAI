# F5 release — integration & v1

The final phase: the integration surface (`regulus status`), the npm
installer, user docs, and the v1 close — built through the Sextant
itself. Read down to `## Log` before working.

## Acceptance

### Scenario: status reports the project truthfully

- Given a project with regulus.json, receipts and a pending candidate
- When `regulus status` runs
- Then it reports the runtime gate, the gauntlet configuration, today's evolution count against the cap, pending candidates, admitted lessons and receipts — or a clean refusal naming what is missing

### Scenario: the binary ships through npm

- Given a regular `npm install` of the regulus package
- When the `regulus` command runs
- Then the platform launcher finds the shipped binary and reports its version

### Scenario: a newcomer can start from the README alone

- Given a reader with OpenCode V2 and no prior exposure
- When they read the README
- Then install, the five movements, the guardians and every command are understandable without reading the source

### Scenario: the final gauntlet is green with receipts

- Given the v1 close
- When `regulus gate run regulus-ai-v1/T6` executes
- Then all seven layers pass, receipts record each layer, and the ledger carries the story of the release

## Tasks

| ID | Spec | Work | Route | Commit |
|---|---|---|---|---|
| F5.1 | S1 | `regulus status` — the integration surface (doctor, gauntlet config, evolution vs cap, candidates, lessons, receipts) | inline | — |
| F5.2 | S1 | npm package: launcher, build script with version stamping | inline | — |
| F5.3 | S1 | README quickstart + commands reference | inline | — |
| F5.4 | S1 | v1 close: final gauntlet, PRD/charts close, release notes | inline | — |

## Log

- **L1**: F5 opened as a Chart with acceptance scenarios before
  implementation (the Sextant's CHART movement, applied to the release
  itself). User authorization: "procede por favor" under the standing
  "continue through the pending phases" instruction.
- **L2**: P1 CLOSED 2026-10-30 — S1-S4 holds en arbol: S1 `internal/cli/status.go:35-135` (runtime, gauntlet, evolution vs cap, lessons/pending, receipts, memory, mutation); S2 `bin/regulus.js:1-47` + `package.json:10-16` (bin regulus, files binaries/, v0.8.1; `doctor 0.8.1 (matches package.json)` en live-plus-gaps/L2); S3 `README.md:14-47` quickstart + `README.md:49-87` movements/commands; S4 7 capas -> 12 gates `regulus.json:2-14`, `gate run live-plus-gaps/L2` 12/12 (receipts `.regulus/evidence/live-plus-gaps__L2/` citadas en live-plus-gaps L2) y L3 12/12. Decision: CLOSED (evolucion a 12, no SUPERSEDED por live-plus-gaps/health-gaps). `spec_check charts/f5-release.md` OK. Chart CERRADO.
