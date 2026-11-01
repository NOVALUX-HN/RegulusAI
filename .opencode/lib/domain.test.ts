// Committed checks for the plugin's pure domain logic (G1.4, V5.2).
// Run with `bun test` from .opencode/ (also in CI). No OpenCode host
// needed: lib/domain.ts has zero host imports by design.
import { beforeAll, describe, expect, test } from "bun:test";
import {
  checkContract,
  clearDomainsCache,
  distinctiveWords,
  extractPaths,
  findChartRefs,
  findDomainInChart,
  formatGateFailure,
  formatGateResult,
  formatResolveLog,
  GATE_RUN_TIMEOUT_MS_DEFAULT,
  GATE_RUN_TIMEOUT_MS_FLOOR,
  gateRunTimeoutMs,
  REGULUS_CONTRACT,
  resolveDomain,
  suggestSkills,
  TtlCache,
  withTimeout,
  type RunBin,
} from "./domain";
import { renderDisciplineDefault, renderGuidanceDefaults } from "../../scripts/sync-plugin-defaults";

describe("findChartRefs", () => {
  test("order and dedupe", () => {
    expect(findChartRefs("see charts/a.md and charts/a.md plus charts/b-c.md")).toEqual([
      "charts/a.md",
      "charts/b-c.md",
    ]);
  });
  test("ignores non-chart mentions", () => {
    expect(findChartRefs("no refs here")).toEqual([]);
  });
});

describe("findDomainInChart", () => {
  test("reads the Domain line", () => {
    expect(findDomainInChart("# T\n\nDomain: go\n")).toBe("go");
  });
  test("absent line yields empty", () => {
    expect(findDomainInChart("# T\nno line\n")).toBe("");
  });
  test("first match wins", () => {
    expect(findDomainInChart("Domain: ts\nDomain: go\n")).toBe("ts");
  });
});

describe("extractPaths", () => {
  test("basic paths", () => {
    expect(extractPaths('edit {"path":"internal/cli/cli.go"} and docs/a.md')).toEqual([
      "internal/cli/cli.go",
      "docs/a.md",
    ]);
  });
  test("dedupe and cap at 10", () => {
    expect(extractPaths("a/b.go plus a/b.go")).toEqual(["a/b.go"]);
    expect(extractPaths(Array.from({ length: 15 }, (_, i) => `d${i}/f.go`).join(" "))).toHaveLength(10);
  });
  test("URLs, bare words and extensionless tokens excluded", () => {
    expect(extractPaths("see https://example.com/x/y.go for details")).toEqual([]);
    expect(extractPaths("run main now")).toEqual([]);
    expect(extractPaths("the charts goat is loose")).toEqual([]);
  });
});

