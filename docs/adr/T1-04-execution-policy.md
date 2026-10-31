# ADR T1-04 — Execution policy with safe degradation

Date: 2026-10-30 · Status: accepted · Phase 1 (trust foundation)

## Context

Gates execute arbitrary shell from `regulus.json` with the invoker's
privileges, unbounded output, inherited environment and cwd. That is the
harness's purpose (CI runs repo-controlled commands), but the core should
constrain and record the conditions, not just inherit them.

## Decision

`internal/gates` enforces a per-project execution policy (`execution`
block, all optional, safe defaults):

- timeouts: per-gate or 600 s default, clamped to `[1, max]` (default max
  3600 s); refusals and kills map to BLOCKED verdicts;
- classification: `standard` | `privileged` (recorded) | `destructive`
  (catastrophic patterns only — root wipes, mkfs, `of=/dev/*`, fork bombs;
  quoted spans stripped, `rm -rf ./build` stays standard). Destructive
  requires `allow_destructive`, an allowlist optionally restricts first
  tokens; violations refuse before execution (BLOCKED/policy_violation,
  receipt still written as evidence of refusal);
- cwd pinned to the project root; `env_unset` removes exact-named variables
  (values are never recorded anywhere — only the names hash in the
  manifest);
- output capped at 8 MB per stream: flooding gates are killed
  (BLOCKED/infrastructure_failure) instead of buffered;
- Docker is opt-in per gate (`isolate: docker` + image): when the daemon is
  unavailable the gate runs host-contained and the fallback is recorded in
  the receipt (`isolation` + stderr note), never silent.

No cgroups, no seccomp, no sandbox: host permissions remain the boundary;
the policy is a backstop with receipts, documented as such.

## Consequences

- New restriction classes need policy tests incl. false-positive guards.
- Docker stays a degraded-safe option, never a requirement.
