# devin-install — regulus visible y usable en Devin Desktop/CLI

User authorization: "revises la instalacion con Devin ya que estoy en el devin desktop y no veo en ningun lado a regulus para poder usarlo" (2026-10-30). Causa raíz probada por lead: `.devin/mcp_config.json:4` pide `command: regulus` pero `regulus` no estaba en PATH (solo `bin/regulus.exe` local); `binaries/` trae 0.8.1 sin fixes; Desktop no tiene CLI `devin` (solo `devin-desktop`), así que el quickstart `devin mcp add` de `docs/devin.md:66` no aplica. Fix local ya aplicado inline: `bin/regulus.exe` (dev, con fallback ShRunner) copiado a `C:/Users/Andragon/bin/regulus.exe` (en PATH) — `regulus contract` → `regulus.contract/v1`, `mcp initialize` → `serverInfo contract v1` por stdio. Este chart fija el repo para que el próximo usuario no pase por lo mismo. Read down to `## Log` before working.

## Acceptance

### Scenario: regulus resolves on PATH after install

- Given a Windows machine with Go, without `regulus` on PATH, with this repo cloned
- When the install entrypoint runs
- Then `regulus contract` prints `regulus.contract/v1` and `regulus --version` reports the stamped version

### Scenario: MCP server spawns from the committed pack

- Given the repo opened in Devin Desktop (or CLI) with `regulus` on PATH
- When the client starts the `regulus` server from `.devin/mcp_config.json`
- Then `initialize` returns `serverInfo` with `contract regulus.contract/v1` over stdio

### Scenario: docs tell the Desktop truth

- Given a Devin Desktop user with no `devin` CLI on PATH
- When they read the Devin install docs
- Then the Desktop path (MCP panel + `regulus` on PATH + `/regulus-workflow`) works without any `devin mcp add` step, and the CLI path stays documented separately

## Tasks

| ID | Spec | Work | Route |
|---|---|---|---|
| D1 | S1–S2 | portable install entrypoint (`scripts/install-devin.ps1` + bash mirror if trivial) that builds, installs to a PATH dir, verifies `contract` + MCP `initialize` smoke | @regulus-worker |
| D2 | S3 | docs fix (`docs/devin.md` Desktop-first quickstart, no `devin` CLI assumption) + `.devin/mcp_config.json` robustness note | @regulus-worker |

## Log

- **L1**: Chart opened 2026-10-30 from user pain in Devin Desktop. Local unblock already done inline (copy to `~/bin`, contract+MCP verified). Repo fix follows.
- **L2**: D1+D2 CLOSED + PROVE — `scripts/install-devin.ps1` (build 0.8.1 + ~/bin + contract + MCP initialize smoke, EXIT 0 ×2, Go-missing guard EXIT 1); `docs/devin.md` Desktop-first (CLI-only marcada, fallback 2 líneas). `spec_check` 3 OK (D1+D2). Install verificado en PATH: `regulus 0.8.1`, `contract v1`, `initialize serverInfo v1`.
- **L3**: Chart CERRADO 2026-10-30 vía `polish-gaps/P3` (S4): D1/D2 reverificados — `scripts/install-devin.ps1` + `scripts/install-devin.bash` (S1 `contract` + S2 MCP `initialize`), `docs/devin.md` Desktop-first (CLI-only marcada §CLI-only, Desktop sin `devin` CLI §Devin Desktop, robustness note `command` absoluta), `.devin/mcp_config.json` (`command: regulus`, `args: [mcp]`). Tasks CLOSED desde L2, nada pendiente. `spec_check` 3 OK.
