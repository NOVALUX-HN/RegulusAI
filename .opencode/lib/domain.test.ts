// Committed checks for the plugin's pure domain logic (G1.4, V5.2).
// Run with `bun test` from .opencode/ (also in CI). No OpenCode host
// needed: lib/domain.ts has zero host imports by design.
import { beforeAll, describe, expect, test } from "bun:test";
import {
  clearDomainsCache,
  distinctiveWords,
  extractPaths,
  findChartRefs,
  findDomainInChart,
  resolveDomain,
  suggestSkills,
  type RunBin,
} from "./domain";

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

  test("every tool wires its declared params to CLI flags", () => {
    const blocks = toolBlocks();
    expect(blocks.length).toBeGreaterThan(10);
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
});
