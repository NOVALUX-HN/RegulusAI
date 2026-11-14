# AGENTS.md — Regulus en Codex

Codex es cliente MCP de Regulus. No uses el antiguo Codex MCP server (retirado sep-2026).

1. MCP: `.codex/config.toml` → `regulus mcp` stdio. Verifica con `regulus contract` → `regulus.contract/v1`.
2. Skills: `.agents/skills/regulus/SKILL.md` (`/regulus`, user-trigger). Subagentes solo si tu versión Codex los soporta; si no, trabaja inline — receipts y close iguales.
3. Hooks Codex: solo si hay API documentada y estable; fallback skills/MCP/CLI conserva lo esencial.
4. Sandbox/approvals de Codex los controla el usuario; Regulus no amplía permisos.
5. Setup: build Go 1.27 `go build -o bin/regulus ./cmd/regulus`, PATH, `regulus doctor`. Uninstall: borra `.codex/config.toml` entrada regulus + `.agents/skills/regulus/` solo si lo instalaste vos (backup previo).

Degradación explícita: sin hooks → sin lifecycle events; sin subagentes → inline; sin MCP → CLI `regulus gate run`, `regulus memory`, `regulus skills list`.
