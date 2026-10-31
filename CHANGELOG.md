# Changelog — Regulus AI

All notable changes, newest first. Format follows Keep a Changelog;
versioning is `vMAJOR.MINOR` tags on main (release tags are cut at
chart close — see G3.9).

## [Unreleased]

### Phase 5 — Almanac engineering memory (personal use)

- Memory systems derived from type (episodic/semantic/decision/procedural/
  evolution) with `--system` filters; no duplicated stores (docs/memory.md
  §7, adr/T5-01).
- Provenance: `source` (agent|human|core|import), `origin` (project
  identity); imports are marked and stay PROPOSED until reviewed.
- Temporal validity: optional `expires_at`; retrieval hides expired,
  rejected, invalidated and superseded observations unless
  `--include-stale` is passed.
- Knowledge governance: OBSERVED → VERIFIED (`memory verify`, needs cited
  evidence) / REJECTED (`memory reject --reason`); lessons ACCEPTED;
  SUPERSEDED computed from relations; append-only `status.jsonl` audit
  trail; new relation kinds (`invalidated_by`, `derived_from`,
  `verified_by`, `related_to`).
- Retrieval filters on both channels (lexical + hybrid) without touching
  ranking weights; scoped `memory context [--task --path --system --budget]`;
  save keeps unique ids even on coarse clocks (adr/T5-02).
- Trust boundaries: untrusted-DATA banner on retrieved output; imported
  knowledge is never instructions.

### Phase 4 — Decision Intelligence (personal use)

- Structured decision contracts (`internal/decisions`, `regulus decide
  propose --file`): problem, proposal, scope, alternatives with trade-offs,
  constraints, risks, consequences, evidence refs, confidence,
  reversibility, artifacts, commits, task, related, critical; shape
  validated at propose time (docs/decisions.md, adr/T4-01).
- Assess pipeline (`decide assess [--json]`): PERCEPTION → EVIDENCE →
  POLICY → ASSESSMENT → DECISION with ten pass|fail|abstain criteria and
  coded findings (contradiction incl. zero-dep, missing alternatives,
  unmitigated risk, hidden dependency, obsolete scope, insufficient
  evidence, unsupported assumptions).
- Governed resolution (`decide resolve/show/list`): assessment advises,
  policy gates (zero blocking findings), humans approve critical decisions
  and supersedes (`--approve-by "nick: reason"`); atomic accept+supersede
  (no double-decided scopes, no deadlock); refusals persist nothing;
  INCONCLUSIVE abstention; integrity revalidates on show (adr/T4-02).

### Phase 3 — Gauntlet independent verification (personal use)

- Gate metadata with behavior-preserving defaults (`id`, `category`, `when`,
  `on_fail: halt|advisory`, `required`, `profiles`): advisory failures record
  and continue, `when` skips only optional gates (required always run,
  unevaluable conditions fail open with a receipt note), skips recorded in
  result + manifest. `gate run --profile NAME` (quick/full/reserved/declared).
- Guardian governance (`internal/guardians`): six-guardian registry with
  bounded responsibilities; `regulus gate evaluate` validates (known
  guardian, crew rejected, parseable verdict, non-empty note, existing
  evidence) and appends to `evaluations.jsonl` — read-only by construction.
- Core consolidation (`regulus gate verdict`): receipts decide (FAIL beats
  BLOCKED beats INCONCLUSIVE beats PASS), evaluations flagged AGREE/DISPUTE
  and counted, never applied; measured rates (disagreement, inconclusive,
  manifest-linked completeness) instead of invented FP/FN.
- `docs/gauntlet.md`, `docs/adr/T3-01`, `T3-02`; repo `regulus.json`
  dogfoods categories + required + expensive-gate profiles (default run
  unchanged: all 12 gates).

### Phase 2 — Sextant discipline (personal use)

- Task classification TRIVIAL/STANDARD/COMPLEX/CRITICAL (`internal/sextant`,
  `regulus task classify`): depth proportional to risk; risk markers force
  CRITICAL and TRIVIAL never delegates (docs/sextant.md, adr/T2-01).
- Structured chart contracts (`Class:` + `## Contract`): required keys per
  level, acceptance↔scenario traceability, Risk/Class agreement; `chart init`
  scaffolds a valid contract.
- Delegation + execution governance: `regulus task check` (routes, specs,
  dependencies incl. cycles, blockers with reasons, REPLAN format) and
  `regulus task status`.
- Core-verified completion: `regulus task close` checks contract, spec,
  hash-verified all-PASS receipts, closed dependencies and verifier evidence
  for CRITICAL; flips Status and appends CLOSED, or lists every unmet
  condition (docs/adr/T2-02). Skill/agent/discipline prose updated to match;
  plugin sync literals regenerated, token budgets green.

