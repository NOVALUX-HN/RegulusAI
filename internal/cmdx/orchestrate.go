package cmdx

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

func orchestrateUsage() string {
	return "usage: regulus orchestrate agents [--json] | regulus orchestrate decide --domain <d> --lang <l> --complexity <low|medium|high> --risk <low|medium|high|critical> [--cap <a,b>] [--constraint <a,b>] [--output <a,b>] [--criteria <a,b>] [--history <file>] [--rates <file>] [--budget-time-ms N] [--budget-tokens N] [--budget-cost N] [--max-delegations N] [--unavailable <a,b>] [--fixed <agent>] [--dry-run] [--out <dir>] [--json] | regulus orchestrate evaluate --dynamic <file> --fixed <file> [--rates <file>] [--json] | regulus orchestrate parallel --tasks <file> [--dry-run] [--json] | regulus orchestrate orphans [--prune] [--json]"
}

// OrchestrateCmd routes `regulus orchestrate ...`. Moved from
// internal/cli (orchestrate.go) without behavior change.
func (io IO) OrchestrateCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, orchestrateUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, orchestrateUsage())
		return 2
	}
	switch args[0] {
	case "agents":
		return io.OrchestrateAgentsCmd(args[1:])
	case "decide":
		return io.OrchestrateDecideCmd(args[1:])
	case "evaluate":
		return io.OrchestrateEvaluateCmd(args[1:])
	case "parallel":
		return io.OrchestrateParallelCmd(args[1:])
	case "orphans":
		return io.OrchestrateOrphansCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "orchestrate: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, orchestrateUsage())
		return 2
	}
}

