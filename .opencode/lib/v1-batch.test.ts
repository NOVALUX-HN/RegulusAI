// harness-total/V1: trace-hook batch/async (cola+flush) + domainFromFiles
// paraleliza+cachea sin romper orden.
//
// Falla-antes: TraceBatcher no existe y domainFromFiles es secuencial sin
// cache — estos tests fallan hasta implementar V1. La no-perdida va con
// prueba (drain FIFO total, fallos no saltan items), no con afirmacion.
import { describe, expect, test } from "bun:test";
import {
  clearDomainHitCache,
  clearDomainsCache,
  resolveDomain,
  TraceBatcher,
  type RunBin,
} from "./domain";

const ok = { exitCode: 0, output: "trace recorded" };

function recordArgv(call: string): string[] {
  return ["trace", "record", "--session", "s1", "--message", "m", "--call", call, "--tool", "read", "--status", "completed"];
}

describe("TraceBatcher (harness-total/V1, falla-antes)", () => {
  test("flush drains every enqueued record in FIFO order, none lost", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return ok;
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c3"), label: "session s1" });
    expect(q.pending()).toBe(3);
    const results = await q.flush(runBin);
    // T7 speed-tokens: one flush batches into a single record-batch
    // spawn; FIFO order survives inside the NDJSON lines and results.
    expect(seen).toHaveLength(1);
    expect(seen[0].slice(0, 3)).toEqual(["trace", "record-batch", "--ndjson"]);
    expect(seen[0].slice(3).map((l) => (JSON.parse(l) as { call: string }).call)).toEqual(["c1", "c2", "c3"]);
    expect(results.map((r) => r.ok)).toEqual([true, true, true]);
    expect(q.pending()).toBe(0);
  });

  test("a failed batch is reported on every entry, none lost (no loss)", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return { exitCode: 1, output: "boom" };
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c3"), label: "session s1" });
    const results = await q.flush(runBin);
    // One batch spawn carried all three; its failure lands on every
    // entry result (best-effort, never thrown) and drains the queue.
    expect(seen).toHaveLength(1);
    expect(results.map((r) => r.ok)).toEqual([false, false, false]);
    expect(results[1].error).toContain("boom");
    expect(q.pending()).toBe(0);
  });

  test("items enqueued mid-flush ride the same drain (cola+flush coalescing)", async () => {
    const seen: string[][] = [];
    let q: TraceBatcher | null = null;
    const runBin: RunBin = async (a) => {
      seen.push(a);
      const body = a.slice(3).join("\n");
      if (body.includes('"c1"') && q !== null && q.pending() === 0) {
        q.enqueue({ argv: recordArgv("c-late"), label: "session s1" });
      }
      return ok;
    };
    q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    const results = await q.flush(runBin);
    // The in-flight batch already left, so the late entry rides the
    // next batch of the same drain: same flush() call, two spawns,
    // nothing lost, queue empty.
    expect(seen).toHaveLength(2);
    expect(seen[0].slice(0, 3)).toEqual(["trace", "record-batch", "--ndjson"]);
    expect(seen[1].slice(0, 3)).toEqual(["trace", "record-batch", "--ndjson"]);
    expect(seen[1].slice(3).map((l) => (JSON.parse(l) as { call: string }).call)).toEqual(["c-late"]);
    expect(results).toHaveLength(2);
    expect(q.pending()).toBe(0);
  });

  test("spawn throw (abort/ENOENT) is reported on the batch, never thrown", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      throw new Error("spawn ENOENT");
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    const results = await q.flush(runBin);
    expect(seen).toHaveLength(1);
    expect(results.map((r) => r.ok)).toEqual([false, false]);
    expect(results[0].error).toContain("ENOENT");
    expect(q.pending()).toBe(0);
  });

  test("trace hook batches through the queue with guaranteed flush (source pin)", async () => {
    const src = await Bun.file(import.meta.dir + "/../plugins/regulus.ts").text();
    expect(src).toContain("TraceBatcher");
    // Honest logging stays: failures name the session, never swallowed.
    expect(src).toContain("[regulus] trace hook");
  });
});

