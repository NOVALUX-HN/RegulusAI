# ADR T4-01 — Decisions as assessable contracts, not autonomous approvals

Date: 2026-10-30 · Status: accepted · Phase 4 (Decision Intelligence)

## Context

Technical decisions lived in prose (charts, logs, chat): unpriced
alternatives, unnamed risks, unlinked evidence, no memory of what was
decided and why. An evaluation layer grown on top of that would either
rubber-stamp prose or hallucinate rigor.

## Decision

Decisions are structured contracts (`internal/decisions`) with validated
shape at propose time and assessed substance at assess time: ten explicit
criteria, each pass|fail|abstain, over resolved evidence refs, with a
mechanical-proxy discipline documented per criterion. The pipeline
(PERCEPTION → EVIDENCE → POLICY → ASSESSMENT → DECISION) prints every
stage; `decide assess --json` exposes the full record. Findings are coded
(contradiction, missing alternatives, unmitigated risk, hidden dependency,
obsolete scope, insufficient evidence, unsupported assumptions) with
blocking vs warning severity.

## Consequences

- Assessment output is deterministic given the same store: re-running
  assess on unchanged evidence yields the same recommendation.
- Proxies are honest by construction (shared substance, named costs,
  declared mitigations); anything finer needs a human reader, and the
  system abstains instead of guessing.
- New criteria require tests pinning their pass/fail/abstain behavior.
