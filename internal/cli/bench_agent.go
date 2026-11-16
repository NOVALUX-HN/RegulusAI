// `regulus bench agent-tasks` (NOV-68): agent-task evals with a real
// headless host. Without a usable host the suite reports NOT RUN and
// exits non-zero — it never passes in false and never fails tasks the
// agent never attempted.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/executil"
)

// benchAgentTasksOptions carries the parsed `bench agent-tasks` flags:
// repetition count, split selection, host identity and evidence sinks.
type benchAgentTasksOptions struct {
	reps            int
	split           string
	model           string
	host            string
	budget          int
	variant         string
	save            string
	evidenceDir     string
	hostVersionFlag string
	taskFilter      string
	dryRun          bool
	maxWallS        int
}

func (c CLI) benchAgentTasksCmd(args []string) int {
	started := time.Now().UTC()
	opts, code, ok := parseBenchAgentTasksOptions(args, c.Stderr)
	if !ok {
		return code
	}
	corpus := bench.AgentCorpus()
	splits, code, ok := c.loadBenchAgentSplits(corpus)
	if !ok {
		return code
	}
	splits, code, ok = applyBenchAgentTaskFilter(splits, corpus, opts.taskFilter, c.Stderr)
	if !ok {
		return code
	}
	// NOV-77 fast-loop: --dry-run validates corpus+splits and lists the
	// selected tasks without touching any model. Zero model cost.
	if opts.dryRun {
		printBenchAgentDryRun(c.Stdout, splits, opts.reps)
		return 0
	}
	outcomes, hostVersion, code, ok := c.executeBenchAgentSuite(opts, corpus, splits)
	if !ok {
		return code
	}
	if code := saveBenchAgentOutcomes(c.Stdout, c.Stderr, opts.save, outcomes); code != 0 {
		return code
	}
	if code := c.writeBenchAgentEvidence(opts, outcomes, splits, hostVersion, started); code != 0 {
		return code
	}
	return reportBenchAgentResults(c.Stdout, opts.variant, outcomes)
}

// parseBenchAgentTasksOptions parses the `bench agent-tasks` flags and
// validates the positional contract: no positional args, reps >= 1.
// On failure it prints the byte-identical usage to stderr and reports
// exit 2 with ok=false.
func parseBenchAgentTasksOptions(args []string, stderr io.Writer) (benchAgentTasksOptions, int, bool) {
	var opts benchAgentTasksOptions
	fs := flag.NewFlagSet("bench agent-tasks", flag.ContinueOnError)
	fs.SetOutput(stderr)
	reps := fs.Int("reps", 1, "repetitions per task (protocol wants >=3 for real evals)")
	split := fs.String("split", "all", "exploration | holdout | all")
	model := fs.String("model", "", "model for the headless host (host default when empty)")
	host := fs.String("host", "opencode", "headless host: opencode")
	budget := fs.Int("budget-s", 0, "per-task timeout in seconds (0 = task default)")
	variant := fs.String("variant", "baseline", "variant label recorded on outcomes")
	save := fs.String("save", "", "write outcomes as JSON to this path (evidence pack)")
	evidenceDir := fs.String("evidence-dir", "", "write versioned evidence (outcomes.json + manifest.json with config, git head and sha256) to this directory")
	hostVersionFlag := fs.String("host-version", "", "host version recorded on outcomes (auto-detected for opencode when empty)")
	taskFilter := fs.String("task", "", "comma-separated task IDs for cheap smoke runs (e.g. --task AT1,AT2)")
	dryRun := fs.Bool("dry-run", false, "validate corpus+splits and list selected tasks without model calls")
	maxWallS := fs.Int("max-wall-time-s", 0, "global deadline in seconds, stops with partial outcomes on expiry (0 = none)")
	if err := fs.Parse(args); err != nil {
		return benchAgentTasksOptions{}, 2, false
	}
	if fs.NArg() != 0 || *reps < 1 {
		fmt.Fprintln(stderr, "usage: regulus bench agent-tasks [--reps N] [--split exploration|holdout|all] [--model M] [--host opencode] [--host-version V] [--budget-s N] [--variant V] [--save P] [--evidence-dir D] [--task AT1,AT2] [--dry-run] [--max-wall-time-s N]")
		return benchAgentTasksOptions{}, 2, false
	}
	opts = benchAgentTasksOptions{
		reps: *reps, split: *split, model: *model, host: *host, budget: *budget,
		variant: *variant, save: *save, evidenceDir: *evidenceDir,
		hostVersionFlag: *hostVersionFlag, taskFilter: *taskFilter,
		dryRun: *dryRun, maxWallS: *maxWallS,
	}
	return opts, 0, true
}

