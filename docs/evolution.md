# Regulus Evolution — Controlled Self-Improvement (Phase 6)

Evolution proposes; it never self-applies. Every improvement walks
OBSERVE → HYPOTHESIZE → PROPOSE → PROBE → EVALUATE → ADOPT / REJECT
with explicit stage contracts (`evolve.Contract`), isolated experiments,
a pinned baseline, and human-approved adoption. Decisions:
`docs/adr/T6-01`, `T6-02`.

## 1. The pipeline

Stages advance one step (`evolve.Advance`); skips, backward moves and
moves out of ADOPTED/REJECTED are refused. Contracts:

- **OBSERVE**: receipts, traces, guardian findings, gauntlet results,
  costs, memory insights, task outcomes → supported observations.
- **HYPOTHESIZE**: supported observations → hypothesis (problem, root
  cause, change, benefit, regressions, criteria).
- **PROPOSE**: hypothesis → stored experiment (baseline, candidate,
  dataset, model, runtime, parameters, budget).
- **PROBE**: proposal + isolated runner → candidate results (the harness
  itself is never mutated by a probe).
- **EVALUATE**: candidate + baseline results → ADOPT or REJECT verdict.
- **ADOPTED / REJECTED**: terminal; adoption versions the change with
  an audit trail, rejection discards the candidate and keeps the baseline.

## 2. Observation discipline

`evolve.Observe` collects receipt-backed observations (failed and
repeated failures, costs, outcomes); guardian/memory/gauntlet notes
arrive through `evolve.Note` with caller-attested evidence. Support is
counted, not assumed: a single case (`Count < 2`) is isolated and
`CheckSupport` refuses to generalize from it — one failure is never a
general rule.

## 3. Experiments

`ProposeExperiment` stores `exp_*` records under `.regulus/evolution/`
with exclusive ids and re-checkable integrity, mirroring the decisions
pattern (adr/T6-01). States walk
PROPOSED → RUNNING → EVALUATED → ADOPTED | REJECTED. Strategies
(`RunExperiments`) run sequential, parallel or hybrid over an injectable
probe; attempts are capped (`RunCap` + per-experiment `MaxRuns`) and every
attempt is journaled in the experiment history. The runner reloads state
from disk, so caller-held copies can never clobber transitions.

## 4. Metrics, baseline, rollback

`Summarize` aggregates raw trials into the eleven S6 rates; empty input
yields zeros, never NaN. `SetBaseline` pins the stable metrics;
`Adoptable` refuses any candidate that drops success, acceptance, policy
or evidence compliance below baseline or raises the regression rate.
`Adopt` versions the change (`versions/N.json`, monotonic) and advances
the experiment; `Rollback` supersedes a version with a reason and hands
the snapshot back. Versions plus experiment histories are the audit trail.

## 5. Domains and trust

INV changes how the agent works; DOM changes what a practitioner knows.
`CheckDomain` refuses DOM candidates that touch harness fundamentals
(`TouchesFundamentals`) — reclassify as INV or narrow the candidate.
Every proposal is untrusted until `TrustCheck` proves otherwise:
isolated, provenance, validations, regressions, policies, reversible —
verified structurally inside `Adopt`, plus a named human approver. No
checklist, no approval, no adoption. Ever.
