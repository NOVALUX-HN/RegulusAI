package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"regulus-ai/internal/bench"
)

// benchCmd implements `regulus bench ...`: the macizo self-benchmark
// runner (chart macizo-flow/F4). Offline by construction: the suite
// runs local hidden checks only (no network, no model, no tokens), so
// `bench run` works on a plane with no API key. Subcommands:
//
//	list                       the suite prompts (never the hidden answers)
//	run [--surface N] [--save P] [--json]   run the suite, print the table
//	baseline [--set] [--json]  show (or record) the tracked baseline
//	compare --base A --candidate B [--json]  contrast table + ADOPT/REJECT
//	tiers | tier-plan | tier-report  F11.3 escalonated L0-L3 selection over the agent corpus
//
// benchTopUsage is the top-level bench usage: every subcommand family
// stays listed so --help documents tiers alongside the suite.
func benchTopUsage() string {
	return "usage: regulus bench list [--json] | regulus bench run [--surface <name>] [--save <path>] [--json] | regulus bench baseline [--set] [--json] | regulus bench compare --base <path> --candidate <path> [--json] | regulus bench agent-tasks [--reps N] [--split exploration|holdout|all] [--model M] [--host opencode] [--host-version V] [--budget-s N] [--variant V] [--save P] [--evidence-dir D] | regulus bench agent-compare --base <json> --candidate <json> | regulus bench agent-report --evidence-dir <dir> | regulus bench mission [--split exploration|holdout|all] [--holdout] [--save P] [--json] [--dry-run] | regulus bench host-journeys [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin] | regulus bench host-capabilities [--run] [--model M] [--out DIR] [--host all|opencode|claude|codex|devin] | regulus bench lab agents|run|compare|flow|report [...] | regulus bench tiers [--json] | regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--json] | regulus bench tier-report --evidence-dir <dir> | regulus bench diag --run <id> [--baseline <id>] [--evidence-dir <dir>] [--thresholds <file>] [--json]"
}

func (c CLI) benchCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, benchTopUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, benchTopUsage())
		return 2
	}
	switch args[0] {
	case "list":
		return c.benchListCmd(args[1:])
	case "run":
		return c.benchRunCmd(args[1:])
	case "baseline":
		return c.benchBaselineCmd(args[1:])
	case "compare":
		return c.benchCompareCmd(args[1:])
	case "agent-tasks":
		return c.benchAgentTasksCmd(args[1:])
	case "agent-compare":
		return c.benchAgentCompareCmd(args[1:])
	case "agent-report":
		return c.benchAgentReportCmd(args[1:])
	case "mission":
		return c.benchMissionCmd(args[1:])
	case "lab":
		return c.benchLabCmd(args[1:])
	case "host-journeys":
		return c.benchHostJourneysCmd(args[1:])
	case "host-capabilities":
		return c.benchHostCapabilitiesCmd(args[1:])
	case "tiers":
		return c.benchTiersCmd(args[1:])
	case "tier-plan":
		return c.benchTierPlanCmd(args[1:])
	case "tier-report":
		return c.benchTierReportCmd(args[1:])
	case "diag":
		return c.benchDiagCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "bench: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, benchTopUsage())
		return 2
	}
}

// benchListCmd prints the suite prompts. The hidden checks never
// print: the worker sees tasks, the runner knows answers.
func (c CLI) benchListCmd(args []string) int {
	fs := flag.NewFlagSet("bench list", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the suite as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench list [--json]")
		return 2
	}
	tasks := bench.Suite()
	if *asJSON {
		type listed struct {
			ID, Title, Kind, Prompt string
		}
		out := make([]listed, 0, len(tasks))
		for _, t := range tasks {
			out = append(out, listed{t.ID, t.Title, t.Kind, t.Prompt})
		}
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench list: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "bench suite %s: %d task(s) (offline, hidden checks)\n", bench.SuiteVersion, len(tasks))
	for _, t := range tasks {
		fmt.Fprintf(c.Stdout, "%-4s %-28s [%s] %s\n", t.ID, t.Title, t.Kind, t.Prompt)
	}
	return 0
}

