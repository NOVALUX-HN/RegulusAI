# Capability inventory — F16.1 audit (NOV-134)

Baseline: `develop`, audit performed at `HEAD 5d57337`, committed as `45a0b8a`, clean tree,
2026-11-25. Audit-only artifact:
every claim below cites code that was read in this checkout. Nothing here is a
roadmap promise; the gaps are what F16.2–F16.4 may act on, and only if they
survive their own evidence check.

Scope reminder from the parent phase: reuse F6/F11. Do not rebuild adapters, do
not promise parity, do not add hosts without demonstrated need.

## 1. What already exists (do not rebuild)

| Surface | Location | What it provides |
|---|---|---|
| Plugin↔core contract | `internal/contract/contract.go:8` | `Version = "regulus.contract/v1"`, exact-match `Supports` |
| MCP contract | `internal/mcp/version.go:7` | `ContractVersion = "regulus.mcp/v1"`, negotiate/refuse |
| Adapter contract | `internal/runtime/adapter.go:95` | `Adapter{Name,Capabilities,Open}`, `Session{CheckPermission,Provide,Context,Report,Fail,Close}` |
| Negotiation | `internal/runtime/negotiation.go:8` | `Negotiate(required,offered)` returns granted+missing, never assumes |
| Adapter registry | `internal/runtime/registry.go:8` | self-registering; refuses empty name / nil adapter |
| Capability matrix | `internal/bench/hostcapabilities.go:37` | 12 dimensions, 4 host manifests, fold rule `ApplyJourneyOutcome` |
| Journey probes | `internal/bench/hostjourney.go` | `ProbeHost`, `DefaultHostProbes`, `executeHostJourney` |
| MCP server-side policy | `internal/mcp/policy.go:24` | deny-by-default, `regulus.mcp-policy/v1`, closed scopes |
| Skill registry | `internal/skills/registry.go:16` | scans per-host skill dirs, metadata only |
| Host evidence | `internal/hostcfg/evidence.go:32` | read-only redacted snapshots, `observed` vs fixture |
| Rendered matrix | `docs/conformity-matrix.md:49` | 4 hosts × 12 dimensions + reconciliation table |

## 2. Host reality (what actually executes)

| Host | Declared as | How it really runs | Limitation |
|---|---|---|---|
| OpenCode V2 | primary, `probada` | `.opencode/plugins/regulus.ts` (TypeScript plugin) | The only product adapter; not a Go `runtime.Adapter` |
| Codex | `probada` ×12 | MCP stdio (`.codex/config.toml`) + `.agents/skills/` | No `runtime.Adapter`; hooks/subagents/limits are version-gated or inline |
| Claude Code | `probada` ×12 | MCP (`.mcp.json`) + `.claude/skills`, `.claude/agents` | Same: no Go adapter, capabilities are config-level |
| Devin | mixed `no-ejecutada` / `no-soportada` / `desconocida` | `.devin/` pack + docs, desktop-only | No non-interactive CLI, so nothing is ever observed |
| HTTP | minimal read-only | `internal/httpapi/server.go:1` | Proves the envelope, not host support |

`internal/runtime/registry.go:8` states it plainly: *"no production code in this
package registers anything… Today that means no Go adapter at all."*

## 3. Capability state vocabulary: implemented vs required

Implemented — exactly four words, and the file forbids a fifth
(`internal/bench/hostcapabilities.go:59`: *"the only four words the matrix may contain"*):

```
probada · no-soportada · no-ejecutada · desconocida
```

Required by F16.2 (spec S4): `unavailable`, `partial`, `inferred`, `verified`, `degraded`.

| Required state | Nearest existing | Status |
|---|---|---|
| `verified` | `probada` | exists, different word |
| `unavailable` | `no-soportada` | exists, different word |
| `inferred` | — | **missing** |
| `partial` | — | **missing** |
| `degraded` | — | **missing** |

## 4. Gap G1 — evidence attribution over-claims (the substantive one)

`hostcapabilities.go:118-125` defines the dimensions a journey actually exercises and
says in its own comment: *"Hooks, subagents, permissions and limits are declared,
never journey-met."*

`JourneyCoveredCaps()` therefore covers 8 of 12: install, sessions, tools, gate, close,
tokens, filesystem, MCP.

But **all three** host manifests mark all twelve as `probada` — `opencodeCaps`
(`:147-159`) exactly like `codexCaps` (`:161-174`) and `claudeCaps` (`:176-189`) — and for
the four uncovered dimensions they cite journey evidence anyway:

```go
verified(CapHooks,       "version-gated: …", "contract tests + " + journeyGreen)
verified(CapSubagents,   "version-gated: …", "contract tests + " + journeyGreen)
verified(CapPermissions, "respeta permisos…", "contract tests + " + journeyGreen)
verified(CapLimits,      "presupuestos y cotas…", "contract tests + " + journeyGreen)
```

`journeyGreen` is a single fixture citation (NOV-76, 2026-11-16). So **three hosts × four
dimensions = twelve claims** assert a verified state backed by evidence the same file says
does not cover them. `ValidateCapabilitiesAttributable` cannot catch this: it validates
that the *word* is in the allowlist, not that the citation supports the dimension.

`ValidateNoProbeAsJourney` (`:345`) is the nearest guard and does not help either: it
builds the covered set from `JourneyCoveredCaps()` (`:346-349`) but applies it only to
`ClassProbe` manifests (`:357`), so the journey-class manifests carrying the over-claim are
never checked. **G1 currently has zero enforcement.**