### Phase 1 — trust foundation (personal use)

- Centralized verdicts (`internal/verdict`): PASS/FAIL/BLOCKED/INCONCLUSIVE
  with six reasons; the core adjudicates, evaluations are recorded and can
  never override (docs/adr/T1-01).
- Candidate manifests: every `gate run` pins HEAD + diff hash,
  modified/untracked inventories, config fingerprint, tool versions,
  environment-names hash and task→artifact links next to its receipts
  (docs/adr/T1-02).
- `regulus evidence verify [--task]`: recomputes hashes, cross-checks
  manifests, quarantines corruption to `corrupt/` (moved, never deleted),
  fails closed on future schemas without crypto (docs/adr/T1-03).
- Execution policy (`execution` block + per-gate `isolate`/
  `reproducibility`): timeout clamps, destructive/allowlist classification,
  cwd pin, env unset list, 8 MB output cap with kill, opt-in Docker with
  recorded host fallback (docs/adr/T1-04). `gate run --dry-run` plans
  without executing; footer prints `verdict:` + `manifest:` lines.
- `docs/trust.md` documents the trust story; repo `regulus.json`
  dogfoods the `execution` block.

## [0.9.0] — 2026-10-30 — twelve gates + memory depth + doctor V2 (personal use)

Same branch (`feat/v0.4-evolution`); tags cut on main at merge.

Batch delivered since 0.8.1 (HEAD `c599145`):

- Health gaps: gauntlet at 12 gates (spec, ts-typecheck, ts-unit-tests,
  gofmt added); deterministic CI (cache off without go.sum, perf
  verdict, receipts artifact).
- Live-plus gaps + polish gaps: live OpenCode proof surface and
  polish items closed.
- Almanac robustez + g1-g4: retrieval depth (hybrid, relate, vindex,
  embed, miss-rate evidence) and memory CLI coverage.
- Doctor V2 refusals: doctor refuses V1 with guidance, warns on
  stale/unstamped binaries vs package.json.

Release gauntlet `bin/regulus gate run release-0-9-0/V1`: 12/12 pass
(receipts below, append-only).

| Layer | Gate | Receipt |
|---|---|---|
| 1 | typecheck | `rec_18e3623a05ee8890` |
| 2 | lint | `rec_18e3623a47723438` |
| 3 | build | `rec_18e3623a666c2128` |
| 4 | unit-tests | `rec_18e3623aab0f9e54` |
| 5 | coverage-floor (83.90 >= 80.00) | `rec_18e362416e9583b0` |
| 6 | mutation | `rec_18e3624196fd2efc` |
| 7 | crap | `rec_18e3624c410e0d08` |
| 8 | vulncheck | `rec_18e3624c575ba610` |
| 9 | spec | `rec_18e3624d049dc394` |
| 10 | ts-typecheck | `rec_18e3624d19c83254` |
| 11 | ts-unit-tests | `rec_18e3624d339fab44` |
| 12 | gofmt | `rec_18e3624dcb8b2474` |

## [0.8.1] — 2026-10-28 — self-audit fixes (personal use)

Same branch (`feat/v0.4-evolution`); tags cut on main at merge.

Resolves the Oct 28 self-audit (live probing + scout report):

- Plugin cwd fix: the CLI child process spawns with the project root as
  its working directory, so relative paths resolve from a live session;
  `resolveRegulus` refuses a mismatched core contract before any regulus
  tool executes.
- Tool-surface parity closed at 16: `memory_history` registered as a
  plugin tool; the sync guard asserts the exact tool count of 16 and each
  tool's declared params against its CLI flags.
- Docs truth sync: the `docs/agents.md` table raised to the full 16
  tools; `docs/architecture.md` lists the implemented hooks
  (`tool.execute.after`, 2× `session.context`, `session.compaction`); the
  README quickstart builds from source (`scripts/build.bash`) instead of
  advertising the unpublished npm package; the v0.8.0 journey entry names
  the test that exists (`TestV07MemoryLevels`).
- CI evidence visibility: the gauntlet workflow renders
  `regulus evidence list` into `docs/EVIDENCE.md` and commits it with
  `[skip ci]`.
- God-file split: `internal/cli/cli.go` (1,931 lines) and
  `internal/almanac/almanac.go` (931) moved into same-package sibling
  files — move-only, no behavior change.

## [0.8.0] — 2026-10-28 — tool-surface parity (personal use)

Same branch (`feat/v0.4-evolution`); tags cut on main at merge.

