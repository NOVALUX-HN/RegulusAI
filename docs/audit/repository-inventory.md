# RegulusAI — Repository Inventory (Phase 0 audit)

Date: 2026-10-30 · Auditor: Principal Software Architect (read-only audit, no code changes)
Repo root: this directory · HEAD branch: `feat/v0.4-evolution` (detached history notes say tags cut on main at merge)
Package: `regulus-ai@0.9.0` · Module: `regulus-ai` · Go 1.27 · Zero Go dependencies (`go.mod` has no `require`)

## 1. Top-level layout

| Path | What it is | Evidence |
|---|---|---|
| `cmd/regulus/main.go` | Single binary entrypoint; wires `internal/cli` + `internal/doctor`, `version` via `-ldflags -X main.version` | `cmd/regulus/main.go` |
| `internal/` | Go core: 18 packages (see §3) | `ls internal` |
| `.opencode/plugins/regulus.ts` | OpenCode V2 boundary plugin (only component typed against the OpenCode SDK) | `.opencode/plugins/regulus.ts:1-10` |
| `.opencode/lib/domain.ts` | Pure domain helpers, zero OpenCode imports (testable from plain Bun) | `.opencode/lib/domain.ts:1-2` |
| `.opencode/agents/` | 11 agent definitions (1 primary + 6 guardians + 4 crew) | `ls .opencode/agents` |
| `.opencode/commands/` | 1 command doc (`regulus-yolo.md`); plugin registers 3 slash commands in code | `.opencode/plugins/regulus.ts:539-576` |
| `.opencode/skills/regulus-workflow/` | The only committed skill (the Sextant) | `ls .opencode/skills` |
| `.opencode/discipline.md`, `.opencode/tool-guidance.md` | Editable harness surface (system prompt + tool descriptions) | `.opencode/*.md` |
| `charts/` | 20 feature documents (acceptance contracts) | `ls charts` (20 files, `regulus-ai-v1.md` … `yolo-fix.md`) |
| `docs/` | 7 docs: `architecture.md`, `PRD.md`, `agents.md`, `memory.md`, `memory-v2.md`, `devin.md`, `troubleshooting.md` | `ls docs` |
| `odd/tasks/` | 9 phase/task trackers (`almanac-v2.md`, `v0.4-evolution.md` … `v0.8-tool-parity.md`) | `ls odd/tasks` |
| `.devin/` | Devin pack: 11 mirrored agent profiles + 1 skill + `mcp_config.json` | `ls -R .devin` |
| `.regulus/` | Runtime state (NOT source): almanac store, 569+ receipts, library, traces, `coverage.out`, gate logs | `regulus status` → `receipts: 569 on disk` |
| `regulus.json` | Gauntlet declaration: 12 gates, layers 1–12 | `regulus.json` |
| `bin/` | Local builds (`regulus`, `regulus.exe`, `regulus.js` launcher); `binaries/` 6 release binaries | `ls bin binaries` |
| `scripts/` | `build.bash` / `build.ps1` (6-platform cross-compile), `install-devin.*`, `sync-plugin-defaults.ts` | `ls scripts` |
| `.github/workflows/gauntlet.yml` | CI: typecheck → plugin tests → smoke → 12-layer gauntlet → receipts artifact | `.github/workflows/gauntlet.yml` |
| `opencode.json` | Safe-default permissions (allow + destructive denies, `*.env` ask) | `opencode.json` |

## 2. Technologies, versions, dependencies

| Layer | Technology | Version / pin | Evidence |
|---|---|---|---|
| Core | Go | 1.27 (`go.mod`), toolchain `go1.27.3` observed | `go.mod`, `go version` |
| Core deps | none | zero `require` — invariant enforced by test | `go.mod`; `docs/memory-v2.md` §2 |
| Plugin host | OpenCode V2 (only supported runtime; V1 refused) | plugin `@opencode/plugin@2.2.0`, `typescript@7.0.2`, `@types/bun@1.4.4` | `.opencode/package.json`, `internal/doctor/doctor.go:20` |
| Plugin tests | Bun test | `bun test ./lib/` (also in CI) | `.opencode/lib/domain.test.ts:1-3` |
| Mutation gate | go-gremlins | `v0.6.0` exact pin (CI install) | `gauntlet.yml` |
| Vuln gate | govulncheck | `v0.1.9` exact pin (`golang.org/x/vuln/cmd/govulncheck@v1.9.0`) | `gauntlet.yml` |
| CI actions | checkout@v4, setup-go@v5 (go 1.27, cache off), setup-bun@v2, upload-artifact@v4 | floating majors, intentional per inline comment | `gauntlet.yml` S4 comment |
| Node | launcher + build script | `engines: node >= 18` | `package.json` |
| Release | 6 binaries (darwin/linux/windows × arm64/amd64) | stamped `-ldflags -X main.version` | `scripts/build.bash` |

## 3. Go core — 18 packages and responsibilities

