# Instalador multiagente + Skill Registry (NOV-52 / T33)

Manifest administrado por scope:
- Proyecto: `AGENTS.md`, `CLAUDE.md`, `.agents/skills/regulus/`, `.claude/skills/regulus/`, `.claude/agents/regulus-*.md`, `.codex/config.toml`, `.mcp.json`, `.devin/` (fuente, no copia), `.opencode/skills/regulus-workflow/`.
- Global: `~/.config/devin/` (copias S4), `%APPDATA%\devin\`, `~/bin|~/.local/bin` binario.

Operaciones idempotentes: detección (¿qué falta?), dry-run (describe sin escribir), install, sync, doctor, backup/rollback (timestamped), uninstall (solo lo instalado, backup previo). Si el contenido administrado fue editado a mano o hay conflicto de propiedad → informa y detente. Preservación byte-for-byte de ajeno probada en tests.

Skill Registry (`internal/skills/registry.go`): escanea en orden proyecto→global `.opencode/.agents/.claude/.devin`, primero gana, índice ordenado `{name, source}`. Solo metadata/triggers/rutas; cuerpo bajo demanda. Misma fuente de workflow proyectada a AGENTS.md/CLAUDE.md/skills sin lógica contradictoria.

Windows/macOS/Linux sin shell assumptions ni helpers extra (ver NOV-45 StructuredRunner).
