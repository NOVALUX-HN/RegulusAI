# Verification effectiveness — F20.4 (NOV-153)

The task asks whether the reviewer and the gates **actually detect** defects and false
claims, using fixtures, adversarial diffs and failing tests, and to fix the gaps.

So this is a detection audit, not a feature. It states what an executing test proves,
what was only assumed, and what changed.

## 1. Proven today, by tests that run

| Property | Proof |
|---|---|
| A failing gate stops the batch and blocks | `internal/gates/gates_test.go:121`, `:217`, `:311`, `:354`, `:391` |
| Destructive or variable commands are refused | `internal/gates/policy_allowlist_test.go:61,93,104`, `trust_test.go:13` |
| A tampered receipt is caught and quarantined | `internal/evidence/verify_test.go:80`, `:165` |
| A missing or unlisted manifest fails | `internal/evidence/verify_test.go:275,296` |
| A skipped gate can never prove a run | `internal/sextant/close_latest_test.go:124` |
| A failed gate blocks close, naming the gate | `internal/sextant/close_phases_test.go:59` |
| A legacy receipt without a manifest never closes | `internal/sextant/close_latest_test.go:22` |
| The reviewer picks the right lens and reports violations | `internal/cli/review_test.go:84,107,126,154,250` |
| Adversarial classes are contained | `internal/adversarial/adversarial_test.go` over nine seeds |

That is a real detection surface, and most of it predates this task.

## 2. Gap closed: close could be lied into

**What was missing**: no test forged a chart that *claims* success — task row `done`, a
`CLOSED` line in the Log — over contradicting evidence. Failing evidence blocked clean
charts, and a lying status line was only caught by `CheckProgress`, never by the
evidence-backed `CheckClose`. "Close cannot be lied into" was asserted in prose and
executed by nothing.

**Closed** with `internal/sextant/close_false_claim_test.go`:

* forged claim, no evidence → refused
* forged claim over a **failing** gate → refused, naming `build`
* forged claim over a **tampered** receipt whose PASS fields are intact → refused,
  naming `verify`
* the same claim over real evidence → closes, which is what proves the fixture is
  judging the evidence and not the wording

The last case exists because a refusal test that never shows the positive path can pass
for the wrong reason — a fixture that always fails proves nothing.

## 3. Gaps recorded, not fixed here

### The gauntlet shape check pins a copy, not the configuration

`internal/bench/bench_shape.go:139` builds `realGateConfig()` by hand: the gate names and
commands hardcoded, with layers derived from list position. `TestB9RealGateProfile` is
described as pinning "the real gate-profile contract" and does not read `regulus.json`.

What actually caught the F20.3 drift was the **neighbouring** conformance tests: the
shape file, the `docs/gauntlet.md` table and the README claim all disagreed with the new
configuration, and those failed. B9 itself would not have noticed, because its mirror is
maintained by hand and its layers come from position rather than from the config.

The honest statement is therefore narrower than "nothing checks it": something checks
it, three times over, and the checks are all copies. Adding two gates meant editing four
places by hand — `regulus.json`, `bench/gauntlet-shape.json`, `bench_shape.go` and
`docs/gauntlet.md` — with nothing verifying that the four agree with the real file.

Follow-up: make `realGateConfig` load `regulus.json` (the repo already resolves the root
that way in `internal/cli/manifest_test.go:22`) and delete the hand-written name and
command lists instead of maintaining them. One source of truth, not four copies.

### The adversarial catalogue's `expected` field is decorative

`internal/adversarial/catalog.json` carries an `expected` string per entry, and nothing
parses it: `loadCatalog` checks the id is non-empty and the seed exists, while the actual
expectation lives in a `switch` in the test. Two sources of truth, one of them dead. The
seeds themselves are real and executed, so this is a maintenance defect rather than a
detection hole.

### The reviewer has no ranking test

Lens selection, exact findings (`TODO`, `no test companion`, `violation(s)`, `risk=high`)
and refutation ordering are tested. There is no test that a *worse* finding outranks a
milder one, because the reviewer does not currently rank beyond the refute path.

## 4. What is advisory on purpose

`regulus review` is documented as informational: exit 0, never commits, never writes
receipts (`internal/cli/review.go:37`). That is not a detection weakness to fix — making
it blocking would change what "review" means for every existing workflow. The blocking
detection lives in the gates and in close, which is where it is tested.

## 5. Verification

```bash
bash scripts/verify-wsl.sh ./internal/sextant/
```

`ok regulus-ai/internal/sextant 0.202s` with the four new cases, and the positive case
closing against the same fixture that the forged ones are refused on.
