// Toolset projection and discipline shaping (NOV-67 split).

// TOOLSET_FULL / TOOLSET_MINIMAL / resolveToolset — T2 (macizo-tokens):
// subsets por modo con probe de la API.
//
// Probe (OpenCode plugin SDK 2.2.0,
// `.opencode/node_modules/@opencode/plugin/dist/promise/` +
// `@opencode/schema/dist/tool.d.ts`): tools register once per project
// load — `setup` receives no session (`plugin.d.ts` Context carries
// app/location/options/domains, no session), the `transform` callback is
// synchronous `(editor: ToolEditor) => void` (`registration.d.ts`) with
// no session either, and `Tool.Options` carries no visibility/hidden
// flag (only namespace/permission/codemode/pinned). Tool hooks only
// observe execution (`execute.before`/`execute.after`); they cannot
// unpublish definitions. So per-session conditional registration
// (chart activo vs sesión simple switched at runtime) is NOT possible
// with this API — the Then-alternative applies for that branch.
//
// What the API does allow is load-time subset selection: the transform
// callback runs in our process, so it can branch on process-global
// state (env) and on the project tree (charts/ present). TE_TOOLSET
// selects the subset; auto mode serves the full set when the project
// has charts and the minimal set otherwise. Reversal: TE_TOOLSET=full
// restores all 16 definitions; unset restores auto.
export const TOOLSET_FULL: readonly string[] = [
  "gate_run",
  "spec_check",
  "doctor",
  "memory_note",
  "memory_show",
  "memory_save",
  "memory_search",
  "memory_context",
  "memory_history",
  "evolve_propose",
  "probe_contrast",
  "trace_show",
  "skills_list",
  "trace_sessions",
  "trace_files",
  "probe_compose",
  "audit",
  "evolve_report",
  "evolve_tick",
];

// TOOLSET_MINIMAL is the simple-session subset: verify the install
// (doctor), search the Almanac (memory_search), resume after restart
// (memory_context). Everything chart/gauntlet/probe/trace stays
// full-only. There is no `status` tool: `doctor` covers the install
// check the spec example names.
export const TOOLSET_MINIMAL: readonly string[] = ["doctor", "memory_search", "memory_context"];

export type ToolsetMode = "full" | "minimal";

// resolveToolset selects the subset: explicit TE_TOOLSET=full|minimal
// wins (case/whitespace-tolerant); TE_TOOLSET=auto, unset, or blank
// auto-detects from the project tree (charts present → full). An
// unknown value fails closed to full with a stderr warning — a typo
// must never strip tools. hasCharts defaults to true (fail-closed)
// when the caller cannot detect; setup passes the real scan.
export function resolveToolset(opts?: { env?: string; hasCharts?: boolean }): ToolsetMode {
  const raw = (opts?.env ?? process.env.TE_TOOLSET ?? "").trim().toLowerCase();
  if (raw === "minimal") return "minimal";
  if (raw === "full") return "full";
  if (raw !== "" && raw !== "auto") {
    console.error(`[regulus] toolset: unknown TE_TOOLSET="${raw}" — fail-closed full (want full|minimal|auto)`);
    return "full";
  }
  return (opts?.hasCharts ?? true) ? "full" : "minimal";
}

// isToolEnabled is the registration allowlist: full mode still checks
// membership, so a misspelled name never registers in either mode.
export function isToolEnabled(mode: ToolsetMode, name: string): boolean {
  return mode === "full" ? TOOLSET_FULL.includes(name) : TOOLSET_MINIMAL.includes(name);
}

// toolsetTools returns the registered definition names for a mode —
// the cost pin counts these (plus their guidance chars).
export function toolsetTools(mode: ToolsetMode): readonly string[] {
  return mode === "full" ? TOOLSET_FULL : TOOLSET_MINIMAL;
}

// MINIMAL_CREW_LINE is the 1-line Crew for minimal sessions (T9
// speed-tokens): simple sessions keep the builder without paying the
// full scout/worker/verifier/proposer + guardian roster. Guardians
// stay reachable on demand via `review --lens` (review paths
// untouched).
export const MINIMAL_CREW_LINE =
  "- The Crew: @regulus-worker builds one unit; guardians on demand via `review --lens`.";

// formatDisciplineForToolset renders the discipline text per toolset
// mode: full returns the text byte-identical (roster intact),
// minimal swaps the "- The Crew:" roster line for MINIMAL_CREW_LINE.
// The .md source keeps the full roster (single source); the trim
// happens at render, so trimming only shrinks the injected prompt
// and the discipline token-budget cap stays green. Pure so bun tests
// pin both modes without a host. A missing Crew line passes through
// unchanged (never break context assembly on wording drift).
export function formatDisciplineForToolset(discipline: string, mode: ToolsetMode): string {
  if (mode === "full") return discipline;
  const lines = discipline.split("\n");
  const idx = lines.findIndex((l) => l.trimStart().startsWith("- The Crew:"));
  if (idx < 0) return discipline;
  const next = [...lines];
  next[idx] = MINIMAL_CREW_LINE;
  return next.join("\n");
}
