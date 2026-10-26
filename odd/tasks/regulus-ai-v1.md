# Regulus AI v1 — vertical slice of the three loops

Founding feature: build Regulus AI v1 as a vertical slice — Gauntlet,
Discipline and Evolution, thin but end-to-end — as a personal product, native
to OpenCode. This document is the spec subagents read; handoffs reference it
by task and spec, never by paraphrase. Read down to `## Log` before working.

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

## Tasks

| ID | Spec | Work | Route | Commit |
|---|---|---|---|---|
| T1 (M0) | S1–S4 | Founding documents: PRD, architecture, README, this document, repo skeleton | inline (parent) | initial commit of this repository |
| T2 (M1) | S1, S3 | Gauntlet spine: spec contract, layered gate runner, receipts, first guardian agent | decide at task start (writer-rule evaluation) | — |
| T3 (M2) | S1 | Discipline spine: ODD-lite workflow (sizing, feature docs, delegation by reason), session memory | decide at task start | — |
| T4 (M3) | S2 | Evolution spine: reflect, per-domain lesson library, adoption with evidence, budget ledger | decide at task start | — |
| T5 (M4) | S1–S4 | Dogfood: build one real Regulus feature with Regulus; receipts prove all three loops end-to-end | decide at task start | — |

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
