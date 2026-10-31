# ADR T2-01 — Chart contracts are structured and traced

Date: 2026-10-30 · Status: accepted · Phase 2 (Sextant)

## Context

Charts carried acceptance scenarios (validated by `spec check`) but no
machine-readable statement of what the work covers: scope, dependencies,
artifacts and risk lived in prose, and nothing linked the contract to the
scenarios or the tasks. Disconnected specs pass silently.

## Decision

Charts gain two machine-checked elements: a top-level `Class:` line (like
`Domain:`) and a `## Contract` section with a closed key set, validated by
`internal/sextant` (`task check`). `Acceptance:` must name scenarios that
exist; `Risk:` must agree with `Class:`; required keys grow with the class;
unknown keys are rejected. Absent sections never violate — legacy charts
keep working and opt in by adding the section.

## Consequences

- `chart init` scaffolds a valid STANDARD contract; the scaffold test pins it.
- Inconsistency (missing keys, ghost scenarios, Risk/Class mismatch) fails
  `task check` with line numbers, and blocks `task close`.
- Contract keys evolve only with parser support + tests.