- Plugin↔CLI parity: `memory_search` gains path/limit/any/latest/include-superseded; new `trace_sessions`, `trace_files`, `probe_compose` (16 tools); guidance-sync test extended to required input params.
- `TestV07MemoryLevels`: deterministic end-to-end of the v0.7 memory surface (collapse/latest, any, limit, supersede-hide, doctor, import round-trip, prune protection, per-task cap refusal, trace sessions/prune).
- Troubleshooting +6 entries (reserved build.bash, per-day/per-task caps, prune dry-run, import atomicity, evolve no-evidence, Domain template).
- Hook spawn cut: `library domains` memoized per project dir (trace files and searches uncached); counting test proves 2 hooks = 1 domains spawn.
- `init` names `bun install in .opencode/` explicitly; `doctor` notes the plugin side can't be verified from outside OpenCode (exit codes unchanged).

## [0.7.0] — 2026-10-28 — almanac-level memory (personal use)

Same branch (`feat/v0.4-evolution`); tags cut on main at merge.

- `memory search`: `--latest` (opt-in chain collapse, pinned wins), `--any`, default `--limit 20` (hook injects top 3); confidence-ranked lessons; superseded hidden with `--include-superseded` escape + conflict marking.
- `memory doctor`: health report (counts, oldest/newest, malformed lines, orphan pins/relations).
- `memory import <file>`: restore export bundles, idempotent by id, validated per line.
- `memory prune`: dry-run default, `--yes` executes; lessons + pins immune.
- Per-task evolution caps (`max_per_task`, default half the daily cap); `trace sessions`/`trace prune`.
- Probe evidence separated under `evidence/<task>/probe/`; evolve pack carries a distinct Probe section.
- `regulus doctor` warns on stale/unstamped binaries vs package.json.
- `regulus init [--stack][--harness-from]` + `skills list` + chart `Domain:` template.
- `@regulus-proposer` + guidance/domain sync bun tests, all in CI.

## [0.6.0] — 2026-10-28 — elite hardening + functional completion

Same branch/PR as v0.4+v0.5; tagged together at merge. Two series:

### Elite (E-series) — the harness's own review caught the gaps
- `E1`: OpenCode v2.2.0 install verified (`regulus doctor` V2 green).
- `E3`: plugin domain tests committed (bun:test, run in CI) + CLI
  smokes in the gauntlet workflow.
- `E4`: `@regulus-proposer` — the read-only author consuming the
  evolve evidence pack (closes the authoring half of V5.7).
- `E5`: `regulus init [--stack go|node|smoke]` — fresh-repo scaffold.
- `E6`: skill trigger suggestions in the context hook (conservative
  2-word match, DATA-framed).
- `E7`: per-task evolution caps (`max_per_task`, default half daily).
- `E8`: `memory prune` (dry-run default, --yes executes; lessons +
  pins immune).
- `E9`: `trace sessions` / `trace prune` rotation.
- `E10`: docs swept to the v0.4/v0.5 reality; stamped version
  verified through `build.bash`.

### Functional (F-series) — found by the gauntlet itself (CI red)
- `F1`: compose.go CRAP hotspots split (34.53/32.07 → max 26.68);
  full local gauntlet 8/8 — the verification step CI charged for.
- `F3`: discipline + SKILL teach the new surface (PROBE section now
  the real flow; stale budget-record routing removed).
- `F4`: `evolve_propose`, `probe_contrast`, `trace_show`,
  `skills_list` plugin tools (13 total) + guidance-sync guard test.
- `F5`: `TestV06Functional` — the deterministic end-to-end journey
  (init → gauntlet → traces → contrast → reflect → admit → evolve →
  status).
- `F7`: `@opencode/plugin` 2.1.4 → 2.2.0 (matches installed runtime).
- `F9`: `init --harness-from <repo>` bootstraps the .opencode config
  (config only, never node_modules/lockfiles).

### Pending (owner)
- `F8`/`E2`: live OpenCode session proof (checklist in the feature
  doc). npm publish (token pending), merge + `v0.6.0` tags.

## [0.5.0] — 2026-10-27 — vanguard (harness² mechanisms, personal use)

Branch: `feat/v0.4-evolution` (v0.4 + v0.5 ship together; tags cut on main at merge).

### Added (V5 — vanguard disciplines + system evolution, shipped)
- `V5.1`: guardian + SKILL disciplines — KEEP_BASE and
  no-credit-for-elaborate in `regulus-reviewer`, receipt anonymity
  in `regulus-gate-verifier`, GENERALIZE doctrine in the Sextant
  LEARN step + `reflect` usage guidance.
