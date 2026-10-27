// Package cli dispatches the regulus core CLI.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/budget"
	"regulus-ai/internal/contract"
	"regulus-ai/internal/crap"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/probe"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
	"regulus-ai/internal/spec"
	"regulus-ai/internal/surface"
	"regulus-ai/internal/trace"
)

// CLI holds the injected dependencies of one regulus invocation.
type CLI struct {
	Version string
	Probe   func() doctor.Result
	Runner  gates.CmdRunner
	Stdout  io.Writer
	Stderr  io.Writer
}

// Run executes one CLI invocation and returns the process exit code:
// 0 success, 1 refusal or failure, 2 usage error.
func (c CLI) Run(args []string) int {
	if len(args) == 0 {
		c.usage(c.Stderr)
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Fprintf(c.Stdout, "regulus %s\n", c.Version)
		return 0
	case "help", "--help", "-h":
		c.usage(c.Stdout)
		return 0
	case "contract":
		fmt.Fprintln(c.Stdout, contract.Version)
		return 0
	case "doctor":
		return c.doctor()
	case "spec":
		return c.specCmd(args[1:])
	case "status":
		return c.statusCmd(args[1:])
	case "chart":
		return c.chartCmd(args[1:])
	case "surface":
		return c.surfaceCmd(args[1:])
	case "evidence":
		return c.evidenceCmd(args[1:])
	case "gate":
		return c.gateCmd(args[1:])
	case "memory":
		return c.memoryCmd(args[1:])
	case "budget":
		return c.budgetCmd(args[1:])
	case "library":
		return c.libraryCmd(args[1:])
	case "reflect":
		return c.reflectCmd(args[1:])
	case "probe":
		return c.probeCmd(args[1:])
	case "trace":
		return c.traceCmd(args[1:])
	case "crap":
		return c.crapCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "regulus: unknown command %q\n\n", args[0])
		c.usage(c.Stderr)
		return 2
	}
}

func (c CLI) doctor() int {
	res := c.Probe()
	if res.Supported {
		fmt.Fprintf(c.Stdout, "%s — supported (V2)\n", res.Detected)
		return 0
	}
	fmt.Fprintf(c.Stderr, "doctor: %s\n", res.Reason)
	return 1
}

// specCmd implements `regulus spec check <file>`: validate the
// acceptance-criteria contract of a feature document (PRD R-G1).
func (c CLI) specCmd(args []string) int {
	if len(args) != 2 || args[0] != "check" {
		fmt.Fprintln(c.Stderr, "usage: regulus spec check <file>")
		return 2
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintf(c.Stderr, "spec check: cannot read %s: %v\n", args[1], err)
		return 1
	}
	n, violations := spec.Check(string(data))
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintf(c.Stderr, "spec check: %s\n", v)
		}
		return 1
	}
	fmt.Fprintf(c.Stdout, "spec OK: %d scenario(s)\n", n)
	return 0
}

// gateCmd implements `regulus gate run <task>`: run the layered gauntlet
// and write one append-only receipt per executed gate (PRD R-G2, R-G4).
func (c CLI) gateCmd(args []string) int {
	if len(args) != 2 || args[0] != "run" {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run <task>")
		return 2
	}
	task := args[1]
	if !validTaskID(task) {
		fmt.Fprintf(c.Stderr, "gate run: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", task)
		return 2
	}
	r := c.Runner
	if r == nil {
		r = gates.ShRunner{}
	}
	cfg, err := gates.Load("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	head := gates.GitHead(context.Background(), r)
	res, err := gates.Run(context.Background(), cfg, filepath.Join(".regulus", "evidence", receipts.Slug(task)), task, head, r)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	for _, o := range res.Outcomes {
		fmt.Fprintf(c.Stdout, "layer %d %-16s %s (%dms)\n", o.Layer, o.Name, o.Verdict, o.DurationMS)
	}
	if res.Failed {
		fmt.Fprintf(c.Stdout, "gauntlet: stopped at gate %q — %d/%d gates passed\n", res.FailedGate, res.Passed, res.Executed)
		return 1
	}
	fmt.Fprintf(c.Stdout, "gauntlet: %d/%d gates passed\n", res.Passed, res.Executed)
	return 0
}

var taskIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

// validTaskID accepts task ids usable as receipt metadata without path
// traversal: alphanumeric start, conservative charset, no "..", bounded
// length.
func validTaskID(s string) bool {
	return taskIDRe.MatchString(s) && !strings.Contains(s, "..")
}

// memoryCmd implements the Almanac surface (docs/memory.md):
// note|show (session notes), save|search|history (the full substrate),
// context (the deterministic resume block) — dispatched per subcommand.
// Lessons have one door: regulus reflect.
func (c CLI) memoryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|context")
		return 2
	}
	switch args[0] {
	case "note":
		return c.memoryNoteCmd(args[1:])
	case "show":
		return c.memoryShowCmd(args[1:])
	case "save":
		return c.memorySaveCmd(args[1:])
	case "search":
		return c.memorySearchCmd(args[1:])
	case "history":
		return c.memoryHistoryCmd(args[1:])
	case "pin":
		return c.memoryPinCmd(true, args[1:])
	case "unpin":
		return c.memoryPinCmd(false, args[1:])
	case "relate":
		return c.memoryRelateCmd(args[1:])
	case "export":
		return c.memoryExportCmd(args[1:])
	case "context":
		return c.memoryContextCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "memory: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus memory note|show|save|search|history|pin|unpin|relate|export|context")
		return 2
	}
}

