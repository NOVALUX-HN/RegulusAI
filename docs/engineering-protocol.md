# Engineering protocol (REQP)

One page, because F20.1's audit found the protocol was scattered across seven
documents, the ADRs and the skill — and a protocol nobody can read in one place
cannot be audited as one.

Every entry states how it is **held**, not how it is wished. `regulus protocol`
prints this table from code (`internal/protocol`), and the test suite fails if a
citation stops existing, so the two cannot drift.

```bash
regulus protocol                      # the table
regulus protocol --enforcement enforced
regulus protocol --json
```

## The rule for this page

A practice is `enforced` only when something blocks. Where no deterministic oracle
exists, the entry says `judgement` and the text says why. Marking judgement as
enforced would be the vanity metric F20 excludes: a gate that pretends to measure
taste is worse than an honest gap, because it teaches people to trust it.

| Practice | Held as | Where | Why that level |
|---|---|---|---|
| `subtract` | judgement | `internal/cli/review.go` | Substract before you add. The review lens prints a Necessity Ladder checklist, but printing is not blocking, and a subtraction gate would have to judge new code. |
| `premise` | judgement | — | Attack the premise. Whether a requirement is the right one cannot be computed; it is recorded so its absence is never mistaken for an oversight. |
| `root-cause` | judgement | — | A fix that treats a symptom passes every test the symptom had. Only reading the diff catches it. |
| `tdd` | judgement | — | Test-first when viable. Coverage floor and CRAP measure the result, not the order; they cannot tell a test written before from one written after. |
| `small-units` | enforced | `regulus.json` | Small verifiable units. The `size` gate is required, so no profile skips it. This is the part of the practice that has an oracle. |
| `behavior` | judgement | — | Test behaviour, not implementation. A test asserting internals still passes the suite. |
| `boundaries` | judgement | — | No dedicated check exists. Mutation testing is the closest proxy and runs full-profile only. |
| `idempotency` | enforced | `internal/almanac/dedup.go` | A repeated save hashes to the same key and becomes a no-op; recovery effects are exactly-once per idempotency key. |
| `evidence` | enforced | `internal/sextant/close.go` | Close refuses on any skipped gate, non-pass verdict, missing manifest or hash drift; required gates cannot be skipped by profile (`internal/gates`). |
| `regression` | advisory | `internal/verify/regression.go` | The window comparison reports and gates nothing at the top level. It is enforced only where a decision consumes it: context promotion, bench adoption, evolve comparison. |
| `rollback` | advisory | `internal/evolve/rollback.go` | Rollback exists for evolve versions only. There is no generic code-change rollback, and receipts are deliberately never rewritten: a new run is a new receipt. |

## What is already blocking, and must not be rebuilt

These are correct today. F20.1 asks for integration, not a new stack, so they stay:

* **Required gates are never skipped** by profile or `when` (`internal/gates`).
* **The chart barrier**: an unverifiable chart blocks BUILD before layer 1.
* **The close contract**: spec + contract + manifest schema + non-empty artifacts +
  zero skipped + hash verification + every verdict `pass`.
* **Receipts**: append-only, full-output SHA-256, manifest→receipt→disk cross-check,
  corrupt evidence quarantined and never deleted.
* **Spec contract**: `## Acceptance` with `### Scenario:` and Given→When→Then order.
* **Idempotency** in almanac and recovery.

## What F20.1 deliberately did not do

* **No seven new gates.** Most REQP practices have no deterministic oracle.
* **No change to the review receipt contract.** Review is documented as
  informational and never writes receipts; promoting the checklist to a receipt
  would have broken that invariant to look thorough.
* **No requirement that charts declare their applicable practices.** It would have
  failed every existing chart for no measured benefit.

## Verification

```bash
bash scripts/verify-wsl.sh ./internal/protocol/
regulus protocol
regulus protocol --json
```

## Known limit of the pin

The tripwire proves a citation still **exists**, not that it is still **called**. If
somebody left `contentHash` defined and unreferenced, the idempotency marker would
survive while the enforcement was gone. Markers that name a live condition
(`len(m.Skipped) > 0`) or a gate definition (`"name": "size"`) do not have that
weakness, because removing the behaviour removes the text. The rule when adding a
practice here: prefer a marker that cannot outlive its behaviour.
