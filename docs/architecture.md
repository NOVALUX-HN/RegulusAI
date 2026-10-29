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
│     hooks: tool.execute.after (trace capture), 2× session.context
│            (discipline + domain lessons/skills), session.compaction (resume)
│     duty: translate OpenCode events → Regulus contract; expose regulus tools
├── Guardian agents (.opencode/agents/*.md, read-only permissions)
│     spec-keeper, gate-verifier (others later)
├── Workflow skills (.opencode/skills/*/SKILL.md)
│     regulus-gauntlet, regulus-reflect, regulus-workflow (the Sextant)
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
- **OpenCode V2 only:** the plugin probes `opencode --version` at most once
  per session; V1 and unknown runtimes refuse with a message naming the
  required major. Regulus maintains no V1 compatibility layer — one plugin
  surface against SDK v2 (V2 already broke V1 plugins; carrying two surfaces
  would double the maintenance for no user).

## 5. State model — `.regulus/` (per project, gitignored)

```
.regulus/
├── evidence/<task-slug>/<gate>-<nanos>.json   immutable receipts (append-only)
├── almanac/
│     observations.jsonl    append-only observation log (typed, provenance;
│                           the memory substrate — docs/memory.md)
├── library/<domain>/
│     INDEX.md                          lesson index (regenerated from disk)
│     ├── lessons/<id>/LESSON.md        cue / lesson / evidence / confidence / dimension
│     └── harnesses/<task-slug>/        adopted snapshot: tree/ + EXPERIENCE.md (why + lesson)
├── traces/<session>.jsonl            agent-process capture (tool, order, status, file hints)
├── surface/edits.jsonl               proposed/applied harness-surface edits
├── memory/pins.jsonl, relations.jsonl pins + relations over observations
└── budget.json                        ledger of evolution runs (reflect + probe, per-day and per-task caps)
```

Committed, never under `.regulus/`:

- `regulus.json` — project config: gates (`name`/`layer`/`command`/`timeout_sec`), evolution caps (`max_per_day`, `max_per_task`)
- `charts/<feature>.md` — Charts: the committed feature documents
- `.opencode/discipline.md`, `.opencode/tool-guidance.md` — the editable system prompt and tool descriptions (harness surface; compiled fallbacks in the plugin)

**Single writer:** the core. The plugin and agents never mutate state
directly; they invoke core subcommands.

## 6. Evidence model — receipts

Every executed gate writes one immutable receipt (F1 schema, as implemented):

```json
{
  "id": "rec_18e1fe6dd8c66648",
  "task": "regulus-ai-v1/T2",
  "gate": "typecheck",
  "layer": 1,
  "command": "go build ./...",
  "exit_code": 0,
  "verdict": "pass",
  "started_at": "2026-10-26T05:51:14.4119394Z",
  "duration_ms": 909,
  "stdout": "",
  "stdout_sha256": "e3b0c442…",
  "stderr": "",
  "stderr_sha256": "e3b0c442…",
  "git_head": "f78a132…"
}
```

Captured stdout/stderr are bounded at 8 KiB with a truncation marker; the
SHA-256 fields pin the FULL content. `git_head` names the commit the
gauntlet ran against (empty outside a repository). Files are created with
O_EXCL: append-only, never overwritten — a second run advances to a new
nanosecond slot.

Guardians verify claims against receipts; the human audits receipts, not
code.

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