describe("resolveDomain", () => {
  const fake = (domains: string[], pathHits: Record<string, string>): RunBin => async (a) => {
    if (a[0] === "library" && a[1] === "domains")
      return { exitCode: 0, output: domains.join("\n") + (domains.length ? "\n" : "") };
    if (a[0] === "trace" && a[1] === "files")
      return { exitCode: 0, output: "internal/cli/cli.go\ndocs/memory.md\n" };
    if (a[0] === "memory" && a[1] === "search") {
      const d = a[a.indexOf("--domain") + 1];
      const p = a[a.indexOf("--path") + 1];
      if (pathHits[`${d}|${p}`]) return { exitCode: 0, output: "1 hit(s)\nobs_x lesson t (saved just now)\n" };
      return { exitCode: 0, output: "no matching observations" };
    }
    return { exitCode: 1, output: "" };
  };

  test("message chart ref wins", async () => {
    const dir = await mkTmpCharts({ "feat.md": "# F\n\nDomain: ts\n" });
    try {
      await expect(
        resolveDomain(dir, fake(["go", "ts"], { "ts|docs/memory.md": "y" }), JSON.stringify([{ t: "see charts/feat.md" }]), "s1"),
      ).resolves.toBe("ts");
    } finally {
      await rmTmp(dir);
    }
  });

  test("chart Domain beats files", async () => {
    const dir = await mkTmpCharts({ "f.md": "# F\n\nDomain: go\n" });
    try {
      await expect(
        resolveDomain(dir, fake(["go", "ts"], { "ts|docs/memory.md": "y" }), JSON.stringify([{ t: "see charts/f.md" }]), "s1"),
      ).resolves.toBe("go");
    } finally {
      await rmTmp(dir);
    }
  });

  test("files step decides when charts are silent", async () => {
    const dir = await mkTmpCharts({ "a.md": "# A\nno domain\n", "b.md": "# B\nno domain\n" });
    try {
      await expect(
        resolveDomain(dir, fake(["go", "ts"], { "ts|docs/memory.md": "y" }), JSON.stringify([{ t: "hello" }]), "s1"),
      ).resolves.toBe("ts");
    } finally {
      await rmTmp(dir);
    }
  });

  test("multi-domain with no hits falls back to empty", async () => {
    const dir = await mkTmpCharts({});
    try {
      await expect(resolveDomain(dir, fake(["go", "ts"], {}), JSON.stringify([{ t: "hello" }]), "s1")).resolves.toBe("");
    } finally {
      await rmTmp(dir);
    }
  });

  test("lone library domain without session", async () => {
    const dir = await mkTmpCharts({});
    try {
      await expect(resolveDomain(dir, fake(["go"], {}), JSON.stringify([{ t: "hello" }]))).resolves.toBe("go");
    } finally {
      await rmTmp(dir);
    }
  });

  test("caches library domains across hooks (T4)", async () => {
    clearDomainsCache();
    const dir = await mkTmpCharts({});
    let domainSpawns = 0;
    const counting: RunBin = async (a) => {
      if (a[0] === "library" && a[1] === "domains") domainSpawns++;
      return fake(["go", "ts"], {})(a);
    };
    try {
      await expect(
        resolveDomain(dir, counting, JSON.stringify([{ t: "hello" }]), "sess-cache"),
      ).resolves.toBe("");
      await expect(
        resolveDomain(dir, counting, JSON.stringify([{ t: "hello" }]), "sess-cache"),
      ).resolves.toBe("");
      // Two hooks, one domains spawn: trace files + searches re-run,
      // the domains list does not (it only changes on admit).
      expect(domainSpawns).toBe(1);
    } finally {
      await rmTmp(dir);
      clearDomainsCache();
    }
  });
});

