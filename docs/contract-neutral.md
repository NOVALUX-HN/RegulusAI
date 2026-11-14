# Contrato neutral multiagente v1 (NOV-48 / T28)

Dueño: Go core (estado, gates, receipts, Almanac, Evolution). OpenCode V2 = adaptador existente. Sin SDKs propietarios en core (`grep regulus-ai/internal/runtime` solo tests; `boundary_test` pin stdlib-only + `go list -m all==regulus-ai`).

Lifecycle: `Adapter{Name,Capabilities,Open}` → `Session{CheckPermission,Provide,Context,Report,Fail,Close}` (`internal/runtime/adapter.go`). Negociación explícita `Negotiate(required,offered)->granted,missing` (`negotiation.go`); sin simulación de hooks/delegación ausente. Requeridas/opcionales/degradadas por host; fallback inline/MCP/skills.

Versionado: `regulus.contract/v1` (`internal/contract/contract.go:7`, `Supported=[v1]`), `regulus.mcp/v1` + protocolo `2024-11-05` (`internal/mcp/version.go`), `docs/contracts.md` 3 familias + `manifest check` fail. Paridad TS verificada `contract_test.go:TestParityWithTypeScriptMirror`.

Pruebas sin hosts: `contract_test.go` (pin, Supports, paridad), `mcp_test.go` (pin, refuse, 17 tools, discovery), `runtime/adapter_test.go:TestNoProductAdaptersRegistered`, `acceptance_test.go`.

Matriz (fuente oficial, versión probada localmente donde aplica):
- OpenCode V2: hooks + 19 tools plugin + agents; probado `bun test ./lib/`, `bunx tsc`.
- Codex CLI: MCP cliente + AGENTS.md + `.agents/skills`; hooks solo si API estable; probado `regulus contract` + MCP `initialize`.
- Claude Code: `CLAUDE.md` + skills + MCP `.mcp.json` + hooks settings opt-in; probado idem.
- Devin CLI/Desktop/Cloud: MCP principal + skills/perfiles; Cloud sin persistencia disco (export/import explícito); probado install + `contract`.

Nada del core importa `opencode|claude|codex|devin`.
