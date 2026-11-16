# Contributing (NOV-86 Paso 66)

## Language policy

Working language is **English** for code, code comments, contracts,
governance docs (`SECURITY.md`, this file, `CODE_OF_CONDUCT.md`) and
commit messages. **Spanish is accepted** in issues, discussions,
`charts/`, `odd/tasks/` and `docs/close-traceability.md`. Existing
Spanish docs stay as they are — do not rewrite them for language
reasons; new user-facing docs default to English.

## Branches and releases

- Work lands on `develop` via pull request; `main` only moves by merge.
  Tags (`v*`) are cut on `main` only, never on `develop`.
- A PR needs **one review from an account other than the author** and
  green checks: `Gauntlet`, `Windows compatibility`, `Traceable close`
  and `Windows protected audit`. Direct pushes to `main`/`develop` are
  blocked; `CODEOWNERS` routes `internal/gates`, `internal/evolve`,
  `internal/hostcfg` and `.github/workflows` to mandatory review.

## Definition of done (close)

Every task closes with evidence, never with a bare statement:

- Commit(s) citing `NOV-xx` in subject or body; multi-issue commits
  need a `Justified:` trailer or `--justified` at close time.
- `regulus task close --issue NOV-xx <chart> <id>` runs the checks
  (scope, receipts, and for P0 an independent verdict); use
  `--verify-only` to preview without writing.
- Linear comment in the finished-task format (`## T<n> NOV-xx`,
  what was done + result with commands, exit codes, commits, receipts).
- P0 additionally needs an independent verdict recorded as a receipt.

Details: `docs/close-traceability.md`. CI enforces the linkage per PR
(`Traceable close` job).

## Local verification before pushing

```bash
regulus gate run [--profile quick|full]   # --yes only on repos you trust
go test ./...                             # unit tests
go vet ./internal/...                     # static analysis (subset pinned in CI)
gofmt -l internal/ cmd/                  # must print nothing
```

Toolchain pins live in CI (`go install ...@vX.Y.Z`); match them
locally. `bun install --frozen-lockfile` inside `.opencode/`.

## Commit format

`feat(scope): ...`, `fix(scope): ...`, `docs(scope): ...` with the
issue in subject or body, e.g. `feat(nov-82): ... (NOV-82)`.
`size check`, docs-drift and manifest gates run in CI — keep tests and
docs alongside the behavior in the same commit.