// memoryNoteCmd implements `regulus memory note <text>`.
func (c CLI) memoryNoteCmd(args []string) int {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note <text>")
		return 2
	}
	title := text
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:60])
	}
	head := gates.GitHead(context.Background(), c.runner())
	if _, err := almanac.Save(".", &almanac.Observation{
		Type: almanac.TypeSessionNote, Title: title, Content: text,
		Provenance: &almanac.Provenance{GitHead: head},
	}, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory note: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "memory: note saved")
	return 0
}

// memoryShowCmd implements `regulus memory show`.
func (c CLI) memoryShowCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory show")
		return 2
	}
	notes, err := almanac.Notes(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory show: %v\n", err)
		return 1
	}
	if len(notes) == 0 {
		fmt.Fprintln(c.Stdout, "(no session notes)")
		return 0
	}
	for _, n := range notes {
		fmt.Fprintf(c.Stdout, "- %s %s\n", n.SavedAt.Format(time.RFC3339), n.Content)
	}
	return 0
}

// memorySaveCmd implements `regulus memory save --type --title [...]`.
// Lesson-type saves are refused: lessons enter through reflect.
func (c CLI) memorySaveCmd(args []string) int {
	fs := flag.NewFlagSet("memory save", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	typ := fs.String("type", "", "decision|discovery|preference|session-note (required)")
	title := fs.String("title", "", "short, searchable title (required)")
	topicKey := fs.String("topic-key", "", "group for evolving topics (revision chain)")
	domain := fs.String("domain", "", "domain bucket")
	task := fs.String("task", "", "task id for provenance")
	paths := fs.String("path", "", "affected paths, comma-separated")
	receiptsFlag := fs.String("receipt", "", "receipt paths for provenance, comma-separated")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	content := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if *typ == "" || *title == "" || content == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory save --type <t> --title <t> [--topic-key --domain --task --path <a,b> --receipt <r1,r2>] <content>")
		return 2
	}
	if *typ == almanac.TypeLesson {
		fmt.Fprintln(c.Stderr, "memory save: lessons enter through regulus reflect (evidence-gated)")
		return 1
	}
	head := gates.GitHead(context.Background(), c.runner())
	if _, err := almanac.Save(".", &almanac.Observation{
		Type: *typ, Title: *title, Content: content,
		TopicKey: *topicKey, Domain: *domain,
		Provenance: &almanac.Provenance{Task: *task, GitHead: head, Paths: splitList(*paths), Receipts: splitList(*receiptsFlag)},
	}, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory save: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, "memory: observation saved")
	return 0
}

// memorySearchCmd implements `regulus memory search [terms]` with
// bucket flags.
func (c CLI) memorySearchCmd(args []string) int {
	fs := flag.NewFlagSet("memory search", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	topicKey := fs.String("topic-key", "", "bucket: topic key")
	domain := fs.String("domain", "", "bucket: domain")
	typ := fs.String("type", "", "bucket: observation type")
	dimension := fs.String("dimension", "", "bucket: lesson dimension (inv | dom)")
	path := fs.String("path", "", "bucket: provenance path (exact match)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	hits, err := almanac.Search(".", almanac.Query{Terms: fs.Args(), TopicKey: *topicKey, Domain: *domain, Type: *typ, Dimension: *dimension, Path: *path})
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory search: %v\n", err)
		return 1
	}
	if len(hits) == 0 {
		fmt.Fprintln(c.Stdout, "no matching observations")
		return 0
	}
	fmt.Fprintf(c.Stdout, "%d hit(s)\n", len(hits))
	now := time.Now().UTC()
	for _, h := range hits {
		fmt.Fprintf(c.Stdout, "%s %s %s (saved %s%s)\n", h.ID, h.Type, h.Title, humanAge(now, h.SavedAt), c.commitsBehindSuffix(h))
	}
	return 0
}

// memoryHistoryCmd implements `regulus memory history --topic-key`.
func (c CLI) memoryHistoryCmd(args []string) int {
	fs := flag.NewFlagSet("memory history", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	topicKey := fs.String("topic-key", "", "the revision chain to list (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *topicKey == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory history --topic-key <key>")
		return 2
	}
	chain, err := almanac.History(".", *topicKey)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory history: %v\n", err)
		return 1
	}
	if len(chain) == 0 {
		fmt.Fprintln(c.Stdout, "(no observations with that topic key)")
		return 0
	}
	for _, o := range chain {
		fmt.Fprintf(c.Stdout, "%s %s %s %s\n", o.SavedAt.Format(time.RFC3339), o.ID, o.Type, o.Title)
	}
	return 0
}

