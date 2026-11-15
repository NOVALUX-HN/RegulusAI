// Trace recording, batching and domain resolution (NOV-67 split).

import type { RunBin } from "./domain-charts.js";
import { findDomainInChart, findChartRefs } from "./domain-charts.js";
import { TtlCache } from "./domain-runtime.js";

// TRACE_RECORD_STR/NUM_FLAGS map `trace record` flags to record-batch
// NDJSON keys (trace.Entry field names). `--files` is the only
// non-scalar: comma-joined on argv, an array in NDJSON (mirroring Go
// splitList). T7 speed-tokens: flush converts queued record argv into
// these objects so one flush costs one `trace record-batch` spawn.
const TRACE_RECORD_STR_FLAGS: ReadonlyMap<string, string> = new Map([
  ["--session", "session"],
  ["--message", "message"],
  ["--call", "call"],
  ["--tool", "tool"],
  ["--status", "status"],
  ["--task", "task"],
  ["--execution-id", "execution_id"],
  ["--model", "model"],
  ["--host", "host"],
  ["--host-version", "host_version"],
  ["--usage-source", "usage_source"],
]);

const TRACE_RECORD_NUM_FLAGS: ReadonlyMap<string, string> = new Map([
  ["--duration-ms", "duration_ms"],
  ["--output-bytes", "output_bytes"],
  ["--input-tokens", "input_tokens"],
  ["--output-tokens", "output_tokens"],
  ["--cache-read-tokens", "cache_read"],
  ["--cache-write-tokens", "cache_write"],
]);

// usageFlags builds the host-usage `trace record` flags from an
// execute.after event usage payload. The current OpenCode plugin API
// does not expose per-call usage on the event (Nov-73 verified against
// the installed SDK: tool/session/agent/message/id/input/status only),
// so a missing or malformed payload yields no flags and the hook keeps
// recording cost without usage — never estimated, never zero-filled.
// When a future host (or SDK) exposes usage, the same flags flow into
// the batcher (maps above) instead of falling back to single spawns.
export function usageFlags(usage: unknown): string[] {
  if (typeof usage !== "object" || usage === null || Array.isArray(usage)) return [];
  const u = usage as Record<string, unknown>;
  const out: string[] = [];
  const num = (v: unknown): number | null =>
    typeof v === "number" && Number.isFinite(v) && v >= 0 && Math.floor(v) === v ? v : null;
  const str = (v: unknown): string | null =>
    typeof v === "string" && v.trim() !== "" && !/[\r\n]/.test(v) && v.length <= 512 ? v : null;
  const tokens: Array<[string, unknown]> = [
    ["--input-tokens", u.input_tokens ?? u.inputTokens],
    ["--output-tokens", u.output_tokens ?? u.outputTokens],
    ["--cache-read-tokens", u.cache_read_tokens ?? u.cacheRead],
    ["--cache-write-tokens", u.cache_write_tokens ?? u.cacheWrite],
  ];
  let anyUsage = false;
  for (const [flag, v] of tokens) {
    const n = num(v);
    if (n !== null && n > 0) {
      out.push(flag, String(n));
      anyUsage = true;
    }
  }
  const texts: Array<[string, unknown]> = [
    ["--model", u.model],
    ["--host", u.host],
    ["--host-version", u.host_version ?? u.hostVersion],
  ];
  for (const [flag, v] of texts) {
    const s = str(v);
    if (s !== null) {
      out.push(flag, s);
      anyUsage = true;
    }
  }
  if (anyUsage) out.push("--usage-source", "measured-host");
  return out;
}

