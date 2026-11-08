package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/trace"
)

// traceCmd implements `regulus trace record|show|files|sessions|prune|cost|usage`:
// minimal agent-process capture (G2.6). `record` appends one tool
// execution to the session trace; `show` prints the diffable
// fingerprint (sequence, tool, status) for probe-with-contrast
// (G2.7); `files` prints the distinct touched paths in call order;
// `sessions` lists sessions with traces; `prune` rotates a session
// trace keeping the last N entries; `cost` reports the per-task proxy
// cost (calls, wall time, output bytes; host tokens only when the host
// exposes them, plus the always-measured local token proxy chars/4)
// folded auditably from the trace logs on disk (F1).
func (c CLI) traceCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N] | regulus trace record-batch --ndjson | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N] | regulus trace cost [--task <t>] | regulus trace usage [--json]")
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N] | regulus trace record-batch --ndjson | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N] | regulus trace cost [--task <t>] | regulus trace usage [--json]")
		return 2
	}
	switch args[0] {
	case "record":
		return c.traceRecordCmd(args[1:])
	case "record-batch":
		return c.traceRecordBatchCmd(args[1:])
	case "show":
		return c.traceShowCmd(args[1:])
	case "files":
		return c.traceFilesCmd(args[1:])
	case "sessions":
		return c.traceSessionsCmd(args[1:])
	case "prune":
		return c.tracePruneCmd(args[1:])
	case "cost":
		return c.traceCostCmd(args[1:])
	case "usage":
		return c.traceUsageCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "trace: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N] | regulus trace record-batch --ndjson | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N] | regulus trace cost [--task <t>] | regulus trace usage [--json]")
		return 2
	}
}

// traceRecordCmd implements `regulus trace record --session <s> --message
// <m> --call <c> --tool <t> --status completed|error [--files <a,b>]`:
// appends one tool execution to the session trace. The F1 cost flags
// (--task, --duration-ms, --output-bytes, --input-tokens,
// --output-tokens) record the call's proxy cost; token flags carry
// host-exposed counts only and stay 0 when the host exposes none
// (OpenCode exposes none — never estimated).
func (c CLI) traceRecordCmd(args []string) int {
	fs := flag.NewFlagSet("trace record", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required)")
	message := fs.String("message", "", "the message id (required)")
	call := fs.String("call", "", "the tool call id (required)")
	tool := fs.String("tool", "", "the tool name (required)")
	status := fs.String("status", "", "completed | error (required)")
	files := fs.String("files", "", "touched paths, comma-separated (summaries only, never values)")
	task := fs.String("task", "", "the task this call served (per-task cost attribution)")
	durationMS := fs.Int64("duration-ms", 0, "wall time of the call in ms (proxy cost)")
	outputBytes := fs.Int64("output-bytes", 0, "output size in bytes (proxy cost)")
	inputTokens := fs.Int64("input-tokens", 0, "host-exposed input tokens only (0 = unknown, never estimated)")
	outputTokens := fs.Int64("output-tokens", 0, "host-exposed output tokens only (0 = unknown, never estimated)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" || *message == "" || *call == "" || *tool == "" || *status == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N]")
		return 2
	}
	if _, err := trace.Record(c.dir(), trace.Entry{
		Session: *session, Message: *message, Call: *call, Tool: *tool, Status: *status,
		Files: splitList(*files), Task: *task,
		DurationMS: *durationMS, OutputBytes: *outputBytes,
		InputTokens: *inputTokens, OutputTokens: *outputTokens,
	}, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "trace record: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "trace recorded")
	return 0
}

// traceStdin is the NDJSON source for `trace record-batch --ndjson`
// (os.Stdin in production; tests substitute a strings.Reader so the
// in-process CLI stays hermetic without touching cli.go surfaces).
var traceStdin io.Reader = os.Stdin

// traceRecordBatchCmd implements `regulus trace record-batch --ndjson`:
// appends many tool executions to the session traces in one spawn (T7
// speed-tokens: one hook flush = one spawn, same fields as `trace
// record` including duration/output-bytes). Each entry is one NDJSON
// object per line with the trace.Entry field names (session, message,
// call, tool, status, files[], task, duration_ms, output_bytes,
// input_tokens, output_tokens); blank lines are skipped.
//
// Entries arrive on stdin; when stdin carries no lines and positional
// args are present, each arg is one NDJSON line instead — the OpenCode
// plugin spawn helper cannot inject stdin, so the hook batches through
// argv while shells pipe stdin. Positional args win without touching
// stdin, so a spawn with inherited stdin never blocks. Recording stops
// at the first bad line (exit 1 naming it); earlier lines stay
// recorded, exactly as sequential `trace record` calls would leave
// them. Success prints one summary line.
func (c CLI) traceRecordBatchCmd(args []string) int {
	fs := flag.NewFlagSet("trace record-batch", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	ndjson := fs.Bool("ndjson", false, "read entries as NDJSON (one object per line)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*ndjson {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record-batch --ndjson")
		return 2
	}
	var lines []string
	if rest := fs.Args(); len(rest) > 0 {
		for _, ln := range rest {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			lines = append(lines, ln)
		}
	} else {
		sc := bufio.NewScanner(traceStdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			if ln := strings.TrimSpace(sc.Text()); ln != "" {
				lines = append(lines, ln)
			}
		}
		if err := sc.Err(); err != nil {
			fmt.Fprintf(c.Stderr, "trace record-batch: %v\n", err)
			return 1
		}
	}
	now := time.Now().UTC()
	for i, ln := range lines {
		var e trace.Entry
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			fmt.Fprintf(c.Stderr, "trace record-batch: line %d: malformed entry: %v\n", i+1, err)
			return 1
		}
		if _, err := trace.Record(c.dir(), e, now); err != nil {
			fmt.Fprintf(c.Stderr, "trace record-batch: line %d: %v\n", i+1, err)
			return 1
		}
	}
	fmt.Fprintf(c.Stdout, "trace recorded %d entr(y/ies)\n", len(lines))
	return 0
}

