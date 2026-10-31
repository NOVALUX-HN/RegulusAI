# RegulusAI — Dependency Map (Phase 0 audit)

Legend: `A → B` = A imports / spawns / reads B. All Go imports verified by package headers; TS edges by
import statements; runtime edges by `regulus.ts` spawn sites and `gauntlet.yml` steps.

## 1. Module-level graph

```
opencode host (V2, external)
  └── .opencode/plugins/regulus.ts ──spawns──▶ bin/regulus (or PATH regulus)
        │ imports: ../lib/domain (pure, zero host imports)
        │ reads:   discipline.md, tool-guidance.md, charts/, regulus.json
        └── hooks: tool.execute.after ──spawns──▶ regulus trace record
                  session.context    ──spawns──▶ regulus memory context / skills
                  session.compaction ──spawns──▶ regulus memory context (resume)

bin/regulus (cmd/regulus/main.go)
  └── internal/cli (dispatcher switch, cli.go:30-79)
        ├── gates ──uses──▶ receipts (write), executil (HideWindow), sh -c
        ├── evidence ──reads──▶ receipts (read-back)
        ├── spec (pure validator, no deps)
        ├── almanac (JSONL store; hybrid/vindex/embed submodules)
        ├── library ──reads──▶ receipts (evidence gate); writes INDEX.md
        ├── budget (JSONL ledger; imported by reflect/probe/surface)
        ├── reflect ──reads──▶ almanac, gates(spec), library, receipts, spec
        ├── surface ──reads──▶ budget, reflect; writes whitelisted harness files
        ├── evolve ──reads──▶ budget, library, receipts, trace
        ├── probe ──runs──▶ gates.Run (real receipts) + budget record
        │     └── compose (3-variant merge) reuses contrast primitives
        ├── trace (JSONL store, no value capture)
        ├── doctor ──spawns──▶ `opencode --version` (15s timeout) via executil
        ├── skills (SKILL.md frontmatter scan)
        ├── crap (go/ast + coverprofile; pure analysis)
        ├── contract (const only; imported by cli + mcp + plugin-side domain.ts mirror)
        ├── mcp (stdio JSON-RPC; reuses CLI code paths via mcpExec, never shell-outs)
        └── yolo (rewrites opencode.json + regulus-worker.md frontmatter)
```

## 2. External dependency edges (all pinned or intentional)

| Edge | Target | Pin | Enforced by |
|---|---|---|---|
| core build | Go toolchain 1.27 | `go.mod`, CI `setup-go@v5 go 1.27` | compiler |
| core runtime | zero modules | no `require` | `TestZeroDepsGoMod` (invariant cited in CI) |
| plugin compile | `@opencode/plugin@2.2.0`, `typescript@7.0.2`, `@types/bun@1.4.4` | exact in `.opencode/package.json` | `bunx tsc --noEmit` gate |
| mutation gate | `go-gremlins/gremlins@v0.6.0` | exact (CI install) | layer-6 gate at runtime |
| vuln gate | `golang.org/x/vuln/cmd/govulncheck@v1.9.0` | exact (CI install) | layer-8 gate at runtime |
| CI runners | `checkout@v4`, `setup-go@v5`, `setup-bun@v2`, `upload-artifact@v4` | floating majors, intentional | inline S4 comment in `gauntlet.yml` |
| doctor probe | `opencode` binary ≥ v2 | refused otherwise | `internal/doctor` (15s timeout) |
| release | `node` (version stamp), Go cross-compile | `engines: node>=18` | `scripts/build.bash` |
| npm dist | none (unpublished) | — | README states build-from-source |

## 3. Data-flow edges (state)

| Writer | Store | Reader | Integrity |
|---|---|---|---|
| gates.Run | `.regulus/evidence/<task>/*.json` | evidence, reflect, evolve, humans | SHA-256 pinned per receipt, append-only |
| memory tools | `.regulus/almanac/observations.jsonl` (+ pins/relations) | memory search/context/history, evolve | append-only revision chains |
| reflect.Admit | `.regulus/library/<domain>/` + INDEX.md | library, evolve, context hook | evidence-gated, guardian-validated, budget-capped |
| budget | `.regulus/budget.json` | status, evolve | refusals recorded |
| trace hook | `.regulus/traces/<session>.jsonl` | trace show/files/sessions, evolve pack | values excluded by design |
| surface.Apply | whitelisted `.opencode/**` files | plugin (next session) | guardian + budget gated |
| plugin hooks | OpenCode session context (DATA blocks) | model (framed as data) | `dataBlock()` framing, never instructions |

## 4. Coupling hotspots (cross-ref architecture-assessment §3)

1. **Tool-registry triangle**: `internal/cli/*Cmd` ↔ `regulus.ts editor.add ×16` ↔ `mcpTools() ×12`.
   Shared-shape but hand-duplicated; drift confirmed (4 tools missing in MCP).
2. **`CLI` god struct**: every command file hangs off one struct (`cli.go:21-28`); test construction cost
   grows with each command (95 tests in `cli_test.go` alone).
3. **`domain.ts` mirror**: `REGULUS_CONTRACT` and timeout/parse logic duplicated between Go (`contract.go`,
   `gates.go`) and TS (`domain.ts`) — safe (values, not logic) but must change together on contract bump.
4. **Charts as interface**: charts are the human↔agent contract consumed by spec gate, reflect, and all
   agents by convention; no schema beyond the spec validator — adequate, keep.

## 5. Blast-radius notes

- `regulus.json` edit → changes what CI executes (shell commands). Highest-privilege file in the repo.
- `.opencode/discipline.md` + `tool-guidance.md` edits → change every session's system prompt (via surface
  gate only — correct).
- `.regulus/` deletion → evidence, memory, lessons, traces lost (receipts artifact in CI retains 7 days).
  No backup/restore command exists beyond `memory export|import` (see AUD-P2-07 in improvement-backlog).
- `opencode.json` / `regulus-worker.md` edited by `yolo` — reversible via `yolo off`.