// memoryPinCmd implements `regulus memory pin|unpin <id>`.
func (c CLI) memoryPinCmd(pin bool, args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory pin|unpin <id>")
		return 2
	}
	if err := almanac.Pin(".", args[0], pin, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory: %v\n", err)
		return 1
	}
	if pin {
		fmt.Fprintf(c.Stdout, "pinned: %s\n", args[0])
	} else {
		fmt.Fprintf(c.Stdout, "unpinned: %s\n", args[0])
	}
	return 0
}

// memoryRelateCmd implements `regulus memory relate --id --relation --to`.
func (c CLI) memoryRelateCmd(args []string) int {
	fs := flag.NewFlagSet("memory relate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	from := fs.String("id", "", "the observation (required)")
	relation := fs.String("relation", "", "supersedes | conflicts_with | related (required)")
	to := fs.String("to", "", "the other observation (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *from == "" || *relation == "" || *to == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory relate --id <a> --relation <r> --to <b>")
		return 2
	}
	if err := almanac.Relate(".", *from, *relation, *to, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory relate: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "recorded: %s %s %s\n", *from, *relation, *to)
	return 0
}

// memoryExportCmd implements `regulus memory export` — the team bundle.
func (c CLI) memoryExportCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory export")
		return 2
	}
	paths, err := almanac.Export(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory export: %v\n", err)
		return 1
	}
	for _, p := range paths {
		fmt.Fprintf(c.Stdout, "exported: %s\n", p)
	}
	fmt.Fprintln(c.Stdout, "commit .regulus-export/ to share the Almanac with your team")
	return 0
}

// memoryContextCmd implements `regulus memory context` — the resume
// block, DATA-framed.
func (c CLI) memoryContextCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory context")
		return 2
	}
	ctx, err := almanac.Context(".", time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory context: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, ctx)
	return 0
}

// runner returns the injected runner or the real shell runner.
func (c CLI) runner() gates.CmdRunner {
	if c.Runner != nil {
		return c.Runner
	}
	return gates.ShRunner{}
}

// splitList splits a comma-separated flag value, trimming spaces.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// commitsBehindSuffix renders the git staleness annotation for an
// observation with a provenance git head — empty when unknown.
func (c CLI) commitsBehindSuffix(o almanac.Observation) string {
	if o.Provenance == nil || o.Provenance.GitHead == "" {
		return ""
	}
	head := o.Provenance.GitHead
	out, _, _, err := c.runner().Run(context.Background(), "git rev-list --count "+head+"..HEAD")
	if err != nil || strings.TrimSpace(out) == "" {
		return ""
	}
	n := strings.TrimSpace(out)
	if n == "0" {
		return ""
	}
	return ", " + n + " commits behind"
}

// humanAge renders a coarse age annotation for staleness awareness.
func humanAge(now, saved time.Time) string {
	d := now.Sub(saved)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// reflectCmd implements `regulus reflect --domain --task --cue --lesson
// [--dimension] [--confidence] <feature-doc>`: admit an evidence-gated lesson (PRD
// R-E1/R-E2).
func (c CLI) reflectCmd(args []string) int {
	fs := flag.NewFlagSet("reflect", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "lesson domain (required)")
	dimension := fs.String("dimension", "", "inv (how the agent works) | dom (what a practitioner knows)")
	task := fs.String("task", "", "task id whose receipts are the evidence (required)")
	cue := fs.String("cue", "", "when this pattern appears (required)")
	lesson := fs.String("lesson", "", "what to do (required)")
	confidence := fs.String("confidence", "low", "low | medium | high")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || *cue == "" || *lesson == "" || fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus reflect --domain <d> --task <t> --cue <c> --lesson <l> [--dimension inv|dom] [--confidence low|medium|high] <feature-doc>")
		fmt.Fprintln(c.Stderr, "lesson guidance: anchor workspace facts ('in this workspace, ...'), keep procedure transferable across projects, prefer skills/scripts over always-on prose")
		return 2
	}
	r := c.Runner
	if r == nil {
		r = gates.ShRunner{}
	}
	head := gates.GitHead(context.Background(), r)
	l, err := reflect.Run(".", reflect.Input{
		FeatureDoc: fs.Arg(0), Domain: *domain, Dimension: *dimension, Task: *task, Cue: *cue, Lesson: *lesson,
		Confidence: *confidence, Now: time.Now().UTC(),
	}, head)
	if err != nil {
		fmt.Fprintf(c.Stderr, "reflect: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "candidate proposed: %s\n", l.ID)
	fmt.Fprintf(c.Stdout, "guardian check, then: regulus library admit --domain %s --candidate %s --guardian <name>\n", l.Domain, l.ID)
	return 0
}

// libraryCmd implements `regulus library list|candidates|show|domains
// [--domain] | admit --domain --candidate --guardian |
// forget|retain --domain --task` — dispatched to
// one handler per subcommand.
func (c CLI) libraryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library list|candidates|show|domains|forget|retain [--domain <name>] | regulus library admit --domain <d> --candidate <id> --guardian <name> | regulus library forget|retain --domain <d> --task <t>")
		return 2
	}
	switch args[0] {
	case "domains":
		return c.libraryDomainsCmd(args[1:])
	case "list":
		return c.libraryListCmd(args[1:])
	case "candidates":
		return c.libraryCandidatesCmd(args[1:])
	case "show":
		return c.libraryShowCmd(args[1:])
	case "admit":
		return c.libraryAdmitCmd(args[1:])
	case "forget":
		return c.libraryForgetCmd(args[1:])
	case "retain":
		return c.libraryRetainCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "library: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus library list|candidates|show|domains|forget|retain [--domain <name>] | regulus library admit --domain --candidate --guardian | regulus library forget|retain --domain <d> --task <t>")
		return 2
	}
}

