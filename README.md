# Regulus AI

**An OpenCode-native, self-improving engineering harness: a guardian gauntlet over the code, discipline over the workflow, and evidence-driven evolution of the harness itself.**

> Status: founding definition complete (F0). Phased roadmap to the complete v1 in the PRD. No code yet.

Regulus AI fuses three proven ideas into one harness for [OpenCode](https://opencode.ai) V2:

- **Runtime discipline** (inspired by [Gentle-AI](https://github.com/Gentleman-Programming/gentle-ai)): task sizing, feature documents with verbatim specs, delegation for a reason — never for size — and receipts for every claim.
- **Recursive harnessing** (inspired by [Harness²](https://github.com/google-research/harness2), Google Research): the harness treats its own components (prompts, guardrails, memory, skills, agents) as an editable surface and improves them from execution evidence, under an explicit budget.
- **The right not to read the code** (Uncle Bob's gauntlet): the human approves acceptance criteria and audits evidence receipts — never line-by-line diffs. Read-only guardian agents verify that the code meets the spec contract through layered deterministic gates (typecheck → lint → build → tests → mutation testing → CRAP analysis).

## Documentation

| Document | Contents |
|---|---|
| [PRD](docs/PRD.md) | Problem, product definition, requirements per loop, phased roadmap (F1–F5), risks |
| [Architecture](docs/architecture.md) | Go core + TS plugin boundary, state model, receipts, gates, evolution flow |
| [ODD feature document](odd/tasks/regulus-ai-v1.md) | Verbatim specs, task list, decision log for v1 |

## Name

*Regulus* — α Leonis, the brightest star of Leo, "the heart of the lion"; Latin for "little king".
