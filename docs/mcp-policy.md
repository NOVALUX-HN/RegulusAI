# MCP capability policy — server-side authorization (F11.8/NOV-94)

`regulus mcp` enforces a capability policy **server-side, before any
side effect**. The stdio wire stays local-trust by design (no auth
handshake, no tokens); authorization is derived from the server's
environment and resolved workspace, never from client arguments.

- Policy version: `regulus.mcp-policy/v1` (constant `mcp.PolicyVersion`,
  pinned in every audited decision).
- Code: `internal/mcp/policy.go` (pure evaluation + matrix),
  `internal/cli/mcp_policy.go` (server principal + audit),
  enforcement in `(CLI).mcpCallTool` (`internal/cli/mcp.go`).
- Audit store: F11.1 execution log
  `.regulus/observability/exec.jsonl` (`event_type: decision`,
  `schema_version: 1`, redacted pre-disk) — see `docs/trace-schema.md`.

This document supersedes the `docs/devin.md` §"MCP trust boundary"
paragraph that states `regulus mcp` has no permission layer: that was
true before F11.8 and is kept as the client-side story (Devin project
permissions remain a complementary layer), but the server is now the
authority. `docs/devin.md` itself is untouched by this change; read
this file as the amendment.

## 1. Threat model

| # | Threat | Assumption | Mitigation (server-side) | Test |
|---|---|---|---|---|
| T1 | Malicious / compromised MCP client | Client input is untrusted, including tool args | Deny-by-default; scopes from `REGULUS_MCP_SCOPES` only; args are DATA | `TestMCPPolicyRestrictedDeniesBeforeEffects`, `TestMCPPolicyLockdownDeniesReadsDiscoverySurvives` |
| T2 | Prompt injection via Almanac content | Retrieved notes/observations are DATA, never instructions | Content stored verbatim, framed by the existing `trustBanner`; injection cannot alter scopes (no scope lives in args) | `TestMCPPolicyPromptInjectionInert` |
| T3 | Tool spoofing (`gate_run `, `../gate_run`, `GATE_RUN`, NUL) | Names are attacker-controlled strings | Exact, case-sensitive dispatch match; anything else is `-32601`; spoofed names never reach policy audit or dispatch | `TestMCPPolicyToolSpoofingNeverDispatches` |
| T4 | Confused deputy (legit scope, hostile path) | One granted read must not become arbitrary file I/O | `spec_check.file`, `audit.profile`, `memory_context.charts` must stay inside the server-resolved workspace; provenance (`paths`/`receipts`) must be repo-relative (metadata, never I/O) | `TestMCPPolicyConfusedDeputyContained`, `TestMCPPolicyTraversalTable` |
| T5 | Path traversal (`..`, absolute, drive letter) | Paths arrive over the wire | Refused for **every** principal (not a scope shortcut): full principals are contained too | `TestPolicyPathTraversalRefused`, `TestMCPPolicyTraversalTable` |
| T6 | Replay (same `tools/call` twice) | No nonces on stdio | Writes are idempotent (`SaveChecked` content-hash no-op: one note stored); every call is audited, so replays are detectable (2 decisions, 1 effect) | `TestMCPPolicyReplayIdempotentAudited` |
| T7 | Privilege escalation via delegation | Delegates are less trusted than parents | `Principal.Delegate` intersects scopes (never widens) and pins host/user/workspace; the evaluator agrees | `TestPolicyDelegateOnlyRestricts`, `TestMCPPolicyDelegationCannotEscalate` |
| T8 | Scope spoofing (`{"scope":"admin"}`, `allow_destructive:true`) | Args may carry authority-shaped keys | Authorization never reads args (spoof keys ignored by construction); `allow_destructive` over MCP is impossible — no tool consults it | `TestPolicyClientArgsNeverAuthorize`, `TestMCPPolicyPromptInjectionInert` |
| T9 | Secret exfiltration into the audit trail | Args may contain credentials | Audit stores **arg names only** (never values); `RecordExec` redacts pre-disk; reasons name keys, never values | `TestMCPPolicySecretsNeverReachAudit`, `TestPolicyReasonsCarryNoValues` |
| T10 | Wet `evolve_tick` as a stealth write | Default looks read-only | Dry (default) needs `evolve:read`; explicit `dry_run:false` needs `evolve:write` and advances the tick cursor | `TestPolicyEvolveTickDryVsWet`, `TestMCPPolicyEvolveTickWetNeedsWrite` |

