# RegulusAI — Testing Assessment (Phase 0 audit)

Scope: committed tests only (45 Go test files + 4 Bun test files observed). No suite was executed by this
audit (Phase 0 rule: no invented results). Coverage numbers below are read from the committed
`.regulus/coverage.out` (total 83.9%, floor 80) — a recorded artifact, not a fresh run.

## 1. Inventory by level

| Level | What exists | Evidence |
|---|---|---|
| Unit (Go) | ~30 packages with `*_test.go`; `cli_test.go` alone has 95 tests incl. refusal paths | `grep -c func Test`: almanac 78 hits across files, cli 95, gates 17, budget 5, doctor 9, yolo 13 … |
| Unit (TS) | 4 files: `domain.test.ts`, `token-budget.test.ts`, `yolo-permissions.test.ts`, `devin-pack.test.ts` | `.opencode/lib/*.test.ts` |
| Contract | `contract_test.go` (2), `checkContract` cases in `domain.test.ts`, CI smoke (`contract` line in Devin blueprint) | `internal/contract/contract_test.go` |
| Integration | CLI end-to-end via `CLI.Run` with fixture dirs (memory/reindex/relate/status suites under `internal/cli/`) | `internal/cli/memory_*_test.go` |
| E2E journeys | Deterministic fixture-project journeys: F1 spine, F2/F3/F4, S8, V06/V07 (`internal/journey/`, 7 files) | `journey_test.go:TestF1VerticalSpine` + `f2/f3/f4/s8/v06/v07` |
| Mutation | gremlins, threshold efficacy 70, scope `internal/receipts`, workers 1, timeout-coefficient 10, 600s cap | `regulus.json` layer 6 |
| CRAP | max 30, top 5, gate layer 7; scorer unit-tested (6 tests) | `regulus.json`, `crap_test.go` |
| Security | refusal-path tests (doctor V1 refusal, `yolo` deny preservation, trace `nobody` refusal in CI smoke) — no adversarial/security suite | `version_test.go`, `yolo_test.go`, `gauntlet.yml` smoke |
| Concurrency | none (no parallel code paths; no `-race` gate) | architecture-assessment §6 |
| Perf | `perf_test.go`, `missrate_test.go` (retrieval depth/latency evidence) | `internal/almanac/*perf*, *missrate*` |
| Formatting | `gofmt` as gate layer 12 + `gofmt_test.go` | `regulus.json`, `internal/gates/gofmt_test.go` |

## 2. Coverage and determinism — strengths

- Floor 80 enforced as a metric gate reading the committed profile (`coverage-floor`, `last-line-percent`);
  recorded total 83.9%. Per-file tails observed at 65–100% (`trace.go` Fingerprint 100%, Prune 65.5%).
- Journeys pin exact outcomes (exit codes, receipt counts, refusal strings) against temp fixture projects —
  isolated (`t.TempDir`, `t.Chdir`), no network, no clock dependence except recorded timestamps.
- Mutation + CRAP + coverage triple-lock the "tests prove behavior" claim (Uncle Bob lineage): coverage says
  lines ran, mutants say assertions bite, CRAP says complex code is covered.

## 3. Gaps (each → backlog item)

1. **Mutation scope is narrow (G1).** Only `internal/receipts` is mutated. The most security-relevant logic
   (gates runner, surface whitelist, reflect evidence-gate, budget caps, MCP dispatch) has unit tests but no
   mutation proof that the tests would catch inverted conditions. → AUD-P2-01 (widen scope incrementally;
   cost is gremlins runtime, hence P2 not P1).
2. **No `-race` gate (G2).** Justified today (no parallel paths), but nothing would catch a future data race
   at introduction time. → AUD-P3-05 (cheap insurance when/if concurrency lands; do not add ceremony now).
3. **Plugin hooks untested (G3).** `regulus.ts` hook bodies (trace capture, context assembly, compaction
   resume, binary resolution, contract refusal) have no harness — `domain.ts` purity is tested, host-coupled
   `regulus.ts` is not. A broken hook fails silently (by design: never break tool execution) which hides
   regressions. → AUD-P3-04.
4. **MCP↔plugin parity untested (G4).** No test asserts the 16 plugin tools and 12 MCP tools cover the same
   CLI surface; drift already occurred (AUD-P0-01). A registry-diff test is cheap and mechanical. → folded
   into AUD-P1-06 acceptance criteria.
5. **Stale-index fallback untested (G5).** `status` reports "semantic index stale" but no journey pins the
   degraded-retrieval behavior contract (lexical fallback correctness + the advisory itself). → AUD-P2-06.
6. **Secret-redaction has no tests because there is no redaction (G6).** Tied to AUD-P2-03; test the deny/warn
   patterns once added.
7. **Destructive-command deny-lists tested only on the TS side** (`yolo-permissions.test.ts`) — the Go
   `yoloTemplate` deny set has `yolo_test.go` (13 tests, good). No gap; noted for completeness.
8. **No flaky-test or timing-sensitive tests observed** — timeouts are generous (600s gates, 15s probe) and
   journeys use fixture echoes, not real gates. Determinism posture is good.

## 4. Regression posture

- 20 committed charts + receipts on disk form a behavioral baseline; `release-0-9-0/V1` 12/12 is the
  release regression proof. Journey tests (F-series + version series V06/V07) act as cross-version
  regression locks.
- Weak spot: charts validate acceptance prose via `spec check` (shape, not semantics) — semantic drift
  ("scenario passes but proves nothing") is caught only by guardian review + mutation, both human/gate
  dependent. Acceptable for the design; no action beyond keeping mutation scope honest (G1).

## 5. Verdict

Testing is the strongest pillar of this repo: layered, deterministic, receipt-pinned, with the floor
enforced in CI rather than aspirational. Gaps are all "prove more of what exists" (mutation scope, parity
test, stale-index contract, hook harness) — no area was found with zero coverage. No test results were
produced or invented by this audit; re-running `gate run` + `bun test` is the reader's one-command
verification (`improvement-backlog.md` verification appendix).
