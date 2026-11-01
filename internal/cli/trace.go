package cli

import (
	"flag"
	"fmt"
	"os"
	"time"

	"regulus-ai/internal/trace"
)

// traceCmd implements `regulus trace record|show|files|sessions|prune|cost`:
// minimal agent-process capture (G2.6). `record` appends one tool
// execution to the session trace; `show` prints the diffable
// fingerprint (sequence, tool, status) for probe-with-contrast
// (G2.7); `files` prints the distinct touched paths in call order;
// `sessions` lists sessions with traces; `prune` rotates a session
// trace keeping the last N entries; `cost` reports the per-task proxy
// cost (calls, wall time, output bytes; tokens only when the host
// exposes them) folded auditably from the trace logs on disk (F1).
func (c CLI) traceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N] | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N] | regulus trace cost [--task <t>]")
		return 2
	}
	switch args[0] {
	case "record":
		return c.traceRecordCmd(args[1:])
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
	default:
		fmt.Fprintf(c.Stderr, "trace: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] [--task <t>] [--duration-ms N] [--output-bytes N] [--input-tokens N] [--output-tokens N] | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N] | regulus trace prune --all [--keep N] | regulus trace cost [--task <t>]")
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
// on disk — calls, errors, wall time, output bytes, and host-exposed
// token counts when any entry carries them. Every number recomputes
// from the same JSONL lines `trace show` and `trace files` read, so
// the report squares with the traces. Costs the host never measured
// read as unknown, never zero.
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
		fmt.Fprintf(c.Stdout, "task %s: %d call(s), %d error(s), %s, %s, tokens %s across %d session(s), %d entr(y/ies)\n",
			tc.Task, tc.Calls, tc.Errors,
			costKnown(tc.TotalDurationMS, "ms", tc.DurationKnown),
			costKnown(tc.TotalOutputBytes, "B", tc.BytesKnown),
			tokensKnown(tc),
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

// tokensKnown renders the token totals, or unknown when no entry
// carried host-exposed counts.
func tokensKnown(tc trace.TaskCost) string {
	if !tc.TokensKnown {
		return "unknown (host exposes none)"
	}
	return fmt.Sprintf("%d in + %d out", tc.TotalInputTokens, tc.TotalOutputTokens)
}
