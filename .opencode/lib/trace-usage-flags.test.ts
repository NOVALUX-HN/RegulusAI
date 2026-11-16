// NOV-73: the execute.after hook forwards host-exposed usage when the
// event carries it. The installed OpenCode plugin SDK does not expose
// per-call usage on the event (tool/session/agent/message/id/input
// only), so usageFlags([]) on absent/malformed payloads is the
// contract — never estimated, never zero-filled. When a host or SDK
// exposes usage, the same flags flow into the batcher (mapped in
// traceRecordArgvToEntry) instead of falling back to single spawns.
import { describe, expect, test } from "bun:test";
import { traceRecordArgvToEntry, usageFlags } from "./domain";

describe("usageFlags (NOV-73 hook passthrough)", () => {
  test("absent usage yields no flags (never estimated)", () => {
    for (const missing of [undefined, null, 42, "x", [], { a: 1 }]) {
      expect(usageFlags(missing)).toEqual([]);
    }
  });

  test("present usage maps to CLI flags with measured-host source", () => {
    expect(
      usageFlags({ input_tokens: 100, output_tokens: 50, model: "m", host: "opencode", host_version: "v" }),
    ).toEqual([
      "--input-tokens", "100",
      "--output-tokens", "50",
      "--model", "m",
      "--host", "opencode",
      "--host-version", "v",
      "--usage-source", "measured-host",
    ]);
  });

  test("camelCase keys and cache counters map too", () => {
    expect(usageFlags({ inputTokens: 7, cache_read_tokens: 3, cache_write_tokens: 2 })).toEqual([
      "--input-tokens", "7",
      "--cache-read-tokens", "3",
      "--cache-write-tokens", "2",
      "--usage-source", "measured-host",
    ]);
  });

  test("zero, negative, fractional and multiline values are dropped", () => {
    expect(usageFlags({ input_tokens: 0, output_tokens: -5 })).toEqual([]);
    expect(usageFlags({ input_tokens: 1.5 })).toEqual([]);
    expect(usageFlags({ input_tokens: 9, model: "a\nb" })).toEqual(["--input-tokens", "9", "--usage-source", "measured-host"]);
    expect(usageFlags({ input_tokens: 9, model: "x".repeat(513) })).toEqual(["--input-tokens", "9", "--usage-source", "measured-host"]);
  });

  test("batched usage flags survive the record-batch mapping", () => {
    const argv = ["trace", "record", "--session", "s", "--message", "m", "--call", "c", "--tool", "t", "--status", "completed",
      ...usageFlags({ input_tokens: 100, output_tokens: 50, cache_read_tokens: 10, model: "m", host: "opencode" })];
    const entry = traceRecordArgvToEntry(argv);
    expect(entry).not.toBeNull();
    expect(entry).toMatchObject({
      input_tokens: 100, output_tokens: 50, cache_read: 10, model: "m", host: "opencode",
    });
  });

  test("hook wires usageFlags into execute.after (source pin)", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const hook = src.slice(src.indexOf('"execute.after"'));
    expect(hook).toContain("usageFlags(");
  });
});
