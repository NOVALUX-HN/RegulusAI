// Chart/domain/skill discovery (NOV-67 split; behavior unchanged).

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

// filterSeenSkills dedups suggestions against history (harness-total/T2):
// drop any suggested skill whose name already appears in recent messages
// (case-insensitive substring). The agent already saw it; re-injecting
// costs context without new signal. Pure so bun tests pin it without a
// host; empty names never match.
export function filterSeenSkills(suggested: Skill[], messagesText: string): Skill[] {
  const hay = messagesText.toLowerCase();
  return suggested.filter((s) => s.name !== "" && !hay.includes(s.name.toLowerCase()));
}
// RunBin runs the regulus binary; injected so tests can fake it.
export type RunBin = (args: string[]) => Promise<{ exitCode: number; output: string }>
