// Regulus domain helpers — pure functions (no OpenCode imports) so the
// detection logic stays testable from plain Bun scripts.
//
// A Chart declares its lesson domain with one line:
//
//   Domain: go
//
// The context hook reads that line from the Chart the session works on
// and injects that domain's lessons instead of the newest of all
// domains. The line is optional: charts without it keep the old
// newest-first behavior.

// findChartRefs returns the distinct charts/<name>.md references in
// order of appearance.
export function findChartRefs(haystack: string): string[] {
  const out: string[] = []
  for (const m of haystack.matchAll(/charts\/([\w][\w.\-]*\.md)/g)) {
    if (!out.includes(m[0])) out.push(m[0])
  }
  return out
}

// findDomainInChart returns the Domain: line value, or "" when absent.
export function findDomainInChart(text: string): string {
  const m = text.match(/^Domain:\s*([A-Za-z0-9_][\w.\-]*)/m)
  return m ? m[1] : ""
}

// extractPaths returns distinct repo-relative file hints from arbitrary
// text (tool inputs): tokens with a slash and a file extension, no
// URLs, capped. Hints, not facts — the consumer matches them against
// lesson provenance paths.
export function extractPaths(text: string): string[] {
  const out: string[] = []
  for (const m of text.matchAll(/(?<![\w:/?#=&.])[\w.\-]+(?:\/[\w.\-]+)+\.\w{1,8}\b/g)) {
    const p = m[0]
    if (p.includes("://")) continue
    if (!out.includes(p)) out.push(p)
    if (out.length >= 10) break
  }
  return out
}

// RunBin runs the regulus binary; injected so tests can fake it.
export type RunBin = (args: string[]) => Promise<{ exitCode: number; output: string }>

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
// domainFromFiles matches session trace files against lesson
// provenance paths: the first (file, domain) pair with a lesson hit
// wins. Bounded (3 files) and fully defensive.
async function domainFromFiles(runBin: RunBin, sessionID: string): Promise<string> {
  if (sessionID === "") return ""
  const tf = await runBin(["trace", "files", "--session", sessionID])
  if (tf.exitCode !== 0) return ""
  const files = tf.output.split("\n").map((f) => f.trim()).filter((f) => f !== "" && !f.startsWith("(")).slice(0, 3)
  if (files.length === 0) return ""
  const dl = await runBin(["library", "domains"])
  if (dl.exitCode !== 0) return ""
  const domains = dl.output.split("\n").map((d) => d.trim()).filter((d) => d !== "")
  for (const f of files) {
    for (const d of domains) {
      const hit = await runBin(["memory", "search", "--type", "lesson", "--domain", d, "--path", f])
      if (hit.exitCode === 0 && hit.output !== "" && !hit.output.startsWith("no matching")) return d
    }
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
    try {
      const fromFiles = await domainFromFiles(runBin, sessionID)
      if (fromFiles !== "") return fromFiles
    } catch {
      // trace lookup failed — fall through to the library signal
    }
    const res = await runBin(["library", "domains"])
    if (res.exitCode === 0) {
      const domains = res.output.split("\n").map((d) => d.trim()).filter((d) => d !== "")
      if (domains.length === 1) return domains[0]
    }
  } catch {
    // never break context assembly on detection
  }
  return ""
}
