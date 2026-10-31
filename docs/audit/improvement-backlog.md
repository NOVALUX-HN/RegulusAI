# RegulusAI — Improvement Backlog (Phase 0 audit)

Priorities: **P0** critical/blocking · **P1** architectural · **P2** important improvement ·
**P3** future optimization. Dependencies named by ID. No code changed by this audit.

## P0 — critical / blocking

**Verdict: no P0 found.** The core builds, the 12-gate gauntlet closed 12/12 at release (`CHANGELOG.md`
0.9.0), coverage 83.9 ≥ 80, doctor/contract/refusal paths tested. The closest candidate was evaluated and
demoted:

- *Candidate: MCP/plugin tool parity (4 tools missing in MCP).* Real gap, but Devin surfaces function for
  their documented scope (gates, memory, traces, doctor); evolution/probe via MCP is enhancement, not outage.
  → Tracked as **AUD-P1-01**, not P0. Downgrade rationale recorded so a future reader can re-raise it if a
  Devin flow is blocked by a missing tool.

## P1 — fundamental for the architecture

### AUD-P1-01 — MCP tool parity (4 tools missing)
- Module: `internal/cli/mcp.go` vs `.opencode/plugins/regulus.ts`.
- Problem: plugin exposes 16 tools; MCP lists 12. Missing: `evolve_propose`, `probe_contrast`,
  `probe_compose`, `skills_list`.
- Evidence: `mcp.go:243-392` (12 `name:` entries) vs `regulus.ts:207-533` (16 `editor.add` blocks).
- Severity/Impact: Medium. Devin CLI/Desktop/Cloud cannot assemble evolution packs or run probes; the
  Harness² loop is headless-inaccessible.
- Dependencies: none (additive).
- Solution: register the 4 missing tools via existing `mcpExec` paths; extend the 0.8.1 sync guard to assert
  MCP↔plugin↔CLI parity.
- Acceptance: `tools/list` returns 16; new tests mirror `mcp_test.go` style per tool; `docs/devin.md` table updated.

### AUD-P1-02 — Stale "two slash commands" header comment
- Module: `.opencode/plugins/regulus.ts:5`, `docs/architecture.md` (hook/command counts).
- Problem: header says two slash commands; code registers three (`regulus-doctor`, `regulus-verify`,
  `regulus-gauntlet`, `regulus.ts:539-576`).
- Evidence: `regulus.ts:5` vs `:539-576`. `docs/agents.md` command table lists all three (docs right, comment wrong).
- Severity/Impact: Low severity, P1 priority — the boundary file's own contract comment is wrong; reviewers
  cannot trust header counts.
- Dependencies: none.
- Solution: fix comment to "three slash commands" + names; add a `domain.test.ts`-style assertion or
  `sync-plugin-defaults` check that command count matches a constant.
- Acceptance: comment lists all three names; a test fails if a fourth command is added without updating it.

### AUD-P1-03 — Receipt integrity is hash-pinned, not signed
- Module: `internal/receipts/receipts.go` (`Capture`, SHA-256 stored inside the receipt).
- Problem: rewriting a receipt file + recomputing hashes is undetectable from the file alone; tamper-evidence
  rests on git history/CI artifacts.
- Evidence: `receipts.go: Capture`; architecture-assessment §9.2; security-assessment F8.
- Severity/Impact: Medium. Fine for personal use; insufficient if receipts ever cross trust boundaries.
- Dependencies: none.
- Solution (opt-in, do NOT default-on): local HMAC key (`~/.regulus/key`) + `evidence verify` command;
  document the threat model first.
- Acceptance: doc states what receipts prove and against whom; if implemented, `evidence verify` passes on
  the 569 committed receipts and fails on a tampered copy.

### AUD-P1-04 — Verify MCP permission enforcement per Devin client
- Module: docs + Devin clients (`internal/cli/mcp.go:10-13`, `docs/devin.md`, `.devin/mcp_config.json`).
- Problem: server enforces nothing by design; the claim "clients enforce" is unverified per client.
- Evidence: `mcp.go:10-13` trust-boundary comment; security-assessment §9.
- Severity/Impact: Medium (High if any client is permissive). One unenforcing client = full CLI power.
- Dependencies: none (process, not code).
- Solution: per-surface checklist (CLI/Desktop/Cloud) proving destructive denies hold through the MCP path;
  record results in `docs/devin.md`.