// traceRecordArgvToEntry converts one `trace record` argv into its
// record-batch NDJSON object. Returns null when the argv is not a
// well-formed record (wrong shape, unknown flag, missing required
// value, non-numeric/negative cost) — flush then records that entry
// with its own spawn, preserving the old error surface exactly instead
// of failing the whole batch on one odd entry. Pure so bun tests pin
// the field mapping without spawning.
export function traceRecordArgvToEntry(argv: string[]): Record<string, unknown> | null {
  if (argv.length < 2 || argv[0] !== "trace" || argv[1] !== "record") return null;
  const tail = argv.slice(2);
  if (tail.length === 0 || tail.length % 2 !== 0) return null;
  const out: Record<string, unknown> = {};
  let files: string[] | undefined;
  for (let i = 0; i < tail.length; i += 2) {
    const flag = tail[i];
    const value = tail[i + 1];
    if (value === undefined) return null;
    const sKey = TRACE_RECORD_STR_FLAGS.get(flag);
    if (sKey !== undefined) {
      if (value.trim() === "") return null;
      out[sKey] = value;
      continue;
    }
    if (flag === "--files") {
      files = value.split(",").map((p) => p.trim()).filter((p) => p !== "");
      continue;
    }
    const nKey = TRACE_RECORD_NUM_FLAGS.get(flag);
    if (nKey !== undefined) {
      if (!/^\d+$/.test(value.trim())) return null;
      out[nKey] = Number(value);
      continue;
    }
    return null; // unknown flag — never guess, fall back to a single spawn
  }
  for (const k of ["session", "message", "call", "tool", "status"]) {
    if (typeof out[k] !== "string" || (out[k] as string).trim() === "") return null;
  }
  if (out["status"] !== "completed" && out["status"] !== "error") return null;
  if (files !== undefined && files.length > 0) out["files"] = files;
  return out;
}

// TraceBatcher (harness-total/V1, T7 speed-tokens): the trace-hook
// queue. The hook no longer pays one awaited `trace record` spawn per
// tool call on its critical path: records enqueue and drain in a
// single-flight background flush (cola+flush). flush() drains until
// empty — every queued record is attempted exactly once, in FIFO order
// — so items enqueued mid-flush (concurrent hooks) ride the next batch
// of the same drain instead of starting a second spawn train.
// T7 batches each drain round into a single `trace record-batch
// --ndjson` spawn (entries serialized as NDJSON argv lines): a 10-tool
// burst costs 1 spawn, not 10 sequential ones. A failed batch is
// reported on every entry of the round but never skips the rest; the
// queue is empty afterwards even when every spawn fails. Durability
// matches the old hook (best-effort, failures logged by the caller,
// never thrown); behavior pinned by lib/v1-batch.test.ts.
export interface TraceEntry {
  argv: string[];
  label: string;
}

export interface TraceFlushResult {
  argv: string[];
  label: string;
  ok: boolean;
  error: string;
}

export class TraceBatcher {
  private readonly queue: TraceEntry[] = [];

  enqueue(entry: TraceEntry): void {
    this.queue.push(entry);
  }

  pending(): number {
    return this.queue.length;
  }

  async flush(runBin: RunBin): Promise<TraceFlushResult[]> {
    const results: TraceFlushResult[] = [];
    while (this.queue.length > 0) {
      const batch = this.queue.splice(0, this.queue.length);
      const slots: TraceFlushResult[] = new Array(batch.length);
      const batched: number[] = [];
      const lines: string[] = [];
      const singles: number[] = [];
      batch.forEach((entry, i) => {
        const obj = traceRecordArgvToEntry(entry.argv);
        if (obj === null) singles.push(i);
        else {
          batched.push(i);
          lines.push(JSON.stringify(obj));
        }
      });
      if (lines.length > 0) {
        try {
          const res = await runBin(["trace", "record-batch", "--ndjson", ...lines]);
          for (const i of batched) {
            slots[i] =
              res.exitCode === 0
                ? { argv: batch[i].argv, label: batch[i].label, ok: true, error: "" }
                : { argv: batch[i].argv, label: batch[i].label, ok: false, error: res.output };
          }
        } catch (err) {
          const msg = (err as Error)?.message ?? String(err);
          for (const i of batched) {
            slots[i] = { argv: batch[i].argv, label: batch[i].label, ok: false, error: msg };
          }
        }
      }
      for (const i of singles) {
        const entry = batch[i];
        try {
          const res = await runBin(entry.argv);
          slots[i] =
            res.exitCode !== 0
              ? { argv: entry.argv, label: entry.label, ok: false, error: res.output }
              : { argv: entry.argv, label: entry.label, ok: true, error: "" };
        } catch (err) {
          slots[i] = { argv: entry.argv, label: entry.label, ok: false, error: (err as Error)?.message ?? String(err) };
        }
      }
      results.push(...slots);
    }
    return results;
  }
}

// domainHitCache (harness-total/V1): memoizes (domain, path) lesson-hit
// lookups for 60s per OpenCode process. Session trace files change as the
// session progresses (new paths still spawn), but repeat assemblies over
// the same files skip the `memory search` spawn. Same staleness class as
// the domains list above (an admit shows up after the TTL); reversal:
// restart the host process, or clearDomainHitCache() in tests.
const DOMAIN_HIT_TTL_MS = 60000;
const domainHitCache = new TtlCache<boolean>(DOMAIN_HIT_TTL_MS);