// traceCmd implements `regulus trace record|show|files`: minimal
// agent-process capture (G2.6). `record` appends one tool execution to
// the session trace; `show` prints the diffable fingerprint (sequence,
// tool, status) for probe-with-contrast (G2.7); `files` prints the
// distinct touched paths in call order.
func (c CLI) traceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] | regulus trace show --session <s> | regulus trace files --session <s>")
		return 2
	}
	switch args[0] {
	case "record":
		return c.traceRecordCmd(args[1:])
	case "show":
		return c.traceShowCmd(args[1:])
	case "files":
		return c.traceFilesCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "trace: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus trace record --session <s> --message <m> --call <c> --tool <t> --status completed|error [--files <a,b>] | regulus trace show --session <s> | regulus trace files --session <s>")
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

// probeCmd implements `regulus probe contrast --task <t> --gate <g> --a
// <cmdA> --b <cmdB>` (run one gate twice and diff the two real receipts
// as evidence, G2.7) and `regulus probe compose --task <t> --gate <g>
// --base <cmd> --a <cmdA> --b <cmdB>` (run one gate three times and
// deterministically merge the variants, flagging conflicts). Exit 0 when
// all runs executed (a non-empty diff, or conflicts, are evidence, not
// failure), 1 on run or refusal errors, 2 on usage errors.
func (c CLI) probeCmd(args []string) int {
	if len(args) == 0 {
		c.probeUsage()
		return 2
	}
	switch args[0] {
	case "contrast":
		return c.probeContrastCmd(args[1:])
	case "compose":
		return c.probeComposeCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "probe: unknown subcommand %q\n", args[0])
		c.probeUsage()
		return 2
	}
}

// probeUsage prints the probe subcommand usages to stderr.
func (c CLI) probeUsage() {
	fmt.Fprintln(c.Stderr, "usage: regulus probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>")
	fmt.Fprintln(c.Stderr, "usage: regulus probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>")
}

// probeContrastCmd implements `regulus probe contrast`.
func (c CLI) probeContrastCmd(args []string) int {
	fs := flag.NewFlagSet("probe contrast", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task id for the evidence receipts (required)")
	gate := fs.String("gate", "", "configured gate to run twice (required)")
	cmdA := fs.String("a", "", "first command variant (required)")
	cmdB := fs.String("b", "", "second command variant (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *gate == "" || *cmdA == "" || *cmdB == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>")
		return 2
	}
	if !validTaskID(*task) {
		fmt.Fprintf(c.Stderr, "probe contrast: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", *task)
		return 2
	}
	diff, ran, err := probe.ContrastWithRunner(".", *task, *gate, *cmdA, *cmdB, time.Now().UTC(), c.runner())
	if err != nil {
		fmt.Fprintf(c.Stderr, "probe contrast: %v\n", err)
		return 1
	}
	if !ran {
		fmt.Fprintln(c.Stderr, "probe contrast: runs did not execute")
		return 1
	}
	if diff == "" {
		fmt.Fprintln(c.Stdout, "no differences: receipts identical")
		return 0
	}
	fmt.Fprintln(c.Stdout, diff)
	return 0
}

// probeComposeCmd implements `regulus probe compose`.
func (c CLI) probeComposeCmd(args []string) int {
	fs := flag.NewFlagSet("probe compose", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task id for the evidence receipts (required)")
	gate := fs.String("gate", "", "configured gate to run three times (required)")
	base := fs.String("base", "", "base command variant (required)")
	cmdA := fs.String("a", "", "first command variant (required)")
	cmdB := fs.String("b", "", "second command variant (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *gate == "" || *base == "" || *cmdA == "" || *cmdB == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>")
		return 2
	}
	if !validTaskID(*task) {
		fmt.Fprintf(c.Stderr, "probe compose: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", *task)
		return 2
	}
	report, _, ran, err := probe.ComposeWithRunner(".", *task, *gate, *base, *cmdA, *cmdB, time.Now().UTC(), c.runner())
	if err != nil {
		fmt.Fprintf(c.Stderr, "probe compose: %v\n", err)
		return 1
	}
	if !ran {
		fmt.Fprintln(c.Stderr, "probe compose: runs did not execute")
		return 1
	}
	// Conflicts are evidence, not failure: exit 0 with the report.
	fmt.Fprint(c.Stdout, report)
	return 0
}

// libraryDomainsCmd implements `regulus library domains`: one domain
// per line, sorted; empty (exit 0, no output) when no library exists.
// The OpenCode plugin reads it to auto-detect the lesson domain.
func (c CLI) libraryDomainsCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library domains")
		return 2
	}
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "library domains: %v\n", err)
		return 1
	}
	for _, d := range domains {
		fmt.Fprintln(c.Stdout, d)
	}
	return 0
}