| Package | Responsibility | Key file |
|---|---|---|
| `cli` | Command dispatch: 20 top-level commands + `--version` (budget, chart, init, contract, crap, doctor, evidence, evolve, gate, library, mcp, memory, probe, reflect, skills, trace, spec, status, yolo) | `internal/cli/cli.go:30-79` |
| `gates` | Layered gauntlet runner (ascending layers, stop-at-first-failure), metric gates, `ShRunner` via `sh -c` fallback, 600s default timeout | `internal/gates/gates.go:1-8,71-72,80-134` |
| `receipts` | Append-only evidence receipts, SHA-256 pinned, 8 KB capture cap, verdicts pass/fail | `internal/receipts/receipts.go` |
| `evidence` | Human audit read-back (`evidence list/show` — receipts, never diffs) | `internal/evidence/evidence.go` |
| `spec` | Chart acceptance-contract validator (Given/When/Then per scenario) | `internal/spec/spec.go` |
| `almanac` | Memory substrate: append-only JSONL observations, typed, provenance, revision chains, deterministic + hybrid/semantic retrieval (`hybrid`, `vindex`, `embed`, `relate`, `dedup`) | `internal/almanac/almanac.go:1-11` |
| `library` | Lesson library `.regulus/library/<domain>/` + regenerated INDEX, confidence levels, INV/DOM dimensions | `internal/library/library.go` |
| `budget` | Evolution ledger (reflect/probe/surface runs per day, cap 10) | `internal/budget/budget.go` |
| `reflect` | Evidence-gated lesson candidates: propose → guardian validate → admit | `internal/reflect/reflect.go` |
| `surface` | Mechanical gate for harness-surface edits (whitelist: `.opencode/skills|agents|plugins|discipline.md|tool-guidance.md`) | `internal/surface/surface.go` |
| `evolve` | Evidence-pack assembler (traces + receipts + INDEX + template); authoring stays with agent | `internal/evolve/evolve.go:1-7` |
| `probe` | Contrast (`contrast`: 1 gate × 2 variants) and compose (1 gate × 3 variants, merge) with budget entries | `internal/probe/probe.go` |
| `trace` | Per-session tool-execution capture (sequence + tool + status only; no values) | `internal/trace/trace.go:1-11` |
| `doctor` | Runtime probe: OpenCode V2 only, 15s timeout, refusal messages | `internal/doctor/doctor.go` |
| `skills` | Trigger registry from `SKILL.md` frontmatter | `internal/skills/skills.go` |
| `contract` | Plugin↔core protocol version (`regulus.contract/v1`) | `internal/contract/contract.go` |
| `crap` | CRAP scorer (complexity² × (1−coverage)³ + complexity) over coverprofile via go/ast | `internal/crap/crap.go` |
| `executil` | `HideWindow` (Windows console-flash fix) | `internal/executil/*`, `docs/troubleshooting.md` |
| `journey` (tests) | Deterministic end-to-end journeys on fixture projects (F1 spine, F2/F3/F4, S8, V06/V07) | `internal/journey/*_test.go` |

## 4. Plugin surface (OpenCode boundary)

- **16 namespaced tools** (`editor.add`, `regulus.ts:207-533`): `gate_run`, `spec_check`, `doctor`, `memory_note`, `memory_show`, `memory_save`, `memory_search`, `memory_context`, `memory_history`, `evolve_propose`, `probe_contrast`, `probe_compose`, `trace_show`, `trace_sessions`, `trace_files`, `skills_list`.
- **3 slash commands** (`ctx.command.transform`, `regulus.ts:537-576`): `/regulus-doctor`, `/regulus-verify`, `/regulus-gauntlet`. NOTE: file header comment says "two slash commands" — stale (see improvement-backlog AUD-P1-02).
- **4 hooks**: `tool.execute.after` (trace capture, never breaks execution), 2× `session.context` (discipline + lessons/skills; almanac context), `session.compaction` (resume). (`regulus.ts:580-660`).
- Binary resolution order prefers `bin/regulus[.exe]` then PATH; contract mismatch refuses before any tool executes (`regulus.ts:157-169`).

## 5. Agents, skills, MCP, Devin

- **Agents (11 files)**: primary `regulus.md` (allow + destructive denies, `webfetch: deny`); 6 guardians read-only (`edit: deny`, bash allowlist, `*`: deny): gate-verifier, spec-keeper, mutation-auditor, crap-analyst, evidence-auditor, reviewer; 4 crew (scout/worker read+write bounded by Chart, verifier, proposer). `docs/agents.md` table matches the tree.
- **Skills**: 1 committed (`regulus-workflow`, the Sextant). `docs/architecture.md` §2 names 3 (`regulus-gauntlet`, `regulus-reflect`, `regulus-workflow`) — 2 have no directory (see AUD-P1-01).
- **MCP (`regulus mcp`, stdio JSON-RPC 2.0, zero deps)**: 12 tools in `tools/list` order (`internal/cli/mcp.go:243-392`): `memory_note/show/save/search/context/history`, `gate_run`, `spec_check`, `doctor`, `trace_show/sessions/files`. Missing vs plugin: `evolve_propose`, `probe_contrast`, `probe_compose`, `skills_list` (see AUD-P0-01). Trust boundary documented in code: server enforces no permission layer; clients enforce (`mcp.go:10-13`).
- **Devin pack**: mirrors agents + skill + MCP config; `docs/devin.md` maps surfaces; Cloud caveat (snapshots immutable → `.regulus/` per-session) documented.

## 6. Runtime state (observed, not source)

`regulus status` (live run during audit): runtime `opencode v2.2.3 — supported (V2)`; 12 gates configured; evolution `10/10 run(s) today` (cap reached); lessons 2 admitted / 0 pending; **569 receipts** across ~35 task dirs; `coverage.out` total **83.9%** (≥ 80 floor); semantic index stale (`memory reindex` advised — operational, not a defect).

## 7. Working-tree note

`git status` shows uncommitted modifications (notably under `internal/almanac/`). The audit reads the working tree as-is; findings cite working-tree content. Nothing was committed, staged, or reverted by this audit.

## 8. Audit limits

Read-only inspection + two live read commands (`help`, `status`); no test suite was executed, no gates were run, no receipts were generated by the auditor. Verdicts below rest on code, docs, and committed tests — not on fresh runs (per Phase 0 rules, no invented results).
