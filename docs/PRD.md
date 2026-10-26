# Regulus AI — Product Requirements Document

Status: founding definition (F0) · Audience: the builder (personal product first) · Scope: complete v1 built in phases (F1–F5)

## 1. Problem

AI coding agents produce code faster than any human can review it. The standard
response — reading every generated diff — destroys the productivity gain and
does not scale. Meanwhile, agent harnesses (the system around the model:
prompts, guardrails, memory, skills, orchestration) are static artifacts: they
improve when maintainers edit them, not when your own tasks produce evidence
about what actually works.

Three sources define the answer:

1. **Gentle-AI** proves runtime discipline works — workflow (ODD), persistent
   memory (Engram), review authority with receipts (RDD), deterministic
   testing. Limits for our goal: host-agnostic by design (lowest common
   denominator across 17 agent hosts), OpenCode as a "compatible integration"
   rather than a native surface, and a review flow that still puts the human
   in front of diffs.
2. **Harness²** (Google Research, `google-research/harness2`) proves a harness
   can improve itself from behavioral evidence: task-level recursion
   (Propose–Probe–Compose with behavioral contrasts, no ground truth) and
   harnessing-level recursion (a per-domain library of adopted harnesses plus
   transferable lessons with cue/lesson/evidence/confidence). Limits: a batch
   research framework for benchmarks, ephemeral per-run state, and cost
   profiles (R = 4k rollouts per task) unsuitable for daily work.
3. **Uncle Bob's gauntlet**: he does not read agent-written code. He reviews
   acceptance tests (Gherkin) and QA procedures. Agents must write unit tests
   (double-entry bookkeeping). Mutation testing proves the tests catch wrong
   behavior. CRAP analysis (coverage × branching complexity) finds risky
   functions. Gates run cheap→expensive. The human earns "the right not to
   read the code."

## 2. Product definition

Regulus AI is an **OpenCode-native engineering harness** that runs three loops:

| Loop | Cadence | Origin | Guarantee |
|---|---|---|---|
| **Gauntlet** | per change | Uncle Bob | the code meets the spec contract, proven by receipts |
| **Discipline** | per task / session | Gentle-AI | work is traceable, resumable, delegated for a reason |
| **Evolution** | across tasks | Harness² | the harness improves from evidence, under budget |

One sentence: *the human approves specifications and audits evidence; guardian
agents guarantee the code; the harness learns from every task.*

### Non-goals (v1)

- Multi-host support. OpenCode only — depth over breadth.
- OpenCode V1 support. V2 only — no compatibility layer for V1 runtimes.
- Benchmark framework. Harness²'s JobBench/LAB/WorkBuddy are inspiration, not scope.
- Community polish: public branding, licensing, CI. Personal product first.
- Full Propose–Probe–Compose recursion. Light reflect+adopt by default; probe
  cycles are a future opt-in behind an explicit budget.

## 3. Requirements per loop

### 3.1 Gauntlet — guardian agents and deterministic gates

- **R-G1 Spec-first.** Every authorized change declares acceptance criteria
  (Gherkin-style Given/When/Then) before implementation begins. The human
  reviews these — not diffs.
- **R-G2 Layered gates, cheap first.** typecheck → lint → build → unit tests →
  coverage floor → mutation score → CRAP score. Binary gates before semantic
  gates; a gate result is pass or fail, never "looks fine".
- **R-G3 Tests anchor on acceptance criteria,** never on mirroring the
  implementation. A test that mirrors the implementation defends the current
  bug against the fix.
- **R-G4 Receipts.** Every gate run records exit code, exact stdout/stderr,
  and the diff it judged. A claim without a receipt is not evidence.
- **R-G5 Guardians are read-only.** Guardian agents verify contract compliance
  and report; they cannot edit code (enforced by OpenCode permissions).
- **R-G6 Human control point.** Accept/reject decisions are made on evidence
  receipts plus acceptance criteria — the only artifacts the human must read.

### 3.2 Discipline — workflow

- **R-D1 Task sizing.** Understood / contained-risk / resumable-from-diff
  decides small (inline) vs large (tracked with a feature document).