export function clearDomainHitCache(): void {
  domainHitCache.clear();
}

function domainHitKey(domain: string, path: string): string {
  return `${domain}\n${path}`;
}

async function readDomain(path: string): Promise<string> {
  try {
    return findDomainInChart(await Bun.file(path).text())
  } catch {
    return ""
  }
}

// resolveDomain detects the lesson domain for the context hook:
// (1) the Domain: line of a chart referenced in recent messages,
// (2) the Domain: line of the lone chart when the repo has exactly
// one, (3) the lone library domain. "" means unfiltered (the old
// newest-first behavior). Every step is defensive: any failure falls
// through to the next, never throwing into the hook.
// domainsCache memoizes `library domains` per project dir for the life
// of the OpenCode process (T4 measurement: `library domains` is one
// ~50ms binary spawn per context assembly; the list only changes when
// a new domain is admitted, so re-spawning every hook is waste).
// Trace files are NOT cached — they grow with every tool call.
const domainsCache = new Map<string, string[]>()

export function clearDomainsCache(): void {
  domainsCache.clear()
}

export async function getDomains(dir: string, runBin: RunBin): Promise<string[] | null> {
  const hit = domainsCache.get(dir)
  if (hit !== undefined) return hit
  const res = await runBin(["library", "domains"])
  if (res.exitCode !== 0) return null
  const domains = res.output.split("\n").map((d) => d.trim()).filter((d) => d !== "")
  domainsCache.set(dir, domains)
  return domains
}

// domainFromFiles matches session trace files against lesson
// provenance paths: the first (file, domain) pair with a lesson hit
// wins. Bounded (3 files) and fully defensive.
//
// harness-total/V1: every (file, domain) pair probes concurrently
// (Promise.all) and the winner is still the first pair in (file,
// domain) order with a hit — parallelism changes latency, never the
// decision. Cache hits skip the spawn entirely.
async function domainFromFiles(runBin: RunBin, sessionID: string, domains: string[]): Promise<string> {
  if (sessionID === "") return ""
  const tf = await runBin(["trace", "files", "--session", sessionID])
  if (tf.exitCode !== 0) return ""
  const files = tf.output.split("\n").map((f) => f.trim()).filter((f) => f !== "" && !f.startsWith("(")).slice(0, 3)
  if (files.length === 0) return ""
  const pairs: Array<{ file: string; domain: string }> = []
  for (const f of files) {
    for (const d of domains) {
      pairs.push({ file: f, domain: d })
    }
  }
  const hits = await Promise.all(
    pairs.map(async (p) => {
      const key = domainHitKey(p.domain, p.file)
      const cached = domainHitCache.get(key)
      if (cached !== undefined) return cached
      let hit = false
      try {
        const res = await runBin(["memory", "search", "--type", "lesson", "--domain", p.domain, "--path", p.file])
        hit = res.exitCode === 0 && res.output !== "" && !res.output.startsWith("no matching")
      } catch {
        hit = false
      }
      domainHitCache.set(key, hit)
      return hit
    }),
  )
  for (let i = 0; i < pairs.length; i++) {
    if (hits[i]) return pairs[i].domain
  }
  return ""
}

export async function resolveDomain(dir: string, runBin: RunBin, messagesText: string, sessionID = ""): Promise<string> {
  try {
    for (const ref of findChartRefs(messagesText)) {
      const domain = await readDomain(`${dir}/${ref}`)
      if (domain !== "") return domain
    }
    try {
      const charts = Array.from(new Bun.Glob("charts/*.md").scanSync({ cwd: dir }))
      if (charts.length === 1) {
        const domain = await readDomain(`${dir}/${charts[0]}`)
        if (domain !== "") return domain
      }
    } catch {
      // no charts or unreadable dir — fall through
    }
    const domains = await getDomains(dir, runBin)
    if (domains !== null) {
      try {
        const fromFiles = await domainFromFiles(runBin, sessionID, domains)
        if (fromFiles !== "") return fromFiles
      } catch {
        // trace lookup failed — fall through to the library signal
      }
      if (domains.length === 1) return domains[0]
      if (domains.length > 1) {
        console.error(`[regulus] domain: no pin found (${domains.length} domains, no chart/file hit) — using unfiltered lessons (newest-first fallback)`)
      }
    }
  } catch {
    // never break context assembly on detection
  }
  return ""
}
