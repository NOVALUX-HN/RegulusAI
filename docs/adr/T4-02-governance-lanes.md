# ADR T4-02 — Three lanes: assessment advises, policy gates, humans approve

Date: 2026-10-30 · Status: accepted · Phase 4 (Decision Intelligence)

## Context

A single "approve" path conflates three different acts: judging quality,
enforcing rules, and taking responsibility. If one mechanism does all
three, either the machine approves what it should not, or the human
rubber-stamps what the machine already decided.

## Decision

`decide resolve` separates the lanes. Assessment (advisory scores +
findings) never moves state. Policy gates are mechanical and absolute:
zero blocking findings for ACCEPT, recorded human approval
(`--approve-by "nick: reason"`) for critical decisions and supersedes,
scope-obsolete blocks unless atomically superseded, rationale required
for REJECT and INCONCLUSIVE. Refusals persist nothing; terminal states
never transition; every transition appends who, when, and why. Abstention
(INCONCLUSIVE) is always available and is the mandated answer when
evidence does not resolve.

## Consequences

- No autonomous approval exists anywhere: the closest path to ACCEPT
  runs through evidence, policy, and (for critical) a named human.
- `decide show` revalidates state/history integrity on read, so
  hand-edited decisions are caught, not trusted.
- Supersede is atomic with the replacing accept: no double-decided
  scopes, no close-order deadlock.
