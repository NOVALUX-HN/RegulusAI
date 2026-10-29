# Troubleshooting

## Terminal windows flash open and shut on Windows whenever regulus runs

The opencode background server has no console on Windows, so every
spawn of the regulus binary (each tool call, each trace record, each
context hook) used to pop a visible terminal window. Fixed in two
places: the plugin spawns with `windowsHide: true`, and the Go code
marks its children via `internal/executil.HideWindow` (doctor probe,
gate runner, project-name lookup). If flashes return, a spawn site is
missing the marking — grep for `exec.Command` and route it through
`executil`.
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

## Surface edits are refused with "is a reserved harness file"

`scripts/build.bash` sits inside the whitelist but is reserved: release
infrastructure changes by hand (a human decision), never as a side
effect of an adopted lesson. Edit it directly; no flag overrides this.

## `regulus reflect` or `probe contrast` says "evolution cap reached"

Two caps gate evolution: per day (`max_per_day`, default 10) and per
task (`max_per_task`, default half the daily cap). A refusal names which
one (`cap N reached` vs `task cap N reached`) and is recorded but free.
Wait for the next day, or raise the cap in `regulus.json`.

## `regulus memory prune` lists entries but removes nothing

That's the design: without `--yes` it's a dry run. Re-run the exact
same command with `--yes` appended to execute. Lessons and pinned
observations are immune and never listed.

## `regulus memory import` says "line N: ..." and imports nothing

The import is atomic: the first invalid line aborts the whole file so
a half-imported log is impossible. Fix the cited line (missing id,
unknown type, lesson without evidence) and re-run — already-imported
ids are skipped, so re-running is safe.

## `regulus evolve propose` says "no evidence"

Proposing needs receipts: run the gauntlet for the task first
(`regulus gate run <task>`), then propose. An evidence gate with no
evidence refuses instead of inventing a lesson.

## New charts lack the `Domain:` line

`regulus chart init` scaffolds `Domain: <name>` at the top — set it to
the lesson domain (e.g. `go`, `docs`) so the context hook injects
that domain's lessons instead of the newest of all domains.
