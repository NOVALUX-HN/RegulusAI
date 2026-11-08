package cli

import (
	"flag"
	"fmt"

	"regulus-ai/internal/crap"
)

// gateCmd implements `regulus gate run [--profile NAME] [--dry-run] <task>`:
// run the layered gauntlet and write one append-only receipt per executed
// gate (PRD R-G2, R-G4). --dry-run prints the execution plan and writes
// nothing. --profile selects required gates plus the profile's members
// (quick = required only, full = everything, the default).
//
// gateCmd is a thin orchestrator: parseGateRunArgs validates flags, task
// and config; renderDryRunPlan prints the plan (no preflight: it writes
// nothing); execAndReport preflights binaries then runs the gauntlet.
// Behavior, messages and exit codes are unchanged.
func (c CLI) gateCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus gate run [--profile NAME] [--dry-run] <task> | regulus gate evaluate ... | regulus gate verdict <task>")
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task> | regulus gate evaluate ... | regulus gate verdict <task>")
		return 2
	}
	switch args[0] {
	case "evaluate":
		return c.gateEvaluateCmd(args[1:])
	case "verdict":
		return c.gateVerdictCmd(args[1:])
	case "run":
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task> | regulus gate evaluate ... | regulus gate verdict <task>")
		return 2
	}
	plan, code, ok := c.parseGateRunArgs(args[1:])
	if !ok {
		return code
	}
	if plan.dryRun {
		return c.renderDryRunPlan(plan)
	}
	return c.execAndReport(plan)
}

// crapCmd implements `regulus crap --profile <coverprofile> [--max N]
// [--top N]`: the layer-7 gauntlet gate (PRD R-G2) — Uncle Bob's CRAP
// formula over a go cover profile.
func (c CLI) crapCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus crap --profile <coverprofile> [--max N] [--top N]")
		return 0
	}
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
	rep, err := crap.Analyze(c.dir(), *profile)
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
