# devin-agents — Regulus seleccionable en Devin como en OpenCode

User authorization: "la idea es que regulus salga como un agente seleccionable en devin para que pueda trabajar tal como trabaja aqui en opencode" + "Port completo" (2026-10-30). Hoy `.devin/agents/` solo trae worker+scout (2); `.opencode/agents/` tiene 11 (orchestrator `regulus.md` mode:primary + worker/scout/verifier/proposer + 6 guardians). Este chart porta el resto adaptado a Devin (frontmatter name/description/allowed-tools, MCP 12 tools + `regulus` por shell para lo que MCP no expone). Read down to `## Log` before working.

## Acceptance

### Scenario: regulus appears as a selectable agent

- Given the repo opened in Devin Desktop with `.devin/agents/regulus.md` present
- When the user opens the agent picker
- Then an agent named `regulus` is listed with the orchestrator description, driving the Sextant through the Crew

### Scenario: verifier and proposer complete the Crew

- Given the same repo state with `.devin/agents/regulus-verifier.md` and `.devin/agents/regulus-proposer.md` present
- When the orchestrator dispatches verification or harness self-improvement
- Then the verifier reproduces read-only via MCP/shell and the proposer authors candidates from `regulus evolve propose` shell output (MCP has no evolve tool)

### Scenario: six guardians verify read-only

- Given `.devin/agents/` contains the six guardian profiles
- When any guardian runs
- Then it never edits: no write/exec-edit tools, verification only via MCP read tools and read-only `regulus` shell commands

### Scenario: no OpenCode-isms leak into the Devin pack

- Given the completed `.devin/agents/` pack
- When the pack is grepped for OpenCode shapes
- Then there are no `@regulus-*` mentions, no `tools.regulus.*` references, and no `mode:`/`temperature:` frontmatter fields — only Devin `name/description/allowed-tools` (+ `permissions` where needed)

## Tasks

| ID | Spec | Work | Route |
|---|---|---|---|
| A1 | S1–S2 | `.devin/agents/regulus.md` (port de `.opencode/agents/regulus.md`) + `.devin/agents/regulus-verifier.md` (port con MCP/shell read-only) | @regulus-worker |
| A2 | S3–S4 | `.devin/agents/regulus-proposer.md` + 6 guardians (ports read-only adaptados, sin OpenCode-isms) | @regulus-worker |

## Log

- **L1**: Chart opened 2026-10-30 from authorized full-port decision. Investigation: MCP expone 12 tools (`internal/cli/mcp.go`), faltan `evolve_propose/probe_contrast/probe_compose/skills_list` → fallback shell `regulus ...` vía `Exec`.
- **L2**: A1+A2 CLOSED + PROVE — 9 ficheros nuevos en `.devin/agents/` (orchestrator `regulus.md` + verifier + proposer + 6 guardians), `^name:` 11/11, OpenCode-isms 0 (`@regulus-`/`tools.regulus`/`mode:`/`temperature:`). `spec_check` 4 OK (A1+A2+lead). Sin cambios Go (árbol Go idéntico al 9/9 de `win-portability/final`), gauntlet completo no re-corido por markdown-only.
