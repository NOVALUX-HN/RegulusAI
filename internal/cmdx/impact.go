// Impact commands (F12.8/NOV-103): `regulus impact ...` over the
// repository dependency graph (internal/impact).
//
// Handlers take the minimal IO instead of cli.CLI so cli keeps one
// edge (cli->cmdx); behavior, flags, messages and exit codes are
// defined here, byte-identical from the first commit. Every analysis
// links its F11.1 exec trace (run.start + task.end + run.end,
// best-effort: a logging failure warns on stderr and never alters
// the outcome).
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/impact"
	"regulus-ai/internal/observability"
)

func impactUsage() string {
	return "usage: regulus impact inventory|graph|affect|suite|cycles|benchmark|breaking\n" +
		"  regulus impact inventory [--json]\n" +
		"  regulus impact graph [--json]\n" +
		"  regulus impact affect --changed <a,b> [--severity N] [--json]\n" +
		"  regulus impact suite --changed <a,b> [--severity N] [--full <t,...>] [--failed <t,...>] [--json]\n" +
		"  regulus impact cycles [--json]\n" +
		"  regulus impact breaking --before <inventory.json> [--json]\n" +
		"  regulus impact benchmark [--json]"
}

// ImpactCmd routes `regulus impact ...` to one handler per subcommand.
func (io IO) ImpactCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, impactUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
	switch args[0] {
	case "inventory":
		return io.ImpactInventoryCmd(args[1:])
	case "graph":
		return io.ImpactGraphCmd(args[1:])
	case "affect":
		return io.ImpactAffectCmd(args[1:])
	case "suite":
		return io.ImpactSuiteCmd(args[1:])
	case "cycles":
		return io.ImpactCyclesCmd(args[1:])
	case "breaking":
		return io.ImpactBreakingCmd(args[1:])
	case "benchmark":
		return io.ImpactBenchmarkCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "impact: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
}

// recordImpactTrace links one impact analysis to its F11.1 timeline.
// Best-effort: a logging failure warns and never alters the outcome.
func (io IO) recordImpactTrace(command, changed string) {
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact: sin traza F11.1 (%v)\n", err)
		return
	}
	emit := func(e observability.ExecEvent) {
		if _, _, rerr := observability.RecordExec(io.Dir, e, time.Now().UTC()); rerr != nil {
			fmt.Fprintf(io.Stderr, "impact: evento de traza no registrado (%v)\n", rerr)
		}
	}
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunStart, Status: observability.ExecStarted,
		Payload: map[string]any{"command": command, "changed": changed},
	})
	emit(observability.ExecEvent{
		RunID: runID, TaskID: "impact/" + command,
		Type: observability.ExecTaskEnd, Status: observability.ExecSucceeded,
		Payload:      map[string]any{"command": command},
		ErrorDetails: "impact analysis recorded (predicts, never mutates)",
	})
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunEnd, Status: observability.ExecSucceeded,
		Payload: map[string]any{"command": command},
	})
}

