# devin-agent-mode — Regulus as a selectable agent in Devin + installer S3

User authorization: selectable agent mode plus installer S3, 2026-11-07. Goal: /regulus user-only skill driving the regulus orchestrator profile (opt-in, inert otherwise), max-nesting for crew delegation, installer S3 verifying the Devin surface headless, docs updated. Read down to `## Log` before working.

Class: STANDARD

Domain: sextant

## Contract

- Objective: Regulus selectable as an agent in Devin CLI and Desktop (/regulus slash, user-triggered only), inert when not selected; installer proves the Devin surface on any Linux machine.
- Scope: `.devin/skills/regulus/` (new, replaces regulus-workflow), `max-nesting: 2` in `.devin/agents/regulus.md`, S3 block in `scripts/install-devin.bash` (devin CLI check plus skill check plus idempotent MCP registration), `docs/devin.md` updates, local-only `.devin/config.json` (never committed).
- Constraints: no always-on rule (opt-in is the requirement); no changes to gates, core behavior, or the Windows ps1 mirror (parity noted as follow-up); installer stays pure bash plus coreutils, fail-loud with copy-paste remedy.
- Acceptance: S1, S2, S3, S4, S5
- Artifacts: skill dir, agents diff, installer diff, installer logs (this machine plus docker fresh machine), live session transcript cites.
- Dependencies: D1 -> D2 -> D3 -> D4 in series; D5 docs with D3.
- Risk: standard
- Verifier: live Devin session (auth plus trust already granted on this machine).
- Verification: `spec check` plus `devin skills show regulus` plus installer S1/S2/S3 green here and in docker plus one live /regulus smoke citing rec_* with clean tree.
- Completion: D1-D6 CLOSED, spec OK, installer green here plus docker plus live smoke PASS.

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

### Scenario: S5

- Given any directory outside the repo on a machine where the installer ran
- When the user invokes /regulus or the agent lists subagent profiles
- Then the skill resolves to the global copy and the regulus profiles are available, with nothing repo-specific required

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| D1 | S1 | skill /regulus (user-only, subagent plus agent, permissions) plus retire regulus-workflow | inline | CLOSED |
| D2 | S2 | max-nesting 2 in regulus orchestrator profile | inline | CLOSED |
| D3 | S3 | installer S3 block (devin, skill, MCP) | inline | CLOSED |
| D4 | S3 | fresh-machine docker test of installer | inline | CLOSED |
| D5 | S1-S4 | live /regulus smoke in Devin plus docs plus spec check plus gauntlet | inline | CLOSED |
| D6 | S5 | S4 global sync (skill/agents/MCP-user) plus outside-repo proof | inline | CLOSED |

## Log

- L1: Chart opened 2026-11-07 (selectable-agent mode plus installer S3; devin CLI here comes from omarchy system mise config, not portable, installer must detect and fail loud).

- L2: Closed 2026-11-07 — skill /regulus user-only + agent regulus + max-nesting:2 + installer S3 green here and in docker (fail-loud + remedy-install + idempotent), live /regulus smoke cited task check with clean tree, devin-pack contract updated (path+triggers+Read), quick gauntlet 10/10 run_3369a00034b203ff.

- L3: Amendment 2026-11-07 — Regulus must be global (any project): S4 syncs skill+agents to ~/.config/devin and MCP at user scope; S5 proves /regulus outside the repo.

- L4: D6 closed 2026-11-07 — S4 green locally and in docker (devin installed via remedy, global /regulus from /tmp, doctor 11 profiles), spec 5 scenarios, quick gauntlet 10/10 run_a247aa320015e484.
