// D2 (S2+S3) pack validation: the committed `.devin/` pack that makes
// this repo usable from Devin CLI/Desktop/Cloud (stdio MCP + skill +
// agents). Run with `bun test` from .opencode/ (also in CI). Static
// shape checks only — no Devin runtime needed.
import { describe, expect, test } from "bun:test";

const ROOT = `${import.meta.dir}/../..`;
const SKILL = `${ROOT}/.devin/skills/regulus-workflow/SKILL.md`;
const WORKER = `${ROOT}/.devin/agents/regulus-worker.md`;
const SCOUT = `${ROOT}/.devin/agents/regulus-scout.md`;
const MCP_CONFIG = `${ROOT}/.devin/mcp_config.json`;

async function frontmatter(path: string): Promise<{ text: string; fm: any; body: string }> {
  const text = await Bun.file(path).text();
  const m = text.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n/);
  if (!m) throw new Error(`${path} has no frontmatter`);
  return { text, fm: Bun.YAML.parse(m[1]) as any, body: text.slice(m[0].length) };
}

function expectNamed(fm: any): void {
  expect(typeof fm.name).toBe("string");
  expect(fm.name.length).toBeGreaterThan(0);
  expect(typeof fm.description).toBe("string");
  expect(fm.description.length).toBeGreaterThan(0);
}

describe("devin skill", () => {
  test("SKILL.md has name+description, exec in allowed-tools, triggers and permissions", async () => {
    const { fm } = await frontmatter(SKILL);
    expectNamed(fm);
    expect(Array.isArray(fm["allowed-tools"])).toBe(true);
    for (const t of ["read", "grep", "glob", "exec"]) expect(fm["allowed-tools"]).toContain(t);
    expect(fm.triggers).toContain("user");
    expect(fm.triggers).toContain("model");
    const perms = JSON.stringify(fm.permissions ?? {});
    expect(perms).toContain("Read(**)");
    expect(perms).toContain("Exec(regulus *)");
    expect(perms).toContain("Write(**/.env*)");
  });

  test("skill body is the compressed Sextant with no OpenCode-isms", async () => {
    const { body } = await frontmatter(SKILL);
    const lines = body.replace(/\n+$/, "").split("\n");
    expect(lines.length).toBeLessThanOrEqual(40);
    expect(body).toContain("regulus gate run");
    expect(body).toContain("regulus memory");
    expect(body).not.toContain("@");
    expect(body.toLowerCase()).not.toContain("opencode");
    expect(body.toLowerCase()).not.toContain("hook");
  });
});

describe("devin agents", () => {
  test("worker has name+description, the full toolset, and no model pin", async () => {
    const { text, fm, body } = await frontmatter(WORKER);
    expectNamed(fm);
    for (const t of ["read", "grep", "glob", "exec"]) expect(fm["allowed-tools"]).toContain(t);
    expect(fm.model).toBeUndefined();
    expect(text).toContain("router");
    expect(body).toContain("## Log");
    expect(body).toContain("Necessity Ladder");
    expect(body).toContain("regulus gate run");
  });

  test("scout has name+description and no exec/write", async () => {
    const { fm, body } = await frontmatter(SCOUT);
    expectNamed(fm);
    expect(Array.isArray(fm["allowed-tools"])).toBe(true);
    expect(fm["allowed-tools"]).toContain("read");
    expect(fm["allowed-tools"]).toContain("grep");
    expect(fm["allowed-tools"]).toContain("glob");
    expect(fm["allowed-tools"]).not.toContain("exec");
    expect(fm["allowed-tools"]).not.toContain("write");
    expect(fm["allowed-tools"]).not.toContain("edit");
    expect(body).toContain("path:line");
  });
});

describe("devin mcp config", () => {
  test("mcp_config.json wires the regulus server over stdio command+args", async () => {
    const json = JSON.parse(await Bun.file(MCP_CONFIG).text());
    const server = json?.mcpServers?.regulus;
    expect(server).toBeDefined();
    expect(server.command).toBe("regulus");
    expect(server.args).toEqual(["mcp"]);
    expect(typeof server.env).toBe("object");
  });

  test("no .devin/config.json fights user settings", async () => {
    expect(await Bun.file(`${ROOT}/.devin/config.json`).exists()).toBe(false);
  });
});
