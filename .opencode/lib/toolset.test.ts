// T2 (macizo-tokens) pins: load-time subsets por modo.
//
// The OpenCode plugin SDK (@opencode/plugin 2.2.0) registers tools once
// per project load — setup carries no session, the transform callback
// is sync `(editor) => void` with no session, Tool.Options has no
// visibility flag — so per-session conditional registration is
// impossible and the Chart's Then-alternative applies to that branch:
// load-time subsets (TE_TOOLSET) + documented why. These tests pin the
// subset selection, the cost saving, and the plugin wiring.
import { describe, expect, test } from "bun:test";
import {
  applyUsagePruning,
  isToolEnabled,
  parseToolGuidance,
  parseTraceUsage,
  resolveToolset,
  TOOLSET_FULL,
  TOOLSET_MINIMAL,
  toolsetTools,
} from "./domain";

describe("toolset lists (T2 subset pin)", () => {
  test("full pins the 16 registered definitions in registration order", () => {
    expect([...TOOLSET_FULL]).toEqual([
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
    ]);
  });

  test("minimal is the simple-session subset (verify + search + resume)", () => {
    expect([...TOOLSET_MINIMAL]).toEqual(["doctor", "memory_search", "memory_context"]);
  });

  test("minimal is contained in full", () => {
    for (const name of TOOLSET_MINIMAL) expect(TOOLSET_FULL).toContain(name);
  });

  test("unknown names never register in either mode (allowlist)", () => {
    expect(isToolEnabled("full", "status")).toBe(false);
    expect(isToolEnabled("full", "")).toBe(false);
    expect(isToolEnabled("minimal", "gate_run")).toBe(false);
    expect(isToolEnabled("minimal", "status")).toBe(false);
  });

  test("toolsetTools returns the mode list", () => {
    expect([...toolsetTools("full")]).toEqual([...TOOLSET_FULL]);
    expect([...toolsetTools("minimal")]).toEqual([...TOOLSET_MINIMAL]);
  });
});

describe("resolveToolset (TE_TOOLSET selection + reversal)", () => {
  test("explicit full|minimal win over tree detection", () => {
    expect(resolveToolset({ env: "minimal", hasCharts: true })).toBe("minimal");
    expect(resolveToolset({ env: "full", hasCharts: false })).toBe("full");
  });

  test("spelling is case/whitespace-tolerant", () => {
    expect(resolveToolset({ env: "MINIMAL ", hasCharts: true })).toBe("minimal");
    expect(resolveToolset({ env: " Full", hasCharts: false })).toBe("full");
  });

  test("auto/unset/blank detect from charts/", () => {
    expect(resolveToolset({ env: "auto", hasCharts: true })).toBe("full");
    expect(resolveToolset({ env: "auto", hasCharts: false })).toBe("minimal");
    expect(resolveToolset({ env: "", hasCharts: true })).toBe("full");
    expect(resolveToolset({ env: "", hasCharts: false })).toBe("minimal");
    expect(resolveToolset({ hasCharts: false })).toBe("minimal");
  });

  test("unknown values fail closed to full (a typo never strips tools)", () => {
    expect(resolveToolset({ env: "minmal", hasCharts: false })).toBe("full");
    expect(resolveToolset({ env: "off", hasCharts: false })).toBe("full");
  });

  test("missing detection fails closed to full", () => {
    expect(resolveToolset({})).toBe("full");
    expect(resolveToolset()).toBe("full");
  });
});

describe("simple sessions pay fewer definitions (T2 cost pin)", () => {
  test("3/16 definitions and under half the guidance chars", async () => {
    const md = await Bun.file(`${import.meta.dir}/../tool-guidance.md`).text();
    const guidance = parseToolGuidance(md);
    const chars = (names: readonly string[]) =>
      names.reduce((n, name) => n + (guidance[name] ?? "").length, 0) + (guidance["regulus"] ?? "").length;
    expect(toolsetTools("minimal").length).toBe(3);
    expect(toolsetTools("full").length).toBe(16);
    const full = chars(toolsetTools("full"));
    const minimal = chars(toolsetTools("minimal"));
    expect(full).toBeGreaterThan(500);
    expect(minimal).toBeLessThan(full / 2);
  });
});

