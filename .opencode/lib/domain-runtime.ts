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
