// Codegen for the plugin's compiled fallback literals (S10): the .md
// files are the editable source of truth; DEFAULT_DISCIPLINE and
// DEFAULT_TOOL_GUIDANCE in .opencode/plugins/regulus.ts are compiled
// fallbacks for checkouts missing those files. This script renders the
// literals from the .md sources so the duplication never drifts by hand.
//
// Usage: `bun scripts/sync-plugin-defaults.ts --check` (exit 0 in sync,
// exit 1 with the first mismatch) or `--write` (rewrite the TS block).
// Unit-tested via .opencode/lib/domain.test.ts ("plugin defaults
// codegen") against the real files — no shelling out from tests.
import { existsSync } from "node:fs"

const DISCIPLINE_MARKER = "Regulus discipline (evidence-first"

// renderDisciplineDefault extracts the prompt core from discipline.md:
// everything from the marker line to the end, trimmed. The `#` title
// and intro paragraphs stay file-only by design (a fallback must not
// inject meta-commentary into the system prompt).
export function renderDisciplineDefault(md: string): string {
  const lines = md.split("\n")
  const start = lines.findIndex((l) => l.startsWith(DISCIPLINE_MARKER))
  if (start < 0) throw new Error("sync-plugin-defaults: discipline marker line missing")
  return lines.slice(start).join("\n").trim()
}

// renderGuidanceDefaults collapses each `## <name>` section to one
// line — the same collapsing parseToolGuidance applies at runtime, so
// the compiled literal and the live merge can never disagree.
export function renderGuidanceDefaults(md: string): Record<string, string> {
  const out: Record<string, string> = {}
  // slice(1) drops the preamble before the first ## (title + intro) —
  // it is file-only by design, like the discipline header. The runtime
  // parseToolGuidance must skip it the same way (see regulus.ts).
  for (const part of md.split(/^##\s+/m).slice(1)) {
    const nl = part.indexOf("\n")
    if (nl < 0) continue
    const name = part.slice(0, nl).trim()
    const body = part.slice(nl + 1).replace(/\s+/g, " ").trim()
    if (name !== "" && body !== "") out[name] = body
  }
  return out
}

function tsBacktick(s: string): string {
  return s.replace(/\\/g, "\\\\").replace(/`/g, "\\`").replace(/\$\{/g, "\\${")
}

function tsDq(s: string): string {
  return s
    .replace(/\\/g, "\\\\")
    .replace(/"/g, '\\"')
    .replace(/\n/g, "\\n")
}

export function extractDisciplineLiteral(ts: string): string {
  const marker = "const DEFAULT_DISCIPLINE = `";
  const start = ts.indexOf(marker);
  if (start < 0) throw new Error("sync-plugin-defaults: DEFAULT_DISCIPLINE block not found")
  const end = ts.indexOf("`\n\n// loadDiscipline", start + marker.length);
  if (end < 0) throw new Error("sync-plugin-defaults: DEFAULT_DISCIPLINE terminator not found")
  return ts.slice(start + marker.length, end)
}

async function runCheck(tree: string): Promise<string[]> {
  const problems: string[] = []
  const disciplineMd = await Bun.file(`${tree}/.opencode/discipline.md`).text()
  const guidanceMd = await Bun.file(`${tree}/.opencode/tool-guidance.md`).text()
  const ts = await Bun.file(`${tree}/.opencode/plugins/regulus.ts`).text()
  let want: string
  try {
    want = renderDisciplineDefault(disciplineMd)
  } catch (e) {
    return [`discipline: ${(e as Error).message}`]
  }
  if (extractDisciplineLiteral(ts) !== want) problems.push("discipline: DEFAULT_DISCIPLINE differs from .md core (run --write)")
  const rendered = renderGuidanceDefaults(guidanceMd)
  const block = ts.slice(ts.indexOf("const DEFAULT_TOOL_GUIDANCE"), ts.indexOf("// parseToolGuidance"))
  const compiled: Record<string, string> = {}
  for (const m of block.matchAll(/(\w+):\s*\n?\s*"((?:[^"\\]|\\.)*)"/g)) compiled[m[1]] = m[2]
  for (const [name, body] of Object.entries(rendered)) {
    if (!(name in compiled)) problems.push(`guidance: section ## ${name} missing from compiled defaults`)
    else if (compiled[name] !== tsDq(body)) problems.push(`guidance: ## ${name} differs from compiled default`)
  }
  for (const name of Object.keys(compiled)) {
    if (!(name in rendered)) problems.push(`guidance: compiled default ${name} has no ## section`)
  }
  return problems
}

async function runWrite(tree: string): Promise<void> {
  const disciplineMd = await Bun.file(`${tree}/.opencode/discipline.md`).text()
  const guidanceMd = await Bun.file(`${tree}/.opencode/tool-guidance.md`).text()
  const tsPath = `${tree}/.opencode/plugins/regulus.ts`
  let ts = await Bun.file(tsPath).text()
  const current = extractDisciplineLiteral(ts)
  ts = ts.replace(
    `const DEFAULT_DISCIPLINE = \`${current}\``,
    () => `const DEFAULT_DISCIPLINE = \`${tsBacktick(renderDisciplineDefault(disciplineMd))}\``,
  )
  const rendered = renderGuidanceDefaults(guidanceMd)
  for (const [name, body] of Object.entries(rendered)) {
    const re = new RegExp(`(${name}:\\s*\\n?\\s*\")((?:[^\"\\\\]|\\\\.)*)(\")`)
    if (re.test(ts)) ts = ts.replace(re, (_m, pre, _old, post) => `${pre}${tsDq(body)}${post}`)
  }
  await Bun.write(tsPath, ts)
}

if (import.meta.main) {
  const tree = process.cwd()
  const mode = process.argv[2] ?? "--check"
  if (mode === "--write") {
    await runWrite(tree)
    console.log("sync-plugin-defaults: literals regenerated from .md")
  } else if (mode === "--check" || !existsSync(`${tree}/.opencode/plugins/regulus.ts`)) {
    const problems = await runCheck(tree)
    if (problems.length > 0) {
      for (const p of problems) console.error(`sync-plugin-defaults: ${p}`)
      process.exit(1)
    }
    console.log("sync-plugin-defaults: in sync")
  } else {
    console.error("usage: bun scripts/sync-plugin-defaults.ts [--check|--write]")
    process.exit(2)
  }
}
