# NOV-21 — MCP solo local confiable, sin auth multiusuario (T20)

Decisión: proporcional al despliegue real. Solo stdio local.

Evidencia:
- `internal/mcp/descriptors.go:38-` 17 tools solo metadata, sin run.
- `internal/mcp/capabilities.go:5` `2024-11-05`; `version.go:7` `regulus.mcp/v1` con refuse explícito.
- `internal/cli/mcp.go:16-` stdio newline-JSON-RPC 1MB/línea; solo initialize/tools/list/call.
- Transporte único: `command:regulus args:[mcp]` en `.devin/mcp_config.json:3-5`. Sin SSE/HTTP.
- Controles: `local-trust, no auth/token`; cliente hace enforcement (`docs/devin.md:77-`, allow Exec(regulus), rechaza absolutas/`..`).

Si aparece cliente externo no confiable: definir auth/caps mínimas por transporte + tests contrato permitido/denegado. Hoy: documentado, sin capas multi-tenant. Dashboard sigue solo lectura.
