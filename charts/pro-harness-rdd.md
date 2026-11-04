# pro-harness RDD — review with executable authority

Domain: harness

Design chart for Fase 5 (T7/T8): the review stops being a prompt role
and becomes an executable transaction — frozen candidate, independent
lenses, adversarial refuter, bounded correction budget, terminal burn,
human consent. Adapted from gentle-ai's review-integration contract;
cut to what this stack (Go core + OpenCode plugin) can enforce.
Read down to `## Log` before working.

Design decisions (locked here, not in code review):
- Lenses run through the model ONLY for high-risk candidates (the
  assess tier the gauntlet already computes); otherwise deterministic
  checks. Model lenses for everything would price every review like a
  feature; deterministic lenses for high-risk would miss what the
  gauntlet cannot specify.
- Four lenses, 4R order: risk, resilience, readability, reliability.
  The current guardians migrate into them (gate-verifier → reliability,
  reviewer → risk+readability, evidence-auditor → reliability); no
  guardian is deleted, they are re-homed.
- Correction budget: at most 2 correction rounds per lineage, then a
  human decision. Unbounded retry is how reviews become rubber stamps.
- Burn: an approval covers the frozen bytes only; any modification
  after approval invalidates it. Delivery still follows ordinary
  repository policy (commit/push/PR stay human).

## Acceptance

### Scenario: approval covers frozen bytes only

- Given an approved review for a candidate
- When the candidate's bytes change afterwards
- Then reusing the approval is refused, naming the lineage and the drift

### Scenario: high-risk candidates get model lenses

- Given a candidate the assess tier marks high-risk
- When the review runs
- Then the four lenses run through the model with the frozen request
  context, and their verdicts cite frozen line ranges

### Scenario: low-risk candidates get deterministic lenses

- Given a candidate below the high-risk tier
- When the review runs
- Then the lenses run without model calls and the receipt names which
  lens ran in which mode

### Scenario: the refuter is adversarial

- Given lens findings against a candidate
- When the refuter runs
- Then it attempts to disprove each finding from the frozen evidence,
  and only surviving findings block; pre-existing/base-only findings
  are follow-ups, never blockers

### Scenario: the correction budget binds

- Given a lineage that consumed 2 correction rounds
- When a third correction is attempted
- Then it is refused and a human decision is required; the lineage is
  never silently extended

### Scenario: consent and delivery stay separate

- Given an approved review
- When the session ends
- Then no commit, push, PR or release happened from the approval alone;
  delivery decisions name their own authorization

### Scenario: the old guardians survive as lenses

- Given the current guardians (gate-verifier, reviewer,
  evidence-auditor, crap-analyst, mutation-auditor, spec-keeper)
- When the migration lands
- Then every guardian's checks run inside a lens, none is deleted, and
  the gauntlet (`gate run`) behavior is unchanged

## Tasks

| ID | Spec | Work | Route | Commit |
|---|---|---|---|---|
| T6 | S1 | This chart (design lock + acceptance contract) | inline | — |
| T7.1 | S2,S3 | Core: freeze candidate by hash, lineage, correction budget ≤2, burn on drift | inline | — |
| T7.2 | S4,S5 | Core: refuter (pre-existing = follow-up), refusal past budget | inline | — |
| T8.1 | S2,S3 | Plugin: invoke the transaction, model lenses iff high-risk tier | inline | — |
| T8.2 | S6,S7 | Plugin: consent separation + guardian→lens migration, gauntlet unchanged | inline | — |

## Log

- **L1**: Fase 5 needs a design lock before code (highest-risk unit of
  pro-harness). Analysis: gentle-ai's review-integration/v2 gives
  guarantees where Regulus has discipline — frozen candidate, 4R
  lenses, adversarial refuter, bounded correction, terminal burn,
  consent/delivery split. Adapted, not copied: no provider bindings
  (single runtime: this core + its plugin), model lenses gated by the
  existing risk tier, correction budget fixed at 2. User authorization:
  "continua todos los pasos hasta finalizarlos".
