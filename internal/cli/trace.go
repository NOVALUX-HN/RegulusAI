package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/trace"
)

// traceCmd implements `regulus trace record|show|files|sessions|prune`:
// minimal agent-process capture (G2.6). `record` appends one tool
// execution to the session trace; `show` prints the diffable
// fingerprint (sequence, tool, status) for probe-with-contrast
// (G2.7); `files` prints the distinct touched paths in call order;
// `sessions` lists sessions with traces; `prune` rotates a session
// trace keeping the last N entries.
func (c CLI) traceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N]")
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
	default:
		fmt.Fprintf(c.Stderr, "trace: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] | regulus trace show --session <s> | regulus trace files --session <s> | regulus trace sessions | regulus trace prune --session <s> [--keep N]")
		return 2
	}
}

// traceRecordCmd implements `regulus trace record --session <s> --message
// <m> --call <c> --tool <t> --status completed|error [--files <a,b>]`: appends
// one tool execution to the session trace.
func (c CLI) traceRecordCmd(args []string) int {
	fs := flag.NewFlagSet("trace record", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required)")
	message := fs.String("message", "", "the message id (required)")
	call := fs.String("call", "", "the tool call id (required)")
	tool := fs.String("tool", "", "the tool name (required)")
	status := fs.String("status", "", "completed | error (required)")
	files := fs.String("files", "", "touched paths, comma-separated (summaries only, never values)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" || *message == "" || *call == "" || *tool == "" || *status == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>]")
		return 2
	}
	if _, err := trace.Record(".", trace.Entry{
		Session: *session, Message: *message, Call: *call, Tool: *tool, Status: *status,
		Files: splitList(*files),
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
	sessions, err := trace.Sessions(".")
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

// tracePruneCmd implements `regulus trace prune --session <s> [--keep N]`:
// rotation for unbounded sessions — rewrites the trace keeping only
// the last N entries (default 200). Unknown sessions are a no-op.
func (c CLI) tracePruneCmd(args []string) int {
	fs := flag.NewFlagSet("trace prune", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	session := fs.String("session", "", "the session id (required)")
	keep := fs.Int("keep", 200, "keep the last N entries")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *session == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace prune --session <s> [--keep N]")
		return 2
	}
	n, err := trace.Prune(".", *session, *keep)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace prune: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "trace %s: %d entr(y/ies) kept\n", *session, n)
	return 0
}

// traceFilesCmd implements `regulus trace files --session <s>`: prints
// the distinct touched paths in call order, one per line.
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
	entries, err := trace.List(".", *session)
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
		fmt.Fprintf(c.Stdout, "(no trace for session %s)\n", *session)
	}
	return 0
}

// traceShowCmd implements `regulus trace show --session <s>`: prints the
// session fingerprint (sequence, tool, status), one per line.
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
	entries, err := trace.List(".", *session)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace show: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintf(c.Stdout, "(no trace for session %s)\n", *session)
		return 0
	}
	for _, line := range trace.Fingerprint(entries) {
		fmt.Fprintln(c.Stdout, line)
	}
	return 0
}
