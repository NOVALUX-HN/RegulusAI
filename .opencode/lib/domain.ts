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

// Skill is one trigger entry: the skill name plus its one-line
// trigger (the SKILL.md frontmatter description).
export interface Skill {
  name: string;
  description: string;
}

// loadSkills reads the repo's skill registry from SKILL.md frontmatter.
// Missing skills dir or unreadable files yield an empty registry — the
// hook simply suggests nothing.
export async function loadSkills(dir: string): Promise<Skill[]> {
  const out: Skill[] = [];
  try {
    for (const path of Array.from(new Bun.Glob("skills/*/SKILL.md").scanSync({ cwd: `${dir}/.opencode` }))) {
      try {
        const text = await Bun.file(`${dir}/.opencode/${path}`).text();
        const head = text.split("##", 1)[0];
        let name = "";
        let description = "";
        for (const line of head.split("\n")) {
          const t = line.trim();
          if (t.startsWith("name:")) name = t.slice(5).trim();
          else if (t.startsWith("description:")) description = t.slice(12).trim();
        }
        if (name === "") name = path.split("/")[1] ?? path;
        out.push({ name, description });
      } catch {
        // unreadable skill — skip it, keep the rest
      }
    }
  } catch {
    // no skills dir — no suggestions
  }
  out.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  return out;
}

const SKILL_STOPWORDS = new Set([
  "about", "after", "before", "with", "from", "that", "this", "with", "your", "when", "whenever",
  "which", "their", "them", "they", "then", "than", "there", "these", "those", "through",
]);

// distinctiveWords lowercases text into length>=5 non-stopword tokens.
export function distinctiveWords(text: string): string[] {
  const out: string[] = [];
  for (const w of text.toLowerCase().split(/[^a-z0-9]+/)) {
    if (w.length >= 5 && !SKILL_STOPWORDS.has(w) && !out.includes(w)) out.push(w);
  }
  return out;
}

// suggestSkills matches recent message words against each skill's
// name+description. A skill scores one point per shared distinctive
// word; only skills with >=2 points are suggested, best first, at most
// two. Conservative by design: a wrong suggestion costs context.
export function suggestSkills(skills: Skill[], messagesText: string): Skill[] {
  const hay = new Set(distinctiveWords(messagesText));
  if (hay.size === 0) return [];
  const scored = skills
    .map((s) => ({
      skill: s,
      score: distinctiveWords(`${s.name} ${s.description}`).filter((w) => hay.has(w)).length,
    }))
    .filter((e) => e.score >= 2)
    .sort((a, b) => b.score - a.score || (a.skill.name < b.skill.name ? -1 : 1));
  return scored.slice(0, 2).map((e) => e.skill);
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
// domainsCache memoizes `library domains` per project dir for the life
// of the OpenCode process (T4 measurement: `library domains` is one
// ~50ms binary spawn per context assembly; the list only changes when
// a new domain is admitted, so re-spawning every hook is waste).
// Trace files are NOT cached — they grow with every tool call.
const domainsCache = new Map<string, string[]>()

export function clearDomainsCache(): void {
  domainsCache.clear()
}

async function getDomains(dir: string, runBin: RunBin): Promise<string[] | null> {
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
async function domainFromFiles(runBin: RunBin, sessionID: string, domains: string[]): Promise<string> {
  if (sessionID === "") return ""
  const tf = await runBin(["trace", "files", "--session", sessionID])
  if (tf.exitCode !== 0) return ""
  const files = tf.output.split("\n").map((f) => f.trim()).filter((f) => f !== "" && !f.startsWith("(")).slice(0, 3)
  if (files.length === 0) return ""
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
    const domains = await getDomains(dir, runBin)
    if (domains !== null) {
      try {
        const fromFiles = await domainFromFiles(runBin, sessionID, domains)
        if (fromFiles !== "") return fromFiles
      } catch {
        // trace lookup failed — fall through to the library signal
      }
      if (domains.length === 1) return domains[0]
    }
  } catch {
    // never break context assembly on detection
  }
  return ""
}
