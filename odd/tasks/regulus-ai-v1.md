# Regulus AI v1 — a complete harness built in phases

Founding feature: build Regulus AI v1 as a **complete harness** — Gauntlet,
Discipline and Evolution fully developed — as a personal product, native to
OpenCode V2. v1 is reached through phases: F1 ships a thin vertical spine,
F2–F4 complete each loop, F5 integrates and releases. This document is the
spec subagents read; handoffs reference it by task and spec, never by
paraphrase. Read down to `## Log` before working.

## Specs

- **S1**: "quiero crear un harness parecido a gentle ai que se use para
  opencode, pero con ciertas mejoras." — base:
  https://github.com/Gentleman-Programming/gentle-ai
- **S2**: "las mejoras que quiero implementar son las de harness2 … esto con
  el objetivo de llevarlo a un nivel completo." —
  https://github.com/google-research/harness2
- **S3**: "tambien quiero incluir las ideas del uncle Bob creador de clean
  code cuando menciono que no revisa el codigo que la IA genera, sino que
  tiene unos agentes encargados de velar por que el codigo sea de nivel."
- **S4**: founding decisions (user questionnaire, verbatim answers): v1
  scope = "Slice vertical de los 3 loops"; evolution = "Reflect + adopt con
  evidencia"; audience = "Producto personal"; core language request =
  "analisa tanto GO como Rust para ver cual de los dos es mejor" [sic], later
  resolved by the user selecting "Go core (recomendado)" after the analysis.
- **S5**: "todo debe ser para opencode v2, unicamente considerar este."
- **S6**: "Regulus AI debe ser un harness completo desde la v1, asi que
  debemos crear varias fases para ir llevando todo a esa v1 completa e
  increible"

## Acceptance

### Scenario: doctor refuses non-V2 runtimes

- Given `opencode --version` reports V1, fails, or prints unparseable output
- When `regulus doctor` runs
- Then the exit code is 1 and the refusal names the failed condition and the V2 requirement

### Scenario: doctor accepts V2 runtimes

- Given `opencode --version` reports a 2.x version
- When `regulus doctor` runs
- Then the exit code is 0 and the output reports supported (V2)

### Scenario: spec check enforces the acceptance contract

- Given a feature document
- When `regulus spec check` runs on it
- Then the exit code is 0 with the scenario count, or 1 listing every violation with its line number

### Scenario: gates stop at the first failure and write receipts

- Given a fixture project with gates configured and one failing gate
- When `regulus gate run` executes
- Then the exit code is 1, no later gate runs, and an append-only receipt records the failure

### Scenario: reflect admits only evidenced lessons

- Given a closed feature with receipts on disk
- When `regulus reflect` runs
- Then lesson candidates carry cue, lesson, evidence and confidence, and the budget ledger records the run

## Tasks

| ID | Spec | Work | Route | Commit |
|---|---|---|---|---|
| T1 (F0) | S1–S6 | Foundation: PRD, architecture, README, this document, repo skeleton | inline (parent) | 4c135bf |
| T2 (F1) | S1–S6 | Vertical spine end-to-end: CLI skeleton (contract, doctor with V2 probe), spec contract + `spec check`, gate layers 1–4 with append-only receipts, first guardian agent, ODD-lite skill, minimal session memory, minimal reflect + library + budget | decide at task start (writer-rule evaluation) | — |
| T3 (F2) | S1, S3, S5 | Complete Gauntlet: gate layers 5–7 (coverage floor, mutation testing, CRAP), full guardian roster, evidence audit UX, per-project gate thresholds | decide at task start | — |
| T4 (F3) | S1 | Complete Discipline: full ODD workflow + task tooling, subagent roster with permission.task matrices, complete skills, structured session memory + cross-restart resume | decide at task start | — |
| T5 (F4) | S2 | Complete Evolution: lesson admission flow, library retrieval into context, guardian-validated surface edits under budget caps, opt-in probe cycle (k=1) | decide at task start | — |
| T6 (F5) | S1–S6 | Integration & v1 release: complete V2 plugin (all hooks + custom tools), complete CLI UX, journeys suite, npm installer, user docs, final dogfood | decide at task start | — |

## Log

- **L1** (original request, verbatim): "quiero crear un harness parecido a
  gentle ai que se use para opencode, pero con ciertas mejoras. este es el
  repo de gentle ai: https://github.com/Gentleman-Programming/gentle-ai las
  mejoras que quiero implementar son las de harness2:
  https://github.com/google-research/harness2 esto con el objetivo de
  llevarlo a un nivel completo. tambien quiero incluir las ideas del uncle
  Bob creador de clean code cuando menciono que no revisa el codigo que la IA
  genera, sino que tiene unos agentes encargados de velar por que el codigo
  sea de nivel. analiza estos puntos y vayamos definiendo como crearemos esta
  nueva herramienta llamada Regulus AI"
- **L2** (questionnaire decisions): "Slice vertical de los 3 loops" ·
  "Reflect + adopt con evidencia" · "Producto personal" · custom language
  answer "analiza tanto GO como Rust para ver cual de los dos es mejor".
- **L3** (Go-vs-Rust analysis, resolved): facts weighed — Go 1.27 stabilized
  `encoding/json/v2`; OpenCode V2 broke its V1 plugin API (surface churn);
  architectural decision that the TS plugin owns the typed OpenCode boundary
  while the core speaks the Regulus-owned contract. User selected: "Go core
  (recomendado)".
- **L4** (M0 evidence): PRD, architecture, README and this document committed
  in the initial commit of this repository. Design baselines: 8-component
  harness surface (Harness²), receipts as append-only JSON, gate layering
  cheap→expensive (Uncle Bob), budget-gated evolution.
- **L5** (requirement change, verbatim): "1. todo debe ser para opencode v2,
  unicamente considerar este. 2. Regulus AI debe ser un harness completo
  desde la v1, asi que debemos crear varias fases para ir llevando todo a
  esa v1 completa e increible"
- **L6** (roadmap revision): v1 redefined as a complete harness reached
  through phases F1–F5 (F1 = thin vertical spine of the three loops — the
  former M1–M4 compressed; F2–F4 complete each loop; F5 integrates and
  releases). OpenCode V2 only — no V1 compatibility layer. Evolution default
  unchanged (reflect + adopt with evidence, per L2); Propose–Probe–Compose
  enters in F4 as an opt-in k=1 probe cycle behind the budget ledger. PRD §4
  and this document rewritten in the roadmap revision commit.
- **L7** (F1/WU1 evidence): Go module `regulus-ai` (go 1.27); CLI skeleton
  with help/--version/contract/doctor; doctor probes `opencode --version`
  with a 15s budget and refuses V1/missing/unparseable/failing runtimes,
  including the runtime's own output in the refusal — OpenCode V2 only
  (S5). RED observed (14 failing assertions on stubs), then GREEN; go vet
  and gofmt clean; smoke on the real environment took the refusal path
  (the local opencode.exe is currently broken on this machine). Commit:
  a9cc94e.
- **L8** (F1/WU2 evidence): spec contract live — `regulus spec check`
  validates the Acceptance section (Given/When/Then, And-continuation,
  line-numbered violations, all at once). RED observed (15 failing
  assertions: 11 spec package + 4 CLI), then GREEN. This feature document
  now declares F1's acceptance criteria and passes `regulus spec check`
  (dogfood). Commit: this work unit.
