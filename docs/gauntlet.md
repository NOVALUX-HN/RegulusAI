# Regulus Gauntlet — Independent Verification (Phase 3)

The gauntlet proves a change against its contract. Deterministic gates
produce receipts; guardians produce recorded opinions; the core
consolidates. Decisions: `docs/adr/T3-01`, `T3-02`.

## 1. Deterministic gates

Every gate declares `name`, `layer`, `command`, `timeout` — plus, since
Phase 3, `id` (stable identity), `category` (build, test, analysis,
security, proof), `when` (changed-file globs), `on_fail` (`halt` or
`advisory`), `required` (default true) and `profiles` (expensive-gate
membership). All new keys are optional: an undecorated gate behaves
exactly as before.

Execution is progressive by layer and stops at the first mandatory
failure. An `advisory` failure is written honestly as FAIL, then the run
continues; only `halt` failures stop it and fail the run. Skipped gates
(`when` matched nothing on an optional gate) write no receipt; the skip
decision is recorded in the result and the manifest.

## 2. Profiles and mandatory policy

`gate run --profile NAME` runs required gates plus the profile's members:

- `quick` — required gates only (fast feedback);
- `full` — everything (the default when no profile is given);
- any other name must be declared by at least one gate, else the run
  refuses rather than silently running a subset.

Required gates always run: neither profiles nor `when` conditions can omit
them. `gate run --dry-run` prints the resolved plan — including policy,
conditions and reproducibility — and executes nothing.

## 3. Semantic verification: guardians

The six guardians (`regulus-gate-verifier`, `spec-keeper`,
`mutation-auditor`, `crap-analyst`, `evidence-auditor`, `reviewer`) are
read-only specialists with bounded responsibilities. The core holds their
registry and accepts their assessments only through validation:

`regulus gate evaluate --task T --guardian NAME --verdict V --note TEXT [--evidence ID ...]`

Validation rejects unknown guardians, crew evaluators (the implementer
cannot judge its own work), unparseable verdicts, empty notes, and
evidence ids that resolve to nothing in the task dir. Accepted
assessments append to `evaluations.jsonl` in the task dir; receipts,
charts and policy are never touched by evaluation.

## 4. Consolidation: receipts decide

`regulus gate verdict <task>` prints two sections and one verdict:

- **deterministic** — per-gate status from receipts (hash-pinned facts);
- **semantic** — recorded evaluations with AGREE/DISPUTE flags against the
  consolidated verdict;
- **verdict** — FAIL beats BLOCKED beats INCONCLUSIVE beats PASS, derived
  from receipts alone. Evaluations can dispute, never override.

Rates are measured proxies, not invented ground truth: disagreement rate
(disputing evaluations / all evaluations), inconclusive counts on both
sides, and evidence completeness (receipts with manifest links / all
receipts). True false-positive/false-negative rates need an oracle the
harness does not have — the report says so instead of faking it.

## 5. Reproducibility

Skipped and advisory outcomes are evidence too: the manifest pins gate
categories and the skip list, and every receipt carries the policy that
produced it. Re-running the same profile against the same candidate
reproduces the same decisions; `when` conditions that cannot be evaluated
(fresh clone without git) fail open with a receipt note, never silently.

## 6. Toolchain and profiles

Heavy gates need three binaries outside the Go module. The pin source
is CI (`.github/workflows/gauntlet.yml:31-35`); local installs must
match it:

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
```

`bun` comes from the upstream installer (https://bun.sh), then
`bun install` for the project deps (same file, `setup-bun` +
typecheck/test steps). `gremlins` drives layer 6 (mutation),
`govulncheck` layer 8 (vulncheck), `bun` layers 10–11 (TypeScript).
Troubleshooting details live in `docs/troubleshooting.md` ("Toolchain
installs (pinned)"); a missing binary fails the run before layer 1
with the install hint, instead of a mid-run command-not-found.

Profiles select cost, never mandatory proof (§2): `quick` runs
required gates only (fast feedback), `full` runs everything and is the
default. Required gates always run under every profile. The `mutation`
gate keeps `profiles: [full]` but stays `required: true`, so it runs
under `quick` too.

Since T1, mutation is hermetic: the gate exports `git archive HEAD`
(tracked files only) into a fresh temp dir and runs gremlins there, so
untracked bloat such as `node_modules` never reaches it (portable Go:
`os` + `os/exec` + `archive/tar`, no POSIX). When the export fails
(not a git checkout, `git` absent) the gate falls back in-situ and the
receipt says so — `mutation: clean export via git archive HEAD
(hermetic, no untracked files)` vs `warning: mutation without clean
export (not a git checkout)` — while the verdict still comes from
efficacy/exit facts.

Wontfix, with evidence (see `docs/troubleshooting.md` "Wontfix" for
the full citations): no npm publish (owner decision,
`CHANGELOG.md:331-333` G3.8 + `README.md:19-23` build-from-repo); the
plugin-16 vs MCP-12 gap-4 is an intentional pinned divergence
(`internal/cli/mcp_test.go:179-215` +
`docs/devin.md:133-137`).
