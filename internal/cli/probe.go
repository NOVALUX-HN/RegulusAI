package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/probe"
)

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
