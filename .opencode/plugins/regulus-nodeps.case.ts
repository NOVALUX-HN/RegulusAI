// NOV-67: plugin fail-closed without a usable core, in a hermetically
// binary-less environment. This file runs inside a scrubbed subprocess
// (see "missing binary" in regulus.test.ts): Bun.spawn resolves bare
// names without honoring in-process PATH swaps, so the scrubbing must
// happen at process creation, not inside the test.
import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import plugin from "./regulus";

function mockCtx(dir: string) {
  const tools: any[] = [];
  const namespaces: any[] = [];
  const hooks: Array<{ kind: string; name: string }> = [];
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
        transform: async (fn: any) => fn({ add: (_c: any) => {} }),
      },
      session: {
        hook: async (name: string, _fn: any) => void hooks.push({ kind: "session", name }),
        prompt: async () => {},
      },
    },
    tools,
  } as any;
}

describe("plugin fail-closed", () => {
  test("missing binary refuses with the contract reason and registers nothing", async () => {
    const dir = mkdtempSync(join(tmpdir(), "plugin-nodeps-"));
    try {
      const m = mockCtx(dir);
      let err: Error | null = null;
      try {
        await (plugin as any).setup(m.ctx);
      } catch (e) {
        err = e as Error;
      }
      expect(err).not.toBeNull();
      expect(String(err)).toMatch(/no working regulus binary/);
      expect(m.tools).toHaveLength(0);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
