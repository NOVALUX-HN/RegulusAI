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

## `regulus memory reindex` — when and why

Semantic search needs the derived vector index
(`.regulus/almanac/retrieval.bin`), built from your observation log:

```bash
regulus memory reindex
```

Run it once after upgrading to v2.0, and again whenever search or
`suggest` complains the index is stale — any save after the last
reindex makes it stale (the index pins the log's sha256, so staleness
is exact, not heuristic). Reindexing is a pure function of the log:
twice on the same log yields a byte-identical file, and stdout prints
the pinned sha. On an empty log it refuses ("no observations log yet")
— save an observation first.

## `regulus memory search` warns "semantic index stale or missing — lexical only"

That is the designed degrade path, not an error: the index is missing,
stale (the log changed since the last reindex), or corrupt, so search
ran lexical-only and told you on stderr. Results are still correct,
just without vector recall. Fix it with `regulus memory reindex`. To
force the old path on purpose (no warning), pass `--lexical`:

```bash
regulus memory search --lexical <terms>
```

Fresh index + no flag = hybrid ranking, silently. Fresh index + flag =
lexical, silently. Stale/missing index without the flag = lexical +
warning on stderr (stdout matches the `--lexical` output exactly).

## `regulus memory save` refuses with "near-duplicate save (matches ... similarity ..."

Saves scoring above 0.92 cosine against a live observation are refused
instead of stored as silent near-duplicates. The message names the
matching observation id and its similarity. Two cases:

- It really is a new revision of the same thought: re-save with
  `--topic-key <existing-key>` to append it to that chain instead.
- It is a different thought the hash embedder confuses: reword the
  title/content until the similarity drops (unrelated observations in
  the eval fixture score 0.26 or lower, far below the bar).

Refused saves exit non-zero and leave the log untouched. `memory import`
is exempt (re-importing identical content must succeed), and
observations hidden by a `supersedes` relation are excluded from the
comparison.

## `regulus memory relate --suggest <id>` says "requires a fresh semantic index"

Suggestions ARE the vector channel — there is no lexical fallback, so a
missing or stale index refuses (with a `reindex` pointer) instead of
presenting keyword hits as similarities. Run `regulus memory reindex`
first. Notes on the output:

- One line per candidate: `<id> <cosine> <title>`, top-5 above 0.5.
- Suggestion-only: nothing is written. Confirm with a plain relate:
  `regulus memory relate --id <a> --relation related --to <b>`.
- The source itself and `supersedes`-hidden observations never appear.
  No candidates ("no suggestions above threshold") means nothing cleared
  the 0.5 floor, not an error.
