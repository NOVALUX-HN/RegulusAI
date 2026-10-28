# Changelog — Regulus AI

All notable changes, newest first. Format follows Keep a Changelog;
versioning is `vMAJOR.MINOR` tags on main (release tags are cut at
chart close — see G3.9).

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