// benchSurface validates the --surface label: conservative charset,
// no traversal, so report files stay attributable and safe.
func benchSurface(s string) bool {
	if s == "" || len(s) > 64 || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-', r == '/':
		default:
			return false
		}
	}
	return true
}

// benchRunCmd runs the suite offline and prints the per-task table
// plus totals. --save writes the comparable report for later
// contrast; without it nothing is written.
// benchRunExitCode is the gate verdict: 0 when every task passed, 1 otherwise.
func benchRunExitCode(rep bench.Report) int {
	if rep.Failed > 0 || rep.Passed != len(rep.Tasks) {
		return 1
	}
	return 0
}

func (c CLI) benchRunCmd(args []string) int {
	fs := flag.NewFlagSet("bench run", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	surface := fs.String("surface", "tree", "surface label recorded in the report")
	save := fs.String("save", "", "write the comparable report JSON to this path")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || !benchSurface(*surface) {
		fmt.Fprintln(c.Stderr, "usage: regulus bench run [--surface <name>] [--save <path>] [--json]")
		return 2
	}
	if strings.HasPrefix(*save, "-") {
		fmt.Fprintln(c.Stderr, "usage: regulus bench run [--surface <name>] [--save <path>] [--json]")
		return 2
	}
	rep := bench.Run(*surface, gitHeadBounded(c.runner()))
	if *save != "" {
		if err := bench.Save(*save, rep); err != nil {
			fmt.Fprintf(c.Stderr, "bench run: %v\n", err)
			return 1
		}
	}
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench run: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return benchRunExitCode(rep)
	}
	for _, t := range rep.Tasks {
		mark := "PASS"
		if !t.Pass {
			mark = "FAIL"
		}
		fmt.Fprintf(c.Stdout, "%-4s %-28s %s (%dms, proxy=%d) %s\n", t.ID, t.Title, mark, t.DurationMS, t.ProxyTokens, t.Detail)
	}
	fmt.Fprintf(c.Stdout, "bench: quality %d/%d passed on surface %q\n", rep.Passed, len(rep.Tasks), rep.Surface)
	fmt.Fprintf(c.Stdout, "bench: cost proxy=%d tokens (measured-local chars/4, host tokens unknown) wall=%dms detail=%dB\n", rep.TotalProxyTokens, rep.TotalMS, rep.OutputBytes)
	fmt.Fprintf(c.Stdout, "bench: quality per kproxy=%.2f passes/kproxy (informational, never gates ADOPT)\n", rep.QualityPerKProxy)
	if *save != "" {
		fmt.Fprintf(c.Stdout, "bench: report saved to %s\n", *save)
	}
	return benchRunExitCode(rep)
}

