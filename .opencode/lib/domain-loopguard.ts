// Loop-guard streak detection (NOV-67 split).

// LOOPGUARD_THRESHOLD / TE_LOOPGUARD — T3 (macizo-tokens): deterministic
// loop guard. After the same tool fails with the same normalized message
// LOOPGUARD_THRESHOLD times in a row within one session, the plugin
// appends a one-line redirect nudge to the error. The nudge never stops
// the agent by itself (no throw, no block): it is advisory and the agent
// may ignore it. Reversal: set TE_LOOPGUARD=off (also 0/false/no/disabled)
// to disable the nudge entirely; anything else (including unset) keeps it.
export const LOOPGUARD_THRESHOLD = 3;
export const LOOPGUARD_MAX_SESSIONS = 200;

// normalizeLoopMessage reduces a failure message to its diffable shape:
// CRLF-tolerant, whitespace-collapsed, trimmed, lowercased, bounded.
// Numbers and paths are kept verbatim so distinct failures stay distinct
// (a line-number change alone must reset the streak, not extend it).
export function normalizeLoopMessage(message: string): string {
  return message
    .replace(/\r\n/g, "\n")
    .replace(/\s+/g, " ")
    .trim()
    .toLowerCase()
    .slice(0, 200);
}

// loopSignature is the streak key: tool name plus normalized message.
// A different tool or a different message starts a new streak.
export function loopSignature(tool: string, message: string): string {
  return `${tool}\n${normalizeLoopMessage(message)}`;
}

// isLoopGuardEnabled resolves the TE_LOOPGUARD kill switch. Pure (env
// injectable) so bun tests pin the default and every off-spelling
// without touching the process environment.
export function isLoopGuardEnabled(env?: string): boolean {
  const raw = (env ?? process.env.TE_LOOPGUARD ?? "").trim().toLowerCase();
  return raw !== "off" && raw !== "0" && raw !== "false" && raw !== "no" && raw !== "disabled";
}

// formatLoopRedirect renders the nudge: exactly one line naming the
// looping tool, suggesting a different approach, and documenting the
// TE_LOOPGUARD=off reversal inline so the agent sees it where it acts.
export function formatLoopRedirect(tool: string): string {
  const name = tool.replace(/[\r\n]+/g, " ").replace(/\s+/g, " ").trim() || "tool";
  return `[regulus loop-guard] possible loop in ${name} (×3 same failure): try a different approach (re-read the error, change file/command, or ask for help) — advisory, ignore if not applicable (TE_LOOPGUARD=off disables it).`;
}

// LoopGuard counts consecutive same-signature failures per session.
// observe(sessionID, tool, message, ok) returns the one-line redirect on
// the LOOPGUARD_THRESHOLD-th consecutive identical failure and resets
// the streak, otherwise null. Any success (ok=true) or any different
// signature restarts the streak, so normal progress never trips the
// nudge. Empty session ids are ignored (null). The per-session map is
// capped at LOOPGUARD_MAX_SESSIONS (oldest evicted) so a long-lived
// host process cannot grow it without bound.
export class LoopGuard {
  private readonly counts = new Map<string, { sig: string; count: number }>();

  observe(sessionID: string, tool: string, message: string, ok: boolean): string | null {
    if (sessionID === "") return null;
    if (ok) {
      this.counts.delete(sessionID);
      return null;
    }
    const sig = loopSignature(tool, message);
    const prev = this.counts.get(sessionID);
    const count = prev !== undefined && prev.sig === sig ? prev.count + 1 : 1;
    if (count >= LOOPGUARD_THRESHOLD) {
      this.counts.delete(sessionID);
      return formatLoopRedirect(tool);
    }
    if (!this.counts.has(sessionID) && this.counts.size >= LOOPGUARD_MAX_SESSIONS) {
      const oldest = this.counts.keys().next();
      if (!oldest.done) this.counts.delete(oldest.value);
    }
    this.counts.set(sessionID, { sig, count });
    return null;
  }

  reset(sessionID?: string): void {
    if (sessionID === undefined) this.counts.clear();
    else this.counts.delete(sessionID);
  }
}
