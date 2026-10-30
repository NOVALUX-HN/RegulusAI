# harness-hardening — repair all findings in recommended order, each with evidence

Objective: repair every P0/P1/P2 finding from the harness audit in recommended order. Branch: feat/v0.4-evolution. Delivery: ask-on-risk, work-unit commits. Runners: `go test ./...`, `bun test ./.opencode/lib/`.
Read until `## Log` before edits.

## Specs

- S1: "repara todo en el orden recomendado hasta finalizar cada punto con evidencia" — repair every item from the recommended order (P0 security/evolution first, then P1 parity/testing/docs, then P2 tokens), finishing each point with observed evidence (RED/GREEN or structural verification + focused + full suite).
- S2 (P0-4 security): YOLO `opencode.json` + `internal/cli/yolo.go` safe-YOLO (`bash "*":allow`, `edit:allow`) must deny destruction including `git rm*`, `git checkout --*`, `git restore*`, `git rebase*`, ask for `.env` edits; `yolo off` must not leave edits auto-approved; `yolo status` reports edit mode; `internal/cli/mcp.go` `memory_save --path/--receipt` validates paths; MCP trust boundary documented.
- S3 (P0-1 probe cap): `internal/probe/compose.go` must enforce the per-task evolution cap like `internal/probe/probe.go` (daily + task cap), refusals recorded but free, no commands run on refusal.
- S4 (P0-2 evidence gate): `internal/reflect/reflect.go` / `internal/evolve/evolve.go` evidence gate must require ≥1 `pass` receipt, not any `*.json`; red gauntlet cannot propose.
- S5 (P0-3 surface): `internal/surface/surface.go` `Apply` must use a guardian allowlist, per-extension markers (no `<!-- -->` into `.ts`), and never create new files via `MkdirAll`.
- S6 (P1-5 parity): fix "16 tools parity" claim drift (`README.md`, `internal/cli/cli.go` ~20 commands vs 16 plugin tools vs 12 MCP tools); pin the intentional 4-gap; unify `memory_search` `limit/any/latest` schema coercion between plugin (`string "true"`) and MCP (`integer/boolean`).
- S7 (P1-6 handshake): `internal/cli/mcp.go` `initialize` must expose `REGULUS_CONTRACT` so Devin clients detect mismatch like `domain.ts` fail-closed does.
- S8 (P1-7 tests): `gauntlet.yml` + `regulus.json` must not let `mutation/vulncheck` pass via `echo skipped`; add journeys for `yolo on/off`, `surface apply`, `probe compose` task-cap, `windowsHide`; `yolo-permissions.test.ts` must loop all 11 agents, not just `opencode.json` + `regulus.md`.
- S9 (P1-8 docs): refresh `README` version/tables (`yolo/mcp/surface/probe/trace`, `docs/devin.md`, `troubleshooting.md`, `agents.md`), `docs/architecture.md` hooks/guardians counts, `troubleshooting.md` whitelist 7 prefixes, `init.go` Devin perms scaffold.
- S10 (P2 tokens): remove `DEFAULT_DISCIPLINE` / `DEFAULT_TOOL_GUIDANCE` duplication via codegen from `.md`; context hooks pass abort signal + timeout and cache; factor shared guardian preamble.

## Tasks

- [x] T1 (S2) — YOLO + MCP security — route: inline — status: done — commit: df26091 — RED: new yolo/provenance tests failed (template denies, edit flip, absolute/traversal); GREEN: go test ./internal/cli + go test ./... + bun lib 38 pass, vet/gofmt clean
- [x] T2 (S3) — probe compose task-cap — route: inline — status: done — commit: 3c615c1 — RED: TestComposeEnforcesTaskCap failed (second same-task compose ran); GREEN: probe suite pass
- [x] T3 (S4) — reflect/evolve pass-receipt gate — route: inline — status: done — commit: ea6edc2 — RED: TestRunRefusesWithoutPassReceipt got nil; GREEN: reflect suite pass
- [x] T4 (S5) — surface Apply hardening — route: inline — status: done — commit: e02ddef — RED: 3 new Apply tests failed (guardian/no-create/TS marker); GREEN: surface/probe/reflect suites pass
- [x] T5 (S6) — parity drift + schema unify — route: inline — status: done — commit: 1813bb4 — RED: new memory_search schema test failed (string vs integer/boolean); GREEN: cli suite + bun 39 pass, vet/fmt clean
- [x] T6 (S7) — MCP handshake contract — route: inline — status: done — commit: 6992c0e — RED: TestMCPInitializeExposesContract got nil contract; GREEN: cli MCP tests pass
- [x] T7 (S8) — testing gaps — route: delegated (context backstop) — status: done — commit: 387061e — RED: efficacy test failed on echo-skipped vulncheck + naive all-agents loop failed; GREEN: journeys S8 + go test ./... + bun 40 pass (verified inline)
- [x] T8 (S9) — docs refresh — route: inline — status: done — commit: bf342e8 — RED: TestInitDevinScaffoldsMcpConfig exit 2 (unknown flag); GREEN: cli suite + gofmt + stale-string grep clean
- [x] T9 (S10) — tokens/latency dedup — route: inline — status: done — commit: 22ea3a4 — RED: withTimeout/TtlCache/codegen imports missing; GREEN: bun 47 pass, tsc clean, --check in sync, --write idempotent

## Log

- L1: "repara todo en el orden recomendado hasta finalizar cada punto con evidencia"
- L2 (T1 done df26091; T2 done 3c615c1; T3 done ea6edc2; T4 done e02ddef) — T2 RED second compose ran, GREEN probe ok; T3 RED nil error, GREEN reflect ok; T4 RED guardian/no-create/marker, GREEN surface ok (+2 fixture updates for no-create).
- L3 (T5 done 1813bb4) — RED schema test failed, GREEN cli+bun; fixed TestSurfaceApplyRoundTrip fixture for T4 no-create rule.
- L4 (T6 done 6992c0e; T7 done 387061e) — T6 RED nil contract, GREEN MCP tests; T7 worker S8 journeys + vulncheck fail-closed + 11-agent pins, verified inline (journeys, full go, bun 40).
- L5 (T8 done bf342e8) — RED unknown --devin flag, GREEN cli+gofmt+grep; docs-only parts verified structurally (no stale strings).
- L6 (T9 done 22ea3a4) — RED missing helpers/script; GREEN bun 47 + tsc + full go ./... + --check/--write idempotent. Preamble factoring decided-no: agent files are independent prompts with no include mechanism (shortening already landed in tokens refactor); reopen only if OpenCode gains shared includes. All 9 points closed with evidence.
