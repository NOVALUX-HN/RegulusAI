// `regulus orchestrate ...` (F12.1/NOV-96): adaptive agent
// orchestration with explainable routing.
//
// Subcommands:
//
//	agents                                   the routing registry (agents, capabilities, evidence, limits)
//	decide --domain D --lang L ...           route one task descriptor (proposes, never executes)
//	evaluate --dynamic F --fixed F [...]     contrast dynamic vs fixed routing on observed results
//
// Registration rides init() into cliDispatch (this file owns the
// orchestrate surface; cli.go stays untouched). The decision is
// deterministic: same registry, matrix, history, descriptor, budgets
// and limits always print the same bytes (the trace hint is
// recording metadata, fixed on dry-run). Dry-run computes and
// prints, and records nothing.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/trace"
)

func init() {
	cliDispatch["orchestrate"] = func(c CLI, args []string) int { return c.orchestrateCmd(args) }
}

func orchestrateUsage() string {
	return "usage: regulus orchestrate agents [--json] | regulus orchestrate decide --domain <d> --lang <l> --complexity <low|medium|high> --risk <low|medium|high|critical> [--cap <a,b>] [--constraint <a,b>] [--output <a,b>] [--criteria <a,b>] [--history <file>] [--rates <file>] [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] [--max-delegations N] [--unavailable <a,b>] [--fixed <agent>] [--dry-run] [--out <dir>] [--json] | regulus orchestrate evaluate --dynamic <file> --fixed <file> [--rates <file>] [--json] | regulus orchestrate parallel --tasks <file> [--dry-run] [--json] | regulus orchestrate orphans [--prune] [--json]"
}

func (c CLI) orchestrateCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, orchestrateUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, orchestrateUsage())
		return 2
	}
	switch args[0] {
	case "agents":
		return c.orchestrateAgentsCmd(args[1:])
	case "decide":
		return c.orchestrateDecideCmd(args[1:])
	case "evaluate":
		return c.orchestrateEvaluateCmd(args[1:])
	case "parallel":
		return c.orchestrateParallelCmd(args[1:])
	case "orphans":
		return c.orchestrateOrphansCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "orchestrate: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, orchestrateUsage())
		return 2
	}
}