- Acceptance: checklist committed with dates/versions; any gap gets its own P0.

### AUD-P1-05 — Make `regulus.json` shell-trust explicit on first run
- Module: `internal/gates` (`ShRunner`, `gates.go:80-134`), `regulus init`, docs.
- Problem: cloning a repo and running the gauntlet executes its commands with user privileges; nothing
  prompts a first-run audit on untrusted clones.
- Evidence: `gates.go:80-134`; dependency-map §5 (highest-privilege file).
- Severity/Impact: Medium. By design for a dev harness; the fix is visibility, not restriction.
- Dependencies: none.
- Solution: `gate run` prints the resolved command list and, when `.regulus/` has no prior receipts for the
  repo (first run), requires `--yes`/`--audit` acknowledgement; document in README quickstart + `init`.
- Acceptance: journey test pins first-run refusal + acknowledgement flow; README documents it.

### AUD-P1-06 — Single tool manifest (kill registry triplication)
- Module: `internal/cli/*`, `regulus.ts` tools block, `mcpTools()`, `docs/agents.md` table, `tool-guidance.md`.
- Problem: adding one tool edits 3 registries + 2 docs by hand; drift already happened twice (16-vs-12,
  16-vs-`agents.md` pre-0.8.1).
- Evidence: architecture-assessment §3 (blemish 1), §7 (friction 1); 0.8.1 changelog parity saga.
- Severity/Impact: Medium. Structural tax on every future tool.
- Dependencies: none (enables AUD-P1-01 permanently).
- Solution: one manifest (e.g. `tools.json`: name, description key, params, surfaces) consumed by CLI help,
  plugin registration, MCP `tools/list`, and docs generation (`sync-plugin-defaults.ts` extended).
- Acceptance: parity test fails on any divergence; adding a tool touches manifest + handler only.

## P2 — important improvements

### AUD-P2-01 — Widen mutation scope beyond `internal/receipts`
- Module: `regulus.json` layer 6; `internal/gates`, `surface`, `reflect`, `budget`, `mcp`.
- Problem: efficacy-70 proof covers only receipts; inverted conditions in gates/surface/reflect/budget/MCP
  dispatch are unit-tested but not mutation-proven.
- Evidence: `regulus.json` mutation command (`./internal/receipts`); testing-assessment G1.
- Severity/Impact: Medium (confidence, not correctness). Cost: gremlins runtime per package.
- Dependencies: none.
- Solution: add packages one at a time (`gates` first), keep per-package thresholds honest; record runtime.
- Acceptance: each added package has a threshold + green receipt; README/CI notes total runtime.

### AUD-P2-02 — Committed binaries drift control
- Module: `binaries/`, `bin/`, `scripts/build.bash`, `internal/doctor` (stale/unstamped warnings).
- Problem: 6 release binaries committed; nothing proves they match source. Doctor warns, CI doesn't verify.
- Evidence: `ls binaries` (6 files); `build.bash`; implementation-status §16.
- Severity/Impact: Medium-Low (supply chain). Mitigated by build-from-source docs.
- Dependencies: none.
- Solution (either): CI rebuild-and-compare job (bit-for-bit or version+smoke), or stop committing binaries
  and publish checksums instead.
- Acceptance: CI fails on drift, or binaries removed from git with reproducible build doc.

### AUD-P2-03 — Secret hygiene for `memory save`
- Module: `internal/almanac`, `internal/cli/memory*.go`, MCP `memory_save`.
- Problem: observation content stored verbatim; pasted secrets persist in plaintext JSONL, committed if
  exported (`memory export`).
- Evidence: no redaction pass found in almanac/memory paths; security-assessment F7.
- Severity/Impact: Medium-Low. No exfiltration path, but the store invites accidental secret persistence.
- Dependencies: none.
- Solution: deny/warn patterns for likely-secret shapes (keys, tokens) at save time + `memory doctor` check;
  document that Almanac is not a secret store.
- Acceptance: tests pin refusal/warning on fixtures; `troubleshooting.md` documents it.

### AUD-P2-05 — Bound worker surfaces (convention → verification)
- Module: crew agents (`.opencode/agents/regulus-worker.md`), `trace files`, Evolution.
- Problem: "one bounded unit" is prompt text; nothing checks post-hoc what a worker touched.
- Evidence: no surface measurement in code; security-assessment F6.
- Severity/Impact: Medium-Low. Host permissions bound damage; this is about discipline proof.
- Dependencies: none.
- Solution: after each worker unit, record `trace files` + diff stat against the Chart's declared paths in
  the task receipt/log; flag out-of-surface touches for reviewer.
