// Runtime probing, caches and timeouts (NOV-67 split).

import type { ProbeRun } from "./domain-guidance.js";
import { checkContract } from "./domain.js";
import { findDomainInChart } from "./domain-charts.js";
import { staleDoctorRefusal } from "./domain-guidance.js";

// pickRegulus probes each candidate binary in order: doctor must accept
// THIS runtime, then `contract` must answer exactly REGULUS_CONTRACT
// (via checkContract, S3). First match wins (S2); every skip records
// why so the caller fails fast with the full list. A contract-probe
// spawn failure propagates like the plugin's inline loop always did —
// only the doctor probe is guarded per candidate.
export async function pickRegulus(bins: string[], probe: ProbeRun): Promise<{ bin: string; failures: string[] }> {
  const failures: string[] = []
  for (const bin of bins) {
    let res: { exitCode: number; output: string }
    try {
      res = await probe(bin, ["doctor"])
    } catch (err) {
      failures.push(`${bin}: ${(err as Error).message}`)
      continue
    }
    if (res.exitCode !== 0) {
      failures.push(`${bin}: ${res.output}`)
      continue
    }
    // A runnable core on an older surface answers the contract but
    // breaks plugin commands (stale-core refusal above): skip it with
    // its rebuild reason before spending the contract probe.
    const stale = staleDoctorRefusal(res.output)
    if (stale !== "") {
      failures.push(`${bin}: ${stale}`)
      continue
    }
    const refusal = checkContract(await probe(bin, ["contract"]))
    if (refusal === "") {
      return { bin, failures }
    }
    failures.push(`${bin}: ${refusal}`)
  }
  return { bin: "", failures }
}


// withTimeout races promise work against a millisecond budget (and an
// optional outer AbortSignal): hooks spawn a local binary that must
// never hang context assembly forever. The timer is always cleared.
export function withTimeout<T>(work: Promise<T>, ms: number, signal?: AbortSignal): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new Error("regulus: aborted before start"))
      return
    }
    const timer = setTimeout(() => {
      reject(new Error(`regulus: binary timed out after ${ms}ms`))
    }, ms)
    const onAbort = () => {
      clearTimeout(timer)
      reject(new Error("regulus: aborted"))
    }
    signal?.addEventListener("abort", onAbort, { once: true })
    work.then(
      (v) => {
        clearTimeout(timer)
        signal?.removeEventListener("abort", onAbort)
        resolve(v)
      },
      (e) => {
        clearTimeout(timer)
        signal?.removeEventListener("abort", onAbort)
        reject(e)
      },
    )
  })
}

// TtlCache is a tiny time-bounded memo: hook-frequent, slow-changing
// reads (admitted lessons per domain) skip the binary spawn while
// fresh, then refetch. Staleness window is explicit per use.
export class TtlCache<V> {
  private readonly ttlMs: number
  private readonly entries = new Map<string, { at: number; value: V }>()
  constructor(ttlMs: number) {
    this.ttlMs = ttlMs
  }
  get(key: string): V | undefined {
    const hit = this.entries.get(key)
    if (hit === undefined) return undefined
    if (Date.now() - hit.at > this.ttlMs) {
      this.entries.delete(key)
      return undefined
    }
    return hit.value
  }
  set(key: string, value: V): void {
    this.entries.set(key, { at: Date.now(), value })
  }
  clear(): void {
    this.entries.clear()
  }
}
