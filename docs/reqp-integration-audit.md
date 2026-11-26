# REQP integration audit — F20.1 (NOV-150)

Baseline: `develop`, audit at `HEAD 02c09fb`, clean tree. Read-only artefact.

The parent asks to "apply and make measurable a transversal REQP, evolving reviewer,
audit, gates, coverage, mutation, fuzzing, CI and quality metrics that already
exist, **without creating a new reviewer/tooling stack**".

So the first question is not "what should we build" but "what already blocks". The
answer is uneven, and this document states it per practice rather than as a
comfortable summary.

## 1. What actually blocks today (reuse this, do not rebuild)

| Surface | Location | What it enforces |
|---|---|---|
| Gauntlet | `regulus.json:7-23` | 23 gates; the nine `required: true` ones (`typecheck`, `lint`, `build`, `unit-tests`, `coverage-floor>=80%`, `crap(max30)`, `spec`, `gofmt`, `size`) are never skipped by profile |
| Required never skipped | `internal/gates/gates.go:73,83` | `IsRequired()` + `EffectiveOnFail()` |
| Profile split | `internal/gates/gates_plan.go:35` | `quick` = required only; `full` = all |
| Chart barrier | `internal/cli/gate_run.go:282,384` | an unverifiable chart blocks BUILD before layer 1 |
| DoD enforcement | `internal/sextant/close.go:34,155` | spec OK + Contract present + manifest schema 1 + non-empty artifacts + **zero `Skipped`** + `evidence.Verify` clean + every verdict `pass`; anything else → `Unmet`, close refused |
| Receipt integrity | `internal/receipts/receipts.go`, `internal/evidence/verify.go:57,149` | append-only `O_EXCL`, full stdout/stderr SHA-256, manifest→receipt→disk cross-check, corrupt quarantined never deleted |
| Spec contract | `internal/spec/spec.go:129` | `## Acceptance` with `### Scenario:` and Given→When→Then order, all violations reported at once |
| Idempotency | `internal/almanac/dedup.go:91` | `contentHash` — a repeated save is a no-op |
| Effect idempotency | `internal/recovery/recovery.go:115,195` | `EffectResult` exactly-once per idempotency key |

This is the half of REQP that is real. It is measured, it blocks, and it cannot be
skipped.

## 2. What is prose only (the honest gap)

| Practice | Today | Status |
|---|---|---|
| Subtract before you add | `SKILL.md`, `internal/sextant/classify.go` strings, the Necessity checklist printed by `internal/cli/review.go:397` | **prose** — advisory print, no gate |
| Attack the premise | same | **prose** |
| Root cause first | same | **prose** |
| TDD when appropriate | nothing; the nearest proxies are `coverage-floor` 80%, `crap<=30`, optional mutation, and `reviewUntestedGo` advice | **unenforced** |
| Small verifiable units | no unit-size limit anywhere | **unenforced** |
| Behavior testing | the suite exists, but no check distinguishes behaviour from implementation | **prose** |
| Boundaries | no boundary-coverage check | **prose** |
| General regression | `internal/verify/regression.go` folds a baseline window but gates nothing; enforced only in niches (`context/promotion.go:58`, `bench/bench.go:237`, `evolve/compare*`) | **advisory outside niches** |
| General rollback | only `internal/evolve/rollback.go` for evolve versions | **evolve-only** |

And two structural observations:

* **Sextant's phases are prose.** `Authorize → Chart → Build → Prove → Learn` lives
  in the skill and in classifier strings; the code enforces *completion*
  (`close.go`), never phase order. No state machine forces the sequence.
* **There is no single engineering-protocol document.** It is scattered across
  `docs/sextant.md`, `gauntlet.md`, `review.md`, `verification.md`,
  `contracts.md`, `quality-intelligence.md`, `evolution.md`, the ADRs and the
  skill. A protocol nobody can read in one place cannot be audited as one.

## 3. What F20.1 should therefore do (minimal, no new stack)

1. **Publish the protocol in one document** that states, per practice, whether it
   is enforced, advisory or prose — this table is the seed.
2. **Move only what is cheap and honest from prose to enforcement.** The close path
   already refuses on evidence; the review lens already computes the Necessity
   checklist. Promoting those two from "printed" to "recorded in a receipt" is
   integration, not a new stack.
3. **Do not invent seven new gates.** Most REQP practices are judgement calls with
   no deterministic oracle; a gate that pretends otherwise is a vanity metric,
   which the phase excludes.

## 4. Explicit non-gaps (do not rebuild)

* Required gates, chart barrier, profile split — enforced and correct.
* Close/DoD enforcement including the zero-skipped rule — the strongest existing
  REQP evidence.
* Receipt hashing, cross-checking and quarantine — enforced and honest.
* Spec contract validation — enforced.
* Almanac and recovery idempotency — enforced.

## 5. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/gates/ ./internal/sextant/ ./internal/receipts/ ./internal/evidence/ ./internal/spec/
```

This document changes no code: it is F20.1's audit deliverable, and it conditions
F20.2 (tooling), F20.3 (mutation) and F20.4 (reviewer effectiveness) because none of
them can be measured against a protocol that is not written down.
