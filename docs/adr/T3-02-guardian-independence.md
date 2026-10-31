# ADR T3-02 — Guardian evaluations are recorded, never authoritative

Date: 2026-10-30 · Status: accepted · Phase 3 (Gauntlet)

## Context

Guardian assessments lived in chat: no identity binding, no schema, no
record, no way to tell agreement from override. An evaluator could be the
implementer in disguise, and a semantic opinion could silently outrank a
deterministic receipt.

## Decision

The core holds the guardian registry (six read-only specialists) and the
only write path for assessments: validated `gate evaluate` appends to an
`evaluations.jsonl` log per task. Crew roles are rejected as evaluators;
unknown guardians, unparseable verdicts, empty notes and dangling evidence
refs are rejected. Consolidation (`gate verdict`) derives the verdict from
receipts alone (FAIL > BLOCKED > INCONCLUSIVE > PASS) and flags each
evaluation AGREE/DISPUTE. Reported rates are measured proxies —
disagreement, inconclusive counts, manifest-linked completeness — because
true FP/FN rates require an oracle we refuse to invent.

## Consequences

- Evaluation is read-only by construction (test pins receipts/charts
  untouched; only the log is appended).
- A dispute is a first-class signal for humans, never a veto over facts.
- New guardians require registry + tests, not just a markdown file.
