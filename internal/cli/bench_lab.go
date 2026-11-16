// `regulus bench lab ...` (F11.2/NOV-88): the Agent Lab — isolated
// single runs, same-task comparisons and integrated flows over the
// shared agent-task fixtures, without launching the corpus.
//
// Subcommands:
//
//	agents                                   list the registered agents
//	run --agent <a> --task <t> ...           one isolated execution
//	compare --agents <a,b> --task <t> ...    same task, same limits, side by side
//	flow --agent <a> --task <t> ...          run plus the F11.1 timeline inline
//	report --evidence-dir <dir>              render a stored Lab run
//
// Selection is always one task: no Lab mode runs the full corpus.
// Replay is exact: --config loads an exported configuration and
// refuses any selection flag alongside it. Nothing here routes
// agents automatically (F12.1): compare contrasts, it never routes.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"regulus-ai/internal/bench"
)

// labOptions carries the parsed Lab selection: shared by run, flow
// and compare (compare takes agents, the others take agent).
type labOptions struct {
	mode        string
	agent       string
	agents      string
	task        string
	host        string
	hostVersion string
	model       string
	budget      int
	seed        int64
	retries     int
	variant     string
	config      string
	export      string
	evidenceDir string
	hasSeed     bool
	hasBudget   bool
	hasRetries  bool
	hasVariant  bool
	hasHost     bool
	hasModel    bool
	hasHostVer  bool
	hasAgent    bool
	hasAgents   bool
	hasTask     bool
}

func (c CLI) benchLabCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, labUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, labUsage())
		return 2
	}
	switch args[0] {
	case "agents":
		return c.benchLabAgentsCmd(args[1:])
	case "run":
		return c.benchLabRunCmd(bench.LabModeSingle, args[1:])
	case "compare":
		return c.benchLabCompareCmd(args[1:])
	case "flow":
		return c.benchLabRunCmd(bench.LabModeFlow, args[1:])
	case "report":
		return c.benchLabReportCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "bench lab: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, labUsage())
		return 2
	}
}

func labUsage() string {
	return "usage: regulus bench lab agents [--json] | regulus bench lab run --agent <a> --task <t> [--host H] [--model M] [--host-version V] [--budget-s N] [--seed N] [--retries N] [--variant V] [--config F] [--export F] [--evidence-dir D] | regulus bench lab compare --agents <a,b> --task <t> [--host H] [--model M] [--host-version V] [--budget-s N] [--seed N] [--retries N] [--variant V] [--config F] [--export F] [--evidence-dir D] | regulus bench lab flow --agent <a> --task <t> [--host H] [--model M] [--host-version V] [--budget-s N] [--seed N] [--retries N] [--variant V] [--export F] [--evidence-dir D] | regulus bench lab report --evidence-dir <dir>"
}

// benchLabAgentsCmd implements `regulus bench lab agents`: the
// inventory — responsibility, prompt/skill, tools, host and inputs
// per registered agent.
func (c CLI) benchLabAgentsCmd(args []string) int {
	fs := flag.NewFlagSet("bench lab agents", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the registry as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench lab agents [--json]")
		return 2
	}
	reg := bench.LabRegistry()
	if *asJSON {
		raw, err := json.MarshalIndent(reg, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench lab agents: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "agent-lab %s: %d agent(s)\n", bench.LabVersion, len(reg))
	for _, a := range reg {
		fmt.Fprintf(c.Stdout, "- %s: %s\n  skill %s host %s tools [%s] inputs [%s]\n",
			a.ID, a.Responsibility, a.Skill, a.Host,
			strings.Join(a.Tools, ", "), strings.Join(a.Inputs, ", "))
		if a.Prompt != "" {
			fmt.Fprintf(c.Stdout, "  prompt: %s\n", a.Prompt)
		}
	}
	return 0
}

// parseBenchLabFlags parses the shared Lab selection flags. The mode
// decides which identity flags are required later, not here.
func parseBenchLabFlags(mode string, args []string, stderr io.Writer) (labOptions, int, bool) {
	var opts labOptions
	opts.mode = mode
	fs := flag.NewFlagSet("bench lab "+mode, flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "the registered agent (run|flow)")
	agents := fs.String("agents", "", "comma-separated registered agents (compare, at least two)")
	task := fs.String("task", "", "the fixture task id (required, never the full corpus)")
	host := fs.String("host", "", "headless host override (default: the agent registry host)")
	hostVersion := fs.String("host-version", "", "host version recorded on outcomes")
	model := fs.String("model", "", "model for the headless host (host default when empty)")
	budget := fs.Int("budget-s", 0, "per-execution timeout in seconds (0 = task default, only lowers)")
	seed := fs.Int64("seed", 0, "replay seed (default: the splits seed)")
	retries := fs.Int("retries", 0, "transient host-infra retries (0..5)")
	variant := fs.String("variant", "", "variant label recorded on outcomes (default lab)")
	config := fs.String("config", "", "replay an exported configuration exactly")
	export := fs.String("export", "", "write the executed configuration as replayable JSON")
	evidenceDir := fs.String("evidence-dir", "", "write Lab evidence (config + outcomes + reports) to this directory")
	if err := fs.Parse(args); err != nil {
		return labOptions{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, labUsage())
		return labOptions{}, 2, false
	}
	opts = labOptions{
		mode: mode, agent: *agent, agents: *agents, task: *task,
		host: *host, hostVersion: *hostVersion, model: *model,
		budget: *budget, seed: *seed, retries: *retries, variant: *variant,
		config: *config, export: *export, evidenceDir: *evidenceDir,
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "seed":
			opts.hasSeed = true
		case "budget-s":
			opts.hasBudget = true
		case "retries":
			opts.hasRetries = true
		case "variant":
			opts.hasVariant = true
		case "host":
			opts.hasHost = true
		case "model":
			opts.hasModel = true
		case "host-version":
			opts.hasHostVer = true
		case "agent":
			opts.hasAgent = true
		case "agents":
			opts.hasAgents = true
		case "task":
			opts.hasTask = true
		}
	})
	return opts, 0, true
}

