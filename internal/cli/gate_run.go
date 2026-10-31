package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
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

// gateToolHint maps a gate binary to its install hint. Pinned versions
// match .github/workflows/gauntlet.yml:31-35 (gremlins v0.6.0,
// govulncheck v1.9.0); bun/bunx and go point at their install pages.
// Unknown tools fall back to the troubleshooting doc so the message
// always carries a hint.
func gateToolHint(tool string) string {
	switch strings.ToLower(tool) {
	case "gremlins":
		return "go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0"
	case "govulncheck":
		return "go install golang.org/x/vuln/cmd/govulncheck@v1.9.0"
	case "bun", "bunx":
		return "https://bun.sh"
	case "go":
		return "https://go.dev/dl/"
	default:
		return "see docs/troubleshooting.md"
	}
}

// preflightFirstToken extracts the binary of a gate command: the first
// whitespace token, unquoted, basenamed on both separators, without a
// trailing .exe. `go run ./cmd/regulus ...` maps to `go`,
// `./regulus.exe crap ...` maps to `regulus`.
func preflightFirstToken(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	first := strings.Trim(fields[0], `"'`)
	if i := strings.LastIndex(first, "/"); i >= 0 {
		first = first[i+1:]
	}
	if i := strings.LastIndex(first, `\`); i >= 0 {
		first = first[i+1:]
	}
	return strings.TrimSuffix(first, ".exe")
}

// preflightShellResolved reports whether a gate binary resolves via the
// shell rather than PATH, so preflight must not LookPath it. Gates run
// through `sh -c` / `pwsh -Command` / `powershell -Command` / `cmd /C`
// (see ShRunner), where builtins resolve without any binary on PATH —
// `echo` is the one the fixtures and the `init --stack smoke` scaffold
// actually use, the rest are its common siblings. `regulus` is the
// self-reference to the running harness binary (`regulus ...`,
// `./regulus.exe ...`): it is not on PATH by design, so there is
// nothing to preflight. Match is case-insensitive; callers pass the
// preflightFirstToken form (already basenamed, .exe stripped).
func preflightShellResolved(tool string) bool {
	switch strings.ToLower(tool) {
	case "echo", "cd", "dir", "del", "erase", "copy", "move", "ren",
		"rename", "type", "set", "rem", "cls", "exit", "pwd", "export",
		"source", "alias", "unalias", "true", "false", "test", "printf",
		"command", "history",
		"regulus":
		return true
	}
	return false
}

// preflightMissingTools returns the deduped tools of the selected gates
// that lookPath cannot resolve, in first-seen order, each with its
// install hint. Tokens that resolve via the shell (builtins,
// self-reference) are skipped, never reported. Pure: no I/O, no
// execution.
func preflightMissingTools(selected []gates.Gate, lookPath func(string) (string, error)) []string {
	var missing []string
	seen := map[string]bool{}
	for _, g := range selected {
		tool := preflightFirstToken(g.Command)
		if tool == "" || seen[tool] {
			continue
		}
		seen[tool] = true
		if preflightShellResolved(tool) {
			continue
		}
		if _, err := lookPath(tool); err != nil {
			missing = append(missing, tool+" (install: "+gateToolHint(tool)+")")
		}
	}
	return missing
}

// execAndReport runs the gauntlet for one task and prints the per-layer
// lines plus the gauntlet/verdict/manifest footers. Before any layer it
// preflights the selected gates' binaries (LookPath, fail-fast with
// install hints, real-shell runs only). Exit 1 on load, preflight, run
// or gate failure; exit 0 when the gauntlet passes (advisory-only
// failures also exit 0, exactly as before).
func (c CLI) execAndReport(plan gateRunArgs) int {
	cfgBytes, err := os.ReadFile("regulus.json")
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	// Preflight (S2): before any layer runs and before any receipt is
	// written, resolve the binary of every SELECTED gate (profile-aware)
	// via LookPath and fail fast listing the missing ones with install
	// hints. Real-shell runs only: test doubles (non-Sh runners) provide
	// their own commands, so there is nothing on PATH to check. --dry-run
	// never reaches here: the plan only prints commands and writes
	// nothing, so preflight is exec-only by design.
	if plan.runner == nil || dryRunIsSh(plan.runner) {
		ordered, serr := gates.SelectGates(plan.cfg, plan.profile)
		if serr != nil {
			fmt.Fprintf(c.Stderr, "gate run: %v\n", serr)
			return 1
		}
		if missing := preflightMissingTools(ordered, exec.LookPath); len(missing) > 0 {
			fmt.Fprintf(c.Stderr, "gate run: missing required tools: %s\n", strings.Join(missing, ", "))
			return 1
		}
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