async function mkTmpCharts(files: Record<string, string>): Promise<string> {
  const dir = `${process.env.TMPDIR ?? "/tmp"}/dom-test-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
  await Bun.$`mkdir -p ${dir}/charts`.quiet();
  for (const [name, content] of Object.entries(files)) await Bun.write(`${dir}/charts/${name}`, content);
  return dir;
}

async function rmTmp(dir: string): Promise<void> {
  await Bun.$`rm -rf ${dir}`.quiet();
}

describe("suggestSkills", () => {
  const skills = [
    { name: "regulus-workflow", description: "The Sextant evidence-first workflow: authorize, chart, build, prove, learn" },
    { name: "other", description: "Unrelated gardening notes about roses" },
  ];
  test("matches on shared distinctive words", () => {
    const got = suggestSkills(skills, "run the evidence-first workflow and prove the chart");
    expect(got.map((s) => s.name)).toEqual(["regulus-workflow"]);
  });
  test("needs two shared words", () => {
    expect(suggestSkills(skills, "workflow stuff")).toEqual([]);
  });
  test("caps at two and sorts by score", () => {
    const many = [
      { name: "a-one", description: "evidence workflow chart prove learn authorize build" },
      { name: "b-two", description: "evidence workflow chart" },
      { name: "c-three", description: "evidence workflow chart prove learn" },
    ];
    expect(suggestSkills(many, "evidence workflow chart prove learn authorize").map((s) => s.name)).toEqual([
      "a-one",
      "c-three",
    ]);
  });
  test("distinctiveWords filters short and stop words", () => {
    expect(distinctiveWords("Run the charts when ready")).toEqual(["charts", "ready"]);
  });
});

describe("checkContract (S3)", () => {
  test("the core speaking the required version passes", () => {
    expect(checkContract({ exitCode: 0, output: "regulus.contract/v1" })).toBe("");
  });
  test("the CLI's trailing newline (fmt.Fprintln) does not upset the match", () => {
    expect(checkContract({ exitCode: 0, output: `regulus.contract/v1\n` })).toBe("");
  });
  test("a mismatched core is refused, naming the required version", () => {
    const refusal = checkContract({ exitCode: 0, output: "regulus.contract/v2" });
    expect(refusal).toContain(`"regulus.contract/v2"`);
    expect(refusal).toContain(REGULUS_CONTRACT);
    expect(refusal).toContain("Update the regulus binary");
  });
  test("an older binary without the contract command fails closed (guidance, not trust)", () => {
    const refusal = checkContract({ exitCode: 2, output: `regulus: unknown command "contract"` });
    expect(refusal).toContain(REGULUS_CONTRACT);
    expect(refusal).toContain("contract check");
  });
});
describe("tool guidance sync", () => {
  test("tool-guidance.md sections match the compiled defaults", async () => {
    const root = import.meta.dir + "/../..";
    const md = await Bun.file(`${root}/.opencode/tool-guidance.md`).text();
    const ts = await Bun.file(`${root}/.opencode/plugins/regulus.ts`).text();
    const block = ts.slice(ts.indexOf("const DEFAULT_TOOL_GUIDANCE"), ts.indexOf("// parseToolGuidance"));
    const defaults: Record<string, string> = {};
    for (const m of block.matchAll(/(\w+):\s*\n?\s*"((?:[^"\\]|\\.)*)"/g)) defaults[m[1]] = m[2];
    const sections = md.split(/^##\s+/m).slice(1).map((p) => {
      const nl = p.indexOf("\n");
      return [p.slice(0, nl).trim(), p.slice(nl + 1).replace(/\s+/g, " ").trim()];
    }) as [string, string][];
    expect(sections.length).toBeGreaterThan(0);
    for (const [name, body] of sections) {
      expect(defaults[name]).toBeDefined();
      expect(body).toBe(defaults[name]);
    }
    for (const name of Object.keys(defaults)) {
      expect(sections.find(([n]) => n === name)).toBeDefined();
    }
  });
});

describe("tool param sync", () => {
  const src = { text: "" as string };
  beforeAll(async () => {
    const root = import.meta.dir + "/../..";
    src.text = await Bun.file(`${root}/.opencode/plugins/regulus.ts`).text();
  });

  function toolBlocks(): { name: string; body: string }[] {
    const starts: number[] = [];
    let i = 0;
    for (;;) {
      const k = src.text.indexOf("editor.add({", i);
      if (k < 0) break;
      starts.push(k);
      i = k + 1;
    }
    const out: { name: string; body: string }[] = [];
    for (let n = 0; n < starts.length; n++) {
      const body = src.text.slice(starts[n], n + 1 < starts.length ? starts[n + 1] : src.text.length);
      if (!body.includes('namespace: "regulus"')) continue;
      const m = body.match(/name: "([^"]+)"/);
      if (m) out.push({ name: m[1], body });
    }
    return out;
  }

  // Brace-match the properties object: regex-to-indentation leaks
  // sibling keys (required, additionalProperties) into the set.
  // The opening `properties: {` line itself is not a prop.
  function declaredProps(body: string): Set<string> {
    const start = body.indexOf("properties: {");
    if (start < 0) return new Set();
    let depth = 0;
    for (let k = start; k < body.length; k++) {
      if (body[k] === "{") depth++;
      if (body[k] === "}") {
        depth--;
        if (depth === 0) {
          const inner = body.slice(start, k);
          return new Set(
            [...inner.matchAll(/^\s*(\w+)\s*:\s*\{/gm)]
              .map((m) => m[1])
              .filter((p) => p !== "properties"),
          );
        }
      }
    }
    return new Set();
  }

  // Pre-existing CLI convention: memory_save's comma-joined inputs keep
  // singular --flags for plural props. Documented per-tool so the
  // check stays honest instead of fuzzy-matching names.
  const FLAG_EXCEPTIONS: Record<string, string> = {
    "memory_save::--path": "paths",
    "memory_save::--receipt": "receipts",
  };
  const kebab = (p: string) => "--" + p.replace(/_/g, "-");
  const propOf = (tool: string, f: string) =>
    FLAG_EXCEPTIONS[`${tool}::${f}`] ?? f.slice(2).replace(/-/g, "_");

  // Agent surface pinned at 16 (self-audit U1): the OpenCode agent
  // surface carries 16 named tools — not full CLI parity (the CLI has
  // ~20 commands; surface/reflect/library/budget/crap/status/evidence
  // stay CLI-only by design). Exact count and exact names; a new agent
  // tool means a test edit, not a silent drift.
  const TOOL_NAMES = [
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
  ];

  test("the tool surface is exactly 16 named tools", () => {
    const blocks = toolBlocks();
    expect(blocks.map((b) => b.name).sort()).toEqual([...TOOL_NAMES].sort());
    expect(blocks.length).toBe(16);
  });

  test("every tool wires its declared params to CLI flags", () => {
    const blocks = toolBlocks();
    expect(blocks.length).toBe(16);
    for (const { name, body } of blocks) {
      const props = declaredProps(body);
      const reqM = body.match(/required: \[([^\]]*)\]/);
      const required: string[] = reqM ? [...reqM[1].matchAll(/"([^"]+)"/g)].map((m) => m[1]) : [];
      const flags = new Set([...body.matchAll(/"(--[a-z][\w-]*)"/g)].map((m) => m[1]));
      const destM = body.match(/const \{([^}]*)\} = input/);
      const destructured = new Set(
        destM ? [...destM[1].split(",")].map((s) => s.trim()).filter((s) => s !== "") : [],
      );
      for (const r of required) {
        expect(props.has(r)).toBe(true);
        expect(destructured.has(r) || flags.has(kebab(r))).toBe(true);
      }
      for (const p of props) {
        expect(destructured.has(p) || flags.has(kebab(p))).toBe(true);
      }
      for (const f of flags) {
        expect(props.has(propOf(name, f))).toBe(true);
      }
    }
  });

  test("memory_search schema matches MCP coercion (integer limit, boolean flags)", () => {
    const block = toolBlocks().find((b) => b.name === "memory_search");
    expect(block).toBeDefined();
    const body = block!.body;
    expect(body).toContain('limit: { type: "integer"');
    expect(body).toContain('any: { type: "boolean"');
    expect(body).toContain('latest: { type: "boolean"');
    expect(body).toContain('include_superseded: { type: "boolean"');
  });

  test("memory_history's one param reaches --topic-key (S4 flag parity)", () => {
    const block = toolBlocks().find((b) => b.name === "memory_history");
    expect(block).toBeDefined();
    const body = block!.body;
    expect(body).toContain(`"memory", "history", "--topic-key", topic_key`);
    expect(declaredProps(body)).toEqual(new Set(["topic_key"]));
    expect(body).toContain(`required: ["topic_key"]`);
  });
});

describe("withTimeout", () => {
  test("fast work resolves", async () => {
    await expect(withTimeout(Promise.resolve("ok"), 100)).resolves.toBe("ok");
  });
  test("slow work rejects after the budget", async () => {
    const slow = new Promise<string>((resolve) => setTimeout(() => resolve("late"), 500));
    await expect(withTimeout(slow, 20)).rejects.toThrow("timed out");
  });
  test("outer abort rejects", async () => {
    const ctrl = new AbortController();
    ctrl.abort();
    const slow = new Promise<string>((resolve) => setTimeout(() => resolve("late"), 50));
    await expect(withTimeout(slow, 1000, ctrl.signal)).rejects.toThrow();
  });
});

describe("TtlCache", () => {
  test("serves within TTL, refetches after expiry", async () => {
    const cache = new TtlCache<string>(15);
    expect(cache.get("k")).toBeUndefined();
    cache.set("k", "v");
    expect(cache.get("k")).toBe("v");
    await new Promise((r) => setTimeout(r, 40));
    expect(cache.get("k")).toBeUndefined();
  });
  test("clear drops entries", () => {
    const cache = new TtlCache<string>(60000);
    cache.set("k", "v");
    cache.clear();
    expect(cache.get("k")).toBeUndefined();
  });
});

describe("plugin defaults codegen", () => {
  test("rendered discipline core equals the compiled literal", async () => {
    const root = import.meta.dir + "/../..";
    const md = await Bun.file(`${root}/.opencode/discipline.md`).text();
    const ts = await Bun.file(`${root}/.opencode/plugins/regulus.ts`).text();
    const start = ts.indexOf("const DEFAULT_DISCIPLINE = `") + "const DEFAULT_DISCIPLINE = `".length;
    let end = ts.indexOf("`\n\n// loadDiscipline", start);
    if (end < 0) end = ts.indexOf("`\r\n\r\n// loadDiscipline", start);
    expect(start).toBeGreaterThan(0);
    expect(end).toBeGreaterThan(start);
    expect(renderDisciplineDefault(md)).toBe(ts.slice(start, end));
  });
  test("rendered guidance equals the compiled defaults", async () => {
    const root = import.meta.dir + "/../..";
    const md = await Bun.file(`${root}/.opencode/tool-guidance.md`).text();
    const ts = await Bun.file(`${root}/.opencode/plugins/regulus.ts`).text();
    const block = ts.slice(ts.indexOf("const DEFAULT_TOOL_GUIDANCE"), ts.indexOf("// parseToolGuidance"));
    const defaults: Record<string, string> = {};
    for (const m of block.matchAll(/(\w+):\s*\n?\s*"((?:[^"\\]|\\.)*)"/g)) defaults[m[1]] = m[2];
    expect(renderGuidanceDefaults(md)).toEqual(defaults);
  });
});

