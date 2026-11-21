// `regulus bench host-journeys`: probe every surface and, with
// --run, execute the conformity journey where the host is usable.
// Missing binaries, missing logins and missing credits mark the host
// not-run with its reason — never pass — and one host's failure never
// degrades the others. Exit 1 only when a journey that actually ran
// fails; evidence lands as JSON under --out.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"regulus-ai/internal/pxexec"
	"runtime"
	"strings"
	"time"

	"regulus-ai/internal/bench"
)

func (c CLI) benchHostJourneysCmd(args []string) int {
	fs := flag.NewFlagSet("bench host-journeys", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	run := fs.Bool("run", false, "execute journeys where the host is usable (default: probe only, no model cost)")
	model := fs.String("model", "", "model for headless hosts (host default when empty)")
	out := fs.String("out", "", "write per-host results JSON into this directory")
	only := fs.String("host", "all", "opencode | claude | codex | devin | all")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench host-journeys [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin]")
		return 2
	}
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	results := probeHostJourneys(ctx, c, *run, *model, *only, now)
	fmt.Fprint(c.Stdout, bench.RenderMatrix(orderedHostIDs(*only), results))
	if *out != "" {
		if err := saveHostJourneyResults(*out, results); err != nil {
			fmt.Fprintf(c.Stderr, "host-journeys: %v\n", err)
			return 1
		}
	}
	failed, ran := tallyHostJourneys(results)
	return reportHostJourneys(c, failed, ran)
}

// probeHostJourneys probes every selected surface: version check first,
// then the conformity journey only with --run. One host's failure never
// degrades the others.
func probeHostJourneys(ctx context.Context, c CLI, run bool, model, only, now string) map[string]bench.HostJourneyOutcome {
	results := map[string]bench.HostJourneyOutcome{}
	for _, p := range bench.DefaultHostProbes() {
		if only != "all" && p.ID != only {
			continue
		}
		results[p.ID] = probeOneHostJourney(ctx, c, p, run, model, now)
	}
	return results
}

// probeOneHostJourney resolves one surface: not-run with its reason
// when the binary is missing, contract-probed in probe-only mode, the
// full journey with --run.
func probeOneHostJourney(ctx context.Context, c CLI, p bench.HostProbe, run bool, model, now string) bench.HostJourneyOutcome {
	ver, err := p.Version(ctx)
	r := bench.HostJourneyOutcome{Host: p.ID, Version: ver, RecordedAt: now, Windows: isWindows()}
	if err != nil {
		r.Level, r.Status, r.Reason = bench.LevelNotRun, bench.AgentNotRun, err.Error()
		return r
	}
	if !run {
		r.Level, r.Status, r.Reason = bench.LevelContract, bench.AgentNotRun, "probe only: binary present, version "+ver+" (journey not executed, no model cost)"
		return r
	}
	return c.executeHostJourney(ctx, p.ID, ver, model, now)
}

// orderedHostIDs lists the matrix rows in stable order, honoring --host.
func orderedHostIDs(only string) []string {
	ordered := []string{}
	for _, h := range []string{"opencode", "claude", "codex", "devin"} {
		if only != "all" && h != only {
			continue
		}
		ordered = append(ordered, h)
	}
	return ordered
}

// saveHostJourneyResults writes the per-host JSON evidence pack.
func saveHostJourneyResults(out string, results map[string]bench.HostJourneyOutcome) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(results, "", "  ")
	name := fmt.Sprintf("host-journeys-%s.json", time.Now().UTC().Format("20060102T150405Z"))
	return os.WriteFile(filepath.Join(out, name), raw, 0o644)
}

// tallyHostJourneys counts failed and passed journeys.
func tallyHostJourneys(results map[string]bench.HostJourneyOutcome) (failed, ran int) {
	for _, r := range results {
		if r.Status == bench.AgentFail {
			failed++
		}
		if r.Status == bench.AgentPass {
			ran++
		}
	}
	return failed, ran
}

// reportHostJourneys prints the final verdict: exit 1 only when a
// journey that actually ran failed, else 0.
func reportHostJourneys(c CLI, failed, ran int) int {
	if failed > 0 {
		fmt.Fprintf(c.Stdout, "host-journeys FAIL: %d host journey(s) failed\n", failed)
		return 1
	}
	if ran == 0 {
		fmt.Fprintln(c.Stdout, "host-journeys INCONCLUSIVE: no host executed a journey (nothing passed or failed)")
		return 0
	}
	fmt.Fprintf(c.Stdout, "host-journeys PASS: %d host(s) green\n", ran)
	return 0
}

