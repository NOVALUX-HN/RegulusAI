// Usage parsing and measured-use pruning (NOV-67 split).

import type { RunBin } from "./domain-charts.js";
import { TOOLSET_MINIMAL, toolsetTools } from "./domain-toolset.js";
import type { ToolsetMode } from "./domain-toolset.js";

// TraceToolUsage is one row of `trace usage --json`: a host-side tool
// name with its measured calls and distinct session count.
export interface TraceToolUsage {
  tool: string;
  calls: number;
  sessions: number;
}

// PRUNE_MIN_SESSIONS is the evidence floor for usage pruning: below
// it nothing prunes — a fresh repo must not lose tools for lack of
// history. Sessions are cumulative (no recency): a pruned tool
// returns by itself once traces show calls again, and TE_TOOLSET=full
// restores everything in one step.
export const PRUNE_MIN_SESSIONS = 3;

// parseTraceUsage reads `trace usage --json` defensively: any shape
// violation (bad JSON, missing sessions count, ragged rows) yields an
// empty report, and the pruner fails open to the full set — measuring
// must never break setup.
export function parseTraceUsage(text: string): { sessions: number; tools: TraceToolUsage[] } {
  try {
    const raw = JSON.parse(text) as { sessions?: unknown; tools?: unknown };
    if (typeof raw !== "object" || raw === null) return { sessions: 0, tools: [] };
    if (typeof raw.sessions !== "number") return { sessions: 0, tools: [] };
    if (!Array.isArray(raw.tools)) return { sessions: 0, tools: [] };
    const tools: TraceToolUsage[] = [];
    for (const row of raw.tools) {
      if (typeof row !== "object" || row === null) continue;
      const r = row as { tool?: unknown; calls?: unknown; sessions?: unknown };
      if (typeof r.tool !== "string" || typeof r.calls !== "number") continue;
      const n = Array.isArray(r.sessions) ? r.sessions.length : 0;
      tools.push({ tool: r.tool, calls: r.calls, sessions: n });
    }
    return { sessions: raw.sessions, tools };
  } catch {
    return { sessions: 0, tools: [] };
  }
}

// applyUsagePruning trims the full set by measured use (T3): a
// full-only tool with zero recorded calls across the evidence floor
// is left out and named in pruned. The minimal set never prunes
// (safe floor); explicit modes skip measuring; thin or malformed
// usage fails open to the unpruned set. Trace names are host-side,
// so a regulus tool matches by suffix (regulus_gate_run covers
// gate_run); an exact match counts too.
export function applyUsagePruning(
  mode: ToolsetMode,
  usage: { sessions: number; tools: TraceToolUsage[] },
  explicit = false,
): { tools: readonly string[]; pruned: string[] } {
  const base = toolsetTools(mode);
  if (mode !== "full" || explicit) return { tools: base, pruned: [] };
  if (usage.sessions < PRUNE_MIN_SESSIONS) return { tools: base, pruned: [] };
  const called = (name: string) =>
    usage.tools.some((u) => u.calls > 0 && (u.tool === name || u.tool.endsWith(`_${name}`)));
  const tools = base.filter(
    (name) => (TOOLSET_MINIMAL as readonly string[]).includes(name) || called(name),
  );
  const pruned = base.filter((name) => !tools.includes(name));
  return { tools, pruned };
}

// usageCache (T6 speed-tokens): memoizes the parsed `trace usage
// --json` report per project dir for the life of the host process.
// Every session setup used to pay one spawn for usage pruning; now
// only the first setup per dir spawns and every later setup for the
// same dir resolves with zero spawns (pinned by lib/toolset.test.ts).
//
// Lazy-vs-cache decision: true laziness — deferring the full/minimal
// decision until the first actual tool call — is infeasible in this
// host lifecycle. Tools register once per project load in the sync
// `transform` callback (no session, no async spawn possible; see
// TOOLSET_FULL above), so the decision must stay load-time and only
// the measurement is cached instead. Failures (spawn throw, non-zero
// exit, malformed JSON) never pin the cache: the setup fails open to
// the unpruned set and the next setup retries.
const usageCache = new Map<string, { sessions: number; tools: TraceToolUsage[] }>();

export function clearUsageCache(): void {
  usageCache.clear();
}

export async function getCachedUsage(
  dir: string,
  runBin: RunBin,
): Promise<{ sessions: number; tools: TraceToolUsage[] } | null> {
  const hit = usageCache.get(dir);
  if (hit !== undefined) return hit;
  let res: { exitCode: number; output: string };
  try {
    res = await runBin(["trace", "usage", "--json"]);
  } catch (err) {
    console.error(`[regulus] toolset: usage pruning skipped (${(err as Error)?.message ?? String(err)}) — full set kept`);
    return null;
  }
  if (res.exitCode !== 0) return null;
  const parsed = parseTraceUsage(res.output);
  usageCache.set(dir, parsed);
  return parsed;
}
