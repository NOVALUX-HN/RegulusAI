# Phase 7 — Runtime Independence

Header: Regulus Core stays agent-SDK free (stdlib-only, pinned by test);
one common adapter contract with capability negotiation; the existing
OpenCode adapter is preserved untouched; MCP exposes core paths without
duplicating logic; versioned contracts; no fictitious adapters.

## Specs

S1: "## 1. CORE INDEPENDENCE El Core debe concentrar: Policies.
Verification. Evidence. Receipts. Memory. Evolution. Decision Assessment.
No debe depender de SDKs específicos de agentes."
S2: "## 2. RUNTIME ADAPTER CONTRACT Diseña un contrato común para
adaptadores. Debe contemplar: Lifecycle events. Task context. Agent
identity. Tool capabilities. Permission boundaries. Execution metadata.
Context exchange. Result reporting. Error propagation. No asumas que todos
los runtimes ofrecen las mismas capacidades. Define capability
negotiation."
S3: "## 3. ADAPTER ARCHITECTURE Conserva el runtime actualmente soportado.
Diseña contratos extensibles para: OpenCode. Claude Code. Codex. Otros
agentes. No declares soporte sin implementación y pruebas."
S4: "## 4. MCP Fortalece MCP como interfaz agnóstica. Define: Tools.
Resources. Prompts cuando correspondan. Authentication. Authorization.
Capability discovery. Error contracts. Versioning. MCP debe exponer el
Core, no duplicar su lógica."
S5: "## 5. CLI AND HTTP Evalúa interfaces adicionales: CLI. HTTP API.
Define contratos consistentes. Evita que cada interfaz implemente sus
propias políticas."
S6: "## 6. VERSIONING Implementa versionado de contratos. Gestiona
compatibilidad y migraciones."
S7: "## 7. TESTING Crea: Adapter contract tests. Capability tests.
Protocol tests. Error handling tests. Compatibility tests. Integration
tests."
S8: "## 8. ACCEPTANCE CRITERIA Regulus debe demostrar que: 1. El Core no
depende de un SDK específico. 2. Los adaptadores tienen responsabilidades
delimitadas. 3. MCP expone capacidades del Core. 4. Las políticas son
comunes a todos los runtimes. 5. Los contratos están versionados. 6. El
soporte de cada runtime está demostrado mediante pruebas. No implementes
adaptadores ficticios. Actualiza arquitectura y documentación."

## Tasks

- T1 (S1): stdlib-only boundary test + core inventory — route: inline — commit: —
- T2 (S2,S3): internal/runtime contract + capability negotiation + registry (opencode preserved, rest unclaimed) + tests — route: delegated (gentle-ai-worker) — commit: c1a7d4d
- T3 (S4,S6): internal/mcp descriptors + capability discovery + error/version contracts + initialize wiring — route: delegated (gentle-ai-worker) — commit: ab6a890
- T4 (S5): shared interface envelope + minimal read-only HTTP adapter + consistency tests — route: delegated (gentle-ai-worker) — commit: 2061953
- T5 (S7,S8): contract/capability/protocol/error/compat/integration tests + acceptance — route: delegated (gentle-ai-worker, test-only) — commit: bbec55c
- T6 (docs): architecture + ADRs + CHANGELOG + full verification + final report — route: inline — commit: 96d8931

## Log

L1 (user original verbatim): "# PHASE 7 — RUNTIME INDEPENDENCE

## MISSION

Actúa como Principal Platform Architect especializado en sistemas extensibles, protocolos, MCP y arquitectura de adaptadores.

Evoluciona la arquitectura del proyecto actual para que Regulus Core sea independiente del runtime de agentes.

Preserva el adaptador existente y su funcionamiento.

## 1. CORE INDEPENDENCE

El Core debe concentrar:

- Policies.
- Verification.
- Evidence.
- Receipts.
- Memory.
- Evolution.
- Decision Assessment.

No debe depender de SDKs específicos de agentes.

## 2. RUNTIME ADAPTER CONTRACT

Diseña un contrato común para adaptadores.

Debe contemplar:

- Lifecycle events.
- Task context.
- Agent identity.
- Tool capabilities.
- Permission boundaries.
- Execution metadata.
- Context exchange.
- Result reporting.
- Error propagation.

No asumas que todos los runtimes ofrecen las mismas capacidades.

Define capability negotiation.

## 3. ADAPTER ARCHITECTURE

Conserva el runtime actualmente soportado.

Diseña contratos extensibles para:

- OpenCode.
- Claude Code.
- Codex.
- Otros agentes.

No declares soporte sin implementación y pruebas.

## 4. MCP

Fortalece MCP como interfaz agnóstica.

Define:

- Tools.
- Resources.
- Prompts cuando correspondan.
- Authentication.
- Authorization.
- Capability discovery.
- Error contracts.
- Versioning.

MCP debe exponer el Core, no duplicar su lógica.

## 5. CLI AND HTTP

Evalúa interfaces adicionales:

- CLI.
- HTTP API.

Define contratos consistentes.

Evita que cada interfaz implemente sus propias políticas.

## 6. VERSIONING

Implementa versionado de contratos.

Gestiona compatibilidad y migraciones.

## 7. TESTING

Crea:

- Adapter contract tests.
- Capability tests.
- Protocol tests.
- Error handling tests.
- Compatibility tests.
- Integration tests.

## 8. ACCEPTANCE CRITERIA

Regulus debe demostrar que:

1. El Core no depende de un SDK específico.
2. Los adaptadores tienen responsabilidades delimitadas.
3. MCP expone capacidades del Core.
4. Las políticas son comunes a todos los runtimes.
5. Los contratos están versionados.
6. El soporte de cada runtime está demostrado mediante pruebas.

No implementes adaptadores ficticios.

Actualiza arquitectura y documentación."

L3 (2026-10-31, Phase 7 completed): T1-T6 done on develop (5e36c06, c1a7d4d, ab6a890, 2061953, bbec55c, 96d8931). Full `go vet` + `build` + `go test ./...` green, 0 FAIL. T2-T5 delegated (gentle-ai-worker, assess each: writer self-verification stands, no verifier). No fictitious adapters; HTTP read-only proving shared envelope.

L2 (decision): go.mod has zero external requires — pin it with a test
(`go list -m all` shows only stdlib + main module) instead of trusting
it. The TS OpenCode plugin stays the supported adapter, preserved
untouched; no new product adapters (S3/S8 forbid fictitious support) —
extensibility is proven by the contract plus a test-only in-memory
adapter, never shipped as support. MCP keeps delegating to CLI core
paths (no logic duplication); HTTP is a minimal read-only subset proving
shared-policy routing. Versioning extends the existing
internal/contract pin pattern.
