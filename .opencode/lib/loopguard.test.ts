// T3 (macizo-tokens) loop guard: deterministic same-failure ×3 →
// one-line reversible redirect, normal progress untouched.
// Run with `bun test` from .opencode/ (also in CI via the ts-unit-tests
// gate). lib/domain.ts has zero host imports by design.
import { describe, expect, test } from "bun:test";
import {
  formatLoopRedirect,
  isLoopGuardEnabled,
  LOOPGUARD_MAX_SESSIONS,
  LOOPGUARD_THRESHOLD,
  LoopGuard,
  loopSignature,
  normalizeLoopMessage,
} from "./domain";

describe("loop guard threshold (T3)", () => {
  test("fires on the third consecutive identical failure", () => {
    expect(LOOPGUARD_THRESHOLD).toBe(3);
  });
});

describe("normalizeLoopMessage", () => {
  test("case, whitespace and CRLF variants share one signature", () => {
    expect(normalizeLoopMessage("  FAILED   x\r\n")).toBe(normalizeLoopMessage("failed x"));
  });
  test("empty and blank collapse to empty", () => {
    expect(normalizeLoopMessage("   ")).toBe("");
  });
  test("bounded: long messages truncate", () => {
    expect(normalizeLoopMessage("x".repeat(500)).length).toBeLessThanOrEqual(200);
  });
  test("numbers and paths stay verbatim so distinct failures differ", () => {
    expect(normalizeLoopMessage("error at line 10")).not.toBe(normalizeLoopMessage("error at line 11"));
  });
});

describe("loopSignature", () => {
  test("tool plus normalized message", () => {
    expect(loopSignature("bash", "  BOOM ")).toBe(loopSignature("bash", "boom"));
  });
  test("different tool or different message starts a new streak", () => {
    expect(loopSignature("bash", "boom")).not.toBe(loopSignature("read", "boom"));
    expect(loopSignature("bash", "boom")).not.toBe(loopSignature("bash", "other"));
  });
});

describe("isLoopGuardEnabled (TE_LOOPGUARD kill switch)", () => {
  test("unset and blank stay enabled", () => {
    const prev = process.env.TE_LOOPGUARD;
    delete process.env.TE_LOOPGUARD;
    try {
      expect(isLoopGuardEnabled()).toBe(true);
      expect(isLoopGuardEnabled("")).toBe(true);
      expect(isLoopGuardEnabled("   ")).toBe(true);
    } finally {
      if (prev !== undefined) process.env.TE_LOOPGUARD = prev;
    }
  });
  test("off-spellings disable", () => {
    for (const off of ["off", "OFF", " Off ", "0", "false", "FALSE", "no", "disabled"]) {
      expect(isLoopGuardEnabled(off)).toBe(false);
    }
  });
  test("anything else stays enabled", () => {
    for (const on of ["on", "1", "true", "yes", "banana"]) {
      expect(isLoopGuardEnabled(on)).toBe(true);
    }
  });
  test("reads TE_LOOPGUARD from the process env when no arg", () => {
    const prev = process.env.TE_LOOPGUARD;
    process.env.TE_LOOPGUARD = "off";
    try {
      expect(isLoopGuardEnabled()).toBe(false);
    } finally {
      if (prev === undefined) delete process.env.TE_LOOPGUARD;
      else process.env.TE_LOOPGUARD = prev;
    }
  });
});

describe("formatLoopRedirect", () => {
  test("exactly one line naming the tool with the reversal inline", () => {
    const line = formatLoopRedirect("bash");
    expect(line).toContain("bash");
    expect(line).toContain("×3");
    expect(line).not.toContain("\n");
    expect(line).not.toContain("\r");
    expect(line).toContain("TE_LOOPGUARD=off");
  });
  test("tool names with newlines cannot break the one-line shape", () => {
    expect(formatLoopRedirect("ba\nsh")).not.toContain("\n");
  });
});