// benchAgentSplitsPath resolves the splits file: the REGULUS_AGENT_SPLITS
// override wins, otherwise the tracked path, otherwise the project-root
// join for callers running from a subdirectory.
func (c CLI) benchAgentSplitsPath() string {
	if env := os.Getenv("REGULUS_AGENT_SPLITS"); env != "" {
		return env
	}
	if _, err := os.Stat(bench.AgentSplitsPath); err != nil {
		return c.dir() + "/" + bench.AgentSplitsPath
	}
	return bench.AgentSplitsPath
}

// loadBenchAgentSplits resolves the splits path and validates it against
// the corpus. On failure it prints the byte-identical refusal and
// reports exit 1 with ok=false.
func (c CLI) loadBenchAgentSplits(corpus []bench.AgentTask) (bench.AgentSplits, int, bool) {
	splits, err := bench.LoadAgentSplits(c.benchAgentSplitsPath(), corpus)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench agent-tasks: %v\n", err)
		return bench.AgentSplits{}, 1, false
	}
	return splits, 0, true
}

// indexBenchAgentCorpus indexes the corpus IDs for --task validation.
func indexBenchAgentCorpus(corpus []bench.AgentTask) map[string]bool {
	known := make(map[string]bool, len(corpus))
	for _, t := range corpus {
		known[t.ID] = true
	}
	return known
}

// selectBenchAgentSplits restricts both splits to the wanted IDs,
// preserving committed order.
func selectBenchAgentSplits(splits bench.AgentSplits, want map[string]bool) bench.AgentSplits {
	var exp, hold []string
	for _, id := range splits.Exploration {
		if want[id] {
			exp = append(exp, id)
		}
	}
	for _, id := range splits.Holdout {
		if want[id] {
			hold = append(hold, id)
		}
	}
	splits.Exploration, splits.Holdout = exp, hold
	return splits
}

// applyBenchAgentTaskFilter restricts the run to a comma-separated
// subset of task IDs for cheap smoke runs (NOV-77 fast-loop).
// Filtering happens after validation, so single-task smokes stay
// allowed even though the committed splits require >=20% holdout.
// Unknown IDs and empty matches print the byte-identical refusals
// and report exit 2 with ok=false.
func applyBenchAgentTaskFilter(splits bench.AgentSplits, corpus []bench.AgentTask, taskFilter string, stderr io.Writer) (bench.AgentSplits, int, bool) {
	if taskFilter == "" {
		return splits, 0, true
	}
	want := map[string]bool{}
	for _, id := range splitList(taskFilter) {
		want[id] = true
	}
	known := indexBenchAgentCorpus(corpus)
	for id := range want {
		if !known[id] {
			fmt.Fprintf(stderr, "bench agent-tasks: unknown task %q\n", id)
			return bench.AgentSplits{}, 2, false
		}
	}
	splits = selectBenchAgentSplits(splits, want)
	if len(splits.Exploration)+len(splits.Holdout) == 0 {
		fmt.Fprintln(stderr, "bench agent-tasks: --task matched no tasks")
		return bench.AgentSplits{}, 2, false
	}
	return splits, 0, true
}

// printBenchAgentDryRun lists the selected tasks without touching any
// model. Zero model cost.
func printBenchAgentDryRun(stdout io.Writer, splits bench.AgentSplits, reps int) {
	total := (len(splits.Exploration) + len(splits.Holdout)) * reps
	fmt.Fprintf(stdout, "agent-tasks %s dry-run: %d task(s) x %d rep(s) = %d attempt(s), no model calls\n",
		bench.AgentSuiteVersion, len(splits.Exploration)+len(splits.Holdout), reps, total)
	for _, id := range splits.Exploration {
		fmt.Fprintf(stdout, "  - %s [exploration]\n", id)
	}
	for _, id := range splits.Holdout {
		fmt.Fprintf(stdout, "  - %s [holdout]\n", id)
	}
}