// resolveBenchLabConfig turns parsed flags into a validated,
// replayable LabConfig: --config replays exactly (any selection flag
// alongside it is a usage error so a replay cannot silently differ),
// otherwise the flags build the config against the corpus.
func resolveBenchLabConfig(opts labOptions, corpus []bench.AgentTask, splits bench.AgentSplits, stderr io.Writer) (bench.LabConfig, int, bool) {
	if opts.config != "" {
		if opts.hasAgent || opts.hasAgents || opts.hasTask || opts.hasHost || opts.hasModel ||
			opts.hasHostVer || opts.hasBudget || opts.hasSeed || opts.hasRetries || opts.hasVariant {
			fmt.Fprintln(stderr, "bench lab: --config replays exactly, drop selection flags (--agent/--agents/--task/--host/--model/--host-version/--budget-s/--seed/--retries/--variant)")
			return bench.LabConfig{}, 2, false
		}
		cfg, err := bench.LoadLabConfig(opts.config, corpus)
		if err != nil {
			fmt.Fprintf(stderr, "bench lab: %v\n", err)
			return bench.LabConfig{}, 1, false
		}
		if opts.mode != "" && cfg.Mode != opts.mode {
			fmt.Fprintf(stderr, "bench lab: config mode is %q, not %q\n", cfg.Mode, opts.mode)
			return bench.LabConfig{}, 1, false
		}
		return cfg, 0, true
	}
	cfg := bench.LabConfig{Mode: opts.mode, Task: opts.task, Host: opts.host,
		Model: opts.model, BudgetS: opts.budget, Retries: opts.retries, Variant: opts.variant}
	if opts.hasSeed {
		cfg.Seed = opts.seed
	} else {
		cfg.Seed = splits.Seed
	}
	switch opts.mode {
	case bench.LabModeCompare:
		cfg.Agents = splitList(opts.agents)
	default:
		cfg.Agent = opts.agent
	}
	if err := bench.ValidateLabConfig(cfg, corpus); err != nil {
		fmt.Fprintf(stderr, "bench lab: %v\n", err)
		return bench.LabConfig{}, 2, false
	}
	return cfg, 0, true
}

// labTaskSplit finds one fixture task and its split label.
func labTaskSplit(corpus []bench.AgentTask, splits bench.AgentSplits, id string) (bench.AgentTask, string, bool) {
	for _, t := range corpus {
		if t.ID == id {
			return t, bench.SplitOf(splits, id), true
		}
	}
	return bench.AgentTask{}, "", false
}

// resolveLabRunner materializes the headless host for one agent: the
// --host override wins, otherwise the registry host. An unusable host
// degrades to NOT RUN, never to a verdict on work never done.
func (c CLI) resolveLabRunner(host string) (bench.HeadlessAgent, string, int, bool) {
	return resolveAgentRunner(host)
}

