package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/crap"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/manifest"
)

// gateCmd implements `regulus gate run [--profile NAME] [--dry-run] <task>`:
// run the layered gauntlet and write one append-only receipt per executed
// gate (PRD R-G2, R-G4). --dry-run prints the execution plan and writes
// nothing. --profile selects required gates plus the profile's members
// (quick = required only, full = everything, the default).
func (c CLI) gateCmd(args []string) int {
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
	fs := flag.NewFlagSet("gate run", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	dryRun := fs.Bool("dry-run", false, "print the execution plan without running gates")
	profile := fs.String("profile", "", "run required gates plus this profile's members (quick, full, or a declared name)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task>")
		return 2
	}
	task := fs.Arg(0)
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
	if *dryRun {
		ordered, err := gates.SelectGates(cfg, *profile)
		if err != nil {
			fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
			return 1
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
		max := 3600
		if cfg.Execution != nil && cfg.Execution.MaxTimeoutSec > 0 {
			max = cfg.Execution.MaxTimeoutSec
		}
		dockerImage := ""
		if cfg.Execution != nil {
			dockerImage = cfg.Execution.DockerImage
		}
		_, isSh := r.(gates.ShRunner)
		if !isSh {
			if _, ok := r.(*gates.ShRunner); ok {
				isSh = true
			}
		}
		for _, g := range ordered {
			timeout := g.TimeoutSec
			if timeout <= 0 {
				timeout = gates.DefaultTimeoutSec
			}
			if timeout > max {
				timeout = max
			}
			if timeout < 1 {
				timeout = 1
			}
			classification := gates.Classify(g.Command)
			isolation := "unknown"
			if isSh {
				isolation = "host-contained"
				if g.Isolate == "docker" && dockerImage != "" {
					isolation = "docker"
				}
			}
			repro := g.Reproducibility
			if repro == "" {
				repro = "unknown"
			} else if p := manifest.ParseReproducibility(repro); p != "" {
				repro = p
			}
			policy := g.EffectiveOnFail()
			required := "required"
			if !g.IsRequired() {
				required = "optional"
			}
			cond := "always"
			if len(g.When) > 0 {
				cond = "when " + strings.Join(g.When, ",")
			}
			fmt.Fprintf(c.Stdout, "layer %d %-16s %q timeout=%ds classification=%s isolation=%s reproducibility=%s on_fail=%s %s %s\n", g.Layer, g.Name, g.Command, timeout, classification, isolation, repro, policy, required, cond)
		}
		return 0
	}
	cfgBytes, err := os.ReadFile("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	res, mpath, err := gates.RunTask(context.Background(), cfg, cfgBytes, ".", task, c.Version, contract.Version, *profile, r)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	for _, o := range res.Outcomes {
		if o.Skipped {
			fmt.Fprintf(c.Stdout, "layer %d %-16s skipped (%s)\n", o.Layer, o.Name, o.SkipReason)
			continue
		}
		fmt.Fprintf(c.Stdout, "layer %d %-16s %s (%dms)\n", o.Layer, o.Name, o.Verdict, o.DurationMS)
	}
	if res.Failed {
		fmt.Fprintf(c.Stdout, "gauntlet: stopped at gate %q — %d/%d gates passed\n", res.FailedGate, res.Passed, res.Executed)
	} else {
		fmt.Fprintf(c.Stdout, "gauntlet: %d/%d gates passed\n", res.Passed, res.Executed)
	}
	advisory := 0
	for _, o := range res.Outcomes {
		if o.Advisory && o.Verdict == "fail" {
			advisory++
		}
	}
	if advisory > 0 {
		fmt.Fprintf(c.Stdout, "gauntlet: %d advisory failure(s) recorded, run continues\n", advisory)
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(c.Stdout, "gauntlet: %d gate(s) skipped\n", len(res.Skipped))
	}
	fmt.Fprintf(c.Stdout, "verdict: %s (%s)\n", res.Status, res.Reason)
	fmt.Fprintf(c.Stdout, "manifest: %s\n", mpath)
	if res.Failed {
		return 1
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
