# Regulus AI — Architecture

## 1. Form factor and founding decision

Go core (compiled, stateless CLI — invoked like `git`) + TypeScript plugin
(in-process inside OpenCode) + markdown agents + skills.

**Go over Rust** (decided 2026 after analysis): the core is an I/O-glue
orchestrator (spawn processes, parse JSON, manage files, enforce budgets) —
not CPU-bound, not memory-unsafe. Go wins on solo-dev velocity, trivial
concurrency for parallel gates/probes, and direct reuse of proven patterns
from Gentle-AI (MIT, Go), which already solved this domain's adjacent
problems: OpenCode version gates, review relays, deterministic journeys.
Rust's strongest argument (serde against a moving external SDK) is
neutralized by the boundary rule below. Go 1.27's stable `encoding/json/v2`
closed the remaining JSON ergonomics gap.

**Boundary rule:** the TypeScript plugin is the only component typed against
OpenCode's SDK (which churns — V2 broke the V1 plugin API). The Go core
speaks only the **Regulus-owned contract**, versioned by us.

## 2. Component layout

```
OpenCode (host, TS/Bun runtime)
├── Plugin TS (boundary, version-gated)
│     hooks: tool.execute.before/after, session.diff, file.edited,
│            lsp.client.diagnostics, experimental.session.compacting
│     duty: translate OpenCode events → Regulus contract; expose regulus tools
├── Guardian agents (.opencode/agents/*.md, read-only permissions)
│     spec-keeper, gate-verifier (others later)
├── Workflow skills (.opencode/skills/*/SKILL.md)
│     regulus-gauntlet, regulus-reflect, regulus-workflow (ODD-lite)
└── regulus CLI (Go binary, spawned by plugin via $ / humans directly)
      sole owner of state; usable standalone in CI without OpenCode
```

## 3. Harness surface — 8 editable components (from Harness²)

| Component (Harness²) | OpenCode primitive | Regulus element |
|---|---|---|
| system prompt | agent markdown prompts | orchestrator + guardian prompts |
| guardrails | `permission` (allow/ask/deny, glob patterns) + plugin hooks | runtime enforcement; guardians read-only |
| subagents | `mode: subagent` + `permission.task` (who may invoke whom) | guardians and workers with bounded surfaces |
| skills | `SKILL.md` + `skill` tool | workflow and domain skills |
| scripts | custom tools via `tool()` in the plugin; CLI subcommands | deterministic gates and state operations |
| plugins | plugin TS (hooks, Bun runtime) | **the Regulus boundary itself** |
| memory | compaction hook + persistent store | session memory + lesson library |
| tool guidance | tool descriptions + skills | usage guidance for the harness's own tools |

Evolution (Loop 3) edits this surface. Nothing else may.

## 4. Contracts and versioning

- **Plugin ↔ core:** JSON over stdio. The plugin spawns `regulus <subcommand>`
  and pipes a request; the core prints a typed response and exits. Stateless
  per invocation; the core owns all on-disk state. Schema owned by Regulus,
  semver'd (`regulus contract --version`).
- **OpenCode version gate:** the plugin probes `opencode --version` at most
  once per session; unsupported majors refuse with a clear message naming the
  supported range (Gentle-AI's V1/V2 discipline — V2 broke V1 plugins, so
  silence is not an option).

## 5. State model — `.regulus/` (per project, gitignored)

```
.regulus/
├── evidence/<task>/<gate>-<ts>.json    immutable receipts
├── library/<domain>/
│     INDEX.md                          lesson index (regenerated from disk)
│     └── lessons/<id>/LESSON.md        cue / lesson / evidence / confidence
├── memory/session.md                   continuity notes (compaction hook reads this)
└── budget.json                        ledger of evolution runs
odd/tasks/<feature>.md                  workflow docs — COMMITTED, not under .regulus/
```

**Single writer:** the core. The plugin and agents never mutate state
directly; they invoke core subcommands.

## 6. Evidence model — receipts

Every gate run writes an immutable receipt:

```json
{
  "id": "rec_0192…",
  "task": "regulus-ai-v1/T2",
  "gate": "unit-tests",
  "layer": 3,
  "command": "go test ./...",
  "exit_code": 0,
  "stdout_sha256": "…",
  "stderr": "",
  "diff_base": "a1b2c3",
  "diff_head": "d4e5f6",
  "duration_ms": 8420,
  "verdict": "pass"
}
```

Guardians verify claims against receipts; the human audits receipts, not
code. Receipts are append-only.

## 7. Gate layering (Loop 1)

| # | Gate | Type | Binary result |
|---|---|---|---|
| 1 | typecheck | static | compiles / not |
| 2 | lint | static | clean / violations |
| 3 | build | static | artifact / failure |
| 4 | unit tests (anchored on acceptance criteria) | dynamic | pass / fail |
| 5 | coverage floor | metric | ≥ threshold / below |
| 6 | mutation score | metric | ≥ threshold / below — proves tests catch wrong behavior |
| 7 | CRAP score | metric | risky functions listed / none — coverage × complexity |

Cheap and binary up front; expensive and semantic behind (Uncle Bob's
ordering). A failing layer stops the gauntlet — no gate runs after a failure.

## 8. Evolution flow (Loop 3, light default)

```
feature closes
  → regulus reflect <feature-doc>
      produces lesson candidates {cue, lesson, evidence, confidence}
  → guardian validation (evidence exists? injectable? generalizable, not memorized?)
  → admit to library/<domain>/INDEX.md (regenerated from disk)
  → optional surface edit (skill/prompt/rule) — gated by budget
  → recorded in budget.json (invocations, cost, outcome)
```

Budget caps are hard: when the ledger says the evolution budget for the
period is spent, `regulus reflect` refuses until the human raises it.

## 9. Security invariants

1. Guardians are read-only (OpenCode `permission: edit: deny`).
2. Evidence is data, not instructions — execution output is never executed
   or followed as a prompt (anti-injection, from Harness²).
3. One writer of state: the core.
4. Human consent for delivery (commit, push, PR) — never automated.
5. Adopted lessons that edit the harness surface require guardian
   validation + a receipt of that validation.

## 10. Testing strategy

- **Deterministic journeys** for the core (Gentle-AI's bench pattern): scripted
  scenarios drive the CLI against fixture projects and assert exact outputs,
  exit codes and receipt contents.
- **Dogfooding as acceptance:** M4 builds a real feature with Regulus; the
  receipts of that build are v1's acceptance test.