// OrchestrateAgentsCmd implements `regulus orchestrate agents`.
func (io IO) OrchestrateAgentsCmd(args []string) int {
	fs := flag.NewFlagSet("orchestrate agents", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the routing registry as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus orchestrate agents [--json]")
		return 2
	}
	reg := orchestrate.RegistryFromManifests(bench.DefaultHostManifests(bench.EvidenceStamp()))
	if *asJSON {
		raw, err := json.MarshalIndent(reg, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "orchestrate agents: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "routing %s: %d agent(s)\n", orchestrate.RoutingVersion, len(reg))
	for _, a := range reg {
		fmt.Fprintf(io.Stdout, "- %s: %s\n  host %s solo-hasta %s tools [%s] scopes [%s]\n",
			a.ID, a.Responsibility, a.Host, a.MaxRisk,
			strings.Join(a.Tools, ", "), strings.Join(a.Scopes, ", "))
		for _, cap := range a.Capabilities {
			ev := strings.ReplaceAll(cap.Evidence, "\n", " ")
			lim := strings.ReplaceAll(cap.Limit, "\n", " ")
			fmt.Fprintf(io.Stdout, "  cap %s: %s | límite: %s\n", cap.ID, ev, lim)
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

// OrchestrateDecideCmd implements `regulus orchestrate decide`.
func (io IO) OrchestrateDecideCmd(args []string) int {
	opts, code, ok := parseOrchestrateDecideFlags(args, io.Stderr)
	if !ok {
		return code
	}
	rates, code, ok := loadOrchestrateRates(opts.rates, io.Stderr)
	if !ok {
		return code
	}
	hist, code, ok := loadOrchestrateHistory(opts.history, rates, io.Stderr)
	if !ok {
		return code
	}
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	reg = applyUnavailable(reg, opts.unavailable)
	desc := buildTaskDescriptor(opts)
	dec, code, ok := runDecision(reg, manifests, hist, desc, opts, io.Stderr)
	if !ok {
		return code
	}
	return io.emitDecision(dec, opts)
}

// applyUnavailable marks the named agents unavailable without
// touching the registry order. Empty input returns the registry
// unchanged; unknown ids are ignored like the inline loop.
func applyUnavailable(reg []orchestrate.RoutedAgent, unavailable string) []orchestrate.RoutedAgent {
	if unavailable == "" {
		return reg
	}
	down := map[string]bool{}
	for _, id := range splitList(unavailable) {
		down[id] = true
	}
	for i := range reg {
		if down[reg[i].ID] {
			reg[i].Available = false
		}
	}
	return reg
}

// buildTaskDescriptor renders the parsed decide flags as the pure
// routing input. No validation here: Decide owns the gates so the
// command keeps its byte-identical refusals.
func buildTaskDescriptor(opts orchestrateDecideOptions) orchestrate.TaskDescriptor {
	return orchestrate.TaskDescriptor{
		Domain: opts.domain, Language: opts.lang,
		Complexity: opts.complexity, Risk: opts.risk,
		Constraints: splitList(opts.constraints), Outputs: splitList(opts.outputs),
		Criteria:             splitList(opts.criteria),
		RequiredCapabilities: splitList(opts.caps),
	}
}

// runDecision runs the pure routing verdict. A refusal prints the
// byte-identical prefix and reports exit 2 with ok=false.
func runDecision(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, desc orchestrate.TaskDescriptor, opts orchestrateDecideOptions, stderr io.Writer) (orchestrate.Decision, int, bool) {
	dec, err := orchestrate.Decide(reg, manifests, hist, desc,
		orchestrate.Budgets{TimeMS: opts.budgetTimeMS, Tokens: opts.budgetTokens, Cost: opts.budgetCost},
		opts.maxDelegations, opts.fixed, orchestrate.DefaultScoreWeights())
	if err != nil {
		fmt.Fprintf(stderr, "orchestrate decide: %v\n", err)
		return orchestrate.Decision{}, 2, false
	}
	return dec, 0, true
}

// emitDecision records (unless dry-run) and renders one verdict.
// Dry-run skips the F11.1 trace and the out dir; --json prints the
// raw decision, otherwise the human rendering. An empty primary is
// the justified refusal (exit 1) after rendering.
func (io IO) emitDecision(dec orchestrate.Decision, opts orchestrateDecideOptions) int {
	dec.DryRun = opts.dryRun
	if !opts.dryRun {
		dec.TraceHint = io.recordOrchestrateTrace("orchestrate decide", dec.Task.Domain+"/"+dec.Task.Language)
		if opts.out != "" {
			if code := writeOrchestrateDecision(opts.out, dec, io.Stderr); code != 0 {
				return code
			}
			fmt.Fprintf(io.Stdout, "orchestrate: decision recorded in %s\n", opts.out)
		}
	}
	if opts.asJSON {
		raw, err := json.MarshalIndent(dec, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "orchestrate decide: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
	} else {
		fmt.Fprint(io.Stdout, orchestrate.RenderDecision(dec))
	}
	if dec.Primary == "" {
		fmt.Fprintln(io.Stderr, "orchestrate decide: sin agente elegible (nada asignado sin capability verificada)")
		return 1
	}
	return 0
}

// parseOrchestrateDecideFlags parses the decide flags.
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

// loadOrchestrateRates reads the optional tariff file.
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

// readHistoryFile loads the raw history bytes. An empty path is the
// closed vacia case (no history); a missing file and an oversized
// file refuse with exit 1 like the inline command.
func readHistoryFile(path string, stderr io.Writer) ([]byte, int, bool) {
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
	return raw, 0, true
}

// decodeLabResults parses raw Lab results with strict fields and the
// 10000-result bound, then folds them into per-agent history.
func decodeLabResults(raw []byte, rates []trace.RateCard, stderr io.Writer) ([]orchestrate.AgentHistory, int, bool) {
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

// loadOrchestrateHistory reads observed Lab results.
func loadOrchestrateHistory(path string, rates []trace.RateCard, stderr io.Writer) ([]orchestrate.AgentHistory, int, bool) {
	if path == "" {
		return nil, 0, true
	}
	raw, code, ok := readHistoryFile(path, stderr)
	if !ok {
		return nil, code, false
	}
	return decodeLabResults(raw, rates, stderr)
}

// recordOrchestrateTrace links the decision to its F11.1 timeline.
func (io IO) recordOrchestrateTrace(command, task string) string {
	runID, err := observability.MintRunID()
	if err != nil {
		fmt.Fprintf(io.Stderr, "orchestrate: sin traza F11.1 (%v)\n", err)
		return ""
	}
	root := io.Dir
	emit := func(e observability.ExecEvent) {
		if _, _, rerr := observability.RecordExec(root, e, time.Now().UTC()); rerr != nil {
			fmt.Fprintf(io.Stderr, "orchestrate: evento de traza no registrado (%v)\n", rerr)
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

// writeOrchestrateDecision stores one decision as JSON.
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

// OrchestrateEvaluateCmd implements `regulus orchestrate evaluate`.
func (io IO) OrchestrateEvaluateCmd(args []string) int {
	fs := flag.NewFlagSet("orchestrate evaluate", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	dynamic := fs.String("dynamic", "", "JSON file with dynamic-arm Lab results (required)")
	fixed := fs.String("fixed", "", "JSON file with fixed-arm Lab results (required)")
	rates := fs.String("rates", "", "JSON tariff file (array of trace.RateCard) for money pricing")
	asJSON := fs.Bool("json", false, "print the evaluation as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dynamic == "" || *fixed == "" || fs.NArg() != 0 ||
		strings.HasPrefix(*dynamic, "-") || strings.HasPrefix(*fixed, "-") {
		fmt.Fprintln(io.Stderr, orchestrateUsage())
		return 2
	}
	tariffs, code, ok := loadOrchestrateRates(*rates, io.Stderr)
	if !ok {
		return code
	}
	dyn, code, ok := loadOrchestrateArm("dynamic", *dynamic, io.Stderr)
	if !ok {
		return code
	}
	fix, code, ok := loadOrchestrateArm("fixed", *fixed, io.Stderr)
	if !ok {
		return code
	}
	rep := orchestrate.Evaluate(dyn, fix, tariffs, orchestrate.DefaultEvalThresholds())
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "orchestrate evaluate: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, orchestrate.RenderEvalReport(rep))
	return 0
}

// loadOrchestrateArm reads one evaluation arm.
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
