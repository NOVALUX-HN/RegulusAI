# Regulus Decision Intelligence (Phase 4)

Regulus evaluates technical decisions; it never makes them and never
auto-approves them. A decision is a structured contract, assessed against
evidence and policy, resolved only through governance. Decisions:
`docs/adr/T4-01`, `T4-02`.

## 1. The contract

`regulus decide propose --file decision.json` stores a PROPOSED decision
under `.regulus/decisions/`. Required: `problem`, `proposal`, `scope`,
`confidence` (low|medium|high), `reversibility` (easy|moderate|hard).
Structured: `alternatives[]` (name + trade-off), `tradeoffs[]`,
`constraints[]` (`name: handling`), `risks[]` (risk with mitigation),
`consequences`, `evidence[]` refs, `artifacts[]`, `commits[]`
(hex), `task`, `related[]` (`dec:` ids), `critical`, `rollback`.
Propose validates shape only — substance is the assessment's job, so
`MISSING_ALTERNATIVES` and friends stay detectable instead of being
rejected at the door.

Evidence refs: `rec:<id>`, `receipt:<path>`, `chart:<name>`, `dec:<id>`,
`path:<p>`, `task:<t>`, or bare (auto-detected in that order).

## 2. The pipeline

`regulus decide assess <id> [--json]` runs PERCEPTION → EVIDENCE →
POLICY → ASSESSMENT → DECISION and prints every stage:

- **Perception**: the problem stated and grounded (or not).
- **Evidence**: each ref resolved or not; linked task verdict included.
- **Policy**: evidence-ok, blocking findings, critical flag, obsolete
  scope holder, task verdict.
- **Assessment**: ten criteria (problem exists, problem↔solution fit,
  alternatives, constraints, risks, evidence, architectural consistency,
  complexity, impact, reversibility), each pass|fail|abstain with a note.
- **Decision**: ACCEPT, REJECT, or INCONCLUSIVE (abstention) with rationale.

Checks are documented mechanical proxies (shared words, named costs,
declared mitigations), not simulated judgment. What the core cannot
verify becomes a finding or an abstention — never a confident verdict.

## 3. Findings

Blocking: `MISSING_ALTERNATIVES`, `UNMITIGATED_RISK`,
`INSUFFICIENT_EVIDENCE`, `CONTRADICTION` (including new Go dependencies
vs the zero-dep invariant), `OBSOLETE` (accepted same-scope decision),
`HIDDEN_DEPENDENCY` (artifact resolves to nothing).
Warning: `UNSUPPORTED_ASSUMPTION` (absolute language, nothing behind it).

## 4. Governance

Assessment advises, policy gates, humans approve:

- ACCEPT needs zero blocking findings; critical decisions additionally
  need `--approve-by "nick: reason"`, recorded in history.
- `--supersede OLD` atomically replaces an ACCEPTED same-scope decision
  (also needs human approval) — no scope is ever decided twice, and no
  close-order deadlock: the replacement accepts and the old decision
  supersedes in one operation.
- `--reject` and `--inconclusive` need `--note` (the rationale).
- Refusals change nothing; terminal states never transition.
- `decide show` revalidates integrity (state vs history chain) on every
  read; `decide list [--state S]` shows the trail.

Abstention is first-class: with no resolving evidence the system answers
INCONCLUSIVE and says why, instead of guessing.