describe("contract enforcement wiring (S3)", () => {
  test("resolveRegulus probes `regulus contract` and refuses on mismatch", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const resolveBlock = src.slice(src.indexOf("function resolveRegulus"), src.indexOf("export default"));
    expect(resolveBlock).toContain(`["contract"]`);
    expect(resolveBlock).toContain("checkContract");
    expect(resolveBlock).toContain("REGULUS_CONTRACT");
    // the refusal message carries the required version, per the Chart
    expect(resolveBlock).toMatch(/contract \$\{REGULUS_CONTRACT\}/);
  });
});

describe("gate_run timeout (D3)", () => {
  test("default covers the long gauntlet (>= 600s)", () => {
    expect(GATE_RUN_TIMEOUT_MS_DEFAULT).toBeGreaterThanOrEqual(600_000);
    expect(gateRunTimeoutMs("")).toBe(GATE_RUN_TIMEOUT_MS_DEFAULT);
  });
  test("explicit value passes through, floored at 600s", () => {
    expect(gateRunTimeoutMs("3600000")).toBe(3_600_000);
    expect(gateRunTimeoutMs("600000")).toBe(GATE_RUN_TIMEOUT_MS_FLOOR);
    expect(gateRunTimeoutMs("1000")).toBe(GATE_RUN_TIMEOUT_MS_FLOOR);
  });
  test("blank or non-numeric falls back to the default", () => {
    expect(gateRunTimeoutMs("   ")).toBe(GATE_RUN_TIMEOUT_MS_DEFAULT);
    expect(gateRunTimeoutMs("soon")).toBe(GATE_RUN_TIMEOUT_MS_DEFAULT);
  });
  test("gate_run execute never uses the 30s spawn budget", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const gateBlock = src.slice(src.indexOf('name: "gate_run"'), src.indexOf('name: "spec_check"'));
    expect(gateBlock).toContain("gateRunTimeoutMs()");
    expect(gateBlock).not.toMatch(/timeoutMs\s*=\s*30000/);
    expect(gateBlock).toContain("formatGateResult");
    expect(gateBlock).toContain("formatGateFailure");
  });
});