// benchBaselineCmd shows the tracked baseline, or records it with
// --set (runs the suite against the current tree and writes
// bench/baseline.json).
func (c CLI) benchBaselineCmd(args []string) int {
	fs := flag.NewFlagSet("bench baseline", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	set := fs.Bool("set", false, "record the current tree as the baseline")
	asJSON := fs.Bool("json", false, "print the baseline report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus bench baseline [--set] [--json]")
		return 2
	}
	if *set {
		rep := bench.Run("tree", gitHeadBounded(c.runner()))
		if err := bench.Save(bench.BaselinePath, rep); err != nil {
			fmt.Fprintf(c.Stderr, "bench baseline: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "bench: baseline recorded to %s (%d/%d passed, proxy=%d, q/kproxy=%.2f)\n", bench.BaselinePath, rep.Passed, len(rep.Tasks), rep.TotalProxyTokens, rep.QualityPerKProxy)
		return 0
	}
	rep, err := bench.Load(bench.BaselinePath)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench baseline: %v (record one with `regulus bench baseline --set`)\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench baseline: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "bench baseline %s surface %q head=%s: quality %d/%d passed cost proxy=%d (q/kproxy=%.2f, host unknown, %dms wall)\n",
		rep.Suite, rep.Surface, shortBenchHead(rep.GitHead), rep.Passed, len(rep.Tasks), rep.TotalProxyTokens, rep.QualityPerKProxy, rep.TotalMS)
	for _, t := range rep.Tasks {
		mark := "PASS"
		if !t.Pass {
			mark = "FAIL"
		}
		fmt.Fprintf(c.Stdout, "  %-4s %-28s %s (%dms)\n", t.ID, t.Title, mark, t.DurationMS)
	}
	return 0
}

// shortBenchHead renders the baseline head for the report: 8 hex
// chars, or "nogit" outside a repository.
func shortBenchHead(head string) string {
	if strings.TrimSpace(head) == "" {
		return "nogit"
	}
	h := strings.TrimSpace(head)
	if len(h) > 8 {
		h = h[:8]
	}
	return h
}

// benchCompareCmd contrasts two reports task by task and prints the
// machine verdict: ADOPT (exit 0) or REJECT (exit 1). Durations are
// shown, never judged.
func (c CLI) benchCompareCmd(args []string) int {
	fs := flag.NewFlagSet("bench compare", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	base := fs.String("base", "", "baseline report path (required)")
	cand := fs.String("candidate", "", "candidate report path (required)")
	asJSON := fs.Bool("json", false, "print the comparison as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *base == "" || *cand == "" || fs.NArg() != 0 || strings.HasPrefix(*base, "-") || strings.HasPrefix(*cand, "-") {
		fmt.Fprintln(c.Stderr, "usage: regulus bench compare --base <path> --candidate <path> [--json]")
		return 2
	}
	baseRep, err := bench.Load(*base)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench compare: %v\n", err)
		return 1
	}
	candRep, err := bench.Load(*cand)
	if err != nil {
		fmt.Fprintf(c.Stderr, "bench compare: %v\n", err)
		return 1
	}
	cmp := bench.Compare(baseRep, candRep)
	if *asJSON {
		raw, err := json.MarshalIndent(cmp, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "bench compare: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return benchCompareCode(cmp)
	}
	fmt.Fprintf(c.Stdout, "bench contrast: base %q (%d/%d) vs candidate %q (%d/%d)\n",
		baseRep.Surface, baseRep.Passed, len(baseRep.Tasks),
		candRep.Surface, candRep.Passed, len(candRep.Tasks))
	for _, r := range cmp.Rows {
		fmt.Fprintf(c.Stdout, "  %-4s %-28s base=%s cand=%s delta=%+dms proxydelta=%+d\n",
			r.ID, r.Title, benchPassWord(r.Base), benchPassWord(r.Cand), r.DeltaMS, r.DeltaProxy)
	}
	fmt.Fprintf(c.Stdout, "bench cost: base proxy=%d vs candidate proxy=%d (delta=%+d, reported only, host tokens unknown)\n", cmp.BaseProxy, cmp.CandProxy, cmp.DeltaProxy)
	fmt.Fprintf(c.Stdout, "bench quality per kproxy: base=%.2f vs candidate=%.2f\n", baseRep.QualityPerKProxy, candRep.QualityPerKProxy)
	fmt.Fprintf(c.Stdout, "bench verdict: %s — %s\n", cmp.Verdict, cmp.Reason)
	return benchCompareCode(cmp)
}

// benchPassWord renders one side of the contrast table.
func benchPassWord(pass bool) string {
	if pass {
		return "PASS"
	}
	return "FAIL"
}

// benchCompareCode maps the machine verdict to the process exit:
// ADOPT proceeds (0), REJECT stops (1) — the contrast decides, not
// the reader's judgment.
func benchCompareCode(cmp bench.Comparison) int {
	if cmp.Verdict == "ADOPT" {
		return 0
	}
	return 1
}