// traceSessionsCmd implements `regulus trace sessions`: lists session
// ids with trace logs on disk, oldest filename first (sorted).
func (c CLI) traceSessionsCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace sessions")
		return 2
	}
	sessions, err := trace.Sessions(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace sessions: %v\n", err)
		return 1
	}
	if len(sessions) == 0 {
		fmt.Fprintln(c.Stdout, "(no sessions traced)")
		return 0
	}
	for _, s := range sessions {
		fmt.Fprintln(c.Stdout, s)
	}
	return 0
}

// tracePruneCmd implements `regulus trace prune --session <s> [--keep N]`
// and `regulus trace prune --all [--keep N]`: rotation for unbounded
// sessions — rewrites the trace keeping only the last N entries
// (default 200). Unknown sessions are a no-op. `--all` applies
// `trace.Prune` to every session from `trace.Sessions` and reports
// per session plus a total; files are never removed (prune rewrites
// via temp+rename only).
func (c CLI) tracePruneCmd(args []string) int {
	fs := flag.NewFlagSet("trace prune", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required unless --all)")
	keep := fs.Int("keep", 200, "keep the last N entries")
	all := fs.Bool("all", false, "prune every session")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N]")
		return 2
	}
	if *all {
		sessions, err := trace.Sessions(c.dir())
		if err != nil {
			fmt.Fprintf(c.Stderr, "trace prune: %v\n", err)
			return 1
		}
		total := 0
		for _, s := range sessions {
			n, err := trace.Prune(c.dir(), s, *keep)
			if err != nil {
				fmt.Fprintf(c.Stderr, "trace prune: %v\n", err)
				return 1
			}
			fmt.Fprintf(c.Stdout, "trace %s: %d entr(y/ies) kept\n", s, n)
			total += n
		}
		fmt.Fprintf(c.Stdout, "total: %d entr(y/ies) kept across %d session(s)\n", total, len(sessions))
		return 0
	}
	if *session == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N]")
		return 2
	}
	n, err := trace.Prune(c.dir(), *session, *keep)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace prune: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "trace %s: %d entr(y/ies) kept\n", *session, n)
	return 0
}

// traceFilesCmd implements `regulus trace files --session <s>`: prints
// the distinct touched paths in call order, one per line. A missing
// trace file (never recorded) and a recorded trace with no files are
// different honest outcomes: both exit 0 with distinct messages.
func (c CLI) traceFilesCmd(args []string) int {
	fs := flag.NewFlagSet("trace files", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace files --session <s>")
		return 2
	}
	if _, err := os.Stat(trace.File(c.dir(), *session)); os.IsNotExist(err) {
		fmt.Fprintf(c.Stdout, "no trace file for session %s (never recorded)\n", *session)
		return 0
	}
	entries, err := trace.List(c.dir(), *session)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace files: %v\n", err)
		return 1
	}
	seen := map[string]bool{}
	any := false
	for _, e := range entries {
		for _, f := range e.Files {
			if seen[f] {
				continue
			}
			seen[f] = true
			any = true
			fmt.Fprintln(c.Stdout, f)
		}
	}
	if !any {
		fmt.Fprintf(c.Stdout, "session %s has no files\n", *session)
	}
	return 0
}

// traceShowCmd implements `regulus trace show --session <s>`: prints the
// session fingerprint (sequence, tool, status), one per line. Like
// files, it distinguishes a never-recorded session (no trace file)
// from a recorded-but-empty one — both exit 0 with distinct messages.
func (c CLI) traceShowCmd(args []string) int {
	fs := flag.NewFlagSet("trace show", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace show --session <s>")
		return 2
	}
	if _, err := os.Stat(trace.File(c.dir(), *session)); os.IsNotExist(err) {
		fmt.Fprintf(c.Stdout, "no trace file for session %s (never recorded)\n", *session)
		return 0
	}
	entries, err := trace.List(c.dir(), *session)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace show: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintf(c.Stdout, "session %s has no files (no entries)\n", *session)
		return 0
	}
	for _, line := range trace.Fingerprint(entries) {
		fmt.Fprintln(c.Stdout, line)
	}
	return 0
}