describe("long timeouts where there are gates (plugin-ux S1)", () => {
  // regulus.ts carries the OpenCode host import, so the timeout wiring
  // is pinned by source assertion (precedent: S3/D3 wiring tests).
  // Decision: probe_contrast/probe_compose run real gates and
  // evolve_propose/memory_context assemble large packs — all four reuse
  // the shared gate_run long budget (gateRunTimeoutMs(): 1h default,
  // 600s floor, REGULUS_GATE_RUN_TIMEOUT_MS override) instead of the
  // generic 30s spawn default. One shared override, no new env surface.
  async function toolBlock(name: string, next: string): Promise<string> {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const start = src.indexOf(`name: "${name}"`);
    const end = next === "" ? src.length : src.indexOf(`name: "${next}"`, start);
    return src.slice(start, end);
  }

  test("probe_contrast uses the long budget", async () => {
    const block = await toolBlock("probe_contrast", "trace_show");
    expect(block).toContain("gateRunTimeoutMs()");
  });
  test("probe_compose uses the long budget", async () => {
    const block = await toolBlock("probe_compose", "");
    expect(block).toContain("gateRunTimeoutMs()");
  });
  test("evolve_propose uses the long budget", async () => {
    const block = await toolBlock("evolve_propose", "probe_contrast");
    expect(block).toContain("gateRunTimeoutMs()");
  });
  test("memory_context uses the long budget", async () => {
    const block = await toolBlock("memory_context", "memory_history");
    expect(block).toContain("gateRunTimeoutMs()");
  });
});

