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
| Context engine | `internal/context/context.go:573` | `Select`: ACL → rank → detect → layer → budget trim |
| Context ranking | `internal/context/context.go:322`, `:467` | descriptor terms + domain affinity; essential threshold 0.35; critical always essential |
| Context reporting | `internal/context/context.go:157` | `SavedPct`, `CriticalRecall`, `Duplicates`, `Obsolete`, `Injections`, `BudgetTrimmed` |
| Context budget | `internal/almanac/context.go:46` | 20 items / 20 per list by default, aligned with compaction |
| Token ceilings | `internal/observability/budgets.go:128` | trivial 10/10, standard 20/20, complex 40/40, critical = full with justification |
| Budget enforcement | `internal/execute/budgets.go:19` | labelled estimates against ceilings; `Unknown` never gates |
| Token provenance | `internal/trace/hostusage.go:23`, enforced at `internal/trace/trace.go:173` | `measured-host` / `measured-local` / `estimated` / `proxy` / `unknown` |
| Compaction | `.opencode/plugins/regulus.ts:820`, args at `.opencode/lib/domain.ts:55` | bounded resume hook, 20/20, reversal `REGULUS_COMPACTION_FULL=1` |
| Context A/B | `internal/bench/context_ab_test.go:88` | 20/20 vs 10/10 on one fixture, proxy delta, NOT ADOPTABLE verdict path |

## 2. Gap G0 — the parent's own rule has no enforcement

S6 says critical, security, governance, validation and recovery tools must be
preserved, and that low frequency is never expendable. **Nothing in the code
knows what a critical tool is.** Verified:

```
grep -rn "critical" .opencode/lib/domain-toolset.ts   → empty
```

`TOOLSET_MINIMAL` is three tools chosen by name, and `resolveToolset` picks by
`TE_TOOLSET` plus the presence of `charts/`. No flag, annotation or policy marks
a tool as non-expendable, so nothing prevents a future edit from dropping
`gate_run` or `spec_check` from Full, and nothing would fail.

The nearest existing vocabulary is unrelated: MCP capability classes
(`internal/mcp/policy.go:32`, read/write/execute/secrets/memory/evidence) describe
what a tool may do, not whether it may be removed, and `Critical` on context
fragments (`internal/context/inventory.go:55`) marks knowledge, not tools.

This is the sharpest gap in the phase and the one T2 must close before any
adaptive profile is even conceivable.

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

## 7. Explicit non-gaps (do not spend effort here)

* The provenance vocabulary and its enforcement — present, closed and tested.
* Budget ceilings and the "unknown never gates" rule — correct.
* Context ranking, essential threshold and "critical never omitted" for
  knowledge fragments — correct and tested.
* The compaction reversal switch — present.
* The A/B test's refusal to claim savings on a proxy — exactly the right
  behaviour and must be preserved.

## 8. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/context/ ./internal/trace/ ./internal/bench/ ./internal/almanac/ ./internal/execute/
```

This document changes no code: it is the deliverable F19.1 asks for ("auditar
toolsets, compaction, predictive context, token budget, telemetry y existing
Full/Minimal profiles; definir baseline por host/tarea").