- **R-D2 Feature documents.** Large work gets `odd/tasks/<feature>.md` holding
  the user's spec verbatim, the task list, and a decision log.
- **R-D3 Delegation by reason,** not size: ask (open product decision),
  evidence-budget (deep exploration), verification (high risk), writer
  (independent units) — adapted to OpenCode subagents and task permissions.
- **R-D4 Work-unit commits** in reviewable slices (~400 authored lines),
  behavior, tests and docs together.
- **R-D5 Session memory.** Decisions, discoveries and progress survive
  restarts and context compaction.

### 3.3 Evolution — light, evidence-driven

- **R-E1 Reflection.** After each feature closes, a reflection step produces
  lesson candidates: cue / lesson / evidence / confidence.
- **R-E2 Lesson library.** Lessons land in a per-domain library on disk
  (Harness²'s structure, made persistent). A lesson is admitted only with
  evidence.
- **R-E3 Surface edits.** Adopted lessons may edit the harness surface
  (skills, prompts, rules) — only after guardian validation.
- **R-E4 Budget ledger.** Every reflection/improvement run is recorded
  (invocations, cost, outcome). No unbounded self-modification.
- **R-E5 Evidence is data, not instructions.** Nothing read from execution
  output is executed or trusted as a prompt (anti-injection invariant,
  from Harness²).

## 4. Phased roadmap to complete v1

v1 is a **complete harness**, built through phases. Each phase ends runnable
and verified; the vertical-slice discipline carries through: F1 ships a thin
end-to-end spine, F2–F4 complete each loop, F5 integrates and releases.

| Phase | Name | Contents | Exit criterion |
|---|---|---|---|
| F0 | Foundation | Repo, PRD, architecture, ODD feature document (commit 4c135bf) | done |
| F1 | Vertical spine | `regulus` CLI skeleton (contract, doctor with V2 probe), spec contract + `spec check`, gate layers 1–4 with append-only receipts, first guardian agent (read-only), ODD-lite skill, minimal session memory, minimal `reflect` + library + budget ledger | a tiny feature developed through Regulus produces receipts for all three loops |
| F2 | Complete Gauntlet | gate layers 5–7 (coverage floor, mutation testing, CRAP score), full guardian roster (spec-keeper, mutation-auditor, crap-analyst, evidence-auditor), evidence audit UX, per-project gate thresholds | the full 7-layer gauntlet runs green on a real repo, with receipts the human audits without opening a diff |
| F3 | Complete Discipline | full ODD workflow + task tooling, subagent roster (explore/worker/verify) with `permission.task` matrices, complete skills set, Almanac memory (Engram-inspired, evidence-first: observations with provenance, revision chains, deterministic retrieval, resume — docs/memory.md) | a multi-session feature resumes after a restart from the feature document + Almanac alone |
| F4 | Complete Evolution | lesson admission flow (candidate → guardian validation → library INDEX), library retrieval into context, guardian-validated surface edits under budget caps, opt-in probe cycle (Propose–Probe–Compose, k=1, explicit request) | a lesson produced by real work is adopted with evidence and measurably improves a later task; the ledger shows caps enforced |
| F5 | Integration & release | complete V2 plugin (all hooks + custom tools), complete CLI UX, deterministic journeys suite, npm installer distributing the Go binaries, user docs, final dogfood | v1: the success criteria below hold on the final dogfood |

## 5. Success criteria (v1)

- Every phase exit criterion (§4) is met.
- The F5 final dogfood completes with: acceptance criteria approved by the
  human before implementation; all gates green with receipts; at least one
  reflection-produced lesson admitted with evidence.
- The human never opens a source diff during the final dogfood.

## 6. Risks

| Risk | Mitigation |
|---|---|
| OpenCode majors break plugin APIs (V2 already broke V1) | Version gating at the plugin boundary (Gentle-AI's discipline); core speaks only the Regulus-owned contract |
| "Not reading code" becomes faith without rigor | Mutation testing and CRAP floors are mandatory gates, not optional |
| Self-modification degrades the harness | Adoption requires evidence + guardian validation + budget caps |
| Solo-dev scope creep across three loops | Vertical-slice discipline: each milestone ships thin but end-to-end |