describe("CRLF-tolerant defaults render (H1/S1)", () => {
  const LF_DISCIPLINE = "# Title\n\nIntro stays file-only.\nRegulus discipline (evidence-first harness):\n- one\n- two\n";
  const LF_GUIDANCE = "# Title\n\nIntro.\n\n## alpha\n\nBody one here.\n\n## beta\n\nBody two here.\n";
  const toCRLF = (s: string) => s.replace(/\r\n/g, "\n").replace(/\n/g, "\r\n");

  test("renderDisciplineDefault: CRLF renders identically to LF, no \\r", () => {
    const lf = renderDisciplineDefault(LF_DISCIPLINE);
    const crlf = renderDisciplineDefault(toCRLF(LF_DISCIPLINE));
    expect(crlf).toBe(lf);
    expect(crlf).not.toContain("\r");
    expect(crlf.startsWith("Regulus discipline (evidence-first")).toBe(true);
  });

  test("renderGuidanceDefaults: CRLF renders identically to LF, no \\r", () => {
    const lf = renderGuidanceDefaults(LF_GUIDANCE);
    const crlf = renderGuidanceDefaults(toCRLF(LF_GUIDANCE));
    expect(crlf).toEqual(lf);
    expect(Object.keys(crlf).sort()).toEqual(["alpha", "beta"]);
    for (const v of Object.values(crlf)) expect(v).not.toContain("\r");
  });

  test("renderDisciplineDefault: real discipline.md with CRLF still finds the marker", async () => {
    const root = import.meta.dir + "/../..";
    const md = await Bun.file(`${root}/.opencode/discipline.md`).text();
    expect(renderDisciplineDefault(toCRLF(md))).toBe(renderDisciplineDefault(md));
    expect(renderDisciplineDefault(toCRLF(md))).not.toContain("\r");
  });

  test("renderGuidanceDefaults: real tool-guidance.md with CRLF matches LF", async () => {
    const root = import.meta.dir + "/../..";
    const md = await Bun.file(`${root}/.opencode/tool-guidance.md`).text();
    expect(renderGuidanceDefaults(toCRLF(md))).toEqual(renderGuidanceDefaults(md));
  });

  // regulus.ts carries the OpenCode host import, so its pure helpers
  // are pinned by source assertion (precedent: S3/D3 wiring tests).
  test("regulus.ts parseToolGuidance normalizes CRLF before splitting", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const block = src.slice(src.indexOf("function parseToolGuidance"), src.indexOf("// loadToolGuidance"));
    expect(block).toContain('text.replace(/\\r\\n/g, "\\n")');
    expect(block).toContain("split(/^##\\s+/m)");
  });

  test("regulus.ts dataBlock normalizes CRLF before splitting", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const block = src.slice(src.indexOf("function dataBlock"), src.indexOf("interface RunResult"));
    expect(block).toContain('.replace(/\\r\\n/g, "\\n")');
    expect(block).toContain('.split("\\n")');
  });
});

