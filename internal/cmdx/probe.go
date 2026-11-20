package cmdx

import (
	"flag"
	"fmt"
	"io"
	"time"

	"regulus-ai/internal/probe"
)

// ProbeCmd implements `regulus probe contrast|compose`. Moved from
// internal/cli (probe.go) without behavior change.
func (io IO) ProbeCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		io.ProbeUsage(io.Stdout)
		return 0
	}
	if len(args) == 0 {
		io.ProbeUsage(io.Stderr)
		return 2
	}
	switch args[0] {
	case "contrast":
		return io.ProbeContrastCmd(args[1:])
	case "compose":
		return io.ProbeComposeCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "probe: unknown subcommand %q\n", args[0])
		io.ProbeUsage(io.Stderr)
		return 2
	}
}

// ProbeUsage prints the probe subcommand usages to w.
func (io IO) ProbeUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: regulus probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>")
	fmt.Fprintln(w, "usage: regulus probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>")
}

// ProbeContrastCmd implements `regulus probe contrast`.
func (io IO) ProbeContrastCmd(args []string) int {
	fs := flag.NewFlagSet("probe contrast", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	task := fs.String("task", "", "task id for the evidence receipts (required)")
	gate := fs.String("gate", "", "configured gate to run twice (required)")
	cmdA := fs.String("a", "", "first command variant (required)")
	cmdB := fs.String("b", "", "second command variant (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *gate == "" || *cmdA == "" || *cmdB == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus probe contrast --task <t> --gate <g> --a <cmdA> --b <cmdB>")
		return 2
	}
	if !validTaskID(*task) {
		fmt.Fprintf(io.Stderr, "probe contrast: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", *task)
		return 2
	}
	diff, ran, err := probe.ContrastWithRunner(io.Dir, *task, *gate, *cmdA, *cmdB, time.Now().UTC(), io.Runner)
	if err != nil {
		fmt.Fprintf(io.Stderr, "probe contrast: %v\n", err)
		return 1
	}
	if !ran {
		fmt.Fprintln(io.Stderr, "probe contrast: runs did not execute")
		return 1
	}
	if diff == "" {
		fmt.Fprintln(io.Stdout, "no differences: receipts identical")
		return 0
	}
	fmt.Fprintln(io.Stdout, diff)
	return 0
}

// parseProbeComposeFlags parses `probe compose` flags: required task,
// gate, base and both variants, no positional args. On failure it
// prints the byte-identical usage to stderr and reports exit 2.
func parseProbeComposeFlags(args []string, stderr io.Writer) (task, gate, base, cmdA, cmdB string, code int, ok bool) {
	fs := flag.NewFlagSet("probe compose", flag.ContinueOnError)
	fs.SetOutput(stderr)
	taskOpt := fs.String("task", "", "task id for the evidence receipts (required)")
	gateOpt := fs.String("gate", "", "configured gate to run three times (required)")
	baseOpt := fs.String("base", "", "base command variant (required)")
	cmdAOpt := fs.String("a", "", "first command variant (required)")
	cmdBOpt := fs.String("b", "", "second command variant (required)")
	if err := fs.Parse(args); err != nil {
		return "", "", "", "", "", 2, false
	}
	if *taskOpt == "" || *gateOpt == "" || *baseOpt == "" || *cmdAOpt == "" || *cmdBOpt == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: regulus probe compose --task <t> --gate <g> --base <cmd> --a <cmdA> --b <cmdB>")
		return "", "", "", "", "", 2, false
	}
	return *taskOpt, *gateOpt, *baseOpt, *cmdAOpt, *cmdBOpt, 0, true
}

// finishProbeComposeCmd reports one compose outcome with the
// historical messages and codes: run errors and the defensive
// not-executed case fail with 1, the composed report succeeds with 0
// (conflicts are evidence, never failure).
func finishProbeComposeCmd(io IO, report string, ran bool, err error) int {
	if err != nil {
		fmt.Fprintf(io.Stderr, "probe compose: %v\n", err)
		return 1
	}
	if !ran {
		fmt.Fprintln(io.Stderr, "probe compose: runs did not execute")
		return 1
	}
	// Conflicts are evidence, not failure: exit 0 with the report.
	fmt.Fprint(io.Stdout, report)
	return 0
}

// ProbeComposeCmd implements `regulus probe compose`.
func (io IO) ProbeComposeCmd(args []string) int {
	task, gate, base, cmdA, cmdB, code, ok := parseProbeComposeFlags(args, io.Stderr)
	if !ok {
		return code
	}
	if !validTaskID(task) {
		fmt.Fprintf(io.Stderr, "probe compose: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", task)
		return 2
	}
	report, _, ran, err := probe.ComposeWithRunner(io.Dir, task, gate, base, cmdA, cmdB, time.Now().UTC(), io.Runner)
	return finishProbeComposeCmd(io, report, ran, err)
}
