# Progress signals and stagnation patterns — F17.1 audit (NOV-138)

Baseline: `develop`, audit performed at `HEAD 6f92f9a`, clean tree. Audit-only
artifact: every claim cites code read in this checkout. The gaps listed are what
F17.2–F17.4 may act on, and only if they survive their own evidence check.

Scope reminder from the parent: do not duplicate NOV-100 (`internal/recovery`),
NOV-108 (`internal/failure`), LoopGuard or the F15 executor. Do not kill processes
indiscriminately, do not turn detection into authorization, do not force an
intervention when the host only allows observation.

## 1. What already exists (do not rebuild)

| Surface | Location | What it provides |
|---|---|---|
| Durable recovery | `internal/recovery/recovery.go:28` | nine states, `IsTerminal`, `legalTransitions`, `CheckTransition` |
| Checkpoints / resume | `internal/recovery/transitions.go:37` | `RecordCheckpoint`, `ResumePlan{Skip,Run,Key}`, `Resume` |
| Retries and circuit | `internal/recovery/policy.go:20` | `RetryPolicy` 3 retries, 1s×2 capped at 30s, 60s timeout, 3 failures → 5 min cooldown |
| Exactly-once effects | `internal/recovery/finalize.go:12` | `ApplyEffect(key, fn)`, LIFO `RollbackAll` |
| Error classification | `internal/recovery/classify.go:24` | 9 classes, `Retryable` refuses permanent/auth/cancelled/partial |
| Failure intelligence | `internal/failure/failure.go:29` | 10 classes, 7 decisions, `Policy{MaxReplans 2}` |
| Decision record | `internal/failure/failure.go:163` | cause, class, evidence, decision, reason, discarded, backoff |
| Exec event log | `internal/observability/exec.go:140` | `run/task/agent.start|end`, `tool.call|result`, `retry`, `timeout`, `gate`, `decision`, `error` |
| Span attributes | `internal/observability/trace.go:43` | `tool`, `exit_code`, `duration_ms`, `input_tokens`, `output_tokens` |
| LoopGuard | `.opencode/lib/domain-loopguard.ts:10` | `LOOPGUARD_THRESHOLD = 3`, same-signature streak per session |
| Bench hot-loop alerts | `internal/bench/diag.go:62` | `LoopRepeats 5`, `DupRepeats 3`, `RetryBudget 3`, `SlowStageMS 10000` |

**LoopGuard is real but lives only in the TypeScript plugin.** There is no
`internal/**/loop*` Go equivalent. Its own header says the nudge "never stops the
agent by itself (no throw, no block): it is advisory".

## 2. Signal matrix

Observable per-signal, with the limitation that decides whether it can feed a
classifier.

| Signal | Source | Granularity | Observable | Limitation |
|---|---|---|---|---|
| Tool invoked | `ExecEvent.Payload["tool"]` | per call | yes | payload is free-form; no schema pin |
| Tool outcome | `ExecEvent.Status` | per call | yes | four values, no exit code |
| Exit code | `Span.Attributes["exit_code"]` | per span | yes | spans only; exec events do not carry it |
| Duration | `ExecEvent.DurationMS`, `Span.duration_ms` | per call | yes | wall clock only; no progress notion |
| Tokens | `trace.Entry`, `CostByTask`, metrics, budgets; `bench` `TokensIn/Out` | per agent run | yes | **deliberately absent from spans**: the redaction deny-list scrubs any key containing `token`, so they are excluded rather than silently destroyed (`observability/project.go:108-113`); and absent entirely when the host does not expose them |
| Gate verdict | `ExecEvent` `gate` type + `receipts` | per gate | yes | task-scoped, not call-scoped |
| Retries | `Checkpoint.Retries`, `Policy.RetriesUsed` | per checkpoint | yes | budget only, no per-strategy view |
| Circuit state | `Checkpoint.Circuit{failures, opened_at}` | per host | yes | host-scoped, 5 min, not task-scoped |
| Decision | `DecisionRecord.Decision` | per failure | yes | proposed only; never measured |
| Files touched | `trace` touched paths | per session | yes | not linked to a single attempt |
| Repeated failure streak | LoopGuard (TS) | per session | yes | advisory, TS-only, same session only |
| Repeated tool call | `bench/diag_detect.go:98` | post-hoc | yes | analysis of a finished run, not live |
| Slow tool | `bench/diag.go:57` `SlowToolMS 2000` | per tool | yes | threshold on a finished run |
| Outlier sample bars | `bench/diag.go:37-43` `DiagMinMedianN 5`, `P90N 10`, `P95N 20` | per run | yes | minimum sample before a percentile is claimed |
| Payload caps | `observability/exec.go` `MaxExecPayloadKeys 64`, bytes 8192 | per event | yes | a signal that overflows the cap cannot be read back |

## 3. Stagnation handling today: fragmented, no unified classifier

There is **no `Progress` / `Stagnation` / `RepeatedFailure` type** in `internal/`.
What exists is a set of disconnected mechanisms plus several places that already
compute "no progress" implicitly:

**Explicit detectors.**

1. **LoopGuard** — 3 consecutive same-signature failures → one-line advisory nudge,
   never blocks. TypeScript only, advisory, kill switch `TE_LOOPGUARD`, capped at
   `LOOPGUARD_MAX_SESSIONS 200`.