// libraryListCmd implements `regulus library list [--domain <name>]`.
func (c CLI) libraryListCmd(args []string) int {
	fs := flag.NewFlagSet("library list", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "only this domain's lessons")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library list [--domain <name>]")
		return 2
	}
	if *domain != "" {
		lessons, err := library.List(".", *domain)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library list: %v\n", err)
			return 1
		}
		c.printLessons(*domain, lessons)
		return 0
	}
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "library list: %v\n", err)
		return 1
	}
	if len(domains) == 0 {
		fmt.Fprintln(c.Stdout, "(no lessons admitted yet)")
		return 0
	}
	for _, d := range domains {
		lessons, err := library.List(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library list: %v\n", err)
			return 1
		}
		c.printLessons(d, lessons)
	}
	return 0
}

// libraryCandidatesCmd implements `regulus library candidates
// [--domain <name>]`.
func (c CLI) libraryCandidatesCmd(args []string) int {
	fs := flag.NewFlagSet("library candidates", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "only this domain's candidates")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library candidates [--domain <name>]")
		return 2
	}
	var domains []string
	var err error
	if *domain != "" {
		domains = []string{*domain}
	} else {
		domains, err = library.Domains(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "library candidates: %v\n", err)
			return 1
		}
	}
	any := false
	for _, d := range domains {
		cands, err := library.Candidates(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library candidates: %v\n", err)
			return 1
		}
		if len(cands) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(c.Stdout, "%s (pending):\n", d)
		for _, l := range cands {
			fmt.Fprintf(c.Stdout, "  %s  %s  (%s, %d receipt(s))\n", l.ID, l.Cue, l.Confidence, len(l.Evidence))
		}
	}
	if !any {
		fmt.Fprintln(c.Stdout, "(no candidates pending)")
	}
	return 0
}

// libraryShowCmd implements `regulus library show --domain <name>` —
// the domain INDEX, verbatim, for agent consultation.
func (c CLI) libraryShowCmd(args []string) int {
	fs := flag.NewFlagSet("library show", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the domain index to print (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library show --domain <name>")
		return 2
	}
	data, err := os.ReadFile(library.IndexFile(".", *domain))
	if os.IsNotExist(err) {
		fmt.Fprintln(c.Stdout, "(no lessons admitted yet)")
		return 0
	}
	if err != nil {
		fmt.Fprintf(c.Stderr, "library show: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, string(data))
	return 0
}

// libraryAdmitCmd implements `regulus library admit --domain --candidate
// --guardian` — the second phase of lesson admission.
func (c CLI) libraryAdmitCmd(args []string) int {
	fs := flag.NewFlagSet("library admit", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "candidate's domain (required)")
	candidate := fs.String("candidate", "", "candidate id (required)")
	guardian := fs.String("guardian", "", "who validated the admission (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *candidate == "" || *guardian == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library admit --domain <d> --candidate <id> --guardian <name>")
		return 2
	}
	head := gates.GitHead(context.Background(), c.runner())
	l, err := reflect.Admit(".", *domain, *candidate, *guardian, head, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "library admit: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "lesson admitted: %s (guardian %s)\n", l.ID, l.Guardian)
	fmt.Fprintf(c.Stdout, "index: %s\n", library.IndexFile(".", l.Domain))
	return 0
}

// libraryForgetCmd implements `regulus library forget --domain <d>
// --task <t>`: drop one task snapshot, then rebuild the INDEX.
// Forgetting an unknown task is a no-op success.
func (c CLI) libraryForgetCmd(args []string) int {
	fs := flag.NewFlagSet("library forget", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the snapshot's domain (required)")
	task := fs.String("task", "", "the task snapshot to drop (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library forget --domain <d> --task <t>")
		return 2
	}
	if err := library.Forget(".", *domain, *task); err != nil {
		fmt.Fprintf(c.Stderr, "library forget: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "forgotten: harnesses/%s/ (domain %s)\n", receipts.Slug(*task), *domain)
	return 0
}

// libraryRetainCmd implements `regulus library retain --domain <d>
// --task <t>`: keep only lessons whose task sorts strictly before <t>,
// drop <t> and everything after it, then rebuild the INDEX.
func (c CLI) libraryRetainCmd(args []string) int {
	fs := flag.NewFlagSet("library retain", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the domain to prune (required)")
	task := fs.String("task", "", "keep tasks strictly before this one (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library retain --domain <d> --task <t>")
		return 2
	}
	if err := library.Retain(".", *domain, *task); err != nil {
		fmt.Fprintf(c.Stderr, "library retain: %v\n", err)
		return 1
	}
	lessons, err := library.List(".", *domain)
	if err != nil {
		fmt.Fprintf(c.Stderr, "library retain: %v\n", err)
		return 1
	}
	kept := make([]string, 0, len(lessons))
	for _, l := range lessons {
		if l.Task < *task {
			kept = append(kept, l.Task)
		}
	}
	if len(kept) == 0 {
		fmt.Fprintf(c.Stdout, "retained: (no tasks before %s in domain %s)\n", *task, *domain)
		return 0
	}
	fmt.Fprintf(c.Stdout, "retained: %s (domain %s)\n", strings.Join(kept, ", "), *domain)
	return 0
}

func (c CLI) printLessons(domain string, lessons []library.Lesson) {
	if len(lessons) == 0 {
		fmt.Fprintf(c.Stdout, "%s: (no lessons admitted yet)\n", domain)
		return
	}
	fmt.Fprintf(c.Stdout, "%s:\n", domain)
	for _, l := range lessons {
		fmt.Fprintf(c.Stdout, "  %s  %s  (%s, %d receipt(s))\n", l.ID, l.Cue, l.Confidence, len(l.Evidence))
	}
}

// budgetCmd implements `regulus budget show | record --kind probe`.
func (c CLI) budgetCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus budget show | regulus budget record --kind probe --detail <text>")
		return 2
	}
	switch args[0] {
	case "show":
		if len(args) != 1 {
			fmt.Fprintln(c.Stderr, "usage: regulus budget show")
			return 2
		}
		entries, err := budget.Show(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "budget show: %v\n", err)
			return 1
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Stdout, "(no budget entries)")
			return 0
		}
		for _, e := range entries {
			detail := ""
			if e.Detail != "" {
				detail = " (" + e.Detail + ")"
			}
			fmt.Fprintf(c.Stdout, "%s %s %s %s %s%s\n", e.Timestamp.Format(time.RFC3339), e.Kind, e.Task, e.Domain, e.Outcome, detail)
		}
		return 0
	case "record":
		fs := flag.NewFlagSet("budget record", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		kind := fs.String("kind", "", "probe (required; reflect runs are recorded by regulus reflect itself)")
		detail := fs.String("detail", "", "what was probed")
		task := fs.String("task", "", "task id")
		domain := fs.String("domain", "", "domain")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *kind != budget.KindProbe || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "budget record: only probe runs are recorded manually (reflect runs are recorded by regulus reflect)")
			fmt.Fprintln(c.Stderr, "usage: regulus budget record --kind probe [--detail <text>] [--task <t>] [--domain <d>]")
			return 2
		}
		now := time.Now().UTC()
		cap := reflect.EvolutionCap(".")
		today, err := budget.TodayEvolutionCount(".", now)
		if err != nil {
			fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
			return 1
		}
		if today >= cap {
			if err := budget.Record(".", budget.Entry{Kind: budget.KindProbe, Timestamp: now, Task: *task, Domain: *domain, Outcome: budget.OutcomeRefusedCap, Detail: fmt.Sprintf("cap %d reached", cap)}); err != nil {
				fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
				return 1
			}
			fmt.Fprintf(c.Stderr, "budget record: evolution cap reached: %d run(s) today, cap %d\n", today, cap)
			return 1
		}
		if err := budget.Record(".", budget.Entry{Kind: budget.KindProbe, Timestamp: now, Task: *task, Domain: *domain, Outcome: budget.OutcomeProbeRun, Detail: *detail}); err != nil {
			fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, "probe recorded")
		return 0
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus budget show | regulus budget record --kind probe")
		return 2
	}
}