Out of scope / accepted risks:

- **No wire auth by design.** stdio is local-trust (`TestStdioLocalTrustNoAuthHandshake`
  still holds: discovery payloads carry no auth fields). Remote exposure
  must add its own transport auth; the policy cannot distinguish *which*
  local client called.
- **Audit is best-effort, availability-first.** A logging failure is
  reported on the server stderr and never changes the wire outcome.
  Verify decisions with `regulus trace timeline --run <r>` / `trace show`
  patterns over the exec log.
- **Replay is detectable, not prevented.** Execution tools (`gate_run`)
  re-run on replay by design (receipts are the evidence); state writes
  are idempotent where the store supports it.
- **Workspace paths may contain user names** (e.g. `C:\Users\<name>`)
  and are stored for attribution; the shared redactor deliberately does
  not scrub home paths (see `RedactText` D3 note).
- **Colons fail closed.** A relative value containing a drive-letter
  shape (`a:b`) is refused even on POSIX, where it would be a legal
  filename. No served tool needs such names.

## 2. Tool → capability → policy matrix

Capabilities: `read` (no state change) · `write` (project state) ·
`execute` (process spawn) · `secrets` (secret-shaped data handled,
redacted) · `memory` (Almanac state) · `evidence` (receipt/cursor
writes) · `harness` (evolution state). Sensitive = write or execute:
never runs without server-side authorization.

| Tool | Capabilities | Minimal scopes | Sensitive | Notes |
|---|---|---|---|---|
| `memory_note` | write, secrets, memory | `memory:write` | yes | append-only; idempotent replay; content is DATA |
| `memory_show` | read, memory | `memory:read` | — | |
| `memory_save` | write, secrets, memory | `memory:write` | yes | lesson types still refused by the CLI; provenance metadata-only, repo-relative |
| `memory_search` | read, memory | `memory:read` | — | buckets are filters, not reads |
| `memory_context` | read, memory | `memory:read` | — | `charts` are file reads → workspace-contained |
| `memory_history` | read, memory | `memory:read` | — | |
| `gate_run` | execute, evidence | `gate:execute` | yes | spawns gates, writes receipts; destructive gate patterns still refused by the gate layer without `allow_destructive` |
| `spec_check` | read | `spec:read` | — | `file` workspace-contained |
| `doctor` | read | `doctor:read` | — | probe only, no secret values in output |
| `trace_show` | read | `trace:read` | — | |
| `trace_sessions` | read | `trace:read` | — | |
| `trace_files` | read | `trace:read` | — | |
| `audit` | read | `audit:read` | — | `profile` workspace-contained |
| `evolve_report` | read | `evolve:read` | — | |
| `skills_list` | read | `skills:read` | — | pure registry read |
| `evolve_tick` | read (+harness, evidence when wet) | `evolve:read` dry · `evolve:write` wet (`dry_run:false`) | wet only | default dry-run writes nothing |
| `evolve_propose` | read | `evolve:read` | — | read-only pack assembly |
| *(not exposed)* `probe_contrast/compose`, `memory prune/import`, `surface apply` | — | shell-only | — | destructive / management stays out of MCP by policy (`TestMCPIntentionalGapPinsPluginDelta`) |

No MCP tool is destructive and none modifies harness code. The matrix
is code (`mcp.PolicyMatrix()`), pinned both ways against `mcp.Tools()`
by `TestPolicyMatrixCoversTools`.

## 3. Principals, scopes, delegation

A principal separates four dimensions — agent (calling identity,
server-attributed), host, user, workspace (server-resolved project
root) — plus the granted scope set:

- `REGULUS_MCP_SCOPES`: comma-separated. **Unset** = compatible
  default (all ten minimal scopes: current behavior unchanged).
  **Set** = exactly the known names listed (unknown names dropped).
  **Set-but-empty** = lockdown: every `tools/call` denied, discovery
  (`initialize`, `tools/list`) still served.
- `REGULUS_MCP_AGENT` / `REGULUS_MCP_USER`: deployment attribution
  (default `mcp:stdio-client` / OS user), sanitized single-line.
  Client args can never set these.
- Delegation (`Principal.Delegate`): child scopes = parent ∩
  requested; host/user/workspace pinned; agent chain recorded as
  `parent>child`. Delegation only restricts — escalation is refused
  by construction and pinned by test.

