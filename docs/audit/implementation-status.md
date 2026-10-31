# RegulusAI — Implementation Status (Phase 0 audit)

Scale: IMPLEMENTED = code + tests + live evidence · PARTIALLY_IMPLEMENTED = works with scoped gaps ·
DOCUMENTED_ONLY = described, no code path · PROPOSED = explicitly future · DEPRECATED = retired.
Rule: a spec paragraph is not an implementation. Each verdict cites code or tests.

## 1. Gauntlet (Loop 1 — per-change quality gates): IMPLEMENTED

12 layers declared in `regulus.json` and enforced by `internal/gates` (ascending order, stop-at-first-failure,
`gates.go:1-8`): typecheck → lint → build → unit-tests → coverage-floor (80) → mutation (gremlins, efficacy 70,
`internal/receipts` scope) → crap (max 30) → vulncheck → spec (`charts/`) → ts-typecheck → ts-unit-tests → gofmt.
Release `release-0-9-0/V1` closed 12/12 with receipt ids (`CHANGELOG.md` 0.9.0). External binaries required for
layers 6 (gremlins) and 8 (govulncheck) — declared in `docs/troubleshooting.md`, installed in CI.

## 2. Receipts + evidence audit: IMPLEMENTED

Append-only receipts with SHA-256 pins (`internal/receipts/receipts.go`), human audit via
`evidence list|show` (`internal/cli/evidence.go`, `internal/evidence/evidence.go`). 569 receipts on disk across
~35 task dirs (live `status`). Limitation (not a gap): integrity is hash-pinned, not signed — see AUD-P1-03.

## 3. Spec contract (Charts): IMPLEMENTED

`internal/spec/spec.go` validates Given/When/Then per scenario; `spec check <file|dir>` is gauntlet layer 9;
20 charts committed in `charts/`. Journey F1 asserts the spine end-to-end
(`internal/journey/journey_test.go:TestF1VerticalSpine`).

## 4. Almanac memory (Loop 2 substrate): IMPLEMENTED

Append-only JSONL observations, 5 types, first-class provenance, revision chains, pins, relations
(`internal/almanac/almanac.go`); deterministic + hybrid/semantic retrieval v2.0 flat-quantized scan
(`hybrid.go`, `vindex.go`, `embed.go`, `missrate_test.go`, `perf_test.go`); 14 `memory` subcommands in help;
6 plugin tools + 6 MCP tools. `memory history` proves append-only chains. v2.1 ONNX explicitly deferred
(`docs/memory-v2.md` §1/S8) → PROPOSED, correctly labeled, not a gap.

## 5. Evolution (Loop 3 — self-improvement): IMPLEMENTED (bounded)

`reflect` propose→admit with evidence gate + daily cap (`internal/reflect/reflect.go`, `internal/budget/budget.go`,
cap 10); `library` admission + INDEX (`internal/library/library.go`); `surface` whitelist-gated edits
(`internal/surface/surface.go`); `evolve propose` pack assembler (`internal/evolve/evolve.go`);
`probe contrast|compose` (`internal/probe/`). 2 lessons admitted, ledger live (`status`). No autonomous model
loop exists — authoring stays with agents/guardians by design (`evolve.go:1-7`); that is the documented scope,
not a missing piece.

## 6. Plugin boundary (OpenCode V2): IMPLEMENTED