- Acceptance: journey test demonstrates an out-of-surface touch being flagged.

### AUD-P2-06 — Pin the stale-index fallback contract
- Module: `internal/almanac` (hybrid/vindex), `memory reindex`, `status`.
- Problem: "semantic index stale → lexical fallback" is observed behavior with no test locking it in.
- Evidence: live `status` output; testing-assessment G5.
- Severity/Impact: Low. Degraded recall quality, correctly reported.
- Dependencies: none.
- Solution: journey test — stale index → advisory printed + lexical results still correct; document in
  `memory-v2.md`.
- Acceptance: test green; doc states the fallback contract.

### AUD-P2-07 — Backup/restore story for `.regulus/`
- Module: `.regulus/` (evidence, almanac, library, traces), `memory export|import`.
- Problem: only memory has export/import; receipts/traces/library loss (deleted dir, Cloud immutable
  snapshots per `docs/devin.md`) has no documented recovery.
- Evidence: dependency-map §5 blast radius; `devin.md` Cloud caveat.
- Severity/Impact: Low-Medium (data loss scope: harness state, not user code).
- Dependencies: none.
- Solution: document per-store recovery (what is regenerable: INDEX/vindex/retrieval.bin; what is not:
  receipts/traces) + extend export bundle or document CI-artifact restore.
- Acceptance: `troubleshooting.md`/runbook section with restore commands, tested once manually.

### AUD-P2-08 — Tandem DATA-framing for `memory import`
- Module: `internal/almanac/import.go`, plugin context hooks, `memory import`.
- Problem: imported bundles (possibly from other repos) enter context; framing/provenance at import time
  not verified by this audit.
- Evidence: security-assessment F9 (residual risk noted, import path not fully traced).
- Severity/Impact: Low (prompt-injection hygiene).
- Dependencies: none.
- Solution: verify import path frames provenance + DATA boundaries; show origin (repo, git HEAD, date) in
  `memory context` for imported observations.
- Acceptance: imported observation visibly cites origin; test pins the framing.

## P3 — future optimization

- **AUD-P3-01** — Almanac v2.1 ONNX (already PROPOSED in `memory-v2.md` S8). Keep deferred until scale
  demands; no action.
- **AUD-P3-02** — Hash-pin CI actions (vs floating majors). Supply-chain strictness; do when release
  posture demands. Evidence: `gauntlet.yml` S4 comment.
- **AUD-P3-03** — Least-privilege YOLO profile (per-command allowlist instead of `bash "*"`). Convenience
  vs privilege; revisit after F6 evidence. Evidence: `yolo.go`, `yolo-permissions.test.ts`.
- **AUD-P3-04** — Plugin-hook test harness (trace/context/compaction/binary-resolution without a host).
  Would catch silent hook regressions; cost is harness complexity. Evidence: testing-assessment G3.
- **AUD-P3-05** — `-race` gate when/if concurrency lands. No parallel paths today; add with the first one.
  Evidence: architecture-assessment §6.

## Dependency order (recommended)

1. AUD-P1-06 (manifest) → makes AUD-P1-01 permanent and prevents recurrence.
2. AUD-P1-01 + AUD-P1-02 + AUD-P1-05 (parity, comment, first-run trust) — independent, small.
3. AUD-P1-04 (client checklist) before widening MCP surface via AUD-P1-01 usage growth.
4. AUD-P1-03 only if receipts cross trust boundaries (else document-and-defer).
5. P2 in numeric order, except AUD-P2-06 before any retrieval tuning and AUD-P2-08 with the next import change.
6. P3 as triggered by scale, release posture, or evidence — not scheduled.

## Verification appendix (reader's one-command checks — not run by this audit)

- `go build ./... && go vet ./...`
- `go test ./... -coverprofile .regulus/coverage.out` then `go tool cover -func .regulus/coverage.out | tail -1` (expect ≥ 80)
- `bunx tsc --noEmit -p .opencode/tsconfig.json` and `bun test ./.opencode/lib/` (from `.opencode/`)
- `bin/regulus gate run <task>` on a fixture copy (never on the repo root blindly — see AUD-P1-05)
- `bin/regulus status` (runtime, lessons, receipts, stale-index advisory)
