# Context and tool optimization baseline — F19.1 (NOV-146)

Baseline: `develop`, audit performed at `HEAD fb8b7ee`, clean tree. Audit-only
artifact: every claim cites code read in this checkout.

Scope reminders from the parent: do not invent token savings, do not remove tools
by frequency alone, do not create a parallel selector or compactor, and
**low-frequency never means expendable**.

## 1. What exists (do not rebuild)

| Surface | Location | What it does |
|---|---|---|
| Toolset profiles | `.opencode/lib/domain-toolset.ts:25` / `:52` | `TOOLSET_FULL` and `TOOLSET_MINIMAL` (3 tools: `doctor`, `memory_search`, `memory_context`) |
| Profile selection | `.opencode/lib/domain-toolset.ts:62`, run once at `.opencode/plugins/regulus.ts:227` | `TE_TOOLSET=full\|minimal\|auto`; auto serves full when `charts/` exists, minimal otherwise; fail-closed to full |
| **Usage pruning** | `.opencode/lib/domain-usage.ts:20`, applied at `.opencode/plugins/regulus.ts:242-249` | `PRUNE_MIN_SESSIONS = 3`; when the mode is full and `TE_TOOLSET` is **not explicit**, tools are removed by measured use |
| Context engine | `internal/context/context.go:573` | `Select`: ACL → rank → detect → layer → budget trim |
| Context ranking | `internal/context/context.go:322`, `:467` | descriptor terms + domain affinity; essential threshold 0.35; critical always essential |
| Context reporting | `internal/context/context.go:157` | `SavedPct`, `CriticalRecall`, `Duplicates`, `Obsolete`, `Injections`, `BudgetTrimmed` |
| Context budget | `internal/almanac/context.go:46` | 20 items / 20 per list by default, aligned with compaction |
| Token ceilings | `internal/observability/budgets.go:243-249` | trivial 10/10, standard 20/20, complex 40/40, critical = full with justification |
| Budget enforcement | `internal/execute/budgets.go:19` | labelled estimates against ceilings; `Unknown` never gates |
| Token provenance | `internal/trace/hostusage.go:23`, enforced at `internal/trace/trace.go:173` | `measured-host` / `measured-local` / `estimated` / `proxy` / `unknown` |
| Compaction | `.opencode/plugins/regulus.ts:820`, args at `.opencode/lib/domain.ts:55` | bounded resume hook, 20/20, reversal `REGULUS_COMPACTION_FULL=1` |
| Context A/B | `internal/bench/context_ab_test.go:88` | 20/20 vs 10/10 on one fixture, proxy delta, NOT ADOPTABLE verdict path |

## 2. Gap G0 — frequency-based removal already exists, with no criticality guard

S6 says critical, security, governance, validation and recovery tools must be
preserved, and that low frequency is never expendable. The first pass of this
audit reported that nothing marked a tool as critical. That was true but
incomplete, and the fuller answer is worse: **tools are already removed by
frequency.**

* `PRUNE_MIN_SESSIONS = 3` (`domain-usage.ts:20`) is the evidence floor.
* `applyUsagePruning` (`domain-usage.ts:53`) trims the full set by measured use.
* `regulus.ts:242-249` applies it whenever the mode is `full` **and** `TE_TOOLSET`
  is not explicit, printing the pruned names to stderr.

So the phase rule is not unenforced because nothing was built — it is violated by
the current implementation, quietly, for the tools nobody happens to call.
`gate_run`, `spec_check`, `evidence_verify` or `yolo` can be pruned for being
rare, which is exactly what the parent forbids. The escape hatch
(`TE_TOOLSET=full`) is manual and opt-out, not a guarantee.

Verified: `grep -rn "critical" .opencode/lib/domain-toolset.ts` is empty, so
nothing in the pruning path checks expendability. The nearest existing vocabulary
is unrelated: MCP capability classes (`internal/mcp/policy.go:32`) describe what a
tool may do, `ToolPolicy.Sensitive` (`policy.go:278`) marks state-changing tools
for authorisation, and `Critical` on context fragments
(`internal/context/inventory.go:55`) marks knowledge, not tools.

