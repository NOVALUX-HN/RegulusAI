// Gate spawn budgets, results and spawn-error taxonomy (NOV-67 split).

// GATE_RUN_TIMEOUT_MS_DEFAULT bounds one plugin gate_run spawn. The
// gauntlet runs up to 9 gates at up to 600s each, so the old 30s spawn
// budget aborted every real gauntlet and surfaced as null. One hour
// covers any realistic gauntlet (observed: minutes); the caller's
// AbortSignal still cancels earlier. REGULUS_GATE_RUN_TIMEOUT_MS
// overrides in milliseconds, floored at 600s.
export const GATE_RUN_TIMEOUT_MS_DEFAULT = 3_600_000;
export const GATE_RUN_TIMEOUT_MS_FLOOR = 600_000;

// gateRunTimeoutMs resolves the gate_run spawn budget: an explicit
// finite number clamps to the 600s floor, anything else (missing,
// blank, non-numeric) falls back to the one-hour default.
export function gateRunTimeoutMs(env?: string): number {
  const raw = env ?? process.env.REGULUS_GATE_RUN_TIMEOUT_MS ?? "";
  if (raw.trim() === "") return GATE_RUN_TIMEOUT_MS_DEFAULT;
  const n = Number(raw);
  if (!Number.isFinite(n)) return GATE_RUN_TIMEOUT_MS_DEFAULT;
  return Math.max(GATE_RUN_TIMEOUT_MS_FLOOR, Math.floor(n));
}

// GATE_RUN_OUTPUT_MAX bounds the gate_run tool content verbatim,
// mirroring receipts.MaxCapture (8192): a gauntlet run prints minutes
// of per-gate output and would blow the tool context or get cut to
// empty by the host. The full body is always pinned by its SHA-256 so
// nothing is lost without a pointer.
export const GATE_RUN_OUTPUT_MAX = 8192;

// sha256Hex pins a string with its hex SHA-256 (Bun platform hash, no
// dependency) so bounded tool content stays verifiable.
function sha256Hex(s: string): string {
  const h = new Bun.CryptoHasher("sha256");
  h.update(s);
  return h.digest("hex");
}

// formatGateResult turns a gate_run spawn outcome into tool content
// that is never empty, never unbounded, and never drops the exit
// code: empty binary output becomes "(no output)", output past
// GATE_RUN_OUTPUT_MAX is cut to a prefix carrying its sha256, and a
// non-zero exit appends its code — so a long or failing gate can
// never surface as a bare verdict, an empty string, or null.
export function formatGateResult(output: string, exitCode: number): string {
  const full = output === "" ? "(no output)" : output;
  const body =
    full.length > GATE_RUN_OUTPUT_MAX
      ? `${full.slice(0, GATE_RUN_OUTPUT_MAX)}\n…[truncated; full output pinned by sha256]\nsha256: ${sha256Hex(full)}`
      : full;
  return exitCode === 0 ? body : `${body}\nexit code: ${exitCode}`;
}

// BLOCKED_SPAWN_RE matches a spawn the OS refused before exec, as
// plain text. On Windows libuv has no errno for a Device Guard / App
// Control (WDAC) denial, so Bun renders EUNKNOWN ("EUNKNOWN: unknown
// error, uv_spawn", errno -4094) and Node's spawnSync a bare code with
// the same errno — never ENOENT. The matchers below are deliberately
// narrow: a bare `uv_spawn` token also appears in genuine Bun ENOENTs
// ("no such file or directory, uv_spawn '...'"), and a bare
// case-insensitive UNKNOWN would match any "unknown ..." prose, so
// neither rides alone. Structured code/errno (checked in
// explainSpawnError) is the primary signal; text covers string-only
// inputs like gate_run failure lines and captured stderr.
export const BLOCKED_SPAWN_RE = /\bEUNKNOWN\b|unknown error|-4094/i;

export function isBlockedSpawn(message: string): boolean {
  return BLOCKED_SPAWN_RE.test(message);
}

// explainSpawnError turns an opaque spawn refusal into an actionable
// diagnosis. Non-block failures (ENOENT, timeouts, aborts) pass
// through byte-identical so existing pins keep holding; only a block
// signature appends the OS-block explanation (confirm steps + real
// outs). Pure so bun tests pin it without spawning.
export function explainSpawnError(err: unknown, bin: string): string {
  const raw = err instanceof Error ? err.message : String(err);
  const fields = err instanceof Error ? (err as unknown as { code?: unknown; errno?: unknown }) : {};
  const codeStr = typeof fields.code === "string" ? fields.code : "";
  const errnoNum = typeof fields.errno === "number" ? fields.errno : undefined;
  const code = codeStr !== "" ? ` (${codeStr})` : "";
  const base = raw.trim() === "" ? "spawn failed" : raw.trim();
  // Structured code/errno is the primary signal (exact codes only:
  // Bun reports EUNKNOWN/−4094, Node UNKNOWN/−4094, while a genuine
  // missing file is ENOENT/−4058). Text covers string-only inputs.
  const blocked =
    codeStr === "EUNKNOWN" || codeStr === "UNKNOWN" || errnoNum === -4094 || isBlockedSpawn(`${base}${code}`);
  if (!blocked) return base;
  const where = bin.trim() !== "" ? ` "${bin}"` : "";
  const subject = bin.trim() !== "" ? bin : "<bin>";
  return (
    `${base}${code} — the OS refused to start${where} before exec. ` +
    `On Windows this is the Device Guard / App Control (WDAC) block, not a PATH or cwd problem. ` +
    `Confirm: run cmd.exe /C '"${subject}" doctor' (Device Guard prints its organization banner), ` +
    `or check Event Viewer > Microsoft-Windows-CodeIntegrity/Operational, events 3033/3077. ` +
    `Outs: ask IT for a hash/signer exception, or run inside WSL; ` +
    `self-signing (scripts/sign-local.ps1) only helps SmartScreen, and 'go run' only helps where the Go toolchain itself runs. ` +
    `See docs/troubleshooting.md (EUNKNOWN uv_spawn).`
  );
}

// formatGateFailure turns a gate_run spawn failure (timeout, abort,
// missing binary) into tool content instead of a thrown null: one
// diagnostic line carrying the conventional -1 exit code.
export function formatGateFailure(message: string): string {
  const detail = message.trim() === "" ? "gate run: spawn failed" : explainSpawnError(message, "");
  return `${detail}\nexit code: -1`;
}

// formatResolveLog renders the S2 first-match-wins diagnostic: which
// regulus binary resolveRegulus picked and why (doctor OK, contract
// OK). Pure so bun tests pin the wording; the plugin prints it to
// stderr without touching candidate order or match logic.
export function formatResolveLog(bin: string): string {
  return `[regulus] using ${bin} (doctor OK, contract OK)`;
}