describe("binary resolution log (H2/S2)", () => {
  test("formatResolveLog names the winner and why it won", () => {
    const got = formatResolveLog("bin/regulus.exe");
    expect(got).toContain("bin/regulus.exe");
    expect(got).toContain("doctor OK");
    expect(got).toContain("contract OK");
  });

  // regulus.ts carries the OpenCode host import, so the resolve wiring
  // is pinned by source assertion (precedent: S3/D3 wiring tests).
  test("resolveRegulus logs the winner without changing first-match-wins order", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const resolveBlock = src.slice(src.indexOf("function resolveRegulus"), src.indexOf("export default"));
    expect(resolveBlock).toContain("for (const bin of candidates(dir))");
    expect(resolveBlock).toContain("formatResolveLog(bin)");
    expect(resolveBlock).toContain("return bin");
    // candidates() order untouched: env override first, PATH fallback last.
    const candBlock = src.slice(src.indexOf("function candidates"), src.indexOf("// resolveRegulus finds"));
    expect(candBlock.indexOf("REGULUS_BIN")).toBeLessThan(candBlock.indexOf('"regulus"'));
  });
});

describe("gate_run content (D3)", () => {
  test("success passes output through untouched", () => {
    expect(formatGateResult("gauntlet: 2/2 gates passed", 0)).toBe("gauntlet: 2/2 gates passed");
  });
  test("failure appends the non-zero exit code", () => {
    const got = formatGateResult("gauntlet: stopped at gate \"x\"", 1);
    expect(got).toContain("stopped at gate");
    expect(got).toContain("exit code: 1");
  });
  test("empty output never stays empty", () => {
    expect(formatGateResult("", 0)).toBe("(no output)");
    const failed = formatGateResult("", 1);
    expect(failed).toContain("(no output)");
    expect(failed).toContain("exit code: 1");
  });
  test("spawn failure becomes content with exit code -1, never a throw", () => {
    const got = formatGateFailure("timed out after 3600000ms");
    expect(got).toContain("timed out");
    expect(got).toContain("exit code: -1");
    expect(formatGateFailure("   ")).toContain("exit code: -1");
  });
});

describe("honest fallbacks (plugin-ux S4)", () => {
  // regulus.ts carries the OpenCode host import, so the fallback wiring
  // is pinned by source assertion (precedent: S3/D3/S1 wiring tests).
  // Decision: every silent fallback becomes a stderr line naming which
  // file failed and why (read error vs empty vs marker) before using
  // the compiled default — a fallback that swallows the cause lies
  // about which discipline/guidance is in force.
  async function pluginSrc(): Promise<string> {
    return Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
  }

  test("loadDiscipline warns with file, marker, and read error before the default", async () => {
    const src = await pluginSrc();
    const block = src.slice(src.indexOf("async function loadDiscipline"), src.indexOf("const DEFAULT_TOOL_GUIDANCE"));
    expect(block).toContain("console.error");
    expect(block).toContain(".opencode/discipline.md");
    expect(block).toContain("is empty");
    expect(block).toContain("missing marker");
    expect(block).toContain("cannot read");
    expect(block).toContain("DEFAULT_DISCIPLINE");
  });

  test("loadToolGuidance warns with file, marker, and read error before the defaults", async () => {
    const src = await pluginSrc();
    const block = src.slice(src.indexOf("async function loadToolGuidance"), src.indexOf("function dataBlock"));
    expect(block).toContain("console.error");
    expect(block).toContain(".opencode/tool-guidance.md");
    expect(block).toContain("is empty");
    expect(block).toContain("no ## sections");
    expect(block).toContain("cannot read");
    expect(block).toContain("DEFAULT_TOOL_GUIDANCE");
  });

  test("trace hook logs instead of swallowing (throw and non-zero exit)", async () => {
    const src = await pluginSrc();
    const block = src.slice(src.indexOf('ctx.tool.hook("execute.after"'), src.indexOf('ctx.session.hook("context"'));
    expect(block).toContain("console.error");
    expect(block).toContain("[regulus] trace hook");
    expect(block).toContain("exitCode !== 0");
    expect(block).not.toMatch(/catch\s*\{\s*\/\/ hook failures/);
  });

  test("suggestSkills catch logs one line instead of swallowing", async () => {
    const src = await pluginSrc();
    const tail = src.slice(src.indexOf("Skill suggestions (E6)"));
    expect(tail).toContain("console.error");
    expect(tail).toContain("[regulus] suggestSkills");
    expect(tail).not.toMatch(/catch\s*\{\s*\/\/ suggestions are best-effort/);
  });
});
