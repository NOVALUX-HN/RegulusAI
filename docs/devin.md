# Regulus in Devin — CLI, Desktop, Cloud (Windows-first)

One stdio MCP server (`regulus mcp`, zero dependencies) plus the
committed `.devin/` pack (skill, two subagent profiles, MCP config)
covers all three Devin surfaces. No OpenCode runtime needed anywhere.
The opencode plugin (hooks, 16 namespaced tools, opencode agents) does
NOT transfer — the table below shows what maps to what instead.

## What runs where

| Surface | Regulus core (gates, Almanac, charts, spec) | Workflow surface |
|---|---|---|
| Devin CLI (your machine) | `regulus` binary via shell — full | `.devin/skills`, `.devin/agents`, `.devin/mcp_config.json` (this repo) |
| Devin Desktop (native app) | Terminal + MCP — full | Same `.devin/` pack; user paths under `%APPDATA%\devin\` |
| Devin Cloud (Windows VM) | Blueprint builds it — full | Same pack (repo is cloned with it); memory caveat below |

## Devin Cloud — Windows blueprint (copy-paste)

Windows sessions use Git Bash by default, so the Linux-style steps work
unchanged. Needs Windows access enabled on the account (currently
limited availability) and costs ~9% more ACUs than Linux.

```yaml
runs-on: windows

initialize:
  - name: "Install Go"
    uses: github.com/actions/setup-go@v5
    with:
      go-version: "1.27"
  - name: "Build regulus"
    run: |
      go build -trimpath -o bin/regulus ./cmd/regulus
      bin/regulus contract
      bin/regulus doctor || true

maintenance: |
  go build ./...

knowledge:
  - name: test
    contents: go test ./...
  - name: lint
    contents: go vet ./... && gofmt -l internal/ cmd/
```

`doctor` is allowed to fail in the blueprint: it probes for an OpenCode
runtime, which Cloud VMs don't have — the `contract` line is the real
gate (it prints `regulus.contract/v1` and exits 0).

Cloud caveats (honest limits, not bugs):
- Snapshots are immutable: `.regulus/` runtime state does NOT persist
  across sessions. Almanac memory in Cloud lives per-session unless you
  commit a `regulus memory export` bundle and re-import it.
- `regulus gate run` works (pure Go + shell), but gates that assume
  interactive tools will refuse — read the refusal, don't fight it.

## Devin CLI — quickstart (Windows PowerShell)

```powershell
# one-time: build + PATH (or use the absolute path in mcp_config)
go build -trimpath -o bin/regulus ./cmd/regulus
$env:Path += ";$PWD\bin"

# register the MCP server (project scope, committed file also works)
devin mcp add -s project regulus -- regulus mcp

# pre-approve regulus tool calls (CLI permissions, project config)
# .devin/config.json — YOUR file, not committed by this repo:
# { "permissions": { "allow": ["Exec(regulus *)"], "ask": ["Write(**/.env*)"] } }

# use it
devin
> /regulus-workflow implement feature/T2 against charts/feature.md
```

The `.devin/mcp_config.json` in this repo already points at
`regulus mcp` over stdio; it assumes `regulus` is on PATH (or edit the
`command` to an absolute path). `devin mcp list` confirms registration.

## MCP trust boundary (read this before exposing regulus mcp)

`regulus mcp` has no permission layer: OpenCode `permission` blocks do not
apply over MCP. Treat every MCP client as untrusted input — Devin project
permissions (`allow: ["Exec(regulus *)"]`, `ask: ["Write(**/.env*)"]`) are
the enforcement point, not the server. Provenance fields (`memory_save
--path/--receipt`) are metadata only: the server never reads or writes
those paths, stores repo-relative values only, and refuses absolute paths
and `..` traversal. Never point `regulus mcp` at a privileged shell or
mount secrets where a prompt could name them.

## Devin Desktop — notes

Install the native app (Releases page has macOS/Windows/Linux builds).
Open this repo: the same `.devin/` pack applies (user-level overrides
live under `%APPDATA%\devin\` on Windows). MCP servers from
`.devin/mcp_config.json` load like in the CLI; verify in the MCP panel.
Skills invoke with `/regulus-workflow`; subagents `regulus-worker` and
`regulus-scout` appear alongside the built-ins (no model pinned, so the
router picks the cheaper default — pin one only if you need a frontier
model for writes).

## What maps to what (opencode → Devin)

| OpenCode (this repo) | Devin equivalent (this feature) |
|---|---|
| `.opencode/plugins/regulus.ts` (16 tools, hooks) | `regulus mcp` (12 tools, no hooks — Devin lifecycle hooks are separate opt-in) |
| `.opencode/skills/regulus-workflow/` | `.devin/skills/regulus-workflow/` |
| `.opencode/agents/regulus-{worker,scout}.md` | `.devin/agents/regulus-{worker,scout}.md` |
| `opencode.json` permissions | Devin `permissions` allow/deny/ask (project `.devin/config.json`, yours to create) |
| `regulus doctor` V2 gate | `regulus contract` version pin (`regulus.contract/v1`) |
| Almanac on disk (`.regulus/`) | Same files, same CLI — plus the Cloud persistence caveat above |

Deliberately NOT ported: `probe_contrast/compose` (2–3x gate cost per
call), `evolve_propose` (harness self-modification doesn't transfer),
`skills_list` (opencode registry; Devin has its own skills),
`memory prune/import/export` (destructive or management ops — run them
in a local shell, not through the agent).
