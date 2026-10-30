# D3 gate_run null — el tool MCP trunca el gauntlet largo

User authorization: "procede" — chart propio para D3 de e1-e2-live-proof L3. Read down to `## Log` before working.

Domain: sextant

## Acceptance

### Scenario: gate_run MCP no devuelve null en gauntlet largo

- Given un task con gauntlet 9/9 que tarda minutos (mutación incluida)
- When `tools.regulus.gate_run` corre sobre ese task
- Then devuelve veredictos por capa y receipts rec_..., sin null, igual que el binario

### Scenario: gate_run MCP reporta exit code y output acotado

- Given un gate que falla
- When corre vía MCP
- Then el resultado incluye exit code distinto de 0 y el output acotado con sha, sin truncar a vacío

### Scenario: gauntlet binario sigue verde

- Given el fix del timeout/transporte
- When corre `gate run d3-gate-run-null/F1`
- Then 9/9 pass con receipts append-only

## Tasks

| ID | Spec | Work | Route |
|----|------|------|-------|
| F1 | D3 e1-e2-live-proof L3 | plugin gate_run timeout + exitCode + output (regulus.ts run + mcp.go) | @regulus-worker |

## Log

- L1: Chart opened desde E2 D3 — binario 9/9 pero MCP null. Causa scout: plugin run timeout 30s vs gates 600s/gate, execute ignora exitCode.
- L2: F1 CLOSED — Task F1 "plugin gate_run timeout + exitCode + output (regulus.ts run + mcp.go)". Plugin 30s→1h + señal, exitCode propagado, never-empty con sha; MCP live 9/9 demo__T1 sin null + fallo con exit 1 visible; binario d3-gate-run-null/F1 9/9 rec_18e3514a...-rec_18e35163... Chart CERRADO. Falta host reload para TS.
