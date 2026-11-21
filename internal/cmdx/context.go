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

	"regulus-ai/internal/context"
	"regulus-ai/internal/orchestrate"
)

func contextUsage() string {
	return "usage: regulus context inventory [--issues <file>] [--no-memory] [--json] | regulus context select --domain <d> --lang <l> --complexity <low|medium|high> --risk <low|medium|high|critical> [--cap <a,b>] [--constraint <a,b>] [--output <a,b>] [--criteria <a,b>] [--profile <agent>] [--scope <s,...>] [--affinity <d,...>] [--budget-tokens N] [--max-age-days N] [--untrusted <s,...>] [--issues <file>] [--no-memory] [--dry-run] [--out <dir>] [--json]"
}

// ContextCmd routes `regulus context ...`. Moved from internal/cli
// (context.go) without behavior change.
func (io IO) ContextCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, contextUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, contextUsage())
		return 2
	}
	switch args[0] {
	case "inventory":
		return io.ContextInventoryCmd(args[1:])
	case "select":
		return io.ContextSelectCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "context: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, contextUsage())
		return 2
	}
}

// ContextInventoryCmd implements `regulus context inventory`.
func (io IO) ContextInventoryCmd(args []string) int {
	fs := flag.NewFlagSet("context inventory", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	issues := fs.String("issues", "", "JSON file with extra issue entries")
	noMemory := fs.Bool("no-memory", false, "skip the Almanac memory")
	asJSON := fs.Bool("json", false, "print the census as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.HasPrefix(*issues, "-") {
		fmt.Fprintln(io.Stderr, contextUsage())
		return 2
	}
	cfg := context.DefaultInventoryConfig()
	cfg.IssuesFile = *issues
	cfg.IncludeMemory = !*noMemory
	corpus, warnings, err := context.Inventory(io.Dir, cfg)
	if err != nil {
		fmt.Fprintf(io.Stderr, "context inventory: %v\n", err)
		return 1
	}
	byKind := map[string]int{}
	var tokens int64
	for _, f := range corpus.Fragments {
		byKind[f.Kind]++
		tokens += f.Tokens
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(map[string]any{
			"kinds":    byKind,
			"total":    len(corpus.Fragments),
			"tokens":   tokens,
			"warnings": warnings,
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "context inventory: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sortStrings(kinds)
	fmt.Fprintf(io.Stdout, "contexto: %d fragmento(s), %d tokens (proxy F11.5)\n", len(corpus.Fragments), tokens)
	for _, k := range kinds {
		fmt.Fprintf(io.Stdout, "- %s: %d\n", k, byKind[k])
	}
	for _, w := range warnings {
		fmt.Fprintf(io.Stdout, "aviso: %s\n", w)
	}
	return 0
}

// contextSelectOptions carries the parsed `select` flags.
type contextSelectOptions struct {
	domain       string
	lang         string
	complexity   string
	risk         string
	caps         string
	constraints  string
	outputs      string
	criteria     string
	profile      string
	scopes       string
	affinity     string
	budgetTokens int64
	maxAgeDays   int
	untrusted    string
	issues       string
	noMemory     bool
	dryRun       bool
	out          string
	asJSON       bool
}

// ContextSelectCmd implements `regulus context select`.
func (io IO) ContextSelectCmd(args []string) int {
	opts, code, ok := parseContextSelectFlags(args, io.Stderr)
	if !ok {
		return code
	}
	cfg := context.DefaultInventoryConfig()
	cfg.IssuesFile = opts.issues
	cfg.IncludeMemory = !opts.noMemory
	corpus, warnings, err := context.Inventory(io.Dir, cfg)
	if err != nil {
		fmt.Fprintf(io.Stderr, "context select: %v\n", err)
		return 1
	}
	desc := orchestrate.TaskDescriptor{
		Domain: opts.domain, Language: opts.lang,
		Complexity: opts.complexity, Risk: opts.risk,
		Constraints: splitList(opts.constraints), Outputs: splitList(opts.outputs),
		Criteria:             splitList(opts.criteria),
		RequiredCapabilities: splitList(opts.caps),
	}
	if verr := orchestrate.ValidateTaskDescriptor(desc); verr != nil {
		fmt.Fprintf(io.Stderr, "context select: %v\n", verr)
		return 2
	}
	profile := context.Profile{
		AgentID: opts.profile,
		Scopes:  splitList(opts.scopes),
		Domains: splitList(opts.affinity),
	}
	if profile.AgentID == "" {
		profile.AgentID = "default"
	}
	copts := context.Options{BudgetTokens: opts.budgetTokens}
	if opts.maxAgeDays > 0 {
		copts.MaxAge = time.Duration(opts.maxAgeDays) * 24 * time.Hour
	}
	copts.UntrustedSources = splitList(opts.untrusted)
	sel := context.Select(corpus, desc, profile, copts)
	sel.Warnings = append(warnings, sel.Warnings...)
	if !opts.dryRun {
		hint := io.recordOrchestrateTrace("context select", desc.Domain+"/"+desc.Language)
		if opts.out != "" {
			if code := writeContextSelection(opts.out, sel, io.Stderr); code != 0 {
				return code
			}
			fmt.Fprintf(io.Stdout, "contexto: selección registrada en %s\n", opts.out)
		}
		if hint != "" && !opts.asJSON {
			fmt.Fprintf(io.Stdout, "traza: %s\n", hint)
		}
	}
	if opts.asJSON {
		raw, merr := json.MarshalIndent(sel, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "context select: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
	} else {
		fmt.Fprint(io.Stdout, renderContextSelection(sel))
	}
	if sel.Report.CriticalKept < sel.Report.CriticalTotal {
		fmt.Fprintln(io.Stderr, "context select: contexto crítico omitido (falló el fallback conservador)")
		return 1
	}
	if len(corpus.Fragments) == 0 {
		fmt.Fprintln(io.Stderr, "context select: corpus vacío (nada que entregar)")
		return 1
	}
	return 0
}

// parseContextSelectFlags parses the select flags.
func parseContextSelectFlags(args []string, stderr io.Writer) (contextSelectOptions, int, bool) {
	var opts contextSelectOptions
	fs := flag.NewFlagSet("context select", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.domain, "domain", "", "task domain (required)")
	fs.StringVar(&opts.lang, "lang", "", "task language (required)")
	fs.StringVar(&opts.complexity, "complexity", "", "low|medium|high (required)")
	fs.StringVar(&opts.risk, "risk", "", "low|medium|high|critical (required)")
	fs.StringVar(&opts.caps, "cap", "", "comma-separated required F11.7 capabilities")
	fs.StringVar(&opts.constraints, "constraint", "", "comma-separated restrictions")
	fs.StringVar(&opts.outputs, "output", "", "comma-separated expected outputs")
	fs.StringVar(&opts.criteria, "criteria", "", "comma-separated acceptance criteria")
	fs.StringVar(&opts.profile, "profile", "", "receiving agent id (default \"default\")")
	fs.StringVar(&opts.scopes, "scope", "", "comma-separated F11.8 scopes the agent holds")
	fs.StringVar(&opts.affinity, "affinity", "", "comma-separated profile domain affinities")
	fs.Int64Var(&opts.budgetTokens, "budget-tokens", 0, "token ceiling for essential+on-demand (0 = unset)")
	fs.IntVar(&opts.maxAgeDays, "max-age-days", 0, "obsolescence horizon in days (0 = 180-day default)")
	fs.StringVar(&opts.untrusted, "untrusted", "", "comma-separated sources treated as untrusted")
	fs.StringVar(&opts.issues, "issues", "", "JSON file with extra issue entries")
	fs.BoolVar(&opts.noMemory, "no-memory", false, "skip the Almanac memory")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "compute and print without recording anything")
	fs.StringVar(&opts.out, "out", "", "record the selection JSON into this directory")
	fs.BoolVar(&opts.asJSON, "json", false, "print the selection as JSON")
	if err := fs.Parse(args); err != nil {
		return contextSelectOptions{}, 2, false
	}
	if fs.NArg() != 0 || opts.domain == "" || opts.lang == "" || opts.complexity == "" || opts.risk == "" {
		fmt.Fprintln(stderr, contextUsage())
		return contextSelectOptions{}, 2, false
	}
	for _, f := range []string{opts.issues, opts.out} {
		if strings.HasPrefix(f, "-") {
			fmt.Fprintln(stderr, contextUsage())
			return contextSelectOptions{}, 2, false
		}
	}
	if opts.budgetTokens < 0 || opts.maxAgeDays < 0 {
		fmt.Fprintln(stderr, contextUsage())
		return contextSelectOptions{}, 2, false
	}
	return opts, 0, true
}

// writeContextSelection stores one selection as JSON.
func writeContextSelection(dir string, sel context.Selection, stderr io.Writer) int {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "context select: %v\n", err)
		return 1
	}
	raw, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "context select: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "selection.json"), append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "context select: %v\n", err)
		return 1
	}
	return 0
}

// renderContextSelection prints the layered delivery.
func renderContextSelection(sel context.Selection) string {
	var b strings.Builder
	printLayer := func(name string, frags []context.Fragment) {
		fmt.Fprintf(&b, "%s (%d):\n", name, len(frags))
		for _, f := range frags {
			flagStr := ""
			if len(f.Flags) > 0 {
				flagStr = " [" + strings.Join(f.Flags, ",") + "]"
			}
			ver := f.Version
			if ver == "" {
				ver = f.Date.Format("2006-01-02")
			}
			fmt.Fprintf(&b, "- [%s] %s (%s %s, %.2f)%s\n  por qué: %s\n",
				f.Kind, f.ID, f.Source, ver, f.Score, flagStr, f.Reason)
		}
	}
	printLayer(context.LayerEssential, sel.Essential)
	printLayer(context.LayerOnDemand, sel.OnDemand)
	printLayer(context.LayerOptional, sel.Optional)
	r := sel.Report
	fmt.Fprintf(&b, "tokens: corpus %d → imprescindible %d (ahorro %.1f%%); recall crítico %d/%d\n",
		r.VisibleTokens, r.EssentialTokens, r.SavedPct, r.CriticalKept, r.CriticalTotal)
	if r.Duplicates+r.Obsolete+r.Contradictions+r.Untrusted+r.Injections+r.Denied+r.Redacted+r.BudgetTrimmed > 0 {
		fmt.Fprintf(&b, "detección: duplicados %d, obsoletos %d, contradicciones %d, no-confiables %d, inyección %d, denegados %d, redactados %d, recortados %d\n",
			r.Duplicates, r.Obsolete, r.Contradictions, r.Untrusted, r.Injections, r.Denied, r.Redacted, r.BudgetTrimmed)
	}
	for _, w := range sel.Warnings {
		fmt.Fprintf(&b, "aviso: %s\n", w)
	}
	return b.String()
}

// sortStrings orders kind names for the deterministic census.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