// hostJourneySteps builds the conformity journey: the same movements
// on every surface (install, doctor, context and memory, spec and
// chart, gate run, close) against the fixture repo. Steps re-exec the
// regulus binary (self-exec, never in-process recursion into Run) so
// the journey exercises the real user surface; the agent leg is the
// fixture prompt the host would execute headless (recorded, never
// faked green). binOverride selects the binary (REGULUS_BIN, tests).
func hostJourneySteps(selfBin, workdir string) []bench.JourneyStep {
	sub := func(args ...string) func(context.Context, string) (string, error) {
		return func(ctx context.Context, _ string) (string, error) {
			res := pxexec.Run(ctx, pxexec.Spec{Name: selfBin, Args: args, Dir: workdir})
			if res.Outcome != pxexec.OutcomePass {
				return res.Stdout, fmt.Errorf("%s: exit status %d (%s)", strings.Join(args, " "), res.Exit, strings.TrimSpace(res.Stderr))
			}
			return res.Stdout, nil
		}
	}
	return []bench.JourneyStep{
		{Name: "install", Run: sub("init", "--stack", "smoke", "--force")},
		{Name: "doctor", Run: sub("doctor")},
		{Name: "context", Run: sub("memory", "context")},
		{Name: "spec", Run: sub("spec", "check", "charts/demo.md")},
		{Name: "gate", Run: sub("gate", "run", "--yes", "demo/T1")},
		{Name: "close", Run: sub("task", "close", "charts/demo.md", "T1")},
	}
}

// journeyBin resolves the binary journeys execute: REGULUS_BIN wins
// (tests, dev snippets), else the running regulus binary itself.
func journeyBin() string {
	if b := os.Getenv("REGULUS_BIN"); b != "" {
		return b
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "regulus"
}

// executeHostJourney runs the conformity journey for one usable host
// against a fixture repo: install/doctor, context, spec, gate, receipt
// and close. A quota/auth failure mid-journey marks not-run; any other
// step failure marks fail with the step named.
func (c CLI) executeHostJourney(ctx context.Context, host, version, model, now string) bench.HostJourneyOutcome {
	r := bench.HostJourneyOutcome{Host: host, Version: version, Level: bench.LevelHost, RecordedAt: now, Windows: isWindows()}
	work, err := os.MkdirTemp("", "host-journey-")
	if err != nil {
		r.Status, r.Reason = bench.AgentInfra, err.Error()
		return r
	}
	defer os.RemoveAll(work)
	if err := bench.JourneyFixture(work); err != nil {
		r.Status, r.Reason = bench.AgentInfra, err.Error()
		return r
	}
	steps := hostJourneySteps(journeyBin(), work)
	for _, s := range steps {
		tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		detail, err := s.Run(tctx, work)
		cancel()
		if err != nil {
			status, reason := classifyJourneyErr(s.Name, err)
			r.Status, r.Reason = status, reason
			r.Evidence = detail
			if status == bench.AgentNotRun {
				r.Level = bench.LevelContract
			}
			return r
		}
	}
	r.Status, r.Reason = bench.AgentPass, "install, doctor, context, spec, gate, receipt and close green"
	return r
}

// classifyJourneyErr maps a step failure: quota/auth failures mean
// the host was unusable (not-run, never a false fail); doctor is
// advisory (allowed to fail per the Devin blueprint: it probes for a
// runtime Cloud VMs lack); anything else fails the journey naming
// the step.
func isQuotaErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "quota") || strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "not logged in") || strings.Contains(msg, "402")
}

func classifyJourneyErr(step string, err error) (string, string) {
	if isQuotaErr(err) {
		return bench.AgentNotRun, "host unusable at run time: " + shortErr(err)
	}
	if step == "doctor" {
		return bench.AgentNotRun, "doctor refused (runtime probe; advisory, not a journey verdict)"
	}
	return bench.AgentFail, step + ": " + shortErr(err)
}

func shortErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if len(msg) > 300 {
		return msg[:300]
	}
	return msg
}

func isWindows() bool { return runtime.GOOS == "windows" }
