# RegulusAI — Architecture Assessment (Phase 0 audit)

## 1. Current architecture (as built)

```
OpenCode V2 (host, TS/Bun runtime)
├── Plugin TS — boundary, version-gated (regulus.contract/v1)
│     hooks: tool.execute.after + 2× session.context + session.compaction
│     16 namespaced tools · 3 slash commands · DATA-framed memory injection
├── Guardians (6, read-only) + Crew (4, Chart-bounded) + primary orchestrator
├── Skills registry (SKILL.md frontmatter, 1 committed skill)
└── regulus CLI (Go, zero deps, sole owner of state; usable without OpenCode)
      gauntlet → receipts → evidence | spec | almanac | library/budget/reflect/surface/evolve/probe | trace | doctor | mcp | yolo | crap
```

Three loops: **Gauntlet** (per change), **Discipline** (per task — Chart + Sextant + traces), **Evolution**
(across tasks — evidence-gated, budget-capped). Matches `docs/architecture.md` §§1–3 and `docs/PRD.md` §2.

## 2. Separation of responsibilities — strength

- **Boundary rule holds**: only the TS plugin is typed against the OpenCode SDK; Go speaks only the
  Regulus-owned contract (`contract.go`, `architecture.md` §1, `regulus.ts:1-10`). V1→V2 churn is absorbed
  in one file — the single most valuable structural decision in the repo.
- **State ownership is unambiguous**: the compiled core owns `.regulus/`; the plugin translates and spawns
  (`regulus.ts:115-169`); agents never touch state except through tools. No second writer was found.
- **Evidence vs memory vs lessons** are three distinct stores with distinct write paths (receipts append-only
  by gates; observations by memory tools; lessons only via reflect+guardian+budget). The layering is clean
  and each has its own read-back surface (`evidence`, `memory`, `library`).

## 3. Coupling and contracts — strength with two blemishes

- Plugin↔core contract is versioned and enforced pre-execution on both sides (`contract.Version`,
  `checkContract`, `resolveRegulus` refusal). Evolution path is explicit (bump version).
- Internal Go coupling is disciplined: `evolve` only assembles (reads traces/receipts/INDEX), `reflect`
  proposes, `library` admits, `budget` ledgers, `surface` gates — one direction, no cycles observed in
  imports (`budget ← reflect/probe/surface`, `receipts ← gates/evidence/reflect`).
- Blemish 1: MCP (12 tools) and plugin (16 tools) are **two hand-maintained registries** over the same CLI
  (`mcpTools()` vs `editor.add` blocks) with no shared tool table and no parity test — drift already
  happened (AUD-P0-01). The sync guard mentioned in the 0.8.1 changelog covers plugin↔CLI flags, not MCP↔plugin.
- Blemish 2: the harness-surface whitelist (`surface.go`) covers 5 paths while agents/skills evolve;
  worker boundedness is prompt convention, not a code-enforced surface (AUD-P2-05).

## 4. Persistence and state — sound, with one operational edge

- Append-only JSONL (observations, receipts one-file-per-gate, traces) + derived artifacts (INDEX, vindex,
  retrieval.bin) treated as regenerable (`memory reindex`). Single-writer (core CLI) avoids corruption.
- `.regulus/` mixes durable evidence with regenerable cache; the stale-index readout (`status`:
  "semantic index stale") is honest but means retrieval silently degrades to lexical fallback until reindex —
  acceptable, worth one acceptance test (AUD-P2-06).
- Budget ledger records refusals too — good (auditable caps, not silent drops).

## 5. Error handling — mature

Exit-code discipline 0/1/2 across CLI (`cli.go:21-28`); gate failures stop the gauntlet with per-layer
verdicts; hook failures never break tool execution (`regulus.ts:601`); MCP malformed lines get -32700 and
serving continues (`mcp.go:serveMCP`); spawn failures return diagnostic content, never thrown null
(`regulus.ts:207-233`); 95 `cli_test.go` tests witness the refusal paths.

## 6. Concurrency — simple by design, untested at the edges

Gates run sequentially in layer order (stop-at-first-failure); no parallel gate execution, no locks, no
goroutines in the audited paths — data-race surface is minimal. Consequently there are no concurrency tests
and no `-race` gate; the 600s default gate timeout + context cancellation is the only liveness control.
Verdict: appropriate for the current design; if parallel gates ever land, they need a race gate first
(AUD-P3-05).

## 7. Extensibility — good bones, two friction points

- Adding a gate = one `regulus.json` entry (no code). Adding memory buckets/skills = frontmatter + registry.
  Harness self-edits flow through surface proposals with guardian validation — the Harness² loop is real.
- Friction 1: adding a tool means editing **three** registries (CLI command, plugin tool, MCP tool) plus docs
  (`agents.md` table, `tool-guidance.md`) — the 0.8.1 parity saga proves the cost. A single tool manifest
  consumed by all three would remove the class (AUD-P1-06).
- Friction 2: receiver sprawl — `internal/cli` holds ~20 command files around one god `CLI` struct; the
  0.8.1 god-file split moved code without shrinking responsibility. A command registry would simplify
  dispatch and force parity mechanically.

## 8. Runtime independence — strong, honestly scoped

Core runs standalone in CI without OpenCode (`gauntlet.yml` smoke + 12-layer run); doctor refuses V1 with
guidance instead of degrading; Devin path needs no OpenCode. Windows portability is engineered
(`executil.HideWindow`, `build.ps1`, `hide_windows_test.go`), not assumed.

## 9. Architectural risks (ordered)

1. **Registry triplication** (CLI × plugin × MCP) — drift already observed; each new tool triples the miss
   chance. (Structural, P1.)
2. **Unsigned receipts** — tamper-evidence rests on git history, not cryptography; a reviewer cannot
   independently verify a receipt file. (Trust, P1.)
3. **Shell-gate trust** — `regulus.json` commands run via `sh -c` with the invoker's privileges; a cloned
   repo's gauntlet is arbitrary code execution by design. Acceptable for a dev harness, must stay explicit.
   (Trust, P1 — docs + first-run audit.)
4. **MCP permission vacuum** — server-side enforcement absent by design; Devin project permissions are the
   only gate. Correct if every client enforces; catastrophic if one doesn't. (Trust, P1 — verify per client.)
5. **CLI god struct** — `CLI` accumulates every command's dependencies; test setup cost grows per command
   (95 tests already). (Maintainability, P2.)
6. **Derived-index staleness** — silent lexical fallback. (Operational, P2.)

## 10. Simplification opportunities (no refactor performed — Phase 0 rule)

- Single tool manifest → generate plugin tools, MCP tools, help text, and `agents.md` table.
- Command registry in `cli` (name → handler + usage) replacing the `switch` in `cli.go:30-79`.
- Receipt signing (HMAC with local key or git-notes linkage) only if reviewers demand independent verification.
- Nothing else: the zero-dependency, single-binary, append-only design should be conserved (see final report §3).
