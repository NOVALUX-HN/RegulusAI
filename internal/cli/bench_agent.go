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
	"os"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/evolve"
)

func (c CLI) benchAgentTasksCmd(args []string) int {
	fs := flag.NewFlagSet("bench agent-tasks", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	reps := fs.Int("reps", 1, "repetitions per task (protocol wants >=3 for real evals)")
	split := fs.String("split", "all", "exploration | holdout | all")
	model := fs.String("model", "", "model for the headless host (host default when empty)")
	host := fs.String("host", "opencode", "headless host: opencode")
	budget := fs.Int("budget-s", 0, "per-task timeout in seconds (0 = task default)")
	variant := fs.String("variant", "baseline", "variant label recorded on outcomes")
	save := fs.String("save", "", "write outcomes as JSON to this path (evidence pack)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *reps < 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench agent-tasks [--reps N] [--split exploration|holdout|all] [--model M] [--host opencode] [--budget-s N] [--variant V]")
		return 2
	}
	corpus := bench.AgentCorpus()
	var agent bench.HeadlessAgent
	switch *host {
	case "opencode":
		r, err := bench.DefaultOpenCodeRunner()
		if err != nil {
			fmt.Fprintf(c.Stdout, "agent-tasks NOT RUN: %v (suite skipped, nothing passed or failed)\n", err)
			return 3
		}
		agent = r
	default:
		fmt.Fprintf(c.Stdout, "agent-tasks NOT RUN: unknown host %q (suite skipped, nothing passed or failed)\n", *host)
		return 3
	}
	splitsPath := bench.AgentSplitsPath
	if env := os.Getenv("REGULUS_AGENT_SPLITS"); env != "" {
		splitsPath = env
	} else if _, err := os.Stat(splitsPath); err != nil {
		splitsPath = c.dir() + "/" + bench.AgentSplitsPath
	}
	splits, err := bench.LoadAgentSplits(splitsPath, corpus)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench agent-tasks: %v\n", err)
		return 1
	}
	ctx := context.Background()
	outcomes := bench.RunAgentSuite(ctx, agent, corpus, splits, bench.AgentOptions{
		Reps: *reps, Variant: *variant, Model: *model, Host: *host, Split: *split, BudgetS: *budget,
	})
	if *save != "" {
		raw, err := json.MarshalIndent(outcomes, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench agent-tasks: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*save, raw, 0o644); err != nil {
			fmt.Fprintf(c.Stderr, "bench agent-tasks: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "evidence: %d outcome(s) saved to %s\n", len(outcomes), *save)
	}
	sum := bench.SummarizeAgent(*variant, outcomes)
	notRun := 0
	for _, o := range outcomes {
		fmt.Fprintf(c.Stdout, "  - %s [%s] rep=%d seed=%d: %s in=%d out=%d %s (%s)\n",
			o.TaskID, o.Split, o.Rep, o.Seed, o.Status, o.TokensIn, o.TokensOut, o.TokenSource, shortOutcomeDetail(o.Detail))
		if o.Status == bench.AgentNotRun {
			notRun++
		}
	}
	fmt.Fprintf(c.Stdout, "agent-tasks %s: %d task(s), %d evaluable, %d pass, %d fail, %d timeout, %d infra (pass-rate %.2f, reps spread %.2f-%.2f)\n",
		bench.AgentSuiteVersion, sum.Total, sum.Evaluable, sum.Pass, sum.Fail, sum.Timeout, sum.Infra, sum.PassRate, sum.RepMin, sum.RepMax)
	if notRun == sum.Total && sum.Total > 0 {
		fmt.Fprintln(c.Stdout, "agent-tasks NOT RUN: host produced no attempts (nothing passed or failed)")
		return 3
	}
	if sum.Evaluable == 0 {
		fmt.Fprintln(c.Stdout, "agent-tasks INCONCLUSIVE: no evaluable outcomes (infra/timeout only)")
		return 1
	}
	return 0
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
