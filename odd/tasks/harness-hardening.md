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

- [x] T1 (S2) — YOLO + MCP security — route: inline — status: done — commit: 411d03c — RED: new yolo/provenance tests failed (template denies, edit flip, absolute/traversal); GREEN: go test ./internal/cli + go test ./... + bun lib 38 pass, vet/gofmt clean
- [ ] T2 (S3) — probe compose task-cap — route: inline — status: in_progress — commit: —
- [ ] T3 (S4) — reflect/evolve pass-receipt gate — route: inline — status: pending — commit: —
- [ ] T4 (S5) — surface Apply hardening — route: inline — status: pending — commit: —
- [ ] T5 (S6) — parity drift + schema unify — route: inline — status: pending — commit: —
- [ ] T6 (S7) — MCP handshake contract — route: inline — status: pending — commit: —
- [ ] T7 (S8) — testing gaps — route: inline — status: pending — commit: —
- [ ] T8 (S9) — docs refresh — route: inline — status: pending — commit: —
- [ ] T9 (S10) — tokens/latency dedup — route: inline — status: pending — commit: —

## Log

- L1: "repara todo en el orden recomendado hasta finalizar cada punto con evidencia"
- L2 (T1 done 411d03c): YOLO denies + .env ask + off/status edit mode + provenance validation + MCP boundary docs; evidence: RED 3 new tests failed, GREEN cli/full/bun/vet-fmt. Branch slice already >400 from history; feature running count 282 (T1 alone).