// crapCmd implements `regulus crap --profile <coverprofile> [--max N]
// [--top N]`: the layer-7 gauntlet gate (PRD R-G2) — Uncle Bob's CRAP
// formula over a go cover profile.
func (c CLI) crapCmd(args []string) int {
	fs := flag.NewFlagSet("crap", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	profile := fs.String("profile", "", "go cover profile path (required)")
	max := fs.Float64("max", 30, "fail if any function's CRAP exceeds this")
	top := fs.Int("top", 10, "how many worst functions to print")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus crap --profile <coverprofile> [--max N] [--top N]")
		return 2
	}
	rep, err := crap.Analyze(".", *profile)
	if err != nil {
		fmt.Fprintf(c.Stderr, "crap: %v\n", err)
		return 1
	}
	for i, f := range rep.Functions {
		if i >= *top {
			break
		}
		fmt.Fprintf(c.Stdout, "%s:%d %s (complexity %d, coverage %.1f%%) CRAP %.2f\n",
			f.File, f.StartLine, f.Name, f.Complexity, f.Coverage*100, f.CRAP)
	}
	fmt.Fprintf(c.Stdout, "CRAP max: %.2f\n", rep.Max)
	if rep.Max > *max {
		fmt.Fprintf(c.Stderr, "crap: max CRAP %.2f exceeds %.2f — cover or split the worst functions\n", rep.Max, *max)
		return 1
	}
	return 0
}

