# Regulus AI

**An OpenCode-native, self-improving engineering harness: a guardian gauntlet over the code, discipline over the workflow, and evidence-driven evolution of the harness itself.**

> Status: founding definition complete (F0). Phased roadmap to the complete v1 in the PRD. No code yet.

Regulus AI fuses four proven ideas into one harness for [OpenCode](https://opencode.ai) V2:

- **The Sextant** — our evidence-first workflow (task sizing and feature documents adapted from open practice, [Gentle-AI](https://github.com/Gentleman-Programming/gentle-ai) credited): authorize, chart, build, prove, learn — every movement ends in a receipt, a sighting taken against the stars.
- **Recursive harnessing** (inspired by [Harness²](https://github.com/google-research/harness2), Google Research): the harness treats its own components (prompts, guardrails, memory, skills, agents) as an editable surface and improves them from execution evidence, under an explicit budget.
- **The right not to read the code** (Uncle Bob's gauntlet): the human approves acceptance criteria and audits evidence receipts — never line-by-line diffs. Read-only guardian agents verify that the code meets the spec contract through layered deterministic gates (typecheck → lint → build → tests → mutation testing → CRAP analysis).
- **The Necessity Ladder** (craft doctrine adapted from [Ponytail](https://github.com/DietrichGebert/ponytail), MIT): solve the whole problem with the least new code — reuse first, and never cut validation, security or accessibility; a sixth guardian, the reviewer, reads the code the change touches, not just the diff.

## Documentation

| Document | Contents |
|---|---|
| [PRD](docs/PRD.md) | Problem, product definition, requirements per loop, phased roadmap (F1–F5), risks |
| [Architecture](docs/architecture.md) | Go core + TS plugin boundary, state model, receipts, gates, evolution flow |
| [Chart](charts/regulus-ai-v1.md) — the v1 feature document | Verbatim specs, acceptance contract, task list, ship's log |

## Name

*Regulus* — α Leonis, the brightest star of Leo, "the heart of the lion"; Latin for "little king".
