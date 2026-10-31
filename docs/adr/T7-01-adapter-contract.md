# T7-01 — Adapter contract with negotiation, no fictitious runtimes

## Status

Accepted.

## Context

Phase 7 needs one contract for all agent runtimes while only OpenCode is
actually supported. The failure mode is claiming support (claude-code,
codex) backed by nothing but types.

## Decision

`internal/runtime` defines the `Adapter`/`Session` contract plus explicit
capability negotiation (`Negotiate` returns granted + missing; missing is
never silently assumed). The production registry is EMPTY — adapters
self-register. Only "opencode" counts as supported, evidenced by the
preserved TS plugin plus the `regulus.contract/v1` pin both sides check.
Test-only in-memory fakes prove the contract is implementable; their names
and comments mark them test-only, never product support.

## Consequences

Adding a runtime means implementing the contract, registering it, and
proving it with tests — the acceptance test (S8.2/S8.6) fails any shortcut
that registers a name without those.