// statusCmd implements `regulus status` — one look at the project: the
// runtime gate, the gauntlet configuration, today's evolution against
// the cap, pending candidates, admitted lessons and receipts on disk.
func (c CLI) statusCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus status")
		return 2
	}
	res := c.Probe()
	if res.Supported {
		fmt.Fprintf(c.Stdout, "runtime:    %s — supported (V2)\n", res.Detected)
	} else {
		fmt.Fprintf(c.Stdout, "runtime:    NOT SUPPORTED — %s\n", res.Reason)
	}
	cfg, err := gates.Load("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stdout, "gauntlet:   not configured — %v\n", err)
	} else {
		fmt.Fprintf(c.Stdout, "gauntlet:   %d gate(s) configured\n", len(cfg.Gates))
	}
	now := time.Now().UTC()
	today, err := budget.TodayEvolutionCount(".", now)
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: budget: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "evolution:  %d/%d run(s) today\n", today, reflect.EvolutionCap("."))
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: library: %v\n", err)
		return 1
	}
	pending, lessons := 0, 0
	for _, d := range domains {
		cands, err := library.Candidates(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "status: candidates: %v\n", err)
			return 1
		}
		ls, err := library.List(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "status: lessons: %v\n", err)
			return 1
		}
		pending += len(cands)
		lessons += len(ls)
	}
	fmt.Fprintf(c.Stdout, "lessons:    %d admitted, %d candidate(s) pending\n", lessons, pending)
	found, err := filepath.Glob(filepath.Join(".regulus", "evidence", "*", "*.json"))
	if err != nil {
		fmt.Fprintf(c.Stderr, "status: evidence: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "receipts:   %d on disk\n", len(found))
	// gremlins availability for the mutation gate
	if _, _, code, _ := c.runner().Run(context.Background(), "command -v gremlins || command -v \"$HOME/go/bin/gremlins\""); code == 0 {
		fmt.Fprintln(c.Stdout, "mutation:    gremlins available")
	} else {
		fmt.Fprintln(c.Stdout, "mutation:    gremlins NOT found — the mutation gate will fail; go install github.com/go-gremlins/gremlins/cmd/gremlins@latest")
	}
	return 0
}

// chartCmd implements `regulus chart init <name> | list` — scaffold and
// discover Charts (committed feature documents under charts/).
func (c CLI) chartCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus chart init <name> | regulus chart list")
		return 2
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			fmt.Fprintln(c.Stderr, "usage: regulus chart init <name>")
			return 2
		}
		name := strings.TrimSuffix(strings.TrimSuffix(args[1], ".md"), "/")
		if name == "" || strings.ContainsAny(name, `\\/:*?"<>| `) {
			fmt.Fprintln(c.Stderr, "chart init: invalid chart name")
			return 2
		}
		path := filepath.Join("charts", name+".md")
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(c.Stderr, "chart init: %s already exists\n", path)
			return 1
		}
		if err := os.MkdirAll("charts", 0o755); err != nil {
			fmt.Fprintf(c.Stderr, "chart init: %v\n", err)
			return 1
		}
		scaffold := "# " + name + "\n\nWhat this feature is and why it exists.\n\n## Acceptance\n\n### Scenario: the happy path\n\n- Given a clean project\n- When the change runs\n- Then the exit code is 0\n\n## Log\n\n- Chart scaffolded by regulus chart init.\n"
		if err := os.WriteFile(path, []byte(scaffold), 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "chart init: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "chart: %s\n", path)
		fmt.Fprintln(c.Stdout, "fill the Acceptance scenarios before implementation (regulus spec check "+path+")")
		return 0
	case "list":
		paths, err := filepath.Glob(filepath.Join("charts", "*.md"))
		if err != nil {
			fmt.Fprintf(c.Stderr, "chart list: %v\n", err)
			return 1
		}
		if len(paths) == 0 {
			fmt.Fprintln(c.Stdout, "(no charts)")
			return 0
		}
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(c.Stdout, "  %-30s (unreadable: %v)\n", p, err)
				continue
			}
			scenarios, violations := spec.Check(string(data))
			status := fmt.Sprintf("OK (%d scenario%s)", scenarios, plural(scenarios))
			if len(violations) > 0 {
				status = fmt.Sprintf("INVALID (%d violation%s)", len(violations), plural(len(violations)))
			}
			fmt.Fprintf(c.Stdout, "  %-30s %s\n", p, status)
		}
		return 0
	default:
		fmt.Fprintf(c.Stderr, "chart: unknown subcommand %q\n", args[0])
		return 2
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// surfaceCmd implements `regulus surface propose|list|apply` — the
// mechanical gate for harness-surface edits (PRD R-E3).
func (c CLI) surfaceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus surface propose --target <f> --content <c> --reason <r> | list | apply --id <id> --guardian <name>")
		return 2
	}
	switch args[0] {
	case "propose":
		fs := flag.NewFlagSet("surface propose", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		target := fs.String("target", "", "the surface file to edit (required; must be in the whitelist)")
		content := fs.String("content", "", "the text to append (required)")
		reason := fs.String("reason", "", "why — cite the lesson or evidence (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *target == "" || *content == "" || *reason == "" || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus surface propose --target <f> --content <c> --reason <r>")
			return 2
		}
		e, err := surface.Propose(".", *target, *content, *reason, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface propose: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "proposed: %s → %s\n", e.ID, e.Target)
		fmt.Fprintln(c.Stdout, "guardian check, then: regulus surface apply --id "+e.ID+" --guardian <name>")
		return 0
	case "list":
		edits, err := surface.List(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface list: %v\n", err)
			return 1
		}
		if len(edits) == 0 {
			fmt.Fprintln(c.Stdout, "(no surface edits)")
			return 0
		}
		for _, e := range edits {
			fmt.Fprintf(c.Stdout, "%s %s %s (%s)%s\n", e.ID, e.Status, e.Target, e.Reason, guardianSuffix(e))
		}
		return 0
	case "apply":
		fs := flag.NewFlagSet("surface apply", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		id := fs.String("id", "", "the proposal id (required)")
		guardian := fs.String("guardian", "", "who validated (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *id == "" || *guardian == "" || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus surface apply --id <id> --guardian <name>")
			return 2
		}
		e, err := surface.Apply(".", *id, *guardian, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface apply: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "applied: %s → %s (guardian %s)\n", e.ID, e.Target, e.Guardian)
		return 0
	default:
		fmt.Fprintf(c.Stderr, "surface: unknown subcommand %q\n", args[0])
		return 2
	}
}

