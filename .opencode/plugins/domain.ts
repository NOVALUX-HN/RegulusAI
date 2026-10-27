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
export async function resolveDomain(dir: string, runBin: RunBin, messagesText: string): Promise<string> {
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