// benchLabRunCmd implements `regulus bench lab run|flow`: one
// isolated execution. Flow prints the F11.1 timeline reconstruction
// inline after the result. Exit: 0 pass/fail, 1 INCONCLUSIVE
// (timeout/infra), 3 NOT RUN.
func (c CLI) benchLabRunCmd(mode string, args []string) int {
	started := time.Now().UTC()
	opts, code, ok := parseBenchLabFlags(mode, args, c.Stderr)
	if !ok {
		return code
	}
	corpus := bench.AgentCorpus()
	splits, code, ok := c.loadBenchAgentSplits(corpus)
	if !ok {
		return code
	}
	cfg, code, ok := resolveBenchLabConfig(opts, corpus, splits, c.Stderr)
	if !ok {
		return code
	}
	task, split, ok := labTaskSplit(corpus, splits, cfg.Task)
	if !ok {
		fmt.Fprintf(c.Stderr, "bench lab: unknown task %q\n", cfg.Task)
		return 2
	}
	agent, ok := bench.FindLabAgent(cfg.Agent)
	if !ok {
		fmt.Fprintf(c.Stderr, "bench lab: unknown agent %q (see `bench lab agents`)\n", cfg.Agent)
		return 2
	}
	host := agent.Host
	if cfg.Host != "" {
		host = cfg.Host
	}
	runner, notRunMsg, notRunCode, ok := c.resolveLabRunner(host)
	if !ok {
		fmt.Fprintln(c.Stdout, strings.Replace(notRunMsg, "agent-tasks NOT RUN", "lab NOT RUN", 1))
		return notRunCode
	}
	cfg.Host = host
	res := bench.RunLab(context.Background(), runner, agent, task, split, cfg, c.dir())
	if opts.hostVersion != "" {
		res.Outcome.HostVersion = opts.hostVersion
	} else if host == "opencode" {
		res.Outcome.HostVersion = probeOpenCodeVersion()
	}
	if opts.export != "" {
		if err := bench.ExportLabConfig(opts.export, cfg); err != nil {
			fmt.Fprintf(c.Stderr, "bench lab: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "lab: config exported to %s\n", opts.export)
	}
	if opts.evidenceDir != "" {
		if code := c.writeBenchLabEvidence(opts.evidenceDir, cfg, []bench.LabResult{res}, started); code != 0 {
			return code
		}
	}
	printBenchLabResult(c.Stdout, res)
	if mode == bench.LabModeFlow {
		fmt.Fprintln(c.Stdout, "--- trace timeline (F11.1) ---")
		c.traceTimelineCmd([]string{"--run", res.RunID, "--rebuild"})
	}
	return benchLabExit(res.Verdict)
}

// benchLabExit maps a single verdict to the process exit: pass/fail
// conclude (0), timeout/infra never pass (1 INCONCLUSIVE), not-run
// never concludes (3).
func benchLabExit(verdict string) int {
	switch verdict {
	case bench.LabPass, bench.LabFail:
		return 0
	case bench.LabNotRun:
		return 3
	default:
		return 1
	}
}

// printBenchLabResult prints one execution: verdict, duration,
// tokens with provenance, retries, ids and the F11.1 trace hint,
// plus the closing line when the run never concluded.
func printBenchLabResult(stdout io.Writer, res bench.LabResult) {
	fmt.Fprintf(stdout, "lab %s/%s [%s]: %s (%dms, tokens %d+%d %s, retries %d)\n",
		res.AgentID, res.TaskID, res.Split, res.Verdict,
		res.DurationMS, res.TokensIn, res.TokensOut, res.TokenSource, res.Retries)
	fmt.Fprintf(stdout, "run %s agent %s session %s\n", res.RunID, res.AgentExecutionID, res.SessionID)
	fmt.Fprintf(stdout, "trace: %s\n", res.TraceHint)
	switch res.Verdict {
	case bench.LabNotRun:
		fmt.Fprintln(stdout, "lab NOT RUN: execution canceled or never started (nothing passed or failed)")
	case bench.LabTimeout, bench.LabInfraError, bench.LabInconclusive:
		fmt.Fprintln(stdout, "lab INCONCLUSIVE: no evaluable verdict (infra/timeout only)")
	}
}

// benchLabCompareCmd implements `regulus bench lab compare`: every
// listed agent runs the same task with the same limits, sequentially.
// Exit: 0 when at least one result is evaluable (winner or tie
// printed), 1 INCONCLUSIVE, 3 NOT RUN.
func (c CLI) benchLabCompareCmd(args []string) int {
	started := time.Now().UTC()
	opts, code, ok := parseBenchLabFlags(bench.LabModeCompare, args, c.Stderr)
	if !ok {
		return code
	}
	corpus := bench.AgentCorpus()
	splits, code, ok := c.loadBenchAgentSplits(corpus)
	if !ok {
		return code
	}
	cfg, code, ok := resolveBenchLabConfig(opts, corpus, splits, c.Stderr)
	if !ok {
		return code
	}
	task, split, ok := labTaskSplit(corpus, splits, cfg.Task)
	if !ok {
		fmt.Fprintf(c.Stderr, "bench lab: unknown task %q\n", cfg.Task)
		return 2
	}
	var cs []bench.LabContestant
	for _, id := range cfg.Agents {
		agent, ok := bench.FindLabAgent(id)
		if !ok {
			fmt.Fprintf(c.Stderr, "bench lab: unknown agent %q (see `bench lab agents`)\n", id)
			return 2
		}
		host := agent.Host
		if cfg.Host != "" {
			host = cfg.Host
		}
		runner, notRunMsg, notRunCode, ok := c.resolveLabRunner(host)
		if !ok {
			fmt.Fprintln(c.Stdout, strings.Replace(notRunMsg, "agent-tasks NOT RUN", "lab NOT RUN", 1))
			return notRunCode
		}
		cs = append(cs, bench.LabContestant{Agent: agent, Runner: runner})
	}
	cmp, err := bench.CompareLab(context.Background(), cs, task, split, cfg, c.dir())
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench lab: %v\n", err)
		return 1
	}
	probed := ""
	for i := range cmp.Results {
		if opts.hostVersion != "" {
			cmp.Results[i].Outcome.HostVersion = opts.hostVersion
		} else if cmp.Results[i].Outcome.Host == "opencode" {
			if probed == "" {
				probed = probeOpenCodeVersion()
			}
			cmp.Results[i].Outcome.HostVersion = probed
		}
	}
	if opts.export != "" {
		if err := bench.ExportLabConfig(opts.export, cfg); err != nil {
			fmt.Fprintf(c.Stderr, "bench lab: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "lab: config exported to %s\n", opts.export)
	}
	if opts.evidenceDir != "" {
		if code := c.writeBenchLabEvidence(opts.evidenceDir, cfg, cmp.Results, started); code != 0 {
			return code
		}
	}
	fmt.Fprintf(c.Stdout, "lab compare %s [%s]: %d agent(s), %d evaluable\n", cmp.TaskID, cmp.Split, len(cmp.Results), cmp.Evaluable)
	for _, r := range cmp.Results {
		fmt.Fprintf(c.Stdout, "  - %s: %s (%dms, tokens %d+%d %s)\n",
			r.AgentID, r.Verdict, r.DurationMS, r.TokensIn, r.TokensOut, r.TokenSource)
	}
	fmt.Fprintf(c.Stdout, "comparison: winner %s (%s)\n", cmp.Winner, cmp.Reason)
	for _, r := range cmp.Results {
		fmt.Fprintf(c.Stdout, "trace %s: %s\n", r.AgentID, r.TraceHint)
	}
	switch {
	case allLabNotRun(cmp.Results):
		fmt.Fprintln(c.Stdout, "lab NOT RUN: executions canceled or never started (nothing passed or failed)")
		return 3
	case cmp.Evaluable == 0:
		fmt.Fprintln(c.Stdout, "lab INCONCLUSIVE: nothing evaluable (infra/timeout only)")
		return 1
	default:
		return 0
	}
}

func allLabNotRun(results []bench.LabResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if r.Verdict != bench.LabNotRun {
			return false
		}
	}
	return true
}

// writeBenchLabEvidence stores one Lab run (config + shared outcomes
// + full reports). Failures print the byte-identical refusal and
// report exit 1.
func (c CLI) writeBenchLabEvidence(dir string, cfg bench.LabConfig, results []bench.LabResult, started time.Time) int {
	head := gitHeadBounded(c.runner())
	if head == "" {
		head = "unknown"
	}
	if _, err := bench.WriteLabEvidence(dir, cfg, results, head, started.Format(time.RFC3339)); err != nil {
		fmt.Fprintf(c.Stderr, "bench lab: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "lab: evidence recorded in %s\n", dir)
	return 0
}

// benchLabReportCmd implements `regulus bench lab report
// --evidence-dir <dir>`: renders strictly from the verified evidence
// files, never hand-written.
func (c CLI) benchLabReportCmd(args []string) int {
	fs := flag.NewFlagSet("bench lab report", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	dir := fs.String("evidence-dir", "", "Lab evidence directory with config.json + outcomes.json + reports.json (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench lab report --evidence-dir <dir>")
		return 2
	}
	cfg, m, _, results, err := bench.ReadLabEvidence(*dir)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench lab report: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, bench.RenderLabReport(cfg, m, results))
	return 0
}
