# ADR T1-02 — Candidate manifest (a HEAD is not an identity)

Date: 2026-10-30 · Status: accepted · Phase 1 (trust foundation)

## Context

Receipts pinned `git_head`, but a HEAD does not identify the evaluated
content: uncommitted edits, untracked files, the gate configuration, tool
versions and the environment all shape the result. Two runs at the same HEAD
can evaluate different candidates.

## Decision

Every `gate run` builds a candidate manifest (`internal/manifest`,
schema-versioned) before executing: HEAD + `git diff HEAD` hash (capped,
flagged when truncated), modified-file fingerprints (path/size/sha, files
over 1 MB recorded size-only), untracked inventory (git exclude-standard),
config fingerprint + pinned gate list, tool versions
(regulus/contract/go/os/arch), a hash of environment variable *names only*,
the applied execution policy, and reproducibility counts. Missing git
degrades to `available: false`, never to failure. After the run, task→
artifact links (receipt id/file/sha/status/reason) are attached and the
manifest is written atomically next to its receipts.

Reproducibility is an annotation per gate (`exact`|`statistical`|`unknown`,
default `unknown`): claimed by the gate author, counted by the manifest,
never inferred by magic.

## Consequences

- The manifest is the reproducibility record (S5/S7.5); reviewers read it,
  not the worktree diff.
- Manifests share the task evidence dir so `evidence verify` can
  cross-check them; `evidence list` skips them (indexes, not receipts).
- New manifest fields must be additive; readers refuse unknown schemas.
