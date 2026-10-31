# T7-02 — MCP exposes core, HTTP proves the shared envelope

## Status

Accepted.

## Context

S4 demands MCP as the agnostic interface and S5 demands consistent CLI/HTTP
contracts with no per-interface policies. The failure mode is each
interface re-implementing policy (error codes, version checks, capability
lists) and drifting.

## Decision

One direction only: interfaces delegate to core. `internal/mcp` owns
descriptors, discovery, error codes and the `regulus.mcp/v1` pin; the stdio
server keeps its dispatch and gains additive initialize fields. The HTTP
adapter (`internal/httpapi`) is a minimal read-only subset reusing CLI
core paths through buffers and mapping outcomes through the SAME error
constructors — parity is tested, not promised. Drift between descriptor
names and the server tool table fails a test in both directions.

## Consequences

New tools are added once (core path + descriptor); every interface picks
them up with identical semantics. An interface-specific policy would fail
the parity tests.
