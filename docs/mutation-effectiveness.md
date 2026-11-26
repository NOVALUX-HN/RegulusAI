# Mutation effectiveness — F20.3 (NOV-152)

Measured, not asserted. Every number below comes from a `gremlins unleash` run on
this repository on 2026-11-26, with the exact flags of the existing gates
(`--threshold-efficacy 70 --workers 4 --timeout-coefficient 10`).

The parent phase says "no exigir mutation/coverage universales" and asks for selective
mutation with survivors classified by test gap and cost. That is what this record is:
four packages, chosen because they decide integrity, not because they were easy.

## Baseline and after

| Package | Before | Lived | Not covered | After | Lived | Time | Gate |
|---|---|---|---|---|---|---|---|
| `internal/receipts` | **100.00%** | 0 | 11 | — | — | 1m05s | `mutation-receipts` |
| `internal/gates` | **98.64%** | 5 | 41 | **99.73%** | 1 | 5m56s | `mutation-gates` |
| `internal/evidence` | **78.40%** | 27 | 12 | **84.00%** | 20 | 1m11s | none → added |
| `internal/recovery` | **77.83%** | 47 | 22 | not changed | 47 | 1m24s | none → added |

Total cost for the four: about **9m40s**. That is the reason mutation stays off the
quick profile and out of `required`.

## The finding that matters

**Efficacy follows the gates.** The two packages that already had a mutation gate sit
at 98.6% and 100%. The two that had none — and they are the two that protect the
project's integrity claims — sat at 78%.

* `internal/evidence` decides whether a receipt is genuine: hashes, schemas, manifest
  cross-checks. A surviving mutant there means a false proof could pass unobserved.
* `internal/recovery` decides exactly-once effects and budget thresholds.

That is the opposite of the intuitive assignment of effort, and it is only visible
because the weakest packages were measured rather than assumed.

## Survivors, classified

### Closed (test gap, cheap)

| Site | Lived | Kind | What closed it |
|---|---|---|---|
| `internal/gates/validate_helpers.go:91` | 4 | boundary | No test used an id whose characters sat at the edge of `[a-z]`, `[A-Z]`, `[0-9]`. `TestValidateSplitIDCharset` now pins both the inclusive edges (`a z A Z 0 9 - _`) and the bytes immediately outside each range (`` ` { @ [ / : ``). |
| `internal/evidence/evidence.go:104` | 7 | boundary + negation | `isWindowsAbsolute` had **no direct test at all**, and it is a path-escape guard. `TestIsWindowsAbsolute` pins length 2, both letter ranges on both sides, non-letters, and a second byte that is not a colon. |

Measured effect: `gates` 98.64% → **99.73%**, `evidence` 78.40% → **84.00%**.

### Equivalent, with proof (not a test gap)

`internal/gates/quick_lean.go:35` — `CONDITIONALS_BOUNDARY` on
`out[i].Layer < out[j].Layer`. It is the last surviving mutant in `gates`, and it
cannot be killed by a test: `validate` rejects duplicate layers ("duplicate layer"),
so `SelectQuickLean` never sees two gates with the same layer, which is the only input
where `<` and `<=` differ. Writing a tie-breaking test would fail at validation first.

Classification: **equivalent under the current validator**. No code change: adding a
name tie-break would be dead code that no valid input can reach.

### Open (test gap, measured cost)

`internal/recovery`, 47 lived, clustered:

| Site | Lived | Reading |
|---|---|---|
| `progress.go:184` | 4 | budget comparison; no test sits on the threshold |
| `outcome.go:149`, `:152` | 3 + 3 | outcome classification boundaries |
| `progress.go:71`, `:74`, `:80`, `:173`, `:186` | 2 each | classification edges |

These are F17 budget and outcome thresholds. Closing them means boundary tests around
each budget, which is real work and belongs in its own task rather than in this
record. The number is now measured, so it can be tracked instead of guessed.

### Not covered

41 in `gates`, 22 in `recovery`, 12 in `evidence`. Uncovered code is a coverage
question, not a mutation one: the CRAP gate and the 80% floor already answer it. Not
counted as survivors anywhere above, and not conflated with them.

## Decision: measured ratchets, still never required

`regulus.json` now carries four mutation gates. All stay `required: false` and
`profiles: ["full"]`, because the parent forbids universal mutation and the measured
cost is ~10 minutes.

* `mutation-receipts`: 70 → **95** (measured 100%)
* `mutation-gates`: 70 → **95** (measured 99.73%)
* `mutation-evidence`: **new**, 82 (measured 84.00%)
* `mutation-recovery`: **new**, 76 (measured 77.83%)

Each threshold is a **floor to prevent regression, not a quality goal**. They are set
just below the measured value on purpose: a refactor that genuinely lowers efficacy by
three points should stop and explain itself, and the number should be raised again once
the open clusters above are closed. Presenting 76% as a target would be the vanity
metric this phase excludes.

## Reproduce

```bash
export PATH=$HOME/go/bin:$PATH
gremlins unleash ./internal/evidence  --threshold-efficacy 82 --workers 4 --timeout-coefficient 10
gremlins unleash ./internal/recovery  --threshold-efficacy 76 --workers 4 --timeout-coefficient 10
gremlins unleash ./internal/gates     --threshold-efficacy 95 --workers 4 --timeout-coefficient 10
gremlins unleash ./internal/receipts  --threshold-efficacy 95 --workers 4 --timeout-coefficient 10
```

On this Windows host the runs must happen inside WSL: Smart App Control blocks every
freshly built executable, so gremlins cannot fork a test binary natively. That is an
environment fact, documented in `docs/execution-backend.md`, not a property of the
tooling.
