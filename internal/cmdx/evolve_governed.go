package cmdx

// Governed self-evolution cycle commands (F12.7/NOV-102): the CLI over
// internal/evolve governed.go / governed_eval.go. Handlers take the
// minimal IO instead of cli.CLI so cli keeps one edge (cli->cmdx);
// behavior, flags, messages and exit codes are defined here,
// byte-identical from the first commit. The parent owns the dispatch
// pin and usage line in internal/cli (library.go/cli.go): this file
// only extends the `evolve validation` switch and its usage string.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"regulus-ai/internal/evolve"
)

const evolveGovernedUsage = "regulus evolve validation cycle-declare --spec <cycle.json> | regulus evolve validation cycle-propose --cycle <id> --candidate <cid> --content <text> --by <proposer> | regulus evolve validation cycle-link --cycle <id> --plan <val_id> | regulus evolve validation cycle-compare --cycle <id> --by <evaluator> | regulus evolve validation cycle-review --cycle <id> --reviewer <r> --verdict <pass|reject> --reason <t> | regulus evolve validation cycle-approve --cycle <id> --by <human> --reason <t> [--commit <sha> --policy <ref> --manifest <m>] | regulus evolve validation cycle-canary --cycle <id> --by <deployer> --kind <k> --conditions <c1,c2> --reason <t> | regulus evolve validation cycle-monitor --cycle <id> --by <deployer> --healthy <true|false> --detail <t> | regulus evolve validation cycle-promote --cycle <id> --by <deployer> --winner <cid> | regulus evolve validation cycle-rollback --cycle <id> --by <deployer> --reason <t> --condition <c> | regulus evolve validation cycle-report --cycle <id>"

// evolveCycleDispatch is the governed-cycle subverb table: pure router,
// same targets and order as the original switch, behavior unchanged.
var evolveCycleDispatch = map[string]func(IO, []string) int{
	"cycle-declare":  IO.EvolveCycleDeclareCmd,
	"cycle-propose":  IO.EvolveCycleProposeCmd,
	"cycle-link":     IO.EvolveCycleLinkCmd,
	"cycle-compare":  IO.EvolveCycleCompareCmd,
	"cycle-review":   IO.EvolveCycleReviewCmd,
	"cycle-approve":  IO.EvolveCycleApproveCmd,
	"cycle-canary":   IO.EvolveCycleCanaryCmd,
	"cycle-monitor":  IO.EvolveCycleMonitorCmd,
	"cycle-promote":  IO.EvolveCyclePromoteCmd,
	"cycle-rollback": IO.EvolveCycleRollbackCmd,
	"cycle-report":   IO.EvolveCycleReportCmd,
}

// EvolveValidationCycleCmd routes the governed-cycle subverbs.
func (io IO) EvolveValidationCycleCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if fn, ok := evolveCycleDispatch[args[0]]; ok {
		return fn(io, args[1:])
	}
	fmt.Fprintf(io.Stderr, "evolve validation: unknown subcommand %q\n", args[0])
	fmt.Fprintln(io.Stderr, evolveValidationUsage)
	return 2
}

// EvolveCycleDeclareCmd implements `cycle-declare --spec <cycle.json>`.
func (io IO) EvolveCycleDeclareCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-declare", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	specFile := fs.String("spec", "", "governed-cycle spec JSON file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*specFile) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	raw, err := os.ReadFile(*specFile)
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-declare: %v\n", err)
		return 1
	}
	var spec evolve.GovernedSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-declare: malformed spec %q: %v\n", *specFile, err)
		return 1
	}
	c, err := evolve.DeclareGovernedCycle(io.Dir, spec)
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-declare: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: GOVERNED_DECLARED %s (sha %s, K=%d, sensitive=%v)\n", c.ID, shortGovHash(c.CycleSHA256), c.MaxCandidates, c.GovernedSensitive())
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-propose --cycle %s --candidate <cid> --content <text> --by %s\n", c.ID, c.Roles.Proposer)
	return 0
}

func shortGovHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// EvolveCycleProposeCmd implements `cycle-propose`.
func (io IO) EvolveCycleProposeCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-propose", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	candidate := fs.String("candidate", "", "candidate id (required)")
	content := fs.String("content", "", "candidate content the evaluator scores (required)")
	by := fs.String("by", "", "proposer (required, must equal the pinned proposer)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*candidate) == "" ||
		strings.TrimSpace(*content) == "" || strings.TrimSpace(*by) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	cand, err := evolve.ProposeGovernedCandidate(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*candidate), *content, strings.TrimSpace(*by))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-propose: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CANDIDATE_RECORDED %s v%d sha %s\n", cand.ID, cand.N, shortGovHash(cand.ContentSHA))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-link --cycle %s --plan <val_id> | regulus evolve validation cycle-compare --cycle %s --by <evaluator>\n", strings.TrimSpace(*cycle), strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleLinkCmd implements `cycle-link`.
func (io IO) EvolveCycleLinkCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-link", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	plan := fs.String("plan", "", "validation plan id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*plan) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.LinkGovernedPlan(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*plan)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-link: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CYCLE_LINKED %s %s\n", strings.TrimSpace(*cycle), strings.TrimSpace(*plan))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-compare --cycle %s --by <evaluator>\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleCompareCmd implements `cycle-compare`.
func (io IO) EvolveCycleCompareCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-compare", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "evaluator (required, must equal the pinned evaluator)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	ev, err := evolve.CompareGovernedCycle(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*by))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-compare: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CYCLE_COMPARED %s %s\n", strings.TrimSpace(*cycle), ev.PlanVerdict)
	fmt.Fprintf(io.Stdout, "quality: %s\nlatency: %s\ncost: %s\nsecurity: %s\nregressions: %s\nholdout: %s\n",
		ev.Quality, ev.Latency, ev.Cost, ev.Security, ev.Regressions, ev.Holdout)
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-review --cycle %s --reviewer <independent> --verdict pass|reject --reason <text>\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleReviewCmd implements `cycle-review`.
func (io IO) EvolveCycleReviewCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-review", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	reviewer := fs.String("reviewer", "", "independent reviewer (required, outside all pinned roles)")
	verdict := fs.String("verdict", "", "pass|reject (required)")
	reason := fs.String("reason", "", "what was probed and why (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*reviewer) == "" ||
		strings.TrimSpace(*verdict) == "" || strings.TrimSpace(*reason) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.ReviewGovernedCycle(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*reviewer), strings.TrimSpace(*verdict), strings.TrimSpace(*reason)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-review: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: REVIEW_RECORDED %s %s by %s\n", strings.TrimSpace(*cycle), strings.TrimSpace(*verdict), strings.TrimSpace(*reviewer))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-approve --cycle %s --by <human> --reason <text>\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleApproveCmd implements `cycle-approve`.
func (io IO) EvolveCycleApproveCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-approve", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "human approver (required, must equal the pinned approver)")
	reason := fs.String("reason", "", "what risk the human accepts (required)")
	commit := fs.String("commit", "", "commit SHA (required for sensitive changes)")
	policy := fs.String("policy", "", "policy reference (required for sensitive changes)")
	manifest := fs.String("manifest", "", "evidence manifest reference (optional)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" ||
		strings.TrimSpace(*reason) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	err := evolve.ApproveGovernedCycle(io.Dir, strings.TrimSpace(*cycle), evolve.GovernedApproval{
		By: strings.TrimSpace(*by), Reason: strings.TrimSpace(*reason),
		CommitSHA: strings.TrimSpace(*commit), PolicyRef: strings.TrimSpace(*policy),
		ManifestRef: strings.TrimSpace(*manifest),
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-approve: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CYCLE_APPROVED %s by %s\n", strings.TrimSpace(*cycle), strings.TrimSpace(*by))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-canary --cycle %s --by <deployer> --kind <prompt|skill|workflow> --conditions <c1,c2> --reason <text>\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleCanaryCmd implements `cycle-canary`.
func (io IO) EvolveCycleCanaryCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-canary", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "deployer (required, must equal the pinned deployer)")
	kind := fs.String("kind", "", "prompt|skill|workflow (required)")
	conditions := fs.String("conditions", "", "comma-separated preapproved rollback conditions (required)")
	reason := fs.String("reason", "", "why this scope is safe (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" || strings.TrimSpace(*kind) == "" ||
		strings.TrimSpace(*conditions) == "" || strings.TrimSpace(*reason) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	err := evolve.EnterGovernedCanary(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*by),
		evolve.CanaryScope{Kind: strings.TrimSpace(*kind), Reversible: true, Reason: strings.TrimSpace(*reason)},
		strings.Split(strings.TrimSpace(*conditions), ","))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-canary: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CANARY_ENTERED %s kind=%s\n", strings.TrimSpace(*cycle), strings.TrimSpace(*kind))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-monitor --cycle %s --by <deployer> --healthy <true|false> --detail <text>\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleMonitorCmd implements `cycle-monitor`.
func (io IO) EvolveCycleMonitorCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-monitor", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "deployer (required, must equal the pinned deployer)")
	healthy := fs.String("healthy", "", "true|false (required)")
	detail := fs.String("detail", "", "what was observed (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" ||
		strings.TrimSpace(*healthy) == "" || strings.TrimSpace(*detail) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	var ok bool
	switch strings.ToLower(strings.TrimSpace(*healthy)) {
	case "true":
		ok = true
	case "false":
		ok = false
	default:
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.RecordGovernedMonitor(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*by), ok, strings.TrimSpace(*detail)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-monitor: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: MONITOR_RECORDED %s healthy=%v\n", strings.TrimSpace(*cycle), ok)
	if ok {
		fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-promote --cycle %s --by <deployer> --winner <cid>\n", strings.TrimSpace(*cycle))
	} else {
		fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-rollback --cycle %s --by <deployer> --reason <text> --condition <preapproved>\n", strings.TrimSpace(*cycle))
	}
	return 0
}

// EvolveCyclePromoteCmd implements `cycle-promote`.
func (io IO) EvolveCyclePromoteCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-promote", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "deployer (required, must equal the pinned deployer)")
	winner := fs.String("winner", "", "winning candidate id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" ||
		strings.TrimSpace(*winner) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.PromoteGovernedCycle(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*by), strings.TrimSpace(*winner)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-promote: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CYCLE_PROMOTED %s winner %s\n", strings.TrimSpace(*cycle), strings.TrimSpace(*winner))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-report --cycle %s\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleRollbackCmd implements `cycle-rollback`.
func (io IO) EvolveCycleRollbackCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-rollback", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	by := fs.String("by", "", "deployer (required, must equal the pinned deployer)")
	reason := fs.String("reason", "", "why (required)")
	condition := fs.String("condition", "", "invoked condition (required, must be preapproved at canary entry)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || strings.TrimSpace(*by) == "" ||
		strings.TrimSpace(*reason) == "" || strings.TrimSpace(*condition) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.RollbackGovernedCycle(io.Dir, strings.TrimSpace(*cycle), strings.TrimSpace(*by), strings.TrimSpace(*reason), strings.TrimSpace(*condition)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-rollback: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CYCLE_ROLLED_BACK %s (%s)\n", strings.TrimSpace(*cycle), strings.TrimSpace(*condition))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation cycle-report --cycle %s\n", strings.TrimSpace(*cycle))
	return 0
}

// EvolveCycleReportCmd implements `cycle-report`.
func (io IO) EvolveCycleReportCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation cycle-report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	cycle := fs.String("cycle", "", "governed cycle id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*cycle) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	rep, err := evolve.RenderGovernedReport(io.Dir, strings.TrimSpace(*cycle))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation cycle-report: %v\n", err)
		return 1
	}
	fmt.Fprintln(io.Stdout, rep)
	return 0
}
