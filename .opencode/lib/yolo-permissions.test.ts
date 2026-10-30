// YOLO guardrails (safe auto-approve): the catch-all MUST come first and
// the destructive denies after it — opencode evaluates patterns with the
// last match winning. If someone reorders or drops a deny, deletes stop
// being blocked. Run with `bun test` from .opencode/ (also in CI).
import { describe, expect, test } from "bun:test";
import { readdir } from "node:fs/promises";

// Every pattern that must stay denied under YOLO.
const DENIES = [
  "rm *",
  "rmdir *",
  "del *",
  "rd *",
  "git reset --hard*",
  "git clean *",
  "git push *force*",
  "git branch -D*",
  "git stash drop*",
  "git rm*",
  "git checkout --*",
  "git restore*",
  "git rebase*",
  "format *",
  "mkfs*",
  "dd *",
];

async function projectConfig(): Promise<{ text: string; json: any }> {
  const text = await Bun.file(`${import.meta.dir}/../../opencode.json`).text();
  return { text, json: JSON.parse(text) };
}

async function regulusAgent(): Promise<{ text: string; fm: any }> {
  const text = await Bun.file(`${import.meta.dir}/../agents/regulus.md`).text();
  const m = text.match(/^---\n([\s\S]*?)\n---\n/);
  if (!m) throw new Error("regulus.md has no frontmatter");
  return { text, fm: Bun.YAML.parse(m[1]) as any };
}

describe("yolo permissions", () => {
  test("project config allows by default, denies destruction, catch-all first", async () => {
    const { text, json } = await projectConfig();
    const bash = json?.permission?.bash;
    expect(typeof bash).toBe("object");
    expect(bash["*"]).toBe("allow");
    for (const d of DENIES) expect(bash[d]).toBe("deny");
    // Last match wins: the "*" allow must precede every deny in the file.
    const allowAt = text.indexOf('"*": "allow"');
    expect(allowAt).toBeGreaterThan(-1);
    for (const d of DENIES) {
      const at = text.indexOf(`"${d}": "deny"`);
      expect(at).toBeGreaterThan(allowAt);
    }
    expect(json?.permission?.doom_loop).toBe("ask");
    const edit = json?.permission?.edit;
    expect(typeof edit).toBe("object");
    expect(edit["*"]).toBe("allow");
    expect(edit["*.env"]).toBe("ask");
    expect(edit["*.env.*"]).toBe("ask");
    expect(edit["*.env.example"]).toBe("allow");
  });

  test("regulus agent mirrors the guardrails and stays primary", async () => {
    const { text, fm } = await regulusAgent();
    expect(fm.mode).toBe("primary");
    const bash = fm?.permission?.bash;
    expect(bash?.["*"]).toBe("allow");
    for (const d of DENIES) expect(bash?.[d]).toBe("deny");
    const edit = fm?.permission?.edit;
    expect(typeof edit).toBe("object");
    expect(edit?.["*"]).toBe("allow");
    expect(edit?.["*.env"]).toBe("ask");
    const head = text.slice(0, text.indexOf("---", 3));
    const allowAt = head.indexOf('"*": allow');
    expect(allowAt).toBeGreaterThan(-1);
    for (const d of DENIES) {
      const at = head.indexOf(`"${d}": deny`);
      expect(at).toBeGreaterThan(allowAt);
    }
  });

  test("every agent file honors the guardrails (all 11 agents)", async () => {
    const dir = `${import.meta.dir}/../agents`;
    const files = (await readdir(dir)).filter((f) => f.endsWith(".md")).sort();
    expect(files.length).toBe(11);
    for (const file of files) {
      const text = await Bun.file(`${dir}/${file}`).text();
      const m = text.match(/^---\n([\s\S]*?)\n---\n/);
      expect(m).not.toBeNull();
      const fm = Bun.YAML.parse(m![1]) as any;
      const bash = fm?.permission?.bash;
      expect(typeof bash).toBe("object");
      if (file === "regulus.md") {
        // The primary agent is the only one with safe-YOLO allow: it
        // must carry every destructive deny (DENIES pattern, extended).
        expect(fm.mode).toBe("primary");
        expect(bash["*"]).toBe("allow");
        for (const d of DENIES) expect(bash[d]).toBe("deny");
      } else if (file === "regulus-worker.md") {
        // The worker asks by default: never allow, never allow destruction.
        expect(bash["*"]).toBe("ask");
        for (const d of DENIES) expect(bash[d]).not.toBe("allow");
      } else {
        // Read-only guardians deny by default and never edit code.
        expect(bash["*"]).toBe("deny");
        for (const d of DENIES) expect(bash[d]).not.toBe("allow");
        expect(fm?.permission?.edit).toBe("deny");
      }
    }
  });
});