// traceCostCmd implements `regulus trace cost [--task <t>]`: reports
// the per-task proxy cost (F1) folded auditably from the trace logs
// on disk — calls, errors, wall time, output bytes, host-exposed
// token counts when any entry carries them, plus the always-measured
// local token proxy (chars/4 per entry). Every number recomputes
// from the same JSONL lines `trace show` and `trace files` read, so
// the report squares with the traces. Costs the host never measured
// read as unknown, never zero; the local proxy reads as measured.
func (c CLI) traceCostCmd(args []string) int {
	fs := flag.NewFlagSet("trace cost", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "report only this task (default: every task)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace cost [--task <t>]")
		return 2
	}
	costs, err := trace.CostByTask(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace cost: %v\n", err)
		return 1
	}
	if *task != "" {
		filtered := costs[:0]
		for _, tc := range costs {
			if tc.Task == *task {
				filtered = append(filtered, tc)
			}
		}
		costs = filtered
		if len(costs) == 0 {
			fmt.Fprintf(c.Stdout, "no trace cost for task %s (no entries attributed)\n", *task)
			return 0
		}
	}
	if len(costs) == 0 {
		fmt.Fprintln(c.Stdout, "(no trace costs: no sessions traced)")
		return 0
	}
	for _, tc := range costs {
		fmt.Fprintf(c.Stdout, "task %s: %d call(s), %d error(s), %s, %s, tokens %s, local %s across %d session(s), %d entr(y/ies)\n",
			tc.Task, tc.Calls, tc.Errors,
			costKnown(tc.TotalDurationMS, "ms", tc.DurationKnown),
			costKnown(tc.TotalOutputBytes, "B", tc.BytesKnown),
			tokensKnown(tc),
			localTokensKnown(tc),
			len(tc.Sessions), tc.Entries)
	}
	return 0
}

// costKnown renders a proxy total with its unit, marking unmeasured
// costs unknown instead of printing a bare zero.
func costKnown(total int64, unit string, known bool) string {
	if !known {
		return "unknown " + unit + " (unmeasured)"
	}
	return fmt.Sprintf("%d%s", total, unit)
}

// tokensKnown renders the host token totals, or unknown when no entry
// carried host-exposed counts.
func tokensKnown(tc trace.TaskCost) string {
	if !tc.TokensKnown {
		return "unknown (host exposes none)"
	}
	return fmt.Sprintf("%d in + %d out", tc.TotalInputTokens, tc.TotalOutputTokens)
}

// localTokensKnown renders the measured-local token proxy (chars/4),
// always measured once the bucket holds any entry.
func localTokensKnown(tc trace.TaskCost) string {
	if !tc.LocalTokensKnown {
		return "unknown (nothing measured locally)"
	}
	return fmt.Sprintf("~%d proxy tokens (chars/4, measured-local)", tc.TotalLocalTokens)
}

// traceUsageCmd implements `regulus trace usage [--json]`: reports the
// measured per-tool usage (T3) folded auditably from the trace logs on
// disk — one row per called tool (calls + distinct sessions) plus the
// session count that carries the evidence floor for pruning. Human
// text by default, --json for the plugin (which parses the report
// without scraping prose). Every number recomputes from the same
// JSONL lines `trace show` reads, so the report squares with the
// traces. An empty store is an empty report, never an error: a fresh
// repo must not lose tools for lack of history.
func (c CLI) traceUsageCmd(args []string) int {
	fs := flag.NewFlagSet("trace usage", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the usage report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace usage [--json]")
		return 2
	}
	sessions, err := trace.UsageSessions(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace usage: %v\n", err)
		return 1
	}
	usage, err := trace.UsageByTool(c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace usage: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(struct {
			Sessions int               `json:"sessions"`
			Tools    []trace.ToolUsage `json:"tools"`
		}{Sessions: sessions, Tools: usageOrEmpty(usage)}, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "trace usage: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	if sessions == 0 {
		fmt.Fprintln(c.Stdout, "(no usage: no sessions traced)")
		return 0
	}
	fmt.Fprintf(c.Stdout, "sessions: %d\n", sessions)
	for _, u := range usage {
		fmt.Fprintf(c.Stdout, "tool %s: %d call(s) across %d session(s)\n", u.Tool, u.Calls, len(u.Sessions))
	}
	return 0
}

// usageOrEmpty keeps the JSON report shape stable (tools: [] instead
// of tools: null) on an empty store.
func usageOrEmpty(u []trace.ToolUsage) []trace.ToolUsage {
	if u == nil {
		return []trace.ToolUsage{}
	}
	return u
}
