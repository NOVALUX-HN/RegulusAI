// YOLO guardrails (safe auto-approve): the catch-all MUST come first and
// the destructive denies after it — opencode evaluates patterns with the
// last match winning. If someone reorders or drops a deny, deletes stop
// being blocked. `regulus yolo on` flips BOTH the project config and the
// worker agent frontmatter to allow (denies stay denied); `off` restores
// ask. Run with `bun test` from .opencode/ (also in CI).
import { describe, expect, test } from "bun:test";
import { mkdir, mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

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
  const m = text.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n/);
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
    const { json: global } = await projectConfig();
    const globalStar = global?.permission?.bash?.["*"];
    for (const file of files) {
      const text = await Bun.file(`${dir}/${file}`).text();
      const m = text.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n/);
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
        // The worker mirrors the global YOLO switch: `regulus yolo on`
        // flips its catch-all to allow (denies stay denied), `off`
        // restores ask. It must never drift from opencode.json and
        // never allow destruction.
        expect(bash["*"]).toBe(globalStar);
        const head = text.slice(0, text.indexOf("---", 3));
        if (globalStar === "allow") {
          for (const d of DENIES) expect(bash[d]).toBe("deny");
          const starAt = head.indexOf('"*": allow');
          expect(starAt).toBeGreaterThan(-1);
          for (const d of DENIES) {
            const at = head.indexOf(`"${d}": deny`);
            expect(at).toBeGreaterThan(starAt);
          }
        } else {
          expect(bash["*"]).toBe("ask");
          for (const d of DENIES) expect(bash[d]).not.toBe("allow");
        }
      } else {
        // Read-only guardians deny by default and never edit code.
        expect(bash["*"]).toBe("deny");
        for (const d of DENIES) expect(bash[d]).not.toBe("allow");
        expect(fm?.permission?.edit).toBe("deny");
      }
    }
  });

  test(
    "yolo on/off flips the worker frontmatter and reminds /reload",
    async () => {
      const repoRoot = join(import.meta.dir, "..", "..").replaceAll("\\", "/");
      const dir = await mkdtemp(join(tmpdir(), "yolo-worker-"));
      try {
        await writeFile(
          join(dir, "opencode.json"),
          JSON.stringify({ permission: { bash: { "*": "ask", "rm *": "deny" }, edit: "ask" } }),
        );
        const agentDir = join(dir, ".opencode", "agents");
        await mkdir(agentDir, { recursive: true });
        const workerPath = join(agentDir, "regulus-worker.md");
        await writeFile(
          workerPath,
          '---\ndescription: w\nmode: subagent\npermission:\n  bash:\n    "go *": allow\n    "rm *": deny\n    "*": ask\n---\n\nbody\n',
        );
        // Build first (module context), then run the binary in the
        // fixture dir: `go run` cannot resolve a module from a cwd
        // outside one.
        const binName = `regulus-yolo-test${process.platform === "win32" ? ".exe" : ""}`;
        const binPath = join(dir, binName);
        const build = await Bun.$`go build -o ${binPath} ./cmd/regulus`.cwd(repoRoot).quiet().nothrow();
        if (build.exitCode !== 0) throw new Error(`build failed: ${build.stderr.toString()}`);
        const on = await Bun.$`${binPath} yolo on`.cwd(dir).quiet().nothrow();
        if (on.exitCode !== 0) throw new Error(`yolo on failed: ${on.stderr.toString()}`);
        expect(on.exitCode).toBe(0);
        const onOut = on.stdout.toString();
        expect(onOut).toContain("yolo: on");
        expect(onOut).toContain("/reload");
        let worker = await Bun.file(workerPath).text();
        expect(worker).toContain('"*": allow');
        expect(worker).toContain('"rm *": deny');
        expect(worker.indexOf('"*": allow')).toBeLessThan(worker.indexOf('"rm *": deny'));
        expect(worker).toContain('"go *": allow');

        const off = await Bun.$`${binPath} yolo off`.cwd(dir).quiet().nothrow();
        if (off.exitCode !== 0) throw new Error(`yolo off failed: ${off.stderr.toString()}`);
        expect(off.exitCode).toBe(0);
        const offOut = off.stdout.toString();
        expect(offOut).toContain("yolo: off");
        expect(offOut).toContain("/reload");
        worker = await Bun.file(workerPath).text();
        expect(worker).toContain('"*": ask');
        expect(worker).not.toContain('"*": allow');
        expect(worker).toContain('"rm *": deny');
      } finally {
        await rm(dir, { recursive: true, force: true });
      }
    },
    180000,
  );
});
