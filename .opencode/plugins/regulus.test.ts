// NOV-67: direct plugin surface tests — tool registration, hooks and
// degradation without a binary. The plugin module is host-bound, so
// the harness (ctx) is faked and the core is a fake regulus binary
// answering doctor/contract; behavior with a real binary is unchanged.
import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtempSync, mkdirSync, writeFileSync, chmodSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import plugin from "../plugins/regulus";

function fakeBin(dir: string): string {
  const js = join(dir, "fake-regulus.mjs");
  writeFileSync(
    js,
    `const args = process.argv.slice(2);\n` +
      `if (args[0] === "doctor") { console.log("doctor ok"); process.exit(0); }\n` +
      `if (args[0] === "contract") { console.log("regulus.contract/v1"); process.exit(0); }\n` +
      `console.log("unexpected: " + args.join(" ")); process.exit(1);\n`,
  );
  if (process.platform === "win32") {
    const cmd = join(dir, "fake-regulus.cmd");
    writeFileSync(cmd, `@node "${js}" %*\r\n`);
    return cmd;
  }
  chmodSync(js, 0o755);
  return js;
}

function mockCtx(dir: string) {
  const tools: any[] = [];
  const commands: any[] = [];
  const hooks: Array<{ kind: string; name: string }> = [];
  const namespaces: any[] = [];
  return {
    ctx: {
      location: { directory: dir },
      tool: {
        transform: async (fn: any) =>
          fn({
            namespace: (n: any) => void namespaces.push(n),
            add: (t: any) => void tools.push(t),
          }),
        hook: async (name: string, _fn: any) => void hooks.push({ kind: "tool", name }),
      },
      command: {
        transform: async (fn: any) =>
          fn({
            add: (c: any) => void commands.push(c),
          }),
      },
      session: {
        hook: async (name: string, _fn: any) => void hooks.push({ kind: "session", name }),
        prompt: async () => {},
      },
    },
    tools,
    commands,
    hooks,
    namespaces,
  };
}

let dirs: string[] = [];
function fixtureDir(withCharts: boolean): string {
  const dir = mkdtempSync(join(tmpdir(), "plugin-test-"));
  dirs.push(dir);
  if (withCharts) {
    mkdirSync(join(dir, "charts"), { recursive: true });
    writeFileSync(join(dir, "charts", "demo.md"), "# demo\n");
  }
  return dir;
}

beforeEach(() => {
  delete process.env.TE_TOOLSET;
  delete process.env.REGULUS_BIN;
});

afterEach(() => {
  for (const d of dirs) rmSync(d, { recursive: true, force: true });
  dirs = [];
  delete process.env.TE_TOOLSET;
  delete process.env.REGULUS_BIN;
});

describe("plugin surface", () => {
  test("full mode registers the 19-tool namespace", async () => {
    const dir = fixtureDir(true);
    process.env.REGULUS_BIN = fakeBin(dir);
    const m = mockCtx(dir);
    await (plugin as any).setup(m.ctx);
    expect(m.namespaces.map((n: any) => n.name)).toContain("regulus");
    expect(m.tools).toHaveLength(19);
    const names = m.tools.map((t: any) => t.name).sort();
    expect(new Set(names).size).toBe(19);
    expect(names).toContain("gate_run");
    expect(names).toContain("skills_list");
  });

  test("minimal mode registers only the simple-session subset", async () => {
    const dir = fixtureDir(false);
    process.env.REGULUS_BIN = fakeBin(dir);
    process.env.TE_TOOLSET = "minimal";
    const m = mockCtx(dir);
    await (plugin as any).setup(m.ctx);
    expect(m.tools.length).toBeGreaterThan(0);
    expect(m.tools.length).toBeLessThan(19);
    expect(m.tools.map((t: any) => t.name)).toContain("doctor");
  });



  test("session hooks stay registered for context and compaction", async () => {
    const dir = fixtureDir(true);
    process.env.REGULUS_BIN = fakeBin(dir);
    const m = mockCtx(dir);
    await (plugin as any).setup(m.ctx);
    const kinds = m.hooks.map((h) => `${h.kind}:${h.name}`);
    expect(kinds).toContain("tool:execute.after");
    expect(kinds.filter((k) => k.startsWith("session:"))).not.toHaveLength(0);
  });

  test("missing binary fails closed (hermetic subprocess, no core on PATH)", async () => {
    // Bun.spawn resolves bare names without honoring in-process PATH
    // swaps, so the scrubbing happens at process creation: the child
    // sees no usable core anywhere and setup must refuse with zero
    // tools registered.
    const dir = mkdtempSync(join(tmpdir(), "plugin-scrub-"));
    const rawPath = fileURLToPath(new URL("./regulus-nodeps.case.ts", import.meta.url));
    const file = rawPath.split("\\").join("/");
    try {
      const proc = Bun.spawn([process.execPath, "test", file, "--timeout", "60000"], {
        stdout: "pipe",
        stderr: "pipe",
        env: {
          ...process.env,
          PATH: dir,
          HOME: dir,
          USERPROFILE: dir,
          REGULUS_BIN: join(dir, "no-such-binary"),
          NO_COLOR: "1",
        },
      });
      const [out, err, code] = await Promise.all([
        new Response(proc.stdout).text(),
        new Response(proc.stderr).text(),
        proc.exited,
      ]);
      expect(code).toBe(0);
      expect(err).toMatch(/1 pass/);
      expect(err).toMatch(/0 fail/);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