describe("LoopGuard.observe", () => {
  test("nudge on the 3rd identical failure, then the streak resets", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
    const third = guard.observe("s1", "bash", "boom", false);
    expect(typeof third).toBe("string");
    expect(third as string).not.toContain("\n");
    // streak reset: the next identical failure starts a fresh count
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
  });
  test("whitespace/case variants count as the same failure", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s", "bash", "Boom", false)).toBeNull();
    expect(guard.observe("s", "bash", "  boom  ", false)).toBeNull();
    expect(guard.observe("s", "bash", "BOOM", false)).not.toBeNull();
  });
  test("a different message restarts the streak (no false nudge)", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s", "bash", "boom-a", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom-a", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom-b", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom-b", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom-b", false)).not.toBeNull();
  });
  test("a success in between resets (progress stays intact)", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s", "bash", "", true)).toBeNull();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
  });
  test("a different tool restarts the streak", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s", "read", "boom", false)).toBeNull();
    expect(guard.observe("s", "read", "boom", false)).toBeNull();
    expect(guard.observe("s", "read", "boom", false)).not.toBeNull();
  });
  test("sessions are isolated", () => {
    const guard = new LoopGuard();
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s2", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s2", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s1", "bash", "boom", false)).not.toBeNull();
    expect(guard.observe("s2", "bash", "boom", false)).not.toBeNull();
  });
  test("empty session id never fires and never crashes", () => {
    const guard = new LoopGuard();
    expect(guard.observe("", "bash", "boom", false)).toBeNull();
    expect(guard.observe("", "bash", "boom", false)).toBeNull();
    expect(guard.observe("", "bash", "boom", false)).toBeNull();
  });
  test("reset drops one session or everything", () => {
    const guard = new LoopGuard();
    guard.observe("s1", "bash", "boom", false);
    guard.observe("s1", "bash", "boom", false);
    guard.observe("s2", "bash", "boom", false);
    guard.observe("s2", "bash", "boom", false);
    guard.reset("s1");
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
    expect(guard.observe("s2", "bash", "boom", false)).not.toBeNull();
    guard.observe("s1", "bash", "boom", false);
    guard.reset();
    expect(guard.observe("s1", "bash", "boom", false)).toBeNull();
  });
  test("session map stays bounded (oldest evicted)", () => {
    const guard = new LoopGuard();
    for (let i = 0; i < LOOPGUARD_MAX_SESSIONS + 10; i++) {
      guard.observe(`s${i}`, "bash", "boom", false);
      guard.observe(`s${i}`, "bash", "boom", false);
    }
    // s0 was evicted: two failures are not enough for a nudge anymore
    expect(guard.observe("s0", "bash", "boom", false)).toBeNull();
  });
});

describe("loop guard hook wiring (T3 intentional pin)", () => {
  // The hook itself needs the OpenCode host, so — like the trace-hook
  // and skill-suggestion prefix pins — the wiring holds as coarse
  // source pins: the execute.after hook counts failures per session,
  // appends the one-line redirect to the error, never throws, and
  // honors the TE_LOOPGUARD kill switch.
  test("execute.after counts failures and appends the nudge to the error", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const hook = src.slice(src.indexOf('"execute.after"'));
    expect(hook).toContain("isLoopGuardEnabled()");
    expect(hook).toContain("loopGuard.observe(");
    expect(hook).toContain("loopGuard.reset(");
    expect(hook).toContain("holder.message");
    // the reversal is documented on the hook comment and travels inline
    // in the nudge itself (formatLoopRedirect in lib/domain)
    expect(src).toContain("TE_LOOPGUARD=off");
  });
  test("the guard never stops the agent alone (no throw/block in the hook)", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    const hook = src.slice(src.indexOf('"execute.after"'));
    expect(hook).toContain("[regulus loop-guard]");
    // the loop-guard block swallows its own failures like the trace hook
    expect(hook).toContain("catch (err)");
  });
});