This is the sharpest gap in the phase: **T2 must guard the pruning that already
exists** before it considers any new adaptive profile.

### A stale comment worth fixing while there

`regulus.ts:226` says `TE_TOOLSET=full` restores "all 16", while
`TOOLSET_FULL` lists 19 entries. Anyone auditing the surface reads the comment
first, so the number must be either corrected or derived.

## 3. Gap G1 — there is no measured baseline per host and task

Cost folds exist (`CostByTask`, `CostByHost`), and the provenance vocabulary is
honest, but the measurement is not: the host exposes no token counts
(`TokensKnown false`), so most figures are `estimated` or `proxy`. The one context
A/B runs on a single 12-note fixture with no task holdout
(`internal/bench/context_ab_test.go:28`) and reports a proxy delta, explicitly
refusing to claim savings.

Consequence: F19 cannot state "we saved N tokens" anywhere. It can only state
"we moved a proxy on one fixture".

## 4. Gap G2 — selection is not task-aware, and no profile comparison exists

`resolveToolset` keys on an environment variable and the presence of `charts/`.
`context.Select` keys on descriptor relevance and a budget. Neither switches
Full/Minimal by task class, and **no Full-vs-Minimal comparison exists** in
`internal/bench/` (grep for `toolset` returns nothing).

That is why F19.2 says "solo si gap demostrado": today there is no benchmark that
would justify a third profile, and inventing one without that benchmark is
exactly what the phase excludes.

## 5. Gap G3 — context quality and loss are asserted, never measured

`context_fidelity_test.go:15` checks that essentials are *contained*; `SavedPct`
is computed from counts; `CriticalRecall` must be 1. There is no retrieval
precision, no numeric compaction loss, no measurement of whether omitted
information is recovered later, and no task-outcome delta. "Quality preserved" is
a boolean on a fixture, not a measurement.

## 6. What each follow-up may do

| Task | May do | Evidence it must produce |
|---|---|---|
| F19.2 (NOV-147) | Add a criticality notion and, only if the baseline justifies it, a task-aware profile | A test that fails if a critical tool is dropped from a profile, plus a benchmark or an explicit audit-only closure |
| F19.3 (NOV-148) | Measure loss numerically and prove omitted information is recoverable | A loss/recovery measurement, not a containment boolean |
| F19.4 (NOV-149) | Promotion → monitor → rollback with provenance labels | A reversible promotion whose savings are labelled `measured`/`proxy`/`unknown`, never blended |

## 7. Existing machinery F19.2 must not rebuild

The review found four adjacent mechanisms the first pass missed. None of them
guards expendability, but each is close enough that duplicating it would be a
defect:

| Mechanism | Location | What it is |
|---|---|---|
| `ToolPolicy.Sensitive` | `internal/mcp/policy.go:278` | marks state-changing tools (`memory_note`, `gate_run`, `evolve_tick` wet) for authorisation — not for removal |
| Required gates | `regulus.json:7-17`, enforced at `internal/cli/gate_run.go:334` | `required: true` on gates; a missing required tool fails the run, which is the nearest existing notion of non-expendable |
| `RiskCritical` | `internal/verify/verify.go:98` | risk class of a verification item, not of a tool |
| Canary scope | `internal/evolve/promote.go:102-111`, `governed_eval.go:361` | keeps critical surfaces out of a canary — the closest existing "do not touch this" rule |

## 8. Explicit non-gaps (do not spend effort here)

* The provenance vocabulary and its enforcement — present, closed and tested.
* Budget ceilings and the "unknown never gates" rule — correct.
* Context ranking, essential threshold and "critical never omitted" for
  knowledge fragments — correct and tested.
* The compaction reversal switch — present.
* The A/B test's refusal to claim savings on a proxy — exactly the right
  behaviour and must be preserved.

## 9. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/context/ ./internal/trace/ ./internal/bench/ ./internal/almanac/ ./internal/execute/
```

This document changes no code: it is the deliverable F19.1 asks for ("auditar
toolsets, compaction, predictive context, token budget, telemetry y existing
Full/Minimal profiles; definir baseline por host/tarea").