func guardianSuffix(e surface.Edit) string {
	if e.Guardian == "" {
		return ""
	}
	return " — guardian " + e.Guardian
}

// evidenceCmd implements `regulus evidence list [--task <t>] | show
// <file>`: the human audit surface (PRD R-G6) — receipts, never diffs.
func (c CLI) evidenceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file>")
		return 2
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("evidence list", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		task := fs.String("task", "", "list only this task's receipts")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>]")
			return 2
		}
		entries, err := evidence.List(".", *task)
		if err != nil {
			fmt.Fprintf(c.Stderr, "evidence list: %v\n", err)
			return 1
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Stdout, "(no receipts yet)")
			return 0
		}
		for _, e := range entries {
			r := e.Receipt
			metric := ""
			if r.MetricValue != nil {
				metric = fmt.Sprintf(" metric=%.2f %s %.2f", *r.MetricValue, r.MetricOperator, *r.MetricThreshold)
			}
			fmt.Fprintf(c.Stdout, "%s layer%d %-12s %-4s exit=%d %dms%s %s\n",
				r.StartedAt.Format(time.RFC3339), r.Layer, r.Gate, r.Verdict, r.ExitCode, r.DurationMS, metric, e.Path)
		}
		return 0
	case "show":
		if len(args) != 2 {
			fmt.Fprintln(c.Stderr, "usage: regulus evidence show <file>")
			return 2
		}
		rec, err := receipts.ReadFile(args[1])
		if err != nil {
			fmt.Fprintf(c.Stderr, "evidence show: cannot read %s: %v\n", args[1], err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "id:          %s\n", rec.ID)
		fmt.Fprintf(c.Stdout, "task:        %s\n", rec.Task)
		fmt.Fprintf(c.Stdout, "gate:        %s (layer %d)\n", rec.Gate, rec.Layer)
		fmt.Fprintf(c.Stdout, "command:     %s\n", rec.Command)
		fmt.Fprintf(c.Stdout, "verdict:     %s (exit %d)\n", rec.Verdict, rec.ExitCode)
		fmt.Fprintf(c.Stdout, "started_at:  %s\n", rec.StartedAt.Format(time.RFC3339))
		fmt.Fprintf(c.Stdout, "duration_ms: %d\n", rec.DurationMS)
		if rec.MetricValue != nil {
			fmt.Fprintf(c.Stdout, "metric:      %.2f %s %.2f\n", *rec.MetricValue, rec.MetricOperator, *rec.MetricThreshold)
		}
		fmt.Fprintf(c.Stdout, "git_head:    %s\n", rec.GitHead)
		fmt.Fprintf(c.Stdout, "stdout (sha256 %s):\n%s\n", rec.StdoutSHA, rec.Stdout)
		fmt.Fprintf(c.Stdout, "stderr (sha256 %s):\n%s\n", rec.StderrSHA, rec.Stderr)
		return 0
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file>")
		return 2
	}
}

func (c CLI) usage(w io.Writer) {
	fmt.Fprintf(w, `regulus %s — the Regulus AI core CLI

Usage:
  regulus <command> [arguments]

Commands:
  budget show|record   the evolution ledger (show; record a probe run)
  chart init <name> | list             scaffold a Chart, list every Chart
  contract           print the plugin<->core contract version
  crap               score functions with the CRAP formula (regulus crap --help)
  doctor             probe OpenCode and verify V2 runtime support
  evidence list [--task <t>] | show <file>   audit gauntlet receipts
  gate run <task>    run the layered gauntlet and write receipts
  help               show this help
  library list|candidates|show|domains|forget|retain [--domain <name>]  lessons, pending candidates, domain index, snapshot pruning
  library admit --domain <d> --candidate <id> --guardian <name>  admit a validated lesson
  library forget --domain <d> --task <t>  drop one task snapshot
  library retain --domain <d> --task <t>  keep tasks strictly before <t>
  memory note|show|save|search|history|context   the Almanac — persistent memory
  probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>   run one gate twice and diff the receipts
  probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>   run one gate three times and merge the variants
  reflect            admit an evidence-gated lesson (regulus reflect --help)
  trace record|show|files   capture and inspect per-session tool traces (regulus trace --help)
  spec check <file>  validate the acceptance contract of a feature document
  status             one look at the project: runtime, gauntlet, evolution, receipts
  --version          print the regulus version

Exit codes:
  0  success
  1  refusal or failure
  2  usage error
`, c.Version)
}