describe("toolset plugin wiring (T2 intentional pin)", () => {
  // Like the T3 hook pins: the transform needs the OpenCode host, so
  // the wiring holds as coarse source pins — every tool registration
  // goes through the toolset allowlist, the mode resolves at setup
  // from TE_TOOLSET + charts/ detection, and the reversal is
  // documented where it acts.
  test("every tool add goes through the allowlist; commands untouched", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const toolRegion = src.slice(src.indexOf("ctx.tool.transform"), src.indexOf("ctx.command.transform"));
    expect(toolRegion).toContain("isToolEnabled(toolset, tool.name)");
    expect(toolRegion.match(/editor\.add\(\{/g) ?? []).toEqual([]);
    for (const name of TOOLSET_FULL) expect(toolRegion).toContain(`name: "${name}"`);
    const cmdRegion = src.slice(src.indexOf("ctx.command.transform"));
    expect(cmdRegion.match(/editor\.add\(\{/g)?.length).toBe(3);
  });

  test("setup resolves the mode and documents the reversal", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const setup = src.slice(src.indexOf("async setup"), src.indexOf("ctx.tool.transform"));
    expect(setup).toContain("resolveToolset(");
    expect(setup).toContain("hasChartFiles(dir)");
    expect(setup).toContain("TE_TOOLSET=full");
    expect(src).toContain("TE_TOOLSET");
  });
});

describe("lens migration (T8 review transaction)", () => {
  // The verify command wraps the guardian protocol in a review
  // lineage: start freezes, lenses file per-lens verdicts (4R),
  // findings sort through refute, approve binds, and approval never
  // delivers (no commit/push/release from a review).
  async function verifyText(): Promise<string> {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const at = src.indexOf('name: "regulus-verify"');
    return src.slice(at, at + 3000);
  }
  test("verify opens a lineage and closes it with approve", async () => {
    const text = await verifyText();
    expect(text).toContain("review start");
    expect(text).toContain("review approve");
  });
  test("verify files verdicts per 4R lens and sorts findings via refute", async () => {
    const text = await verifyText();
    for (const lens of ["risk", "resilience", "readability", "reliability"]) {
      expect(text).toContain(lens);
    }
    expect(text).toContain("review refute");
  });
  test("approval never delivers", async () => {
    const text = await verifyText();
    expect(text).toContain("Approval is not delivery");
    expect(text).toContain("never commit");
  });
});

describe("usage pruning (T3 measured toolset)", () => {
  // The plugin prunes the full set by measured use: a full-only tool
  // with zero recorded calls across the evidence floor (>= 3 traced
  // sessions) is left out; the minimal set never prunes (safe floor);
  // explicit modes skip measuring; malformed usage fails open to full.
  // Trace tool names are host-side (suffix match); usage JSON comes
  // from `trace usage --json`.
  const usage3 = JSON.stringify({
    sessions: 3,
    tools: [
      { tool: "regulus_doctor", calls: 5, sessions: ["s1", "s2", "s3"] },
      { tool: "regulus_memory_search", calls: 2, sessions: ["s1"] },
    ],
  });
  test("unused full-only tools prune, minimal tools stay", () => {
    const got = applyUsagePruning("full", parseTraceUsage(usage3));
    expect(got.tools).not.toContain("probe_compose");
    expect(got.tools).not.toContain("gate_run");
    expect(got.tools).toContain("doctor");
    expect(got.tools).toContain("memory_search");
    expect(got.tools).toContain("memory_context");
    expect(got.pruned.length).toBeGreaterThan(0);
    expect(got.pruned).toContain("probe_compose");
  });
  test("below the evidence floor nothing prunes", () => {
    const thin = JSON.stringify({ sessions: 2, tools: [] });
    const got = applyUsagePruning("full", parseTraceUsage(thin));
    expect([...got.tools]).toEqual([...TOOLSET_FULL]);
    expect(got.pruned).toEqual([]);
  });
  test("explicit modes skip measuring", () => {
    for (const mode of ["full", "minimal"] as const) {
      const got = applyUsagePruning(mode, parseTraceUsage(usage3), true);
      expect([...got.tools]).toEqual([...toolsetTools(mode)]);
      expect(got.pruned).toEqual([]);
    }
  });
  test("malformed usage fails open to full", () => {
    for (const bad of ["", "not json", JSON.stringify({ tools: [] })]) {
      const got = applyUsagePruning("full", parseTraceUsage(bad));
      expect([...got.tools]).toEqual([...TOOLSET_FULL]);
      expect(got.pruned).toEqual([]);
    }
  });
});