Least-privilege recipes:

```powershell
# read-only observer: memory+trace+report reads, no writes, no execution
$env:REGULUS_MCP_SCOPES = "memory:read,trace:read,audit:read,evolve:read,skills:read,spec:read,doctor:read"

# executor without memory writes (gates may run, notes may not land)
$env:REGULUS_MCP_SCOPES = "memory:read,gate:execute,spec:read,doctor:read,trace:read,audit:read,evolve:read,skills:read"

# lockdown: authorize nothing (discovery still answers)
$env:REGULUS_MCP_SCOPES = ""
```

## 4. Enforcement (where, in code)

1. `(CLI).mcpCallTool` (`internal/cli/mcp.go`): unknown name →
   `-32601` (unchanged); otherwise `mcp.Evaluate(name, args,
   serverPrincipal)`; deny → audit + `-32602
   "policy denied for tool <name>: <reason>"` **before `tool.run`**;
   allow → run → audit with the exit code.
2. `mcp.Evaluate` (`internal/mcp/policy.go`, pure, no I/O): known
   tool → every required scope held → path containment
   (`withinWorkspace` / repo-relative provenance) → traversal-free
   ids. Missing params pass through to the CLI's pinned usage errors
   (byte-identical behavior preserved).
3. `(CLI).mcpAuditDecision` (`internal/cli/mcp_policy.go`): one
   `decision` event per call into the F11.1 exec log — policy
   version, tool, allow/deny, reason (keys only), server principal,
   granted scopes, arg names, exit code (allowed calls). Secrets
   cannot leak: values are never recorded, `RecordExec` redacts
   pre-disk.

Read the audit trail:

```powershell
# decisions for one tool (jq-less: grep the JSONL)
Select-String ".regulus/observability/exec.jsonl" -Pattern '"tool":"gate_run"' | Select-Object -First 5
# or structured:
regulus trace timeline --run <run_id>
regulus trace prune-exec --keep 5000   # retention, same as F11.1
```

## 5. Adversarial suite (threat → test)

Unit (`internal/mcp/policy_test.go`): matrix drift both ways,
defaults authorize all 17 benign calls, deny-by-default (unknown tool,
locked-down principal), env parsing (unknown dropped, empty =
lockdown), delegation restriction + agent-chain sanitization, spoof
keys inert, tick dry/wet scopes, traversal refused for full
principals, workspace-contained absolute allowed / outside refused /
absolute provenance refused, reasons carry no values, `-32602` +
prefix wire shape.

Wire (`internal/cli/mcp_policy_test.go`): default allow audited
(byte-identical result + decision event with policy version, scopes,
arg names, no values); restricted deny before effects (nothing
stored, `missing_scope` audited); lockdown denies reads while
discovery survives; tool-spoofing names → `-32601` with no audit;
prompt injection stored verbatim as DATA with no authorization
change; confused-deputy containment (outside refused, inside
allowed); traversal table over every path-ish key (audited, nothing
stored); replay idempotent (2 decisions, 1 note); delegation cannot
escalate (Go API + wire); secrets never reach the audit log;
wet tick needs `evolve:write` (cursor untouched on deny).

## 6. Compatible migration

- **Wire unchanged for allowed calls**: `tools/list`, `initialize`,
  result shapes, usage errors (`-32602` messages), refusals-as-content
  and exit codes are byte-identical. The only new wire event is the
  `-32602 "policy denied for tool …"` refusal, distinguishable from
  usage errors by prefix.
- **Default behavior unchanged**: with `REGULUS_MCP_SCOPES` unset,
  every one of the 17 tools authorizes exactly as before (plus an
  audit line). Tighten by setting the variable; no flag or schema
  changes are required on clients.
- ** Rollout**: deploy the binary, confirm
  `regulus.mcp-policy/v1` appears in `.regulus/observability/exec.jsonl`
  decisions, then restrict scopes per deployment. Rollback is the
  previous binary (audit lines are additive JSONL; old readers ignore
  unknown `decision` events by type filter).
- **Docs**: this file amends `docs/devin.md` §"MCP trust boundary"
  (server now enforces; Devin permissions stay complementary). The
  `internal/mcp` "no auth handshake" test still passes: discovery
  payloads remain auth-free by design.
