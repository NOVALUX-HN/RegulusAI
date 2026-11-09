// T1 (pro-harness, ponytail): the worker carries the full Necessity
// Ladder as operating lines — scope mapping before writing, the six
// rungs, the edge-case rule, the shortcut format — and the reviewer
// flags shortcut ceilings. Static pins only: presence of the exact
// normative lines, so the doctrine can never silently shrink to a
// slogan. Run with `bun test` from .opencode/ (also in CI).
import { describe, expect, test } from "bun:test";

const ROOT = `${import.meta.dir}/../..`;
const WORKER = `${ROOT}/.opencode/agents/regulus-worker.md`;
const REVIEWER = `${ROOT}/.opencode/agents/regulus-reviewer.md`;

describe("necessity ladder (worker)", () => {
  test("scope mapping, rungs, edge cases and the shortcut format hold", async () => {
    const body = await Bun.file(WORKER).text();
    for (const line of [
      "Map the scope before you write",
      "A house component beats a native widget",
      "Never add a dependency for a few lines",
      "Between options of equal size, take the one that is correct on edge cases",
      "Deletion beats addition",
      "Keep values in the form the platform already gives you",
      "shortcut: <the limit>, <when to upgrade>",
    ]) {
      expect(body).toContain(line);
    }
  });
});

describe("shortcut ceilings (reviewer)", () => {
  test("simplifications with a known limit must carry their upgrade path", async () => {
    const body = await Bun.file(REVIEWER).text();
    expect(body).toContain("shortcut ceilings");
    expect(body).toContain("upgrade path");
  });
});