A second symptom points at the same missing vocabulary: Codex and Claude declare
`CapTokens` as `probada` while their own detail text says *"receipts con tokens cuando el
host los expone, unknown si no"*. A state that means "verified, unless it is not" is
exactly what `inferred` and `partial` exist for.

This is the gap F16.2 must close: an evidence field that can only assert what the cited
run actually exercised, plus the states needed to say "declared but not observed"
(`inferred`) instead of overloading `probada`.

## 5. Gap G2 — "native adapters" means two different things

`README.md:20` says *"Codex, Claude Code and Devin work through their native adapters
(AGENTS.md, CLAUDE.md, `.devin/`)"*, while `internal/runtime/registry.go:8` and
`docs/adr/T7-01-adapter-contract.md:17` state the registry is empty and no Go adapter
exists.

Both are true with different meanings: an *integration surface* (config + skills + MCP)
versus an *`runtime.Adapter` implementation*. The word "adapter" is overloaded, which
invites exactly the parity claim F16's exclusions forbid.

Not a code defect. A wording fix, cheap and safe, and it should land alongside F16.2 so
the vocabulary and the docs agree.

## 6. Gap G3 — Devin is fully declared, never observed

`DefaultHostManifests` declares Devin across the 12 dimensions as **7 not-executed, 3
unsupported and 2 unknown** — zero `probada` — and `hostjourney.go:83` records the reason:
*"desktop-only, no non-interactive CLI"*. The probe always fails, so no Devin capability
is ever observed — the row is strategy documentation, not evidence.

This is the honest state already. The gap is presentation, not detection: a reader
cannot tell "not supported" from "not observable here" without reading the detail string,
because the vocabulary has no `inferred`/`unavailable` distinction for that case.

## 7. What each follow-up task may do

| Task | May do | Evidence it must produce |
|---|---|---|
| F16.2 (NOV-135) | Add the missing states; make `Evidence` assert only what the cited run covered; fix the README wording | A test that rejects a `verified` claim whose citation does not cover the dimension |
| F16.3 (NOV-136) | Controlled fallback when a capability is missing/partial; core must not collapse | A degraded-path journey proving the core keeps working |
| F16.4 (NOV-137) | Fixtures for trust boundary, capability mismatch, malformed event/result, lifecycle errors | Contract tests that fail when the boundary is removed |

## 8. Explicit non-gaps (do not spend effort here)

* The contract versions and their exact-match refusal — working as designed.
* `Negotiate` returning granted+missing — no silent assumption; correct.
* MCP server-side policy and its closed scope vocabulary — intact.
* `ApplyJourneyOutcome` fold rule (probe never promotes or demotes) — correct.
* The four-word vocabulary itself is not wrong, it is *insufficient*; replacing it wholesale
  is a contract change and must be justified by F16.2, not assumed here.

## 9. Verification of this audit

```bash
bash scripts/verify-wsl.sh ./internal/bench/ ./internal/runtime/ ./internal/mcp/ ./internal/skills/
```

Result recorded with the task evidence. This document changes no code: it is the
deliverable F16.1 asks for ("inventario por host/capability con ubicación en código,
pruebas, evidencia, limitaciones y gap real").

## 10. Status after F16.2

* **G1 closed.** The twelve claims are now `soportada`, except the version-gated Codex
  hooks and subagents, which are `inferida`. `ValidateEvidenceCoverage` fails any
  `probada` whose evidence cites the journey for a dimension `JourneyCoveredCaps` does
  not cover, and it runs next to the other validators in `cmdx`, not only in tests.
* **G2 closed.** README no longer calls the config/MCP surface a "native adapter".
* **G3 closed.** The wider vocabulary now separates the cases a reader could not tell apart:
  `no-soportada` (absent), `no-ejecutada` (declared, never attempted here) and
  `desconocida` (no information), with the legend in `docs/conformity-matrix.md`
  explaining each.
* **Fallback (F16.3).** `orchestrate` classifies capabilities by state instead of
  accepting `probada` alone. `degradada`, `parcial`, `soportada` and `inferida` route
  carrying an explicit restriction; only `no-soportada`, `no-ejecutada` and `desconocida`
  are excluded. Before this, the filter at `orchestrate/registry.go` dropped everything
  that was not `probada`, so a capability turning partial silently removed the dimension
  from the agent and the core lost the work instead of degrading it.

### Known limits found by the phase review

* **Validation is single-path.** The three manifest validators run inside `bench
  host-capabilities` only. `orchestrate`, `execute/routing.go`, `planning/planner.go`,
  `mission_plan.go` and `cmdx/recovery.go` build manifests from `DefaultHostManifests`
  without validating them. The shipped defaults are clean and the journey fold promotes
  only covered dimensions, so nothing wrong ships today, but a hand-built or future
  manifest reaches the matrix and the router ungated.
* **The coverage guard keys on the journey citation.** `ValidateEvidenceCoverage` rejects
  a `probada` whose evidence contains the journey citation on an uncovered dimension. A
  `probada` on an uncovered dimension citing something else still passes; only a
  dedicated observation such as an agent journey should ever support that claim, and no
  such evidence exists today.
* **Assignment still needs `probada`.** `orchestrate` routes restricted states, but
  `Decide`/`checkVerified` only assign probada-plus-evidence, and
  `recoveryTargetCaps` filters its `verified` set back to probada. Routing a
  non-observed capability never authorizes required-cap work.