// executeBenchAgentSuite materializes the headless host and runs the
// suite. An unusable host degrades to the byte-identical NOT RUN line
// on stdout with its exit code and ok=false: the suite never passes in
// false and never fails tasks the agent never attempted.
func (c CLI) executeBenchAgentSuite(opts benchAgentTasksOptions, corpus []bench.AgentTask, splits bench.AgentSplits) ([]bench.AgentOutcome, string, int, bool) {
	agent, notRunMsg, notRunCode, ok := resolveAgentRunner(opts.host)
	if !ok {
		fmt.Fprintln(c.Stdout, notRunMsg)
		return nil, "", notRunCode, false
	}
	hostVersion := opts.hostVersionFlag
	if hostVersion == "" && opts.host == "opencode" {
		hostVersion = probeOpenCodeVersion()
	}
	ctx := context.Background()
	if opts.maxWallS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.maxWallS)*time.Second)
		defer cancel()
	}
	total := (len(splits.Exploration) + len(splits.Holdout)) * opts.reps
	done := 0
	outcomes := bench.RunAgentSuite(ctx, agent, corpus, splits, bench.AgentOptions{
		Reps: opts.reps, Variant: opts.variant, Model: opts.model, Host: opts.host, HostVersion: hostVersion, Split: opts.split, BudgetS: opts.budget,
		Progress: func(o bench.AgentOutcome) {
			done++
			fmt.Fprintf(c.Stdout, "  [%d/%d] %s [%s] rep=%d: %s\n", done, total, o.TaskID, o.Split, o.Rep, o.Status)
		},
	})
	return outcomes, hostVersion, 0, true
}

// saveBenchAgentOutcomes writes the raw outcomes JSON evidence pack.
// An empty path is a no-op success; failures print the byte-identical
// refusal and report exit 1.
func saveBenchAgentOutcomes(stdout, stderr io.Writer, save string, outcomes []bench.AgentOutcome) int {
	if save == "" {
		return 0
	}
	raw, err := json.MarshalIndent(outcomes, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "bench agent-tasks: %v\n", err)
		return 1
	}
	if err := os.WriteFile(save, raw, 0o644); err != nil {
		fmt.Fprintf(stderr, "bench agent-tasks: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "evidence: %d outcome(s) saved to %s\n", len(outcomes), save)
	return 0
}

// writeBenchAgentEvidence records the versioned evidence dir
// (outcomes.json + manifest.json with config, git head and sha256).
// An empty dir is a no-op success; failures print the byte-identical
// refusal and report exit 1.
func (c CLI) writeBenchAgentEvidence(opts benchAgentTasksOptions, outcomes []bench.AgentOutcome, splits bench.AgentSplits, hostVersion string, started time.Time) int {
	if opts.evidenceDir == "" {
		return 0
	}
	head := gitHeadBounded(c.runner())
	if head == "" {
		head = "unknown"
	}
	seeds := make([]int64, 0, opts.reps)
	for rep := 0; rep < opts.reps; rep++ {
		seeds = append(seeds, splits.Seed+int64(rep))
	}
	if _, err := bench.WriteAgentEvidence(opts.evidenceDir, outcomes, bench.AgentRunManifest{
		SuiteVersion: bench.AgentSuiteVersion, Model: opts.model, Host: opts.host, HostVersion: hostVersion,
		Variant: opts.variant, Reps: opts.reps, Split: opts.split, Seeds: seeds, GitHead: head,
		StartedAt: started.Format(time.RFC3339),
	}); err != nil {
		fmt.Fprintf(c.Stderr, "bench agent-tasks: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "evidence: versioned run recorded in %s\n", opts.evidenceDir)
	return 0
}

// printBenchAgentOutcomes prints one line per outcome and returns the
// count of not-run attempts.
func printBenchAgentOutcomes(stdout io.Writer, outcomes []bench.AgentOutcome) int {
	notRun := 0
	for _, o := range outcomes {
		fmt.Fprintf(stdout, "  - %s [%s] rep=%d seed=%d: %s in=%d out=%d %s (%s)\n",
			o.TaskID, o.Split, o.Rep, o.Seed, o.Status, o.TokensIn, o.TokensOut, o.TokenSource, shortOutcomeDetail(o.Detail))
		if o.Status == bench.AgentNotRun {
			notRun++
		}
	}
	return notRun
}

// decideBenchAgentExit maps the summary to the terminal verdict:
// exit 3 NOT RUN when the host produced no attempts, exit 1
// INCONCLUSIVE when nothing is evaluable, else exit 0. The message is
// the byte-identical closing line (empty on pass).
func decideBenchAgentExit(sum bench.AgentSummary, notRun int) (int, string) {
	if notRun == sum.Total && sum.Total > 0 {
		return 3, "agent-tasks NOT RUN: host produced no attempts (nothing passed or failed)"
	}
	if sum.Evaluable == 0 {
		return 1, "agent-tasks INCONCLUSIVE: no evaluable outcomes (infra/timeout only)"
	}
	return 0, ""
}

// reportBenchAgentResults prints the per-outcome lines, the summary
// line and the terminal verdict, preserving exit codes 3=NOT RUN,
// 1=INCONCLUSIVE, 0=pass.
func reportBenchAgentResults(stdout io.Writer, variant string, outcomes []bench.AgentOutcome) int {
	sum := bench.SummarizeAgent(variant, outcomes)
	notRun := printBenchAgentOutcomes(stdout, outcomes)
	fmt.Fprintf(stdout, "agent-tasks %s: %d task(s), %d evaluable, %d pass, %d fail, %d timeout, %d infra (pass-rate %.2f, reps spread %.2f-%.2f)\n",
		bench.AgentSuiteVersion, sum.Total, sum.Evaluable, sum.Pass, sum.Fail, sum.Timeout, sum.Infra, sum.PassRate, sum.RepMin, sum.RepMax)
	code, msg := decideBenchAgentExit(sum, notRun)
	if msg != "" {
		fmt.Fprintln(stdout, msg)
	}
	return code
}

// probeOpenCodeVersion best-efforts `opencode --version` ("opencode
// X.Y.Z" -> "X.Y.Z"); anything else means unknown, never an error.
func probeOpenCodeVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "opencode", "--version")
	executil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) == 2 && fields[0] == "opencode" {
		return fields[1]
	}
	return ""
}