16 tools, 4 hooks, 3 slash commands (`repository-inventory.md` §4). Contract `regulus.contract/v1` enforced
both sides (`internal/contract/contract.go`, `domain.ts`, `regulus.ts:169`). Stale header comment ("two slash
commands") → docs bug AUD-P1-02, not a code gap.

## 7. Agents: IMPLEMENTED

11/11 files present; permissions match `docs/agents.md`: guardians `edit: deny` + bash allowlist + `*`: deny
(verified `regulus-crap-analyst.md`, `regulus-evidence-auditor.md`); primary destructive denies mirror
`opencode.json`. Worker bounded-by-Chart is prompt convention, not code enforcement → documented limitation
AUD-P2-05.

## 8. Skills: PARTIALLY_IMPLEMENTED

`regulus-workflow` (Sextant) committed with `SKILL.md` + registry wiring (`internal/skills/skills.go`,
`skills list`, plugin `skills_list`, `suggestSkills` in `domain.ts`). `regulus-gauntlet` and `regulus-reflect`
named in `docs/architecture.md` §2 have no directory under `.opencode/skills/` → DOCUMENTED_ONLY (AUD-P1-01).

## 9. MCP server: PARTIALLY_IMPLEMENTED

Stdio JSON-RPC 2.0, zero deps, 12 tools (`internal/cli/mcp.go:243-392`, 19 `mcp_test.go` tests). 4 plugin tools
have no MCP equivalent: `evolve_propose`, `probe_contrast`, `probe_compose`, `skills_list` → Devin surfaces
cannot propose evolution or run probes (AUD-P0-01). No server-side permission layer by documented design
(`mcp.go:10-13`) → accepted trust boundary, recorded in security-assessment.

## 10. Traces: IMPLEMENTED

`tool.execute.after` hook + `trace record|show|files|sessions|prune`; values deliberately excluded
(`internal/trace/trace.go:1-11`). CI smoke asserts `trace files --session nobody` refusal.

## 11. Doctor V2: IMPLEMENTED

V1/unknown refused with guidance, stale/unstamped binary warnings, 15s probe timeout
(`internal/doctor/doctor.go`). Live run: `opencode v2.2.3 — supported (V2)`.

## 12. YOLO (safe auto-approve): IMPLEMENTED

`yolo on|off|status` flips `opencode.json` + worker frontmatter, destructive denies preserved, guardians
untouched (`internal/cli/yolo.go`, 13 `yolo_test.go` tests, `docs/troubleshooting.md` reminder to `/reload`).

## 13. CRAP: IMPLEMENTED

Pure-Go scorer over coverprofile via go/ast, gate layer 7 (max 30, top 5), 6 `crap_test.go` tests.

## 14. Devin support: IMPLEMENTED

`.devin/` pack (11 agents, 1 skill, `mcp_config.json`), `docs/devin.md` with per-surface mapping and honest
Cloud caveats (immutable snapshots → per-session `.regulus/`). CLI path works without OpenCode.

## 15. CI/CD: IMPLEMENTED

`gauntlet.yml`: deterministic (cache off, no go.sum by invariant `TestZeroDepsGoMod`), plugin typecheck +
domain tests, CLI smoke refusals, 12-layer gauntlet, receipts artifact (7d). EVIDENCE bot-commit loop retired
by explicit merge (`89d8fa0`) → DEPRECATED, correctly retired (see §17).

## 16. Distribution: PARTIALLY_IMPLEMENTED

`scripts/build.bash` cross-compiles 6 stamped binaries; `binaries/` committed; npm package `regulus-ai@0.9.0`
declares `bin: ./bin/regulus.js` but README honestly states **not published — build from source**
(`package.json`, README quickstart). Committed binaries can drift from source; `doctor` warns on unstamped
builds (AUD-P2-02).

## 17. DEPRECATED (correctly retired)

- `docs/EVIDENCE.md` receipts-trail bot commits (merge `89d8fa0`, workflow S8 comment).
- God files `internal/cli/cli.go` (1,931 lines) and `internal/almanac/almanac.go` (931) — split into
  same-package siblings per 0.8.1 changelog (move-only).
- Pre-stamp `go build` binaries (unstamped by design; `doctor` warns).

## 18. PROPOSED (explicitly future, not gaps)

Almanac v2.1 ONNX (`memory-v2.md` S8); broader mutation scope beyond `internal/receipts`; any F5 items in
`charts/f5-release.md` (chart-tracked, out of scope to re-audit here).

## 19. Summary table

| Area | Verdict |
|---|---|
| Gauntlet 12 gates, receipts, spec, traces, doctor, yolo, crap | IMPLEMENTED |
| Almanac + v2.0 retrieval, library, budget, reflect, surface, evolve pack, probe | IMPLEMENTED |
| Plugin 16 tools / 4 hooks / 3 commands, 11 agents, Devin pack, CI | IMPLEMENTED |
| Skills 1/3, MCP 12/16 tools, distribution (unpublished npm, committed binaries) | PARTIALLY_IMPLEMENTED |
| `regulus-gauntlet` / `regulus-reflect` skills | DOCUMENTED_ONLY |
| ONNX v2.1, F5 chart items | PROPOSED |
| EVIDENCE bot-loop, god files, unstamped builds | DEPRECATED (retired) |

No functionality was executed by this audit; verdicts rest on code, tests, and the live `status`/`help`
readouts cited above.
