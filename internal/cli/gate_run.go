package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/contract"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/manifest"
)

// gateRunArgs is the validated budget of one `gate run`: the task, the
// profile selector, the consent flag, the dry-run flag, the loaded config
// and the runner. parseGateRunArgs builds it; renderDryRunPlan and
// execAndReport consume it.
type gateRunArgs struct {
	task    string
	profile string
	quick   bool
	dryRun  bool
	yes     bool
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
	yes := fs.Bool("yes", false, "explicitly consent to run gates here (required for untrusted repos and when no allowlist is configured)")
	profile := fs.String("profile", "", "run required gates plus this profile's members (quick, full, or a declared name)")
	quick := fs.Bool("quick", false, "run the lean daily-loop set only: typecheck, lint, build, unit-tests, spec, gofmt")
	if err := fs.Parse(args); err != nil {
		return gateRunArgs{}, 2, false
	}
	if *quick && *profile != "" {
		fmt.Fprintln(c.Stderr, "gate run: --quick and --profile are mutually exclusive (quick is the lean preset, not a profile)")
		return gateRunArgs{}, 2, false
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--quick] [--dry-run] <task>")
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
	cfg, err := gates.Load(filepath.Join(c.dir(), "regulus.json"))
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return gateRunArgs{}, 1, false
	}
	return gateRunArgs{task: task, profile: *profile, quick: *quick, dryRun: *dryRun, yes: *yes, cfg: cfg, runner: r}, 0, true
}

// selectPlanGates resolves which gates the plan runs: the --quick lean
// preset (exactly typecheck/lint/build/unit-tests/spec/gofmt) or the
// --profile mechanism (unchanged: quick = required only, full =
// everything, custom names by declaration). --quick is mutually exclusive
// with --profile, so the profile path never sees a quick run.
func selectPlanGates(cfg gates.Config, plan gateRunArgs) ([]gates.Gate, error) {
	if plan.quick {
		return gates.SelectQuickLean(cfg)
	}
	return gates.SelectGates(cfg, plan.profile)
}

// renderDryRunPlan prints the execution plan without running anything and
// writes nothing. Selection, layer order, timeout budget, isolation and
// annotations all match the historical dry-run output exactly, plus the
// S6 plan header (dir, gate count) and per-line allowlist verdict.
func (c CLI) renderDryRunPlan(plan gateRunArgs) int {
	ordered, err := selectPlanGates(plan.cfg, plan)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
	fmt.Fprintf(c.Stdout, "plan: dir=%s gates=%d\n", c.dir(), len(ordered))
	max := 3600
	if plan.cfg.Execution != nil && plan.cfg.Execution.MaxTimeoutSec > 0 {
		max = plan.cfg.Execution.MaxTimeoutSec
	}
	dockerImage := ""
	if plan.cfg.Execution != nil {
		dockerImage = plan.cfg.Execution.DockerImage
	}
	isSh := dryRunIsSh(plan.runner)
	var allow []string
	if plan.cfg.Execution != nil {
		allow = plan.cfg.Execution.Allowlist
	}
	for _, g := range ordered {
		fmt.Fprint(c.Stdout, dryRunGateLine(g, max, dockerImage, isSh, allow...))
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
// classification, isolation, reproducibility, failure policy, required mark,
// when-condition, the S6 allowlist verdict (allow/deny, deny by default
// without an allowlist) and the destructive mark. The verdict shows the
// pre-consent policy outcome: --yes consent applies at execution time.
// Pure formatting: no I/O, no execution.
func dryRunGateLine(g gates.Gate, max int, dockerImage string, isSh bool, allowlist ...string) string {
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
	verdict := "deny"
	if gates.AllowedByAllowlist(g.Command, allowlist) {
		verdict = "allow"
	}
	destructive := classification == gates.ClassificationDestructive
	return fmt.Sprintf("layer %d %-16s %q timeout=%ds classification=%s isolation=%s reproducibility=%s on_fail=%s %s %s allowlist=%s destructive=%t\n", g.Layer, g.Name, g.Command, timeout, classification, isolation, repro, policy, required, cond, verdict, destructive)
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

// isTrustedRepo reports whether dir is a trusted repo (S6): the marker
// <root>/.regulus/trusted exists. The marker is created by explicit
// operator action, never by a gate run.
func isTrustedRepo(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".regulus", "trusted"))
	return err == nil && !st.IsDir()
}

// consentCtx carries explicit --yes consent to the executor so executed
// receipts record consent:true (S6). Without --yes the background context
// rides unchanged.
func consentCtx(plan gateRunArgs) context.Context {
	ctx := context.Background()
	if plan.yes {
		ctx = gates.ContextWithConsent(ctx, true)
	}
	return ctx
}

// chartGateViolations refuses the run when the task chart is unverifiable:
// it returns the first readable chart with its violations (both empty when
// no chart gates this task).
func chartGateViolations(dir, task string) (string, []string) {
	base, _, ok := strings.Cut(task, "/")
	if !ok || base == "" {
		return "", nil
	}
	for _, cand := range []string{filepath.Join("charts", base+".md"), filepath.Join(dir, "charts", base+".md")} {
		if data, rerr := os.ReadFile(cand); rerr == nil {
			return cand, collectViolations(string(data))
		}
	}
	return "", nil
}

// enforceGateConsent arms deny-by-default and refuses untrusted repos
// without explicit --yes. It reports (code, true) when the run must stop
// before anything executes or is written (S6). Real-shell runs only:
// test doubles provide their own commands, so policy consent does not
// apply to them.
func enforceGateConsent(c CLI, plan *gateRunArgs) (int, bool) {
	if plan.runner != nil && !dryRunIsSh(plan.runner) {
		return 0, false
	}
	if plan.cfg.Execution == nil {
		plan.cfg.Execution = &gates.ExecutionConfig{}
	}
	plan.cfg.Execution.EnforceDenyByDefault = true
	plan.cfg.Execution.ExplicitYes = plan.yes
	if plan.yes || isTrustedRepo(c.dir()) {
		return 0, false
	}
	if code := c.renderDryRunPlan(*plan); code != 0 {
		return code, true
	}
	fmt.Fprintf(c.Stderr, "gate run: untrusted repo (missing .regulus/trusted) — inspect the plan above and re-run with --yes to consent (or --dry-run to inspect only)\n")
	return 1, true
}

// preflightGateTools fail-fast resolves the binary of every selected gate
// (profile-aware) via LookPath and lists the missing ones with install
// hints. It reports (code, true) when the run must stop (S2). Real-shell
// runs only: test doubles (non-Sh runners) provide their own commands,
// so there is nothing on PATH to check. --dry-run never reaches here.
func preflightGateTools(c CLI, plan gateRunArgs) (int, bool) {
	if plan.runner != nil && !dryRunIsSh(plan.runner) {
		return 0, false
	}
	ordered, serr := selectPlanGates(plan.cfg, plan)
	if serr != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", serr)
		return 1, true
	}
	if missing := preflightMissingTools(ordered, exec.LookPath); len(missing) > 0 {
		fmt.Fprintf(c.Stderr, "gate run: missing required tools: %s\n", strings.Join(missing, ", "))
		return 1, true
	}
	return 0, false
}

