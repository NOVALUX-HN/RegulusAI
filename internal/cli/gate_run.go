package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/manifest"
)

// gateRunArgs is the validated budget of one `gate run`: the task, the
// profile selector, the dry-run flag, the loaded config and the runner.
// parseGateRunArgs builds it; renderDryRunPlan and execAndReport consume it.
type gateRunArgs struct {
	task    string
	profile string
	dryRun  bool
	cfg     gates.Config
	runner  gates.CmdRunner
}

// parseGateRunArgs parses `gate run` flags and validates the task id,
// defaulting a nil runner to the real shell runner and loading
// regulus.json. On failure it prints the same message gateCmd always
// printed and returns the same exit code (2 usage, 1 error).
func (c CLI) parseGateRunArgs(args []string) (gateRunArgs, int, bool) {
	fs := flag.NewFlagSet("gate run", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	dryRun := fs.Bool("dry-run", false, "print the execution plan without running gates")
	profile := fs.String("profile", "", "run required gates plus this profile's members (quick, full, or a declared name)")
	if err := fs.Parse(args); err != nil {
		return gateRunArgs{}, 2, false
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task>")
		return gateRunArgs{}, 2, false
	}
	task := fs.Arg(0)
	if !validTaskID(task) {
		fmt.Fprintf(c.Stderr, "gate run: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", task)
		return gateRunArgs{}, 2, false
	}
	r := c.Runner
	if r == nil {
		r = gates.ShRunner{}
	}
	cfg, err := gates.Load("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return gateRunArgs{}, 1, false
	}
	return gateRunArgs{task: task, profile: *profile, dryRun: *dryRun, cfg: cfg, runner: r}, 0, true
}

// renderDryRunPlan prints the execution plan without running anything and
// writes nothing. Selection, layer order, timeout budget, isolation and
// annotations all match the historical dry-run output exactly.
func (c CLI) renderDryRunPlan(plan gateRunArgs) int {
	ordered, err := gates.SelectGates(plan.cfg, plan.profile)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
	max := 3600
	if plan.cfg.Execution != nil && plan.cfg.Execution.MaxTimeoutSec > 0 {
		max = plan.cfg.Execution.MaxTimeoutSec
	}
	dockerImage := ""
	if plan.cfg.Execution != nil {
		dockerImage = plan.cfg.Execution.DockerImage
	}
	isSh := dryRunIsSh(plan.runner)
	for _, g := range ordered {
		fmt.Fprint(c.Stdout, dryRunGateLine(g, max, dockerImage, isSh))
	}
	return 0
}

// dryRunIsSh reports whether the runner is the real shell runner (value or
// pointer): only then is the dry-run isolation knowable.
func dryRunIsSh(r gates.CmdRunner) bool {
	_, isSh := r.(gates.ShRunner)
	if !isSh {
		if _, ok := r.(*gates.ShRunner); ok {
			isSh = true
		}
	}
	return isSh
}

// dryRunGateLine renders one dry-run plan line: timeout clamp, command
// classification, isolation, reproducibility, failure policy, required mark
// and when-condition. Pure formatting: no I/O, no execution.
func dryRunGateLine(g gates.Gate, max int, dockerImage string, isSh bool) string {
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
	return fmt.Sprintf("layer %d %-16s %q timeout=%ds classification=%s isolation=%s reproducibility=%s on_fail=%s %s %s\n", g.Layer, g.Name, g.Command, timeout, classification, isolation, repro, policy, required, cond)
}

// execAndReport runs the gauntlet for one task and prints the per-layer
// lines plus the gauntlet/verdict/manifest footers. Exit 1 on load, run or
// gate failure; exit 0 when the gauntlet passes (advisory-only failures
// also exit 0, exactly as before).
func (c CLI) execAndReport(plan gateRunArgs) int {
	cfgBytes, err := os.ReadFile("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	res, mpath, err := gates.RunTask(context.Background(), plan.cfg, cfgBytes, ".", plan.task, c.Version, contract.Version, plan.profile, plan.runner)
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
