package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/bench"
)

// BenchTiersCmd implements `regulus bench tiers`. Moved from
// internal/cli (bench_tiers.go) without behavior change.
func (io IO) BenchTiersCmd(args []string) int {
	fs := flag.NewFlagSet("bench tiers", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the level table as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus bench tiers [--json]")
		return 2
	}
	specs := bench.TierSpecs()
	if *asJSON {
		raw, err := json.MarshalIndent(specs, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "bench tiers: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "eval tiers %s: %d level(s), corpus %s (%d tasks, holdout sealed below L3)\n",
		bench.TierVersion, len(specs), bench.AgentSuiteVersion, len(bench.AgentCorpus()))
	for _, s := range specs {
		hold := "holdout sealed"
		if s.IncludesHoldout {
			hold = "holdout included"
		}
		fmt.Fprintf(io.Stdout, "- %s %s: %s [%s, run<=%ds]\n  %s\n",
			s.Tier, s.Name, s.Description, hold, s.RunBudgetS, s.Command)
	}
	return 0
}

// benchTierPlanOptions carries the parsed `bench tier-plan` flags.
type benchTierPlanOptions struct {
	tier        string
	changed     string
	capability  string
	agent       string
	release     bool
	lastL3      string
	jobs        int
	asJSON      bool
	save        string
	evidenceDir string
}

// parseBenchTierPlanOptions parses the plan flags.
func parseBenchTierPlanOptions(args []string, stderr io.Writer) (benchTierPlanOptions, int, bool) {
	var opts benchTierPlanOptions
	fs := flag.NewFlagSet("bench tier-plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tier := fs.String("tier", "auto", "L0|L1|L2|L3 or auto (recommend from --changed)")
	changed := fs.String("changed", "", "comma-separated changed paths for auto/L1 selection")
	capability := fs.String("capability", "", "capability filter for L1 (go|node|shell|python|gaming-trap|concurrency)")
	agent := fs.String("agent", "", "agent label recorded on the plan (selection is agent-independent)")
	release := fs.Bool("release", false, "release run: escalate to L3 (holdout required)")
	lastL3 := fs.String("last-l3", "", "last L3 timestamp RFC3339 (stale plans escalate to L3)")
	jobs := fs.Int("jobs", 0, "parallelism for wall-ceiling estimates (1..8, default 4)")
	asJSON := fs.Bool("json", false, "print the selection as JSON")
	save := fs.String("save", "", "write the selection JSON to this path")
	evidenceDir := fs.String("evidence-dir", "", "write tamper-evident tier plan (tier-plan.json + tier-manifest.json) to this directory")
	if err := fs.Parse(args); err != nil {
		return benchTierPlanOptions{}, 2, false
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
		return benchTierPlanOptions{}, 2, false
	}
	opts = benchTierPlanOptions{
		tier: *tier, changed: *changed, capability: *capability, agent: *agent,
		release: *release, lastL3: *lastL3, jobs: *jobs, asJSON: *asJSON,
		save: *save, evidenceDir: *evidenceDir,
	}
	return opts, 0, true
}

// resolveBenchTierPlan turns flags into a selection.
func resolveBenchTierPlan(opts benchTierPlanOptions, corpus []bench.AgentTask, splits bench.AgentSplits) (bench.TierSelection, string, int, bool) {
	tier := opts.tier
	reason := ""
	if strings.EqualFold(strings.TrimSpace(tier), "auto") || strings.TrimSpace(tier) == "" {
		tier = bench.RecommendTier(splitList(opts.changed))
		reason = fmt.Sprintf("auto recommends %s from the diff", tier)
	} else {
		parsed, err := bench.ParseTier(tier)
		if err != nil {
			return bench.TierSelection{}, "", 2, false
		}
		tier = parsed
	}
	changed := splitList(opts.changed)
	capability, err := bench.ParseTierCapability(opts.capability)
	if err != nil {
		return bench.TierSelection{}, "", 2, false
	}
	if tier == bench.TierL0 && capability != "" {
		return bench.TierSelection{}, "", 2, false
	}
	if opts.jobs < 0 || opts.jobs > bench.TierMaxConcurrency() {
		return bench.TierSelection{}, "", 2, false
	}
	escalated := ""
	if litmusL3Required(opts, tier) {
		if tier != bench.TierL3 {
			escalated = fmt.Sprintf("L3 policy escalates %s to L3 (%s)", tier, litmusL3Why(opts))
			tier = bench.TierL3
		}
	}
	sel, err := bench.SelectTierTasks(tier, corpus, splits, changed, capability)
	if err != nil {
		return bench.TierSelection{}, "", 1, false
	}
	sel.Agent = opts.agent
	if reason != "" && escalated == "" {
		sel.Reason = reason + ": " + sel.Reason
	}
	if escalated != "" {
		sel.Reason = escalated + "; " + sel.Reason
	}
	return sel, escalated, 0, true
}

// litmusL3Required enforces the L3 policy.
func litmusL3Required(opts benchTierPlanOptions, tier string) bool {
	if opts.release {
		return true
	}
	if strings.TrimSpace(opts.lastL3) == "" {
		return false
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(opts.lastL3))
	if err != nil {
		return true
	}
	need, _ := bench.TierL3Required(true, last, time.Now().UTC(), false)
	_ = tier
	return need
}

// litmusL3Why names the escalation for the report.
func litmusL3Why(opts benchTierPlanOptions) string {
	if opts.release {
		return "pre-release L3 required"
	}
	last, err := time.Parse(time.RFC3339, strings.TrimSpace(opts.lastL3))
	if err != nil {
		return "unparseable --last-l3, refusing to assume freshness"
	}
	_, reason := bench.TierL3Required(true, last, time.Now().UTC(), false)
	if reason == "" {
		return "periodic L3 window elapsed"
	}
	return reason
}

// BenchTierPlanCmd implements `regulus bench tier-plan`.
func (io IO) BenchTierPlanCmd(args []string) int {
	started := time.Now().UTC()
	opts, code, ok := parseBenchTierPlanOptions(args, io.Stderr)
	if !ok {
		return code
	}
	// Fail-closed usage errors (exit 2) before any splits I/O so a bad
	// flag is never masked by a missing splits file (exit 1). Mirrors
	// resolveBenchTierPlan validation without needing corpus/splits.
	if tier, capStr := opts.tier, opts.capability; true {
		trimmed := strings.TrimSpace(tier)
		isAuto := trimmed == "" || strings.EqualFold(trimmed, "auto")
		parsedTier := ""
		if !isAuto {
			pt, err := bench.ParseTier(tier)
			if err != nil {
				fmt.Fprintln(io.Stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
				return 2
			}
			parsedTier = pt
		}
		parsedCap, err := bench.ParseTierCapability(capStr)
		if err != nil {
			fmt.Fprintln(io.Stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
			return 2
		}
		if parsedTier == bench.TierL0 && parsedCap != "" {
			fmt.Fprintln(io.Stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
			return 2
		}
		if opts.jobs < 0 || opts.jobs > bench.TierMaxConcurrency() {
			fmt.Fprintln(io.Stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
			return 2
		}
	}
	corpus := bench.AgentCorpus()
	splits, code, ok := loadBenchAgentSplits(io, corpus)
	if !ok {
		return code
	}
	sel, _, code, ok := resolveBenchTierPlan(opts, corpus, splits)
	if !ok {
		if code == 2 {
			fmt.Fprintln(io.Stderr, "usage: regulus bench tier-plan [--tier auto|L0|L1|L2|L3] [--changed f1,f2] [--capability C] [--agent A] [--release] [--last-l3 RFC3339] [--jobs N] [--json] [--save P] [--evidence-dir D]")
		} else {
			fmt.Fprintf(io.Stderr, "bench tier-plan: selection failed\n")
		}
		return code
	}
	jobs := opts.jobs
	if jobs <= 0 {
		jobs = bench.TierDefaultConcurrency()
	}
	if opts.save != "" {
		if code := saveBenchTierPlan(io.Stderr, opts.save, sel); code != 0 {
			return code
		}
		if !opts.asJSON {
			fmt.Fprintf(io.Stdout, "tier: selection saved to %s\n", opts.save)
		}
	}
	if opts.evidenceDir != "" {
		head := gitHeadBounded(io.Runner)
		if head == "" {
			head = "unknown"
		}
		if _, err := bench.WriteTierEvidence(opts.evidenceDir, sel, head, started.Format(time.RFC3339)); err != nil {
			fmt.Fprintf(io.Stderr, "bench tier-plan: %v\n", err)
			return 1
		}
		if !opts.asJSON {
			fmt.Fprintf(io.Stdout, "tier: evidence recorded in %s\n", opts.evidenceDir)
		}
	}
	if opts.asJSON {
		raw, err := json.MarshalIndent(sel, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "bench tier-plan: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	m := bench.TierEvidenceManifest{TierVersion: bench.TierVersion, Tier: sel.Tier, PlanFile: "(plan)"}
	fmt.Fprint(io.Stdout, bench.RenderTierPlan(m, sel, corpus, splits, started))
	_ = jobs
	return 0
}

// saveBenchTierPlan writes the raw selection JSON.
func saveBenchTierPlan(stderr io.Writer, path string, sel bench.TierSelection) int {
	raw, err := json.MarshalIndent(sel, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "bench tier-plan: %v\n", err)
		return 1
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(stderr, "bench tier-plan: %v\n", err)
		return 1
	}
	return 0
}

// BenchTierReportCmd implements `regulus bench tier-report`.
func (io IO) BenchTierReportCmd(args []string) int {
	fs := flag.NewFlagSet("bench tier-report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	dir := fs.String("evidence-dir", "", "tier evidence directory with tier-manifest.json + tier-plan.json (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus bench tier-report --evidence-dir <dir>")
		return 2
	}
	m, sel, err := bench.ReadTierEvidence(*dir)
	if err != nil {
		fmt.Fprintf(io.Stderr, "bench tier-report: %v\n", err)
		return 1
	}
	corpus := bench.AgentCorpus()
	splits, code, ok := loadBenchAgentSplits(io, corpus)
	if !ok {
		return code
	}
	fmt.Fprint(io.Stdout, bench.RenderTierPlan(m, sel, corpus, splits, time.Now().UTC()))
	return 0
}