describe("domainFromFiles parallel+cache (harness-total/V1, falla-antes)", () => {
  async function mkTmpCharts(files: Record<string, string>): Promise<string> {
    const dir = `${process.env.TMPDIR ?? "/tmp"}/v1-test-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    await Bun.$`mkdir -p ${dir}/charts`.quiet();
    for (const [name, content] of Object.entries(files)) await Bun.write(`${dir}/charts/${name}`, content);
    return dir;
  }
  async function rmTmp(dir: string): Promise<void> {
    await Bun.$`rm -rf ${dir}`.quiet();
  }
  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

  test("searches run concurrently, not one spawn at a time", async () => {
    clearDomainsCache();
    clearDomainHitCache();
    const dir = await mkTmpCharts({ "a.md": "# A\nno domain\n", "b.md": "# B\nno domain\n" });
    try {
      let inflight = 0;
      let maxInflight = 0;
      let searches = 0;
      const runBin: RunBin = async (a) => {
        if (a[0] === "library") return { exitCode: 0, output: "d1\nd2\nd3\n" };
        if (a[0] === "trace") return { exitCode: 0, output: "f1.go\nf2.go\n" };
        searches++;
        inflight++;
        maxInflight = Math.max(maxInflight, inflight);
        await sleep(40);
        inflight--;
        return { exitCode: 0, output: "no matching observations" };
      };
      await expect(resolveDomain(dir, runBin, JSON.stringify([{ t: "hello" }]), "sess-par")).resolves.toBe("");
      expect(searches).toBe(2 * 3);
      // Sequential code never overlaps spawns; parallel code does.
      expect(maxInflight).toBeGreaterThan(1);
    } finally {
      await rmTmp(dir);
      clearDomainsCache();
      clearDomainHitCache();
    }
  });

  test("ordered winner wins even when a later pair resolves first", async () => {
    clearDomainsCache();
    clearDomainHitCache();
    const dir = await mkTmpCharts({ "a.md": "# A\nno domain\n", "b.md": "# B\nno domain\n" });
    try {
      const runBin: RunBin = async (a) => {
        if (a[0] === "library") return { exitCode: 0, output: "d1\nd2\n" };
        if (a[0] === "trace") return { exitCode: 0, output: "fa.go\nfb.go\n" };
        const d = a[a.indexOf("--domain") + 1];
        const p = a[a.indexOf("--path") + 1];
        // Logical-first hit is slow; logical-last hit is fast. A racy
        // first-settled-wins impl would return d2; ordered wins => d1.
        if (d === "d1" && p === "fa.go") {
          await sleep(60);
          return { exitCode: 0, output: "1 hit(s)\nobs_x lesson t (saved just now)\n" };
        }
        if (d === "d2" && p === "fb.go") {
          await sleep(5);
          return { exitCode: 0, output: "1 hit(s)\nobs_y lesson t (saved just now)\n" };
        }
        await sleep(5);
        return { exitCode: 0, output: "no matching observations" };
      };
      await expect(resolveDomain(dir, runBin, JSON.stringify([{ t: "hello" }]), "sess-order")).resolves.toBe("d1");
    } finally {
      await rmTmp(dir);
      clearDomainsCache();
      clearDomainHitCache();
    }
  });

  test("second assembly reuses cached hits: no new memory search spawns", async () => {
    clearDomainsCache();
    clearDomainHitCache();
    const dir = await mkTmpCharts({ "a.md": "# A\nno domain\n", "b.md": "# B\nno domain\n" });
    try {
      let searches = 0;
      const runBin: RunBin = async (a) => {
        if (a[0] === "library") return { exitCode: 0, output: "d1\nd2\n" };
        if (a[0] === "trace") return { exitCode: 0, output: "fa.go\nfb.go\n" };
        searches++;
        const d = a[a.indexOf("--domain") + 1];
        const p = a[a.indexOf("--path") + 1];
        if (d === "d2" && p === "fb.go") return { exitCode: 0, output: "1 hit(s)\nobs_y lesson t (saved just now)\n" };
        return { exitCode: 0, output: "no matching observations" };
      };
      await expect(resolveDomain(dir, runBin, JSON.stringify([{ t: "hello" }]), "sess-cache")).resolves.toBe("d2");
      const afterFirst = searches;
      expect(afterFirst).toBeGreaterThan(0);
      await expect(resolveDomain(dir, runBin, JSON.stringify([{ t: "hello" }]), "sess-cache")).resolves.toBe("d2");
      expect(searches).toBe(afterFirst);
    } finally {
      await rmTmp(dir);
      clearDomainsCache();
      clearDomainHitCache();
    }
  });
});

describe("TraceBatcher record-batch (speed-tokens/T7, falla-antes)", () => {
  function recordArgvFull(call: string): string[] {
    return [
      "trace", "record", "--session", "s1", "--message", "m", "--call", call,
      "--tool", "read", "--status", "completed",
      "--files", "a.go, b.go", "--task", "t7",
      "--duration-ms", "120", "--output-bytes", "2048",
      "--input-tokens", "10", "--output-tokens", "5",
    ];
  }

  test("one flush of N entries costs exactly 1 spawn, all reported ok", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return ok;
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c3"), label: "session s1" });
    const results = await q.flush(runBin);
    expect(seen).toHaveLength(1);
    expect(seen[0].slice(0, 3)).toEqual(["trace", "record-batch", "--ndjson"]);
    expect(seen[0].length).toBe(3 + 3);
    expect(results.map((r) => r.ok)).toEqual([true, true, true]);
    expect(results.map((r) => r.label)).toEqual(["session s1", "session s1", "session s1"]);
    expect(q.pending()).toBe(0);
  });

  test("NDJSON lines carry every record field incl. duration/output-bytes", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return ok;
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgvFull("c9"), label: "session s1" });
    await q.flush(runBin);
    expect(seen).toHaveLength(1);
    const lines = seen[0].slice(3);
    expect(lines).toHaveLength(1);
    expect(JSON.parse(lines[0])).toEqual({
      session: "s1", message: "m", call: "c9", tool: "read", status: "completed",
      files: ["a.go", "b.go"], task: "t7",
      duration_ms: 120, output_bytes: 2048, input_tokens: 10, output_tokens: 5,
    });
  });

  test("batch failure marks every entry failed, never throws, queue drained", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return { exitCode: 1, output: "line 2: boom" };
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    const results = await q.flush(runBin);
    expect(seen).toHaveLength(1);
    expect(results.map((r) => r.ok)).toEqual([false, false]);
    expect(results[0].error).toContain("boom");
    expect(results[1].error).toContain("boom");
    expect(q.pending()).toBe(0);
  });

  test("batch spawn throw marks every entry failed, never throws", async () => {
    const runBin: RunBin = async () => {
      throw new Error("spawn ENOENT");
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: recordArgv("c2"), label: "session s1" });
    const results = await q.flush(runBin);
    expect(results.map((r) => r.ok)).toEqual([false, false]);
    expect(results[0].error).toContain("ENOENT");
    expect(q.pending()).toBe(0);
  });

  test("non-record argv falls back to its own spawn (batch + single)", async () => {
    const seen: string[][] = [];
    const runBin: RunBin = async (a) => {
      seen.push(a);
      return ok;
    };
    const q = new TraceBatcher();
    q.enqueue({ argv: recordArgv("c1"), label: "session s1" });
    q.enqueue({ argv: ["trace", "show", "--session", "s1"], label: "session s1" });
    const results = await q.flush(runBin);
    expect(seen).toHaveLength(2);
    expect(seen[0].slice(0, 3)).toEqual(["trace", "record-batch", "--ndjson"]);
    expect(seen[1]).toEqual(["trace", "show", "--session", "s1"]);
    expect(results.map((r) => r.ok)).toEqual([true, true]);
    expect(q.pending()).toBe(0);
  });
});