// ImpactInventoryCmd implements `regulus impact inventory`.
func (io IO) ImpactInventoryCmd(args []string) int {
	fs := flag.NewFlagSet("impact inventory", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the census as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
	inv, err := impact.Census(io.Dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact inventory: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(inv, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact inventory: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	kinds := map[string]int{}
	for _, n := range inv.Nodes {
		kinds[n.Kind]++
	}
	fmt.Fprintf(io.Stdout, "inventory: %d node(s)", len(inv.Nodes))
	for _, k := range []string{impact.KindModule, impact.KindPackage, impact.KindAPI, impact.KindConfig, impact.KindWorkflow, impact.KindDependency, impact.KindTest, impact.KindCommand} {
		if kinds[k] > 0 {
			fmt.Fprintf(io.Stdout, " %s=%d", k, kinds[k])
		}
	}
	fmt.Fprintln(io.Stdout)
	for _, w := range inv.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w)
	}
	io.recordImpactTrace("impact inventory", "")
	return 0
}

// ImpactGraphCmd implements `regulus impact graph`.
func (io IO) ImpactGraphCmd(args []string) int {
	fs := flag.NewFlagSet("impact graph", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the graph as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact graph: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(g, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact graph: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "graph: %d node(s), %d edge(s) (head %s)\n", len(g.Nodes), len(g.Edges), shortHead(g.Head))
	for _, e := range g.Edges {
		fmt.Fprintf(io.Stdout, "%s -> %s [%s] %s %s %s\n", e.From, e.To, e.Source, e.Evidence(), shortCommit(e.Commit), e.Date)
	}
	for _, w := range g.Warnings {
		fmt.Fprintf(io.Stderr, "warning: %s\n", w)
	}
	io.recordImpactTrace("impact graph", "")
	return 0
}

// impactChangedFlags carries the shared --changed/--severity flags.
type impactChangedFlags struct {
	changed  string
	severity float64
	asJSON   bool
}

func (io IO) parseImpactChanged(fsName, usage string, args []string) (impactChangedFlags, *flag.FlagSet, bool) {
	var f impactChangedFlags
	fs := flag.NewFlagSet(fsName, flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	changed := fs.String("changed", "", "changed paths, comma-separated (required)")
	severity := fs.Float64("severity", 0, "observed F12.4 severity 0..1 (alert/failure rate)")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return f, fs, false
	}
	if fs.NArg() != 0 || strings.TrimSpace(*changed) == "" {
		fmt.Fprintln(io.Stderr, usage)
		return f, fs, false
	}
	if *severity < 0 || *severity > 1 {
		fmt.Fprintln(io.Stderr, "impact: --severity must be 0..1 (observed F12.4 rate)")
		return f, fs, false
	}
	return impactChangedFlags{changed: *changed, severity: *severity, asJSON: *asJSON}, fs, true
}

// ImpactAffectCmd implements `regulus impact affect`.
func (io IO) ImpactAffectCmd(args []string) int {
	f, _, ok := io.parseImpactChanged("impact affect",
		"usage: regulus impact affect --changed <a,b> [--severity N] [--json]", args)
	if !ok {
		return 2
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact affect: %v\n", err)
		return 1
	}
	changed := splitList(f.changed)
	imp := impact.MapDiff(g, changed)
	risk := impact.AssessRisk(imp, f.severity)
	if f.asJSON {
		raw, err := json.MarshalIndent(struct {
			Impact impact.Impact `json:"impact"`
			Risk   impact.Risk   `json:"risk"`
		}{imp, risk}, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact affect: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "affect: %d changed, %d direct, %d consumers, %d critical, %d tests\n",
		len(imp.Changed), len(imp.Direct), len(imp.Consumers), len(imp.Critical), len(imp.Tests))
	if len(imp.Direct) > 0 {
		fmt.Fprintf(io.Stdout, "direct: %s\n", strings.Join(imp.Direct, ", "))
	}
	if len(imp.Consumers) > 0 {
		fmt.Fprintf(io.Stdout, "consumers: %s\n", strings.Join(imp.Consumers, ", "))
	}
	if len(imp.Critical) > 0 {
		fmt.Fprintf(io.Stdout, "critical: %s\n", strings.Join(imp.Critical, ", "))
	}
	for _, p := range imp.Paths {
		fmt.Fprintf(io.Stdout, "critical-path: %s\n", p)
	}
	if len(imp.Tests) > 0 {
		fmt.Fprintf(io.Stdout, "tests: %s\n", strings.Join(imp.Tests, ", "))
	}
	fmt.Fprintf(io.Stdout, "risk: %.1f (%s)\n", risk.Score, risk.Level)
	for _, r := range risk.Reasons {
		fmt.Fprintf(io.Stdout, "  - %s\n", r)
	}
	for _, r := range imp.Reasons {
		fmt.Fprintf(io.Stdout, "  - %s\n", r)
	}
	if imp.Widened {
		fmt.Fprintln(io.Stderr, "warning: impacto ampliado (entrada sin nodo verificado): la selección usa la suite completa")
	}
	io.recordImpactTrace("impact affect", f.changed)
	return 0
}

// ImpactSuiteCmd implements `regulus impact suite`.
func (io IO) ImpactSuiteCmd(args []string) int {
	fs := flag.NewFlagSet("impact suite", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	changed := fs.String("changed", "", "changed paths, comma-separated (required)")
	severity := fs.Float64("severity", 0, "observed F12.4 severity 0..1 (alert/failure rate)")
	full := fs.String("full", "", "full suite test ids, comma-separated (default: every test bundle)")
	failed := fs.String("failed", "", "actually failing test ids, comma-separated (default: full, for recall)")
	asJSON := fs.Bool("json", false, "print the selection as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*changed) == "" {
		fmt.Fprintln(io.Stderr, "usage: regulus impact suite --changed <a,b> [--severity N] [--full <t,...>] [--failed <t,...>] [--json]")
		return 2
	}
	if *severity < 0 || *severity > 1 {
		fmt.Fprintln(io.Stderr, "impact: --severity must be 0..1 (observed F12.4 rate)")
		return 2
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact suite: %v\n", err)
		return 1
	}
	imp := impact.MapDiff(g, splitList(*changed))
	var fullList []string
	if strings.TrimSpace(*full) == "" {
		fullList = nil
	} else {
		fullList = splitList(*full)
	}
	sel := impact.SelectSuite(g, imp, fullList)
	actual := splitList(*failed)
	if len(actual) == 0 {
		actual = sel.Full
	}
	sel.Metrics = impact.MeasureSelection(sel.Focused, actual, sel.Full)
	if *asJSON {
		raw, err := json.MarshalIndent(sel, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact suite: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "suite: %d focused of %d (tier %s)", len(sel.Focused), len(sel.Full), sel.Tier)
	if sel.Widened {
		fmt.Fprint(io.Stdout, " [ampliada]")
	}
	fmt.Fprintln(io.Stdout)
	if len(sel.Focused) > 0 {
		fmt.Fprintf(io.Stdout, "focused: %s\n", strings.Join(sel.Focused, ", "))
	}
	if len(sel.Omitted) > 0 {
		fmt.Fprintf(io.Stdout, "omitted: %s (%d sin camino verificado desde el diff)\n", strings.Join(sel.Omitted, ", "), len(sel.Omitted))
	}
	fmt.Fprintf(io.Stdout, "precision: %.2f recall: %.2f f1: %.2f (hit %d/%d, actual %d)\n",
		sel.Metrics.Precision, sel.Metrics.Recall, sel.Metrics.F1, sel.Metrics.Hit, sel.Metrics.Predicted, sel.Metrics.Actual)
	for _, r := range sel.Reasons {
		fmt.Fprintf(io.Stdout, "  - %s\n", r)
	}
	if sel.Widened {
		fmt.Fprintln(io.Stderr, "warning: suite ampliada a completa (incertidumbre): ningún test crítico se omite por inferencia débil")
	}
	io.recordImpactTrace("impact suite", *changed)
	return 0
}

// ImpactCyclesCmd implements `regulus impact cycles`.
func (io IO) ImpactCyclesCmd(args []string) int {
	fs := flag.NewFlagSet("impact cycles", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print cycles as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact cycles: %v\n", err)
		return 1
	}
	cycles := impact.DetectCycles(g)
	hidden := impact.HiddenDeps(g)
	if *asJSON {
		raw, err := json.MarshalIndent(struct {
			Cycles [][]string    `json:"cycles"`
			Hidden []impact.Edge `json:"hidden_deps"`
		}{cycles, hidden}, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact cycles: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	if len(cycles) == 0 {
		fmt.Fprintln(io.Stdout, "cycles: none")
	} else {
		fmt.Fprintf(io.Stdout, "cycles: %d\n", len(cycles))
		for _, c := range cycles {
			fmt.Fprintf(io.Stdout, "  - %s\n", strings.Join(c, " -> "))
		}
	}
	fmt.Fprintf(io.Stdout, "hidden: %d dep(s) no import declares (cli-route/workflow-ref/config-ref)\n", len(hidden))
	for _, e := range hidden {
		fmt.Fprintf(io.Stdout, "  - %s -> %s [%s] %s\n", e.From, e.To, e.Source, e.Evidence())
	}
	io.recordImpactTrace("impact cycles", "")
	return 0
}

// ImpactBreakingCmd implements `regulus impact breaking`.
func (io IO) ImpactBreakingCmd(args []string) int {
	fs := flag.NewFlagSet("impact breaking", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	before := fs.String("before", "", "prior `impact inventory --json` file (required)")
	asJSON := fs.Bool("json", false, "print breaking changes as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(*before) == "" {
		fmt.Fprintln(io.Stderr, "usage: regulus impact breaking --before <inventory.json> [--json]")
		return 2
	}
	raw, err := os.ReadFile(*before)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact breaking: %v\n", err)
		return 1
	}
	var prev impact.Inventory
	if err := json.Unmarshal(raw, &prev); err != nil {
		fmt.Fprintf(io.Stderr, "impact breaking: malformed inventory %q (%v)\n", *before, err)
		return 1
	}
	g, err := impact.BuildGraph(io.Dir, nil, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact breaking: %v\n", err)
		return 1
	}
	changes := impact.BreakingChanges(prev, g)
	if *asJSON {
		out, err := json.MarshalIndent(changes, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact breaking: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(out))
		return 0
	}
	if len(changes) == 0 {
		fmt.Fprintln(io.Stdout, "breaking: none (no package or exported API removed)")
	} else {
		fmt.Fprintf(io.Stdout, "breaking: %d incompatible change(s)\n", len(changes))
		for _, c := range changes {
			fmt.Fprintf(io.Stdout, "  - %s\n", c)
		}
	}
	io.recordImpactTrace("impact breaking", *before)
	return 0
}

// ImpactBenchmarkCmd implements `regulus impact benchmark`.
func (io IO) ImpactBenchmarkCmd(args []string) int {
	fs := flag.NewFlagSet("impact benchmark", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the benchmark as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, impactUsage())
		return 2
	}
	rep, err := impact.RunBenchmark(io.Dir, nil)
	if err != nil {
		fmt.Fprintf(io.Stderr, "impact benchmark: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "impact benchmark: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "benchmark %s: grafo %d ms (%d nodos, %d aristas) vs scan ingenuo %d ms\n",
		rep.Version, rep.GraphBuildMs, rep.Nodes, rep.Edges, rep.NaiveScanMs)
	fmt.Fprintf(io.Stdout, "codegraph: %s\n", rep.CodeGraph)
	fmt.Fprintf(io.Stdout, "decisión: %s\n", rep.Decision)
	for _, n := range rep.Notes {
		fmt.Fprintf(io.Stdout, "  - %s\n", n)
	}
	io.recordImpactTrace("impact benchmark", "")
	return 0
}

// shortHead trims a HEAD sha for one-line output.
func shortHead(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	if h == "" {
		return "worktree"
	}
	return h
}

// shortCommit trims an edge commit for one-line output.
func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}