// orchestrateAgentsCmd implements `regulus orchestrate agents`: the
// routing inventory — every routable agent with its Lab identity,
// host, verified capabilities with evidence and limits, solo risk
// ceiling, scopes and estimates.
func (c CLI) orchestrateAgentsCmd(args []string) int {
	fs := flag.NewFlagSet("orchestrate agents", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the routing registry as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus orchestrate agents [--json]")
		return 2
	}
	reg := orchestrate.RegistryFromManifests(bench.DefaultHostManifests(bench.EvidenceStamp()))
	if *asJSON {
		raw, err := json.MarshalIndent(reg, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "orchestrate agents: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "routing %s: %d agent(s)\n", orchestrate.RoutingVersion, len(reg))
	for _, a := range reg {
		fmt.Fprintf(c.Stdout, "- %s: %s\n  host %s solo-hasta %s tools [%s] scopes [%s]\n",
			a.ID, a.Responsibility, a.Host, a.MaxRisk,
			strings.Join(a.Tools, ", "), strings.Join(a.Scopes, ", "))
		for _, cap := range a.Capabilities {
			ev := strings.ReplaceAll(cap.Evidence, "\n", " ")
			lim := strings.ReplaceAll(cap.Limit, "\n", " ")
			fmt.Fprintf(c.Stdout, "  cap %s: %s | límite: %s\n", cap.ID, ev, lim)
		}
	}
	return 0
}

// orchestrateDecideOptions carries the parsed `decide` flags.
type orchestrateDecideOptions struct {
	domain         string
	lang           string
	complexity     string
	risk           string
	caps           string
	constraints    string
	outputs        string
	criteria       string
	history        string
	rates          string
	budgetTimeMS   int64
	budgetTokens   int64
	budgetCost     float64
	maxDelegations int
	unavailable    string
	fixed          string
	dryRun         bool
	out            string
	asJSON         bool
}

// orchestrateDecideCmd implements `regulus orchestrate decide`: route
// one task descriptor. It proposes — it never executes an agent.
// Exit 0 routes (primary picked, pair when critical), 1 refuses
// (nothing eligible: the exclusions name every gate), 2 is a usage
// error.
func (c CLI) orchestrateDecideCmd(args []string) int {
	opts, code, ok := parseOrchestrateDecideFlags(args, c.Stderr)
	if !ok {
		return code
	}
	rates, code, ok := loadOrchestrateRates(opts.rates, c.Stderr)
	if !ok {
		return code
	}
	hist, code, ok := loadOrchestrateHistory(opts.history, rates, c.Stderr)
	if !ok {
		return code
	}
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	if opts.unavailable != "" {
		down := map[string]bool{}
		for _, id := range splitList(opts.unavailable) {
			down[id] = true
		}
		for i := range reg {
			if down[reg[i].ID] {
				reg[i].Available = false
			}
		}
	}
	desc := orchestrate.TaskDescriptor{
		Domain: opts.domain, Language: opts.lang,
		Complexity: opts.complexity, Risk: opts.risk,
		Constraints: splitList(opts.constraints), Outputs: splitList(opts.outputs),
		Criteria:             splitList(opts.criteria),
		RequiredCapabilities: splitList(opts.caps),
	}
	dec, err := orchestrate.Decide(reg, manifests, hist, desc,
		orchestrate.Budgets{TimeMS: opts.budgetTimeMS, Tokens: opts.budgetTokens, Cost: opts.budgetCost},
		opts.maxDelegations, opts.fixed, orchestrate.DefaultScoreWeights())
	if err != nil {
		fmt.Fprintf(c.Stderr, "orchestrate decide: %v\n", err)
		return 2
	}
	dec.DryRun = opts.dryRun
	if !opts.dryRun {
		dec.TraceHint = c.recordOrchestrateTrace("orchestrate decide", desc.Domain+"/"+desc.Language)
		if opts.out != "" {
			if code := writeOrchestrateDecision(opts.out, dec, c.Stderr); code != 0 {
				return code
			}
			fmt.Fprintf(c.Stdout, "orchestrate: decision recorded in %s\n", opts.out)
		}
	}
	if opts.asJSON {
		raw, err := json.MarshalIndent(dec, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "orchestrate decide: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
	} else {
		fmt.Fprint(c.Stdout, orchestrate.RenderDecision(dec))
	}
	if dec.Primary == "" {
		fmt.Fprintln(c.Stderr, "orchestrate decide: sin agente elegible (nada asignado sin capability verificada)")
		return 1
	}
	return 0
}

// parseOrchestrateDecideFlags parses the decide flags. Failures print
// the byte-identical usage to stderr and report exit 2.
func parseOrchestrateDecideFlags(args []string, stderr io.Writer) (orchestrateDecideOptions, int, bool) {
	var opts orchestrateDecideOptions
	fs := flag.NewFlagSet("orchestrate decide", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.domain, "domain", "", "task domain (required)")
	fs.StringVar(&opts.lang, "lang", "", "task language (required)")
	fs.StringVar(&opts.complexity, "complexity", "", "low|medium|high (required)")
	fs.StringVar(&opts.risk, "risk", "", "low|medium|high|critical (required)")
	fs.StringVar(&opts.caps, "cap", "", "comma-separated required F11.7 capabilities")
	fs.StringVar(&opts.constraints, "constraint", "", "comma-separated restrictions")
	fs.StringVar(&opts.outputs, "output", "", "comma-separated expected outputs")
	fs.StringVar(&opts.criteria, "criteria", "", "comma-separated acceptance criteria")
	fs.StringVar(&opts.history, "history", "", "JSON file with observed Lab results (array of bench.LabResult)")
	fs.StringVar(&opts.rates, "rates", "", "JSON tariff file (array of trace.RateCard) for money pricing")
	fs.Int64Var(&opts.budgetTimeMS, "budget-time-ms", 0, "wall-clock ceiling in ms (0 = unset)")
	fs.Int64Var(&opts.budgetTokens, "budget-tokens", 0, "proxy-token ceiling (0 = unset)")
	fs.Float64Var(&opts.budgetCost, "budget-cost", 0, "money ceiling in tariff currency (0 = unset)")
	fs.IntVar(&opts.maxDelegations, "max-delegations", 1, "delegation chain bound 0..4")
	fs.StringVar(&opts.unavailable, "unavailable", "", "comma-separated agents to mark unavailable")
	fs.StringVar(&opts.fixed, "fixed", "baseline", "fixed-routing agent to contrast against")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "compute and print without recording anything")
	fs.StringVar(&opts.out, "out", "", "record the decision JSON into this directory")
	fs.BoolVar(&opts.asJSON, "json", false, "print the decision as JSON")
	if err := fs.Parse(args); err != nil {
		return orchestrateDecideOptions{}, 2, false
	}
	if fs.NArg() != 0 || opts.domain == "" || opts.lang == "" || opts.complexity == "" || opts.risk == "" {
		fmt.Fprintln(stderr, orchestrateUsage())
		return orchestrateDecideOptions{}, 2, false
	}
	for _, f := range []string{opts.history, opts.rates, opts.out} {
		if strings.HasPrefix(f, "-") {
			fmt.Fprintln(stderr, orchestrateUsage())
			return orchestrateDecideOptions{}, 2, false
		}
	}
	return opts, 0, true
}

// loadOrchestrateRates reads the optional tariff file: strict JSON
// array of trace.RateCard (F11.5). No file means money stays unknown
// (never zero).
func loadOrchestrateRates(path string, stderr io.Writer) ([]trace.RateCard, int, bool) {
	if path == "" {
		return nil, 0, true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return nil, 1, false
	}
	if len(raw) > 64*1024 {
		fmt.Fprintln(stderr, "orchestrate: rates file too large")
		return nil, 1, false
	}
	rates, err := trace.ParseRateCards(raw)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return nil, 1, false
	}
	return rates, 0, true
}

// loadOrchestrateHistory reads observed Lab results and folds them
// into routing history samples (durations, tokens, tariff-priced
// money). Malformed or hostile files fail closed.
func loadOrchestrateHistory(path string, rates []trace.RateCard, stderr io.Writer) ([]orchestrate.AgentHistory, int, bool) {
	if path == "" {
		return nil, 0, true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate: %v\n", err)
		return nil, 1, false
	}
	if len(raw) > 4*1024*1024 {
		fmt.Fprintln(stderr, "orchestrate: history file too large")
		return nil, 1, false
	}
	var results []bench.LabResult
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&results); err != nil {
		fmt.Fprintf(stderr, "orchestrate: malformed history: %v\n", err)
		return nil, 1, false
	}
	if len(results) > 10000 {
		fmt.Fprintln(stderr, "orchestrate: history holds at most 10000 results")
		return nil, 1, false
	}
	return orchestrate.SummarizeHistory(orchestrate.SamplesFromLabResults(results, rates)), 0, true
}

