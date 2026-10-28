// Committed checks for the plugin's pure domain logic (G1.4, V5.2).
// Run with `bun test` from .opencode/ (also in CI). No OpenCode host
// needed: domain.ts has zero host imports by design.
import { describe, expect, test } from "bun:test";
import {
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
