# Troubleshooting

## `regulus doctor` says "opencode binary not found on PATH"

The regulus binary can't find `opencode`. Install OpenCode V2:

```bash
# Official installer (Linux, macOS, WSL, git-bash on Windows)
curl -fsSL https://opencode.ai/v2/install | bash
```

On Windows, the installer adds `~/.opencode/bin` to your PATH via `.bashrc`.
Restart your terminal or run `source ~/.bashrc`.

## `regulus doctor` says "OpenCode V1 is not supported"

Regulus targets OpenCode V2 exclusively. If you have V1 installed, remove it
and install V2 with the command above.

## `regulus status` says "gremlins NOT found"

The mutation gate (layer 6) needs [gremlins](https://github.com/go-gremlins/gremlins):

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
```

## `regulus status` says "no regulus.json — not configured"

Create a `regulus.json` in your project root. See the README quickstart for
a full example with all eight gate layers.

## The gauntlet's mutation gate fails with timeouts

On Windows, gremlins copies the whole module per mutant — including
`node_modules` if present — and every mutant starves to timeout. The gate
runs over a clean `git archive HEAD` export to avoid this; if it still
times out, check that `git archive HEAD` works and that your repo isn't
too large.

## The plugin doesn't find the regulus binary

The plugin searches, in order: `$REGULUS_BIN`, `bin/regulus.exe`,
`bin/regulus`, `binaries/regulus-<platform>-<arch>`, `~/.regulus/bin`,
and PATH. If none exists:

```bash
go build -o bin/regulus ./cmd/regulus
```

## `regulus memory search` shows "N commits behind"

This is the git-distance staleness annotation: the observation's provenance
records the git HEAD at save time, and `git rev-list --count <head>..HEAD`
measures how far you've moved since. Update the observation if the code has
changed.

## Surface edits are refused with "outside the harness surface whitelist"

Surface edits can only target: `.opencode/skills/`, `.opencode/agents/`,
`.opencode/plugins/`, and `docs/`. Code, configs and everything else are
outside the harness surface by design.
