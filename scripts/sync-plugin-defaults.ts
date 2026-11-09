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
  const lines = md.replace(/\r\n/g, "\n").split("\n")
  const start = lines.findIndex((l) => l.startsWith(DISCIPLINE_MARKER))
  if (start < 0) throw new Error("sync-plugin-defaults: discipline marker line missing")
  return lines.slice(start).join("\n").trim()
}

// renderGuidanceDefaults collapses each `## <name>` section to one
// line — the same collapsing parseToolGuidance applies at runtime, so
// the compiled literal and the live merge can never disagree.
export function renderGuidanceDefaults(md: string): Record<string, string> {
  const out: Record<string, string> = {}
  // CRLF-tolerant (H1/S1): Windows checkouts (core.autocrlf=true) hand
  // the .md sources with \r\n; normalize first so the render is byte-
  // identical to LF. trim()/\s+ already absorbed \r, this locks it in.
  // slice(1) drops the preamble before the first ## (title + intro) —
  // it is file-only by design, like the discipline header. The runtime
  // parseToolGuidance must skip it the same way (see lib/domain.ts).
  for (const part of md.replace(/\r\n/g, "\n").split(/^##\s+/m).slice(1)) {
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
  // Scan for the closing unescaped backtick (S4: no comment anchor —
  // comments must stay freely editable; --write escapes ` as \`
  // through tsBacktick, so a backtick behind an odd run of
  // backslashes is literal text, not the terminator).
  let i = start + marker.length;
  for (;;) {
    const q = ts.indexOf("`", i);
    if (q < 0) throw new Error("sync-plugin-defaults: DEFAULT_DISCIPLINE terminator not found")
    let bs = 0;
    for (let k = q - 1; k >= start + marker.length && ts[k] === "\\"; k--) bs++;
    if (bs % 2 === 0) return ts.slice(start + marker.length, q);
    i = q + 1;
  }
}

// extractGuidanceBlock slices the compiled-guidance literal (S4: brace
// match with string awareness, no comment anchor — see above).
export function extractGuidanceBlock(ts: string): string {
  const marker = "const DEFAULT_TOOL_GUIDANCE";
  const start = ts.indexOf(marker);
  if (start < 0) throw new Error("sync-plugin-defaults: DEFAULT_TOOL_GUIDANCE block not found")
  const open = ts.indexOf("{", start);
  if (open < 0) throw new Error("sync-plugin-defaults: DEFAULT_TOOL_GUIDANCE terminator not found")
  let depth = 0;
  let inStr: string | null = null;
  for (let k = open; k < ts.length; k++) {
    const ch = ts[k];
    if (inStr !== null) {
      if (ch === "\\") k++;
      else if (ch === inStr) inStr = null;
      continue;
    }
    if (ch === '"' || ch === "'" || ch === "`") inStr = ch;
    else if (ch === "{") depth++;
    else if (ch === "}") {
      depth--;
      if (depth === 0) return ts.slice(start, k + 1);
    }
  }
  throw new Error("sync-plugin-defaults: DEFAULT_TOOL_GUIDANCE terminator not found")
}

// TS_EXCLUDED_SECTIONS are ## sections that tool-guidance.md keeps as
// the readable source but the TS compiled fallback intentionally omits
// (T6 speed-tokens). `search-default` moved to its skill's reach — the
// md section (still merged at runtime, still copied by init) plus the
// docs/search-default.md recipe — so simple sessions stop paying the
// fallback chars for a default they never inject. runCheck passes when
// an excluded section is missing from the compiled literal, and fails
// when it is (still) compiled: the exclusion documents the move, not a
// drift to re-add silently.
const TS_EXCLUDED_SECTIONS = new Set(["search-default"]);

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
  const block = extractGuidanceBlock(ts)
  const compiled: Record<string, string> = {}
  // Quoted-key tolerant: hyphenated sections (e.g. ## search-default)
  // compile to "name": "..." entries, which bare (\w+): never matches.
  for (const m of block.matchAll(/["']?([\w-]+)["']?\s*:\s*\n?\s*"((?:[^"\\]|\\.)*)"/g)) compiled[m[1]] = m[2]
  for (const [name, body] of Object.entries(rendered)) {
    if (TS_EXCLUDED_SECTIONS.has(name)) continue
    if (!(name in compiled)) problems.push(`guidance: section ## ${name} missing from compiled defaults`)
    else if (compiled[name] !== tsDq(body)) problems.push(`guidance: ## ${name} differs from compiled default`)
  }
  for (const name of Object.keys(compiled)) {
    if (TS_EXCLUDED_SECTIONS.has(name)) problems.push(`guidance: compiled default ${name} is skill-moved (see TS_EXCLUDED_SECTIONS) — remove it`)
    else if (!(name in rendered)) problems.push(`guidance: compiled default ${name} has no ## section`)
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
  for (const name of TS_EXCLUDED_SECTIONS) {
    // Strip skill-moved sections so --write enforces the move instead
    // of re-adding it: remove the whole property line (a TS object
    // tolerates the surviving trailing comma on the previous entry).
    ts = ts.replace(new RegExp(`\n\\s*["']?${name}["']?\\s*:\\s*(?:\\n\\s*)?"(?:[^"\\\\]|\\\\.)*"\\s*,?`), "")
  }
  for (const [name, body] of Object.entries(rendered)) {
    const re = new RegExp(`(["']?${name}["']?\\s*:\\s*\\n?\\s*\")((?:[^\"\\\\]|\\\\.)*)(\")`)
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
