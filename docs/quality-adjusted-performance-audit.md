# Quality-adjusted performance — baseline audit (F21.1 / NOV-154)

The phase asks for reproducible experiments proving that optimisations improve
**results**, not proxies. Before any experiment, this audit answers three questions with
evidence: what already exists, what is actually measured, and what is missing.

Read with `docs/quality-intelligence.md` (degradation over time) and
`docs/evolution-validation.md` (governed validation). This document does not repeat them.

## 1. What exists, and what it really does

| Surface | Location | What it actually does |
|---|---|---|
| Macizo suite | `internal/bench/bench.go:28` | `macizo-bench-v3`, 14 offline checks (B1–B14): behaviour, gate profile, token proxy, lesson context, decision scoring, miss rate. No network, no model. |
| Suite comparison | `internal/bench/bench.go:240` | `Compare` reports per-task rows and a verdict. **Was:** ADOPT on pass count alone. **Now:** refuses to decide below the repetition floor. |
| Baseline file | `bench/baseline.json`, `bench.BaselinePath` | A committed `Report`. It is a **single run with no reps, seed or environment**. |
| Quality windows | `internal/quality/quality.go:MinN=5`, `MinNMedian=5`, `MinNP90=10`, `MinNP95=20` | Below the floor a diagnosis *describes the window and claims no regression*. This is the discipline the phase wants, already built. |
| Quality evidence | `internal/quality/baseline.go:BuildBaseline/WriteBaselineEvidence` | Manifest + SHA-256, fail-closed on tamper, `EvidenceRef{path,sha256,git_head}`. |
| Token provenance | `internal/trace/hostusage.go` | `measured-host`, `measured-local`, `estimated`, `proxy`, `unknown`. Only `opencode` is verified measured-host end to end. |
| Measured token paths | `internal/bench/agenttasks.go:runOne`, `agentlab_run.go:labEvaluateCheck` | Set `measured-host` **only if** input or output tokens are non-zero, otherwise `unknown`. |
| Proxy token paths | `internal/trace/cost.go:EstimateLocalTokens`, `bench.go:Run` | `chars/4` over prompt and detail. Honest because labelled `proxy`, and B10/B11 pin the proxy as a golden rather than pretending it is host data. |
| Agent Lab | `internal/bench/agentlab.go:LabResult` | Runs headless agents on the same fixtures and splits, isolated workdir, jailed check environment, sha-pinned evidence. `CompareLabVerdict` picks the best rank and **shows cost without judging it**. |
| Agent corpus and holdout | `internal/bench/agentcorpus.go:1`, `agentcorpus_hard.go` | **44 tasks: 34 exploration + 10 holdout (22.7%)**, holdout ids tagged. |
| Split enforcement | `internal/bench/agenttasks.go:LoadAgentSplits` | Holdout ≥ 20%, no overlap with exploration, unknown ids refused. |
| Evolution governance | `internal/evolve/rollback.go:Adopt`, `governed_eval.go:PromoteGovernedCycle` | Needs evaluated outcomes, domain check, trust, named baseline, `Adoptable`, human approver; the governed path adds a linked validation plan with repetitions and a **sealed holdout that confirms and never re-decides**. |
| Quality gates | `internal/evolve/robust.go` | Functional ∧ Compliance ∧ NoRegression ∧ Adversarial ∧ Maintainable. |
| Journeys | `internal/journey/*_test.go` | Scripted CLI scenarios in temp dirs asserting exact receipts, exits and blocks. Wired as tests and CI, not as a production gate. |
| Reproducibility capture | `agent_evidence.go:AgentRunManifest`, `agentlab.go:LabConfig` | Already records suite version, model, host, reps, split, seeds, git head, budgets. |

**Holdout status: PRESENT, not absent.** F21.1's question is answered with evidence:
`agent-tasks-v3` carries a committed holdout corpus and the loader refuses overlap.

## 2. The gap this task closed

`bench/baseline.json` is one capture, and `Compare` returned **ADOPT** for one capture
against one candidate run. That is not a comparison; it is two anecdotes with a verdict
attached, and it is exactly the "promote without evidence" the phase excludes.

Closed in `internal/bench/bench.go`:

* `Report` now carries `Reps`, `Seed`, `Env{OS,Arch,Host}` and `FinishedAt`. The
  environment is captured without launching a process.
* `MinReps = 3`. Below it `Compare` returns **INCONCLUSIVE** and names the shortfall
  (`"needs 3 repetitions per side; baseline has 1, candidate has 1"`), never ADOPT.
* `RunN(surface, head, reps, seed)` folds repetitions: **a task passes only if it passed
  in every repetition**, duration is the mean, and quality-per-kproxy is recomputed after
  folding. A check that flips under repetition is not a check.
* `regulus bench baseline --set` defaults to `--reps 3` and accepts `--seed`, so a newly
  recorded baseline is immediately comparable.

Tests: `TestCompareRefusesSingleRun`, `TestCompareRefusesMixedRepetition` (either side
uncontrolled is enough), `TestRunNFoldsRepetitions`, `TestCompareAcceptsControlledBaseline`.
The existing verdict fixtures now declare `Reps: MinReps`, because they test regression
and cost logic, not reproducibility.

## 3. What is deliberately still open

* **The committed baseline is a legacy single-run capture.** Comparisons against it now
  report INCONCLUSIVE. That is the correct answer, not a bug: it has to be re-recorded
  with `regulus bench baseline --set`. Regenerating it from this host would embed an
  environment no other host shares.
* **The macizo tasks are behaviour checks, not optimisation A/Bs.** They pin that the
  system still behaves; they do not measure whether recovery, knowledge or context
  changes improved anything. Building those is F21.3.
* **There is no joint quality-and-cost metric yet.** `QualityPerKProxy` is quality per
  proxy token and treats cost as reported-only. F21.2 owns the normalised
  quality-adjusted metric, and routing it through `context.Evaluate` (F19.4) is what makes
  a proxy-only saving `inconclusive` instead of an adoption.

## 4. The baseline protocol F21.2–F21.4 must follow

1. Both sides run `RunN` with `reps >= MinReps`; below that the answer is INCONCLUSIVE.
2. Same `SuiteVersion` on both sides: reports are comparable only within one version.
3. Record seed and environment with the numbers, or the numbers are not reproducible.
4. Tokens are labelled by provenance. `measured-host` requires real counts; `chars/4` is
   `proxy` and never presented as host usage.
5. Holdout is used to **confirm**, never to select. A result that only appears in the
   development corpus is not evidence.
6. Quality first: no promotion from a cost or proxy saving alone; `Evaluate` returns
   `adopt` only on a saving measured on both sides with essentials preserved and a named
   reversal.

## 5. Verification

```bash
bash scripts/verify-wsl.sh -count=1 ./internal/bench/ ./internal/cli/
regulus bench baseline            # the committed baseline, now visibly a single run
regulus bench baseline --set --reps 3
```