// recordOrchestrateTrace links the decision to its F11.1 timeline:
// one run id, run.start, one task.end, run.end through
// observability.RecordExec (redacted pre-disk). Best effort: a store
// failure warns on stderr and leaves the hint empty rather than
// failing the routing.
func (c CLI) recordOrchestrateTrace(command, task string) string {
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(c.Stderr, "orchestrate: sin traza F11.1 (%v)\n", err)
		return ""
	}
	root := c.dir()
	emit := func(e observability.ExecEvent) {
		if _, _, rerr := observability.RecordExec(root, e, time.Now().UTC()); rerr != nil {
			fmt.Fprintf(c.Stderr, "orchestrate: evento de traza no registrado (%v)\n", rerr)
		}
	}
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunStart, Status: observability.ExecStarted,
		Payload: map[string]any{"command": command},
	})
	emit(observability.ExecEvent{
		RunID: runID, TaskID: "route/" + task,
		Type: observability.ExecTaskEnd, Status: observability.ExecSucceeded,
		Payload:      map[string]any{"command": command},
		ErrorDetails: "routing decision recorded (proposes, never executes)",
	})
	emit(observability.ExecEvent{
		RunID: runID, Type: observability.ExecRunEnd, Status: observability.ExecSucceeded,
		Payload: map[string]any{"command": command},
	})
	return bench.TraceHintFor(runID)
}

