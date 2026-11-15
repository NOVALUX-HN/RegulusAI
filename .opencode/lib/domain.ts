// Regulus domain helpers — pure functions (no OpenCode imports) so the
// detection logic stays testable from plain Bun scripts.
//
// A Chart declares its lesson domain with one line:
//
//   Domain: go
//
// The context hook reads that line from the Chart the session works on
// and injects that domain's lessons instead of the newest of all
// domains. The line is optional: charts without it keep the old
// newest-first behavior.


// REGULUS_CONTRACT is the plugin<->core protocol version this backend
// build speaks. Mirrors the Go core's internal/contract.Version: the
// CLI prints it for `regulus contract`, and the plugin refuses the
// session's regulus tools unless the core speaks exactly this value.
export const REGULUS_CONTRACT = "regulus.contract/v1"

// checkContract turns the result of `regulus contract` into a refusal
// message, or "" when the core speaks exactly the required version.
// Fail-closed: an older binary without the contract command cannot
// prove the contract — the docs' promise is the contract — so it is
// refused with guidance naming the required version.
export function checkContract(result: { exitCode: number; output: string }): string {
  const spoken = result.output.trim();
  if (result.exitCode === 0 && spoken === REGULUS_CONTRACT) return ""
  if (result.exitCode !== 0) {
    return (
      `regulus: contract mismatch — this binary does not answer the contract check (exit ${result.exitCode}), ` +
      `an older core cannot prove ${REGULUS_CONTRACT}. ` +
      `The plugin refuses an unverifiable core: update the regulus binary (rebuild ./cmd/regulus or refresh bin/ plus binaries/).`
    );
  }
  return (
    `regulus: contract mismatch — the core speaks "${spoken}", the plugin requires ${REGULUS_CONTRACT}. ` +
    `Update the regulus binary so core and plugin speak the same contract version.`
  );
}

// COMPACTION_CONTEXT_LIMIT/BUDGET bound the compaction resume block
// (T1): the hook injects the latest 20 notes and 20 items per long
// list instead of the whole store — mirroring the Go core's
// DefaultContextLimit/DefaultContextBudget (parity pinned by the
// compaction-args tests below and the Go parity test). Reversal: set
// REGULUS_COMPACTION_FULL=1 for the explicit-full resume
// (--limit 0 --budget 0); anything else stays bounded.
export const COMPACTION_CONTEXT_LIMIT = 20;
export const COMPACTION_CONTEXT_BUDGET = 20;

// compactionContextArgs builds the compaction hook's `memory context`
// argv: bounded by default, explicit-full when
// REGULUS_COMPACTION_FULL=1. Pure (env injectable) so bun tests pin
// the default and the reversal without spawning.
export function compactionContextArgs(env?: string): string[] {
  if ((env ?? process.env.REGULUS_COMPACTION_FULL ?? "") === "1") {
    return ["memory", "context", "--limit", "0", "--budget", "0"];
  }
  return ["memory", "context", "--limit", String(COMPACTION_CONTEXT_LIMIT), "--budget", String(COMPACTION_CONTEXT_BUDGET)];
}

// contextResolveArgs builds the single-spawn context hook argv (T1
// speed-tokens): `memory context --resolve-domain` resolves the lesson
// domain in-process (chart pins, lone chart, single library domain,
// trace file-hits) and renders context + scoped lessons in one spawn.
// Pure so bun tests pin the argv without spawning.
export function contextResolveArgs(opts?: { sessionID?: string; charts?: string[]; limit?: number; budget?: number; lessons?: number }): string[] {
  const args = ["memory", "context", "--resolve-domain"];
  if (opts?.sessionID) args.push("--session", opts.sessionID);
  for (const c of opts?.charts ?? []) args.push("--chart", c);
  args.push("--limit", String(opts?.limit ?? COMPACTION_CONTEXT_LIMIT), "--budget", String(opts?.budget ?? COMPACTION_CONTEXT_BUDGET), "--lessons", String(opts?.lessons ?? 3));
  return args;
}

export { findChartRefs, findDomainInChart, extractPaths, loadSkills, distinctiveWords, suggestSkills, filterSeenSkills } from "./domain-charts.js";
export type { Skill, RunBin } from "./domain-charts.js";
export { GATE_RUN_TIMEOUT_MS_DEFAULT, GATE_RUN_TIMEOUT_MS_FLOOR, gateRunTimeoutMs, GATE_RUN_OUTPUT_MAX, formatGateResult, BLOCKED_SPAWN_RE, isBlockedSpawn, explainSpawnError, formatGateFailure, formatResolveLog } from "./domain-gates.js";
export { DISCIPLINE_MARKER, loadDiscipline, parseToolGuidance, loadToolGuidance, dataBlock, candidates, staleDoctorRefusal } from "./domain-guidance.js";
export type { ProbeRun } from "./domain-guidance.js";
export { pickRegulus } from "./domain-runtime.js";
export { traceRecordArgvToEntry, TraceBatcher, clearDomainHitCache, clearDomainsCache, withTimeout, TtlCache, resolveDomain } from "./domain-trace.js";
export type { TraceEntry, TraceFlushResult } from "./domain-trace.js";
export { memoryHistoryArgs, memorySaveArgs, memorySearchArgs } from "./domain-memory.js";
export { LOOPGUARD_THRESHOLD, LOOPGUARD_MAX_SESSIONS, normalizeLoopMessage, loopSignature, isLoopGuardEnabled, formatLoopRedirect, LoopGuard } from "./domain-loopguard.js";
export { TOOLSET_FULL, TOOLSET_MINIMAL, resolveToolset, isToolEnabled, toolsetTools, MINIMAL_CREW_LINE, formatDisciplineForToolset } from "./domain-toolset.js";
export type { ToolsetMode } from "./domain-toolset.js";
export { PRUNE_MIN_SESSIONS, parseTraceUsage, applyUsagePruning, clearUsageCache, getCachedUsage } from "./domain-usage.js";
export type { TraceToolUsage } from "./domain-usage.js";
