# Changelog — Regulus AI

All notable changes, newest first. Format follows Keep a Changelog;
versioning is `vMAJOR.MINOR` tags on main (release tags are cut at
chart close — see G3.9).

## [Unreleased] — v0.4 evolution (harness² gap closure)

Branch: `feat/v0.4-evolution`. Closes the gap analysis against
google-research/harness2: Evolution becomes automatic, relevant and
intelligent. G0 slice shipped; G1/G2/G3 in progress.

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

### Planned (G2 — intelligent Evolution)
- `G2.6`: trace capture via the `tool.execute.after` hook.
- `G2.7`: probe with contrast — two gate runs, diffed traces/receipts
  as evidence.

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