// writeOrchestrateDecision stores one decision as JSON next to its
// recording timestamp. Failures print the byte-identical refusal.
func writeOrchestrateDecision(dir string, dec orchestrate.Decision, stderr io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "orchestrate decide: %v\n", err)
		return 1
	}
	raw, err := json.MarshalIndent(dec, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate decide: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "decision.json"), append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "orchestrate decide: %v\n", err)
		return 1
	}
	return 0
}

// orchestrateEvaluateCmd implements `regulus orchestrate evaluate`:
// contrast dynamic vs fixed routing on observed Lab results, with
// optional tariffs for money. The verdict may keep the fixed router:
// KEEP_FIXED exits 0 — it is a conclusion, not a failure.
func (c CLI) orchestrateEvaluateCmd(args []string) int {
	fs := flag.NewFlagSet("orchestrate evaluate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	dynamic := fs.String("dynamic", "", "JSON file with dynamic-arm Lab results (required)")
	fixed := fs.String("fixed", "", "JSON file with fixed-arm Lab results (required)")
	rates := fs.String("rates", "", "JSON tariff file (array of trace.RateCard) for money pricing")
	asJSON := fs.Bool("json", false, "print the evaluation as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dynamic == "" || *fixed == "" || fs.NArg() != 0 ||
		strings.HasPrefix(*dynamic, "-") || strings.HasPrefix(*fixed, "-") {
		fmt.Fprintln(c.Stderr, orchestrateUsage())
		return 2
	}
	tariffs, code, ok := loadOrchestrateRates(*rates, c.Stderr)
	if !ok {
		return code
	}
	dyn, code, ok := loadOrchestrateArm("dynamic", *dynamic, c.Stderr)
	if !ok {
		return code
	}
	fix, code, ok := loadOrchestrateArm("fixed", *fixed, c.Stderr)
	if !ok {
		return code
	}
	rep := orchestrate.Evaluate(dyn, fix, tariffs, orchestrate.DefaultEvalThresholds())
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "orchestrate evaluate: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(c.Stdout, orchestrate.RenderEvalReport(rep))
	return 0
}

// loadOrchestrateArm reads one evaluation arm: strict JSON array of
// bench.LabResult, bounded, failing closed on malformed input.
func loadOrchestrateArm(name, path string, stderr io.Writer) ([]bench.LabResult, int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate evaluate: %v\n", err)
		return nil, 1, false
	}
	if len(raw) > 4*1024*1024 {
		fmt.Fprintf(stderr, "orchestrate evaluate: %s arm too large\n", name)
		return nil, 1, false
	}
	var results []bench.LabResult
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&results); err != nil {
		fmt.Fprintf(stderr, "orchestrate evaluate: malformed %s arm: %v\n", name, err)
		return nil, 1, false
	}
	if len(results) > 10000 {
		fmt.Fprintf(stderr, "orchestrate evaluate: %s arm holds at most 10000 results\n", name)
		return nil, 1, false
	}
	return results, 0, true
}
