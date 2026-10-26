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

	"regulus-ai/internal/budget"
	"regulus-ai/internal/contract"
	"regulus-ai/internal/crap"
	"regulus-ai/internal/doctor"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/memory"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
	"regulus-ai/internal/spec"
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

// memoryCmd implements `regulus memory note|show` — the F1 Almanac slice
// (docs/memory.md): append-only session notes.
func (c CLI) memoryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory note <text> | regulus memory show")
		return 2
	}
	switch args[0] {
	case "note":
		text := strings.TrimSpace(strings.Join(args[1:], " "))
		if text == "" {
			fmt.Fprintln(c.Stderr, "usage: regulus memory note <text>")
			return 2
		}
		if err := memory.Note(".", text, time.Now().UTC()); err != nil {
			fmt.Fprintf(c.Stderr, "memory note: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, "memory: note saved")
		return 0
	case "show":
		out, err := memory.Show(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "memory show: %v\n", err)
			return 1
		}
		if out == "" {
			fmt.Fprintln(c.Stdout, "(no session notes)")
			return 0
		}
		fmt.Fprint(c.Stdout, out)
		return 0
	default:
		fmt.Fprintf(c.Stderr, "memory: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus memory note <text> | regulus memory show")
		return 2
	}
}

// reflectCmd implements `regulus reflect --domain --task --cue --lesson
// [--confidence] <feature-doc>`: admit an evidence-gated lesson (PRD
// R-E1/R-E2).
func (c CLI) reflectCmd(args []string) int {
	fs := flag.NewFlagSet("reflect", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "lesson domain (required)")
	task := fs.String("task", "", "task id whose receipts are the evidence (required)")
	cue := fs.String("cue", "", "when this pattern appears (required)")
	lesson := fs.String("lesson", "", "what to do (required)")
	confidence := fs.String("confidence", "low", "low | medium | high")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || *cue == "" || *lesson == "" || fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus reflect --domain <d> --task <t> --cue <c> --lesson <l> [--confidence low|medium|high] <feature-doc>")
		return 2
	}
	r := c.Runner
	if r == nil {
		r = gates.ShRunner{}
	}
	head := gates.GitHead(context.Background(), r)
	l, err := reflect.Run(".", reflect.Input{
		FeatureDoc: fs.Arg(0), Domain: *domain, Task: *task, Cue: *cue, Lesson: *lesson,
		Confidence: *confidence, Now: time.Now().UTC(),
	}, head)
	if err != nil {
		fmt.Fprintf(c.Stderr, "reflect: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "lesson admitted: %s\n", l.ID)
	fmt.Fprintf(c.Stdout, "index: %s\n", library.IndexFile(".", l.Domain))
	return 0
}

// libraryCmd implements `regulus library list [--domain <name>]`.
func (c CLI) libraryCmd(args []string) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(c.Stderr, "usage: regulus library list [--domain <name>]")
		return 2
	}
	if len(args) == 3 && args[1] == "--domain" {
		lessons, err := library.List(".", args[2])
		if err != nil {
			fmt.Fprintf(c.Stderr, "library list: %v\n", err)
			return 1
		}
		c.printLessons(args[2], lessons)
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus library list [--domain <name>]")
		return 2
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

// budgetCmd implements `regulus budget show`.
func (c CLI) budgetCmd(args []string) int {
	if len(args) != 1 || args[0] != "show" {
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

func (c CLI) usage(w io.Writer) {
	fmt.Fprintf(w, `regulus %s — the Regulus AI core CLI

Usage:
  regulus <command> [arguments]

Commands:
  budget show        show the evolution budget ledger
  contract           print the plugin<->core contract version
  crap               score functions with the CRAP formula (regulus crap --help)
  doctor             probe OpenCode and verify V2 runtime support
  gate run <task>    run the layered gauntlet and write receipts
  help               show this help
  library list [--domain <name>]  list admitted lessons
  memory note|show   session notes — the Almanac F1 slice
  reflect            admit an evidence-gated lesson (regulus reflect --help)
  spec check <file>  validate the acceptance contract of a feature document
  --version          print the regulus version

Exit codes:
  0  success
  1  refusal or failure
  2  usage error
`, c.Version)
}