2. **Bench diagnostics** — `LoopRepeats 5` (same tool in a row), `DupRepeats 3`
   (identical tool+args), `RetryBudget 3`, `SlowToolMS 2000`, `SlowStageMS 10000`,
   detected **after** a run finishes, plus `occurrenceAlerts` for any
   timeout/cancel/failure and `delegationAlerts` for a pending or non-succeeded
   delegation.
3. **Recovery retry budget** — `Retries > MaxRetries` (validated `0..10`) →
   failed; `CircuitOpen` refuses; `Resume` refuses on an exhausted budget or an
   open circuit.
4. **Failure escalation** — `pause_for_approval` / `isolate` / `fail_closed` when a
   budget is spent or evidence is missing; `MaxReplans 0..5`.

**Implicit "no progress" already computed elsewhere** — F17.2 must not rebuild these:

5. **`evolve`** groups failed executions and promotes them to `repeated-failures`
   at `MinSupport = 2` (`internal/evolve/observe.go:33`). That is already a
   repeated-failure classifier with an evidence bar.
6. **`bench` per-attempt `Progress`** callback (`internal/bench/agenttasks.go:160`)
   already reports per-attempt progress for the agent corpus.
7. **`sextant.CheckProgress`** (`internal/sextant/progress.go:60`) validates plan
   progress — duplicate task ids, unmet dependencies, done/blocked. It is
   document-level, not runtime, but it is the existing notion of "the plan has
   not moved".
8. **`ReconstructRun`** marks a run crashed or incomplete from the exec log, which
   is a post-hoc progress verdict.

Each answers a different question at a different level, and none of them says
"this task is stagnating". That is G1 — and the barrier is now narrower than it
first looked: three of the eight already classify something close.

## 4. Gap G1 — no unified progress classification

The phase asks for `Progress` / `Stagnation` / `RepeatedFailure` **at the
appropriate level**. Today the nearest proxies live at three different levels that
cannot see each other: a session-scoped TS advisory, a post-hoc bench analysis, and
per-checkpoint retry counters.

A classifier must therefore be explicit about **scope**, and the real scopes are
not attempt/task/exploration. They are:

* **run** — `RunRecord` (one recovery run, `internal/recovery/recovery.go:125`);
* **task + agent** — `Checkpoint.Key` (`recovery.go:113`);
* **host** — the circuit (`Checkpoint.Circuit{failures, opened_at}`);
* **mission** — `Signal.MissionID` / `DecisionRecord.MissionID` (`internal/failure`).

`Attempt` is an integer counter inside a checkpoint, not a state scope, and
"exploration" is a bench corpus split (`agenttasks.go`), not a recovery or failure
scope. Classifying "the same three failures" as repeated failure or as legitimate
exploration therefore has to be expressed against the checkpoint or the run,
because those are the only scopes that survive across attempts. Nothing in the
code states that distinction today.

## 5. Gap G2 — recovery outcomes carry no before/after evidence

`Checkpoint` records `History[]{attempt, state, class, detail, at}`, `Retries`,
`Applied`, `Compensate`, `Circuit`. `DecisionRecord` records cause, class, evidence,
decision, reason, discarded and backoff as **proposed** values.

Verified: neither package has any `Before`, `After`, `Efficacy`, `Cost` or `Delta`
field. So the system can say what it intended to do and that it finished, but not
whether the action worked, what it cost, or whether a rollback beat the escalation.
That is G2, and it is what F17.3 must add.

## 6. Gap G3 — no false-positive guard

The parent's DoD requires false-positive and false-negative fixtures. None exist.
There is no test proving that a legitimately long or exploratory run **is not**
flagged by a 3-failure streak, a 5-same-tool window or a slow stage.

The project already admits the limit: `docs/gauntlet.md:101` says true FP/FN rates
need an oracle. A fixture cannot prove a rate, but it can and must prove the
boundary cases: a slow-but-progressing run, and a genuinely stuck one.

## 7. What each follow-up task may do

| Task | May do | Evidence it must produce |
|---|---|---|
| F17.2 (NOV-139) | One deterministic classifier with scope, budgets, cooldown and escalation criteria | Fixtures for each class **and** for the long-but-legitimate case that must not flag |
| F17.3 (NOV-140) | Record before/after outcome, cost, retries and rollback-vs-escalation on recovery | A recovery run whose record shows the measured delta, not a proposed command |
| F17.4 (NOV-141) | Prove idempotency, cancellation, rollback and host-aware behaviour | Tests that a second intervention cannot duplicate the first, and that observation-only hosts are never force-intervened |

## 8. Explicit non-gaps (do not spend effort here)

* The durable state machine and its legal transitions — correct and tested.
* `ApplyEffect` exactly-once and LIFO rollback — correct.
* Error classification precedence in `failure/classify.go` — correct and tested.
* The retry/circuit knobs in `RetryPolicy` — correct defaults, validated range.
* LoopGuard's advisory-only stance — intentional; F17 must not turn it into a
  blocker, and S9 forbids converting detection into authorization.

## 9. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/recovery/ ./internal/failure/ ./internal/observability/ ./internal/bench/
```

This document changes no code: it is the deliverable F17.1 asks for ("definir
señales observables de progreso y patrones de estancamiento con alcance y
limitaciones").
