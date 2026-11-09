# obscura-default-search — obscura como buscador por defecto del harness

User authorization: "pero si obscura es la mejor opcion debemos integrarlo dentro del instalador de regulus, asi que consideralo siempre como el buscador por defecto de regulus." 2026-11-03. Read down to `## Log` before working.

Class: STANDARD

## Contract

- Objective: dejar obscura (`browser_navigate + markdown/extract/links/evaluate`) como vía por defecto para consultas internet en fresh-install y en guidance editable, manteniendo `webfetch: deny` y `websearch` vacío como límites conocidos.
- Scope: `.opencode/tool-guidance.md` (sección search default) + literal via `scripts/sync-plugin-defaults.ts` + `.opencode/plugins/regulus.ts`, `.opencode/discipline.md` (1 línea buscador default), `internal/cli/init.go` (scaffold copia guidance, sin overwrite) + `.devin/mcp_config.json` espejo docs, `docs/` nota de receta. Sin tocar `opencode.json` permisos, `regulus.json#allowlist`, gates ni habilitar fetch.
- Constraints: no habilitar `webfetch`; no declarar MCP servers nuevos en `opencode.json`; no cambiar gates; receta probada: DDG html + extract + raw.githubusercontent; `spec check` debe seguir OK.
- Acceptance: O1, O2
- Artifacts: receipts `obscura-default-search/O*` (12/12 por tarea).
- Dependencies: O1 y O2 independientes (ficheros distintos); orden indistinto.
- Risk: standard
- Verification: `spec check charts/obscura-default-search.md` + `gate run obscura-default-search/O*` 12/12 + prueba viva `browser_navigate DDG + extract` documentada.
- Completion: O1–O2 CLOSED vía `task close`.

## Acceptance

### Scenario: O1

- Given `.opencode/tool-guidance.md` hoy no tiene sección search y `webfetch: deny` es intencional en 11 agents
- When se añade sección `## search-default` que declara obscura como default (`browser_navigate` + `browser_extract`/`browser_markdown` + `browser_links`, fallback raw/bing) y se regenera el literal vía `sync-plugin-defaults.ts`
- Then `grep -i obscura .opencode/tool-guidance.md` no vacío, `spec check` da `spec OK`, y `webfetch: deny` sigue intacto

### Scenario: O2

- Given `internal/cli/init.go:19-92` es el scaffold fresh-install y `.devin/mcp_config.json:2-7` solo trae `regulus`, sin guidance de búsqueda
- When `init.go#copyHarnessConfig` garantiza copiar guidance con default obscura y docs explican receta DDG+extract+raw sin tocar `opencode.json` ni `regulus.json#allowlist`
- Then fresh-install trae buscador por defecto documentado, `spec check` OK, y permisos/allowlist sin cambios

## Tasks

| ID | Spec | Work | Route | Status |
|----|------|-------|-------|--------|
| O1 | O1 | guidance: search-default obscura + regen literal | @regulus-worker | done |
| O2 | O2 | installer: scaffold guidance + docs receta | @regulus-worker | done |

## Log

- L1: Chart opened 2026-11-03 desde veredicto obscura (DDG 10/10 3427ms+108ms, raw 410ms+123ms limpio, GitHub HTML 3488ms contaminado, Bing ruidoso, fetch `Permission denied: webfetch`, websearch vacío). Installer real es `init.go` + `install-devin.*` solo PATH/binario; sin `mcp` en `opencode.json`; 0 hits obscura/websearch en repo. Orden blanket "termina todas" vigente (memory note 2026-11-03).

- CLOSED O1 STANDARD spec OK + 12 receipt(s) PASS (run_306f1c34a4740eed).
- CLOSED O2 STANDARD spec OK + 12 receipt(s) PASS (run_049d38d9db12dd8f).