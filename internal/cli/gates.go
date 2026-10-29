package cli

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"regulus-ai/internal/crap"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
)

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
