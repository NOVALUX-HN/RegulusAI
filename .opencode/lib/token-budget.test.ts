// Token budget pins for the harness's per-request fixed overhead.
//
// Every character here is paid on EVERY request (tool definitions ride
// every call; discipline rides every context assembly; both replicate
// into every subagent session). The caps have ~10% headroom over the
// post-diet sizes: tightening wording is fine, fattening trips the test
// and forces a conscious decision. Run with `bun test` from .opencode/.
import { describe, expect, test } from "bun:test";

const DISCIPLINE_CAP = 1400; // .opencode/discipline.md is ~1280
const GUIDANCE_CAP = 1500; // 17 sections are ~1360 collapsed chars
const TS_DESCRIPTIONS_CAP = 2000; // all description literals are ~1820

describe("token budget", () => {
  test("discipline stays lean", async () => {
    const text = await Bun.file(`${import.meta.dir}/../discipline.md`).text();
    expect(text.length).toBeLessThanOrEqual(DISCIPLINE_CAP);
  });

  test("tool guidance stays lean", async () => {
    const text = await Bun.file(`${import.meta.dir}/../tool-guidance.md`).text();
    const sections = text.split(/^##\s+/m).slice(1);
    expect(sections.length).toBeGreaterThan(10);
    let total = 0;
    for (const part of sections) {
      const nl = part.indexOf("\n");
      total += part.slice(nl + 1).replace(/\s+/g, " ").trim().length;
    }
    expect(total).toBeLessThanOrEqual(GUIDANCE_CAP);
  });

  test("compiled description literals stay lean", async () => {
    const src = await Bun.file(`${import.meta.dir}/../plugins/regulus.ts`).text();
    let total = 0;
    for (const m of src.matchAll(/description: "([^"]*)"/g)) total += m[1].length;
    expect(total).toBeGreaterThan(500);
    expect(total).toBeLessThanOrEqual(TS_DESCRIPTIONS_CAP);
  });
});
