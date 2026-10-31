# Regulus Sextant — Engineering Discipline (Phase 2)

The Sextant workflow (authorize, chart, build, prove, learn) with depth
proportional to risk. Machine-checkable in the core (`regulus task ...`,
`internal/sextant`); the skill and agents carry the same rules in prose.
Decisions: `docs/adr/T2-01`, `T2-02`.

## 1. Classify first

`regulus task classify [--code] [--destructive] [--security] [--contracts]
[--concurrency] [--delivery] [--unclear] [--units N] [--deps N]`

| Class | When | Process |
|---|---|---|
| TRIVIAL | read-only, docs, one obvious rule, no risk markers | inline, never delegates; closes on stated acceptance |
| STANDARD | one code unit | Chart + Contract, gauntlet per step, core close |
| COMPLEX | several units or dependencies | scout-first when ground is unknown, one unit per worker |
| CRITICAL | destructive, security, consumed contracts, concurrency, delivery, or unclear risk | all of the above + independent verifier receipts |

**No evasion:** any risk marker forces CRITICAL, however small the change.
`TRIVIAL` with a risk marker is rejected, not delegated.

## 2. The chart carries the contract

`chart init` scaffolds `Class:`, a `## Contract` section (objective, scope,
constraints, acceptance, artifacts, dependencies, risk, verification,
completion, verifier when critical), acceptance scenarios, and a
`## Tasks` table (`ID | Spec | Work | Route | Depends | Status`).

- `Acceptance:` names the scenarios it proves — every name must exist
  under `## Acceptance` (or be `all` with at least one scenario).
- `Risk:` must agree with `Class:`.
- Unknown contract keys are rejected: contracts stay machine-readable.

Legacy charts without `## Contract` keep working (`chart list`, `spec
check` untouched); adding the section opts them into validation.

## 3. Delegation is governed

`regulus task check <chart>` validates every row: known crew routes only
(`inline`, `@regulus-scout/worker/verifier/proposer`), guardians never
take units, delegated units carry their Spec context, TRIVIAL never
delegates, and CRITICAL work requires a `@regulus-verifier` task in the
chart.

## 4. Progress is explicit

`regulus task status <chart>` shows per-task state, blockers, and
violations: unique ids, known and acyclic dependencies, doing/done only
after dependencies close, `blocked: <reason>` always names the blocker,
and `done` without a CLOSED record is flagged — self-report closes
nothing. Rescopes append `REPLAN: <what changed> — <why>` to the Log;
bare `REPLAN:` lines are flagged.

## 5. Completion is core-verified

`regulus task close <chart> <id>` checks the contract, the spec, the
task's receipts (hash-verified, all PASS), closed dependencies, and — for
CRITICAL — passing verifier evidence. On success it flips Status to done
and appends the CLOSED record; otherwise it lists every unmet condition
and changes nothing. Re-running close on closed work stays closed.
