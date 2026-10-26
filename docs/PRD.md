# Regulus AI — Product Requirements Document

Status: founding definition (M0) · Audience: the builder (personal product first) · Scope: v1 vertical slice

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

## 4. v1 milestones (vertical slice)

| ID | Milestone | Contents |
|---|---|---|
| M0 | Foundation | Repo, PRD, architecture, ODD feature document, version-gate decision |
| M1 | Gauntlet spine | Spec contract format, layered gate runner, receipts, first guardian agent |
| M2 | Discipline spine | ODD-lite workflow agents/skills, session memory |
| M3 | Evolution spine | reflect command, lesson library, adoption with evidence, budget ledger |
| M4 | Dogfood | Build one real Regulus feature using Regulus; receipts prove all three loops end-to-end |

## 5. Success criteria (v1)

- The M4 dogfood task completes with: acceptance criteria approved by the
  human before implementation; all gates green with receipts; at least one
  reflection-produced lesson admitted with evidence.
- The human never opens a source diff during M4.

## 6. Risks

| Risk | Mitigation |
|---|---|
| OpenCode majors break plugin APIs (V2 already broke V1) | Version gating at the plugin boundary (Gentle-AI's discipline); core speaks only the Regulus-owned contract |
| "Not reading code" becomes faith without rigor | Mutation testing and CRAP floors are mandatory gates, not optional |
| Self-modification degrades the harness | Adoption requires evidence + guardian validation + budget caps |
| Solo-dev scope creep across three loops | Vertical-slice discipline: each milestone ships thin but end-to-end |
