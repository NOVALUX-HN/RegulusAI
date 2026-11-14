# Devin consolidado CLI/Desktop/Cloud (NOV-51 / T32)

Base existente: `regulus mcp` + `.devin/skills` + perfiles scout/worker + MCP config + instaladores (ver `docs/devin.md`).

Auditoría superficies:
- CLI (tu máquina): shell + PATH, full. Verifica `devin mcp list`, `/regulus`.
- Desktop (app nativa): Terminal + MCP, sin `devin` CLI; `%APPDATA%\devin\` overrides; `mcp_config.json` del repo carga directo.
- Cloud (VM Linux/Windows, snapshots efímeros): repo clonado con pack; `.regulus/` NO persiste → `regulus memory export/import` explícito. Blueprint Windows `runs-on:windows` (costo +9% ACU, disponibilidad limitada).

Divergencia plugin(19) vs MCP(17) intencional (`TestMCPIntentionalGapPinsPluginDelta`): NO portados `probe_contrast/compose`, `evolve_propose`; `evolve_tick` dry-run; `skills_list` lee opencode registry. Portados read-only: audit, evolve_report, skills_list, evolve_tick dry-run. Destructivos (`memory prune/import`, `surface apply`) solo shell. MCP no hereda permisos OpenCode; Devin `allow: Exec(regulus)` es enforcement.

Journeys: install, PATH, MCP connect, contract, read-only, efectos laterales, Cloud persistencia. No se afirma que hooks OpenCode corran en Devin.
