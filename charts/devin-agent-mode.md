# devin-agent-mode — Regulus as a selectable agent in Devin + installer S3

User authorization: selectable agent mode plus installer S3, 2026-11-07. Goal: /regulus user-only skill driving the regulus orchestrator profile (opt-in, inert otherwise), max-nesting for crew delegation, installer S3 verifying the Devin surface headless, docs updated. Read down to `## Log` before working.

Class: STANDARD

Domain: sextant

## Contract

- Objective: Regulus selectable as an agent in Devin CLI and Desktop (/regulus slash, user-triggered only), inert when not selected; installer proves the Devin surface on any Linux machine.
- Scope: `.devin/skills/regulus/` (new, replaces regulus-workflow), `max-nesting: 2` in `.devin/agents/regulus.md`, S3 block in `scripts/install-devin.bash` (devin CLI check plus skill check plus idempotent MCP registration), `docs/devin.md` updates, local-only `.devin/config.json` (never committed).
- Constraints: no always-on rule (opt-in is the requirement); no changes to gates, core behavior, or the Windows ps1 mirror (parity noted as follow-up); installer stays pure bash plus coreutils, fail-loud with copy-paste remedy.
- Acceptance: S1, S2, S3, S4
- Artifacts: skill dir, agents diff, installer diff, installer logs (this machine plus docker fresh machine), live session transcript cites.
- Dependencies: D1 -> D2 -> D3 -> D4 in series; D5 docs with D3.
- Risk: standard
- Verifier: live Devin session (auth plus trust already granted on this machine).
- Verification: `spec check` plus `devin skills show regulus` plus installer S1/S2/S3 green here and in docker plus one live /regulus smoke citing rec_* with clean tree.
- Completion: D1-D5 CLOSED, spec OK, installer green on 2 machines, live smoke PASS.

## Acceptance

### Scenario: S1

- Given the repo on a machine with the Devin CLI
- When the user invokes /regulus (and only then)
- Then the run executes inside the regulus orchestrator profile (subagent true, agent regulus, triggers user-only) and the model never fires it unprompted

### Scenario: S2

- Given a /regulus session with crew work to do
- When the orchestrator needs scout, worker or verifier
- Then it spawns the regulus crew profiles (max-nesting 2) and every done-claim cites rec_* receipts

### Scenario: S3

- Given a fresh Linux machine (docker debian plus Go only, no devin, no regulus)
- When bash scripts/install-devin.bash runs
- Then S1 (contract plus version), S2 (MCP initialize smoke) and S3 (devin CLI present, skill visible, MCP registered idempotently) go green; when devin is absent it fails loud with the exact remedy and nothing half-installed

### Scenario: S4

- Given the installer and skill changes above
- When docs/devin.md is read
- Then it documents /regulus as the selectable entry, S3, the local-only config.json, and the Cloud nesting caveat

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| D1 | S1 | skill /regulus (user-only, subagent plus agent, permissions) plus retire regulus-workflow | inline | CLOSED |
| D2 | S2 | max-nesting 2 in regulus orchestrator profile | inline | CLOSED |
| D3 | S3 | installer S3 block (devin, skill, MCP) | inline | CLOSED |
| D4 | S3 | fresh-machine docker test of installer | inline | CLOSED |
| D5 | S1-S4 | live /regulus smoke in Devin plus docs plus spec check plus gauntlet | inline | CLOSED |

## Log

- L1: Chart opened 2026-11-07 (selectable-agent mode plus installer S3; devin CLI here comes from omarchy system mise config, not portable, installer must detect and fail loud).

- L2: Closed 2026-11-07 — skill /regulus user-only + agent regulus + max-nesting:2 + installer S3 green here and in docker (fail-loud + remedy-install + idempotent), live /regulus smoke cited task check with clean tree, devin-pack contract updated (path+triggers+Read), quick gauntlet 10/10 run_3369a00034b203ff.