// printGateReport prints the per-layer lines plus the gauntlet, verdict
// and manifest footers, mapping the outcome to an exit code: 1 on gate
// failure, 0 when the gauntlet passes (advisory-only failures exit 0).
func printGateReport(c CLI, res gates.Result, mpath, task string) int {
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
		fmt.Fprintf(c.Stdout, "why: gate %q failed — task close stays blocked until the gauntlet passes\n", res.FailedGate)
		fmt.Fprintf(c.Stdout, "next: fix gate %q, inspect evidence with regulus evidence show %s, then re-run: regulus gate run %s\n", res.FailedGate, mpath, task)
		return 1
	}
	return 0
}

// execAndReport runs the gauntlet for one task and prints the per-layer
// lines plus the gauntlet/verdict/manifest footers. Before any layer it
// preflights the selected gates' binaries (LookPath, fail-fast with
// install hints, real-shell runs only). Exit 1 on load, preflight, run
// or gate failure; exit 0 when the gauntlet passes (advisory-only
// failures also exit 0, exactly as before).
func (c CLI) execAndReport(plan gateRunArgs) int {
	if chart, violations := chartGateViolations(c.dir(), plan.task); len(violations) != 0 {
		fmt.Fprintf(c.Stdout, "gate run blocked: chart %s has %d violation(s) — BUILD refused before layer 1\n", chart, len(violations))
		for _, v := range violations {
			fmt.Fprintf(c.Stdout, "violation: %s\n", v)
		}
		fmt.Fprintln(c.Stdout, "why: running gates on an unverifiable chart wastes evidence and hides the real defect")
		fmt.Fprintf(c.Stdout, "next: fix the chart, then run: regulus task check %s\n", chart)
		return 1
	}
	cfgBytes, err := os.ReadFile(filepath.Join(c.dir(), "regulus.json"))
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	if code, stop := enforceGateConsent(c, &plan); stop {
		return code
	}
	if code, stop := preflightGateTools(c, plan); stop {
		return code
	}
	// --quick runs the lean preset through the same RunTask path by
	// narrowing the config: full-of-the-narrowed-config is exactly the
	// lean set, so selection, receipts and the manifest stay consistent.
	runCfg, runBytes, runProfile := plan.cfg, cfgBytes, plan.profile
	if plan.quick {
		lean, serr := gates.SelectQuickLean(plan.cfg)
		if serr != nil {
			fmt.Fprintf(c.Stderr, "gate run: %v\n", serr)
			return 1
		}
		runCfg = plan.cfg
		runCfg.Gates = lean
		runProfile = ""
		if marshaled, merr := json.Marshal(runCfg); merr == nil {
			runBytes = marshaled
		}
	}
	res, mpath, err := gates.RunTask(consentCtx(plan), runCfg, runBytes, c.dir(), plan.task, c.Version, contract.Version, runProfile, plan.runner)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate run: %v\n", err)
		return 1
	}
	return printGateReport(c, res, mpath, plan.task)
}