- `V5.2`: trace file paths — `Entry.Files` (capped summaries, never
  values) + `record --files` + `trace files`; Almanac `Query.Path`
  bucket + `memory search --path`; the hook records touched paths
  and `resolveDomain` tries chart, then files, then lone domain.
- `V5.3`: library snapshots — admitted lessons snapshot the
  executable surface plus `EXPERIENCE.md` (why + transferable
  lesson) under `harnesses/<task>/`; INDEX entries are blocks;
  `library forget` prunes snapshots, `retain` prunes at/after bound.
- `V5.4`: deterministic compose — `probe compose` runs base+A+B
  through the gate, merges non-overlapping changes, flags conflicts
  keeping base; one probe-run budget entry.
- `V5.5`: per-task wall-time in `status` (`time: <task> <dur>`)
  aggregated from receipt durations.
- `V5.6`: `scripts/build.bash` reserved from surface edits
  (hand-edit only) + skill trigger registry (`skills list` from
  SKILL.md frontmatter).
- `V5.7`: system-driven `evolve propose` — evidence pack (session
  fingerprints, task receipts, domain INDEX, task decisions) +
  candidate template; design chart `charts/v0.5-evolve-propose.md`.

## [0.4.0] — 2026-10-27 — evolution (harness² gap closure)

Shipped on the same branch as v0.5, tagged together at merge:
closeness to harness2 — Evolution becomes automatic, relevant
and intelligent.

### Added (G0 — self-editable surface, shipped)
- `G0.1`: `.opencode/discipline.md` — the system prompt as editable
  harness surface; the plugin reads it with a compiled fallback.
- `G0.2`: `.opencode/tool-guidance.md` — all 9 tool descriptions
  (namespace + 8 tools) feed from markdown, merged over compiled
  defaults; missing file or section falls back silently.
- `G0.3`: INV/DOM dimension separation — `Dimension` (`inv` | `dom`,
  unset for legacy) in library lessons, Almanac lesson observations,
  `reflect --dimension`, `memory search --dimension`, and the domain
  INDEX lines.

### Added (G1 — relevant retrieval, shipped)
- `G1.4`: domain-aware lesson injection — `resolveDomain` (chart
  `Domain:` line from message refs → lone chart → lone library domain
  via new `regulus library domains`) scopes the context hook to
  `memory search --type lesson --domain`; `memory_search` tool gains
  the `dimension` bucket.
- `G1.5`: `scripts/` admitted to the surface whitelist
  (`docs/architecture` was already covered by `docs/`).

### Added (G2 — intelligent Evolution, shipped)
- `G2.6`: trace capture via the `tool.execute.after` hook —
  `trace record/show/files`, fingerprint `N tool status`.
- `G2.7`: probe with contrast — two gate runs through the same
  path, receipt diff as evidence (`probe contrast`), cap-enforced.

### Dropped (G3 — product)
- `G3.8`: npm publish — dropped as unnecessary: Regulus is personal
  use (owner decision L14); local build + plugin by path cover the
  workflow, CI builds from source. Revisit if it ever goes public.

### Kept (G3 — product)
- `G3.9`: this changelog lives in the repo; release tags are cut on
  main at merge.

## [v0.3] — refinement (P0 fixes + P1 hardening)
- `P0.1`: CI gate resolves the binary directly (`bin/regulus`).
- `P0.2`: surface edits under budget caps (KindSurface + propose/apply
  ledger recording; over-cap refused and recorded).
- `P0.3`: pinned observations reorder first in search + a Pinned
  section in context. `P0.4`: plugin also probes `binaries/`.
- `P1.1`: tool-name fixes (no `regulus_` prefix). `P1.2`: gremlin
  coverage tests. `P1.3`: `docs/agents.md` + `docs/troubleshooting.md`.
- Gauntlet 8/8 green; 13/13 Go packages; tsc clean.

## [v0.2] — completion (every gap closed)
- Pin/relate/export, surface edits (append-only, marker-delimited),
  lesson injection, `/regulus-verify`, 8-layer gauntlet, CI workflow,
  `govulncheck`, live OpenCode boundary proof (plugin loads, tools
  register, guardian admits a lesson).

## [v1] — the complete harness (F1–F5)
- F1 vertical spine; F2 complete 8-layer gauntlet; F3 Almanac + Crew;
  F4 two-phase evolution (propose → guardian → admit, budget caps,
  probe cycle); F5 npm installer with cross-platform launcher +
  release. Ponytail Necessity Ladder + regulus-reviewer (6th guardian);
  Sextant workflow; Charts as acceptance scenarios.