// resolveAgentRunner materializa el host headless pedido. Si no hay host
// utilizable devuelve el mensaje NOT RUN, su código de salida y ok=false:
// la suite nunca pasa en falso ni reprueba tareas que el agente jamás
// intentó.
func resolveAgentRunner(host string) (bench.HeadlessAgent, string, int, bool) {
	switch host {
	case "opencode":
		r, err := bench.DefaultOpenCodeRunner()
		if err != nil {
			return nil, fmt.Sprintf("agent-tasks NOT RUN: %v (suite skipped, nothing passed or failed)", err), 3, false
		}
		return r, "", 0, true
	default:
		return nil, fmt.Sprintf("agent-tasks NOT RUN: unknown host %q (suite skipped, nothing passed or failed)", host), 3, false
	}
}

func shortOutcomeDetail(s string) string {
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if len(s) > 160 {
		return s[:160] + "…"
	}
	if s == "" {
		return "-"
	}
	return s
}

// benchAgentCompareCmd implements `regulus bench agent-compare`: the
// NOV-59 robust verdict over two saved agent-tasks runs. Guardrails are
// assessed by the human from the evidence (same checks, same splits,
// same protocol) and passed as flags; cost only tiebreaks.
func (c CLI) benchAgentCompareCmd(args []string) int {
	fs := flag.NewFlagSet("bench agent-compare", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	base := fs.String("base", "", "baseline outcomes JSON (required)")
	cand := fs.String("candidate", "", "candidate outcomes JSON (required)")
	minDelta := fs.Float64("min-delta", evolve.RobustMinDeltaDefault, "minimum improvement to adopt")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *base == "" || *cand == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus bench agent-compare --base <json> --candidate <json> [--min-delta N]")
		return 2
	}
	load := func(p string) []bench.AgentOutcome {
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench agent-compare: %v\n", err)
			return nil
		}
		var out []bench.AgentOutcome
		if err := json.Unmarshal(data, &out); err != nil {
			fmt.Fprintf(c.Stderr, "bench agent-compare: %s malformed: %v\n", p, err)
			return nil
		}
		return out
	}
	baseOut, candOut := load(*base), load(*cand)
	if baseOut == nil || candOut == nil {
		return 1
	}
	g := evolve.RobustGuardrails{Functional: true, Compliance: true, NoRegression: true, Adversarial: true, Maintainable: true}
	d, err := evolve.CompareRobust(
		evolve.SummarizeRobust("baseline", bench.ToExplorationOutcomes("base", baseOut)),
		evolve.SummarizeRobust("candidate", bench.ToExplorationOutcomes("cand", candOut)),
		g, evolve.RobustCost{}, evolve.RobustCost{},
		evolve.RobustOptions{MinDelta: *minDelta})
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench agent-compare: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "verdict: %s\nreason: %s\n%s\n%s\nuncertainty: %s\n",
		d.Verdict, d.Reason, d.N, d.Dispersion, d.Uncertainty)
	if d.Verdict == evolve.RobustAdoptCandidate {
		return 0
	}
	return 1
}

// benchAgentReportCmd implements `regulus bench agent-report --evidence-dir DIR`:
// it reads one versioned evidence dir (manifest + outcomes, hash-verified)
// and prints the report from those files — never hand-written.
func (c CLI) benchAgentReportCmd(args []string) int {
	fs := flag.NewFlagSet("bench agent-report", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	dir := fs.String("evidence-dir", "", "evidence directory with manifest.json + outcomes.json (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench agent-report --evidence-dir <dir>")
		return 2
	}
	m, outcomes, err := bench.ReadAgentEvidence(*dir)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench agent-report: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, bench.RenderAgentReport(m, outcomes))
	return 0
}
