# Claude Code — Regulus

Usa skills/MCP y ciclo nativo. Hooks pequeños, deterministas, con timeout; nunca dependen de texto del modelo para enforcement.

1. Guía: `CLAUDE.md` (este repo). Skills: `.claude/skills/regulus/SKILL.md`. Perfiles: `.claude/agents/regulus-{worker,scout}.md`. MCP: `.mcp.json` → `regulus mcp`.
2. Hooks (settings): solo SessionStart/PreToolUse/PostToolUse/Stop que necesites; preserva JSONC/comentarios o falla sin sobrescribir. Guardians read-only vía permisos nativos; el hook no es aprobación.
3. Doctor/sync/uninstall con ownership, backups y preservación de ajeno. No cambies permisos del usuario sin consentimiento.
4. Verifica: `regulus contract`, `regulus doctor`, `regulus skills list`.
