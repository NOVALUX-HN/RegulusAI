# Search default: obscura (recipe)

Web search default for the harness is **obscura**:
`browser_navigate` + `browser_extract`/`browser_markdown` + `browser_links`,
with `browser_evaluate` as fallback probe. Known limits stay as they are:
`webfetch: deny` is intentional and `websearch` is empty — use this recipe
instead. Nothing here changes `opencode.json` permissions,
`regulus.json#execution.allowlist`, gates, or enables `webfetch`.

## Default recipe: DDG html + extract + raw

1. Navigate to the DDG html endpoint (no JS needed):
   `https://html.duckduckgo.com/html/?q=<terms>`
2. Extract result titles (array form):
   `browser_extract` with schema `{"results[]": ".result__title"}`.
3. Open the chosen hit with `browser_navigate` and read it with
   `browser_markdown` (token-dense) or `browser_extract` for fields.
4. For raw files (GitHub, docs), navigate to the
   `raw.githubusercontent.com` URL directly — clean text, no page chrome.

Live proof 2026-11-03 (this repo): DDG html navigate **738ms** + extract
**123ms**, 10 result titles (`Regulus AI …`, `Regulus Labs …`, …);
markdown read clean. Chart L1 verdict (same day): DDG 10/10
(3427ms+108ms), raw 410ms+123ms limpio, GitHub HTML 3488ms contaminado,
Bing ruidoso, `webfetch` → `Permission denied`, `websearch` vacío.

## Fallbacks

- **Bing HTML**: noisier result markup, usable when DDG is blocked;
  same extract-then-markdown flow.
- **GitHub HTML**: contaminated with nav chrome (3.4s+) — prefer the
  `raw.githubusercontent.com` URL for the same file.
- **`browser_evaluate`**: last resort probe (counts, selectors) when
  extract returns nothing.

## Fresh-install: how the default travels

`regulus init --harness-from <repo>` copies `.opencode/tool-guidance.md`
(including the `## search-default` section) into the fresh repo — see
`internal/cli/init.go#copyHarnessConfig`. It never overwrites: an existing
`.opencode/` refuses. Proven by
`TestInitHarnessFromKeepsSearchDefault` (fails when `tool-guidance.md`
is dropped from the copy list).

## `.devin/mcp_config.json` mirror (docs only)

The `.devin/mcp_config.json` mirror registers **only** the `regulus`
stdio server (`command: regulus`, `args: [mcp]`) — unchanged by this
feature. obscura is a host/Devin browser tool, not an MCP server, so the
MCP config format has no obscura slot to fill: no new server is declared
there nor in `opencode.json`. The search default reaches Devin surfaces
through the copied guidance (`## search-default`), not through server
registration. Devin project permissions stay the enforcement point
(see `docs/devin.md` MCP trust boundary).
