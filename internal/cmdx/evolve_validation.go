package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"regulus-ai/internal/evolve"
)

const evolveValidationUsage = "usage: regulus evolve validation declare --plan <spec.json> | regulus evolve validation run --plan <id> --arm <baseline|candidate> --task <t> --seed <n> --rep <n> --model <m> --status <s> --latency-ms <n> --tokens <n> --source <s> [--receipt <r>] [--trace <t>] | regulus evolve validation holdout-run <same> --audit <ref> | regulus evolve validation control --plan <id> --kind <positive|negative> --name <n> --expectation <e> --outcome <o> --held <true|false> [--detail <d>] | regulus evolve validation discard --plan <id> --candidate <c> --reason <r> --by <h> | regulus evolve validation link --plan <id> --exp <id> | regulus evolve validation compare --plan <id> | regulus evolve validation finalize --plan <id> --verdict <v> --approved-by <h> --reason <r> --inherited <adopt|keep|revisit> --inherited-reason <r> --commits <c,...> --manifests <m,...> | regulus evolve validation report --plan <id> | " + evolveGovernedUsage

// EvolveValidationCmd routes the validation subverbs. Moved from
// internal/cli (evolve_validation.go) without behavior change.
func (io IO) EvolveValidationCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	switch args[0] {
	case "declare":
		return io.EvolveValidationDeclareCmd(args[1:])
	case "run":
		return io.EvolveValidationRunCmd(args[1:], false)
	case "holdout-run":
		return io.EvolveValidationRunCmd(args[1:], true)
	case "control":
		return io.EvolveValidationControlCmd(args[1:])
	case "discard":
		return io.EvolveValidationDiscardCmd(args[1:])
	case "link":
		return io.EvolveValidationLinkCmd(args[1:])
	case "compare":
		return io.EvolveValidationCompareCmd(args[1:])
	case "finalize":
		return io.EvolveValidationFinalizeCmd(args[1:])
	case "report":
		return io.EvolveValidationReportCmd(args[1:])
	case "cycle-declare", "cycle-propose", "cycle-link", "cycle-compare", "cycle-review", "cycle-approve", "cycle-canary", "cycle-monitor", "cycle-promote", "cycle-rollback", "cycle-report":
		return io.EvolveValidationCycleCmd(args)
	default:
		fmt.Fprintf(io.Stderr, "evolve validation: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
}

// EvolveValidationDeclareCmd implements `regulus evolve validation declare`.
func (io IO) EvolveValidationDeclareCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation declare", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	planFile := fs.String("plan", "", "pre-registration spec JSON file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*planFile) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	raw, err := os.ReadFile(*planFile)
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation declare: %v\n", err)
		return 1
	}
	var spec evolve.ValidationPlanSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation declare: malformed spec %q: %v\n", *planFile, err)
		return 1
	}
	p, err := evolve.DeclareValidationPlan(io.Dir, spec)
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation declare: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: VALIDATION_DECLARED %s (sha %s)\n", p.ID, shortValHash(p.PlanSHA256))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation run --plan %s --arm baseline|candidate --task <t> --seed <n> --rep <n> --model <m> --status <s> --latency-ms <n> --tokens <n> --source <s>\n", p.ID)
	return 0
}

func shortValHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// EvolveValidationRunCmd implements `run` and `holdout-run`.
func (io IO) EvolveValidationRunCmd(args []string, holdout bool) int {
	name := "evolve validation run"
	if holdout {
		name = "evolve validation holdout-run"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	arm := fs.String("arm", "", "baseline|candidate (required)")
	task := fs.String("task", "", "task id (required)")
	seed := fs.Int64("seed", 0, "seed (required, must be declared)")
	rep := fs.Int("rep", -1, "repetition 0..repetitions-1 (required)")
	model := fs.String("model", "", "model (required, must be declared)")
	status := fs.String("status", "", "pass|fail|timeout|infra|non-evaluable (required)")
	latency := fs.Int64("latency-ms", -1, "attempt wall latency ms (required)")
	tokens := fs.Int64("tokens", -1, "attempt tokens (required)")
	source := fs.String("source", "", "measured-host|measured-local|estimated|proxy|unknown (required)")
	receipt := fs.String("receipt", "", "F11.1 receipt id")
	traceID := fs.String("trace", "", "F11.1 run/execution id")
	audit := fs.String("audit", "", "audit reference (required for holdout-run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || strings.TrimSpace(*arm) == "" || strings.TrimSpace(*task) == "" ||
		*seed == 0 || *rep < 0 || strings.TrimSpace(*model) == "" || strings.TrimSpace(*status) == "" ||
		*latency < 0 || *tokens < 0 || strings.TrimSpace(*source) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if holdout && strings.TrimSpace(*audit) == "" {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	split := evolve.ValidationSplitExploration
	if holdout {
		split = evolve.ValidationSplitHoldout
	}
	trial := evolve.ValidationTrial{
		Arm: strings.TrimSpace(*arm), TaskID: strings.TrimSpace(*task),
		Seed: *seed, Repetition: *rep, Model: strings.TrimSpace(*model),
		Split: split, Status: strings.TrimSpace(*status),
		LatencyMS: *latency, Tokens: *tokens, TokenSource: strings.TrimSpace(*source),
		ReceiptID: strings.TrimSpace(*receipt), TraceID: strings.TrimSpace(*traceID),
	}
	root := io.Dir
	if holdout {
		if _, err := evolve.RecordHoldoutTrial(root, strings.TrimSpace(*plan), strings.TrimSpace(*audit), trial); err != nil {
			fmt.Fprintf(io.Stderr, "evolve validation holdout-run: %v\n", err)
			return 1
		}
		fmt.Fprintf(io.Stdout, "verdict: HOLDOUT_RECORDED %s %s/%s seed=%d rep=%d\n", strings.TrimSpace(*plan), trial.Arm, trial.TaskID, trial.Seed, trial.Repetition)
		fmt.Fprintf(io.Stdout, "next: regulus evolve validation finalize --plan %s --verdict <exploration-verdict> --approved-by <human> ...\n", strings.TrimSpace(*plan))
		return 0
	}
	if _, err := evolve.RecordValidationTrial(root, strings.TrimSpace(*plan), trial); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation run: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: TRIAL_RECORDED %s %s/%s seed=%d rep=%d\n", strings.TrimSpace(*plan), trial.Arm, trial.TaskID, trial.Seed, trial.Repetition)
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation compare --plan %s (after the exploration matrix is complete)\n", strings.TrimSpace(*plan))
	return 0
}

// EvolveValidationControlCmd implements `control`.
func (io IO) EvolveValidationControlCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation control", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	kind := fs.String("kind", "", "positive|negative (required)")
	name := fs.String("name", "", "control name (required)")
	expectation := fs.String("expectation", "", "expected verdict (required)")
	outcome := fs.String("outcome", "", "observed verdict (required)")
	held := fs.String("held", "", "true|false: did the expectation hold (required)")
	detail := fs.String("detail", "", "detail")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || strings.TrimSpace(*kind) == "" || strings.TrimSpace(*name) == "" ||
		strings.TrimSpace(*expectation) == "" || strings.TrimSpace(*outcome) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	var heldBool bool
	switch strings.ToLower(strings.TrimSpace(*held)) {
	case "true":
		heldBool = true
	case "false":
		heldBool = false
	default:
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	err := evolve.RecordValidationControl(io.Dir, strings.TrimSpace(*plan), evolve.ValidationControl{
		Kind: strings.TrimSpace(*kind), Name: strings.TrimSpace(*name),
		Expectation: strings.TrimSpace(*expectation), Outcome: strings.TrimSpace(*outcome),
		Held: heldBool, Detail: strings.TrimSpace(*detail),
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation control: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: CONTROL_RECORDED %s [%s] %s (held=%v)\n", strings.TrimSpace(*plan), strings.TrimSpace(*kind), strings.TrimSpace(*name), heldBool)
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation compare --plan %s\n", strings.TrimSpace(*plan))
	return 0
}

// EvolveValidationDiscardCmd implements `discard`.
func (io IO) EvolveValidationDiscardCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation discard", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	candidate := fs.String("candidate", "", "rejected candidate id (required)")
	reason := fs.String("reason", "", "why it was rejected (required)")
	by := fs.String("by", "", "owner (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || strings.TrimSpace(*candidate) == "" ||
		strings.TrimSpace(*reason) == "" || strings.TrimSpace(*by) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	err := evolve.DiscardValidationCandidate(io.Dir, strings.TrimSpace(*plan), evolve.ValidationDiscard{
		CandidateID: strings.TrimSpace(*candidate), Reason: strings.TrimSpace(*reason), By: strings.TrimSpace(*by),
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation discard: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: DISCARDED %s %s\n", strings.TrimSpace(*plan), strings.TrimSpace(*candidate))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation compare --plan %s\n", strings.TrimSpace(*plan))
	return 0
}

// EvolveValidationLinkCmd implements `link`.
func (io IO) EvolveValidationLinkCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation link", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	exp := fs.String("exp", "", "experiment id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || strings.TrimSpace(*exp) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	if err := evolve.LinkValidationExperiment(io.Dir, strings.TrimSpace(*plan), strings.TrimSpace(*exp)); err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation link: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: LINKED %s %s\n", strings.TrimSpace(*plan), strings.TrimSpace(*exp))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation finalize --plan %s ...\n", strings.TrimSpace(*plan))
	return 0
}

// EvolveValidationCompareCmd implements `compare`.
func (io IO) EvolveValidationCompareCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation compare", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	cmp, err := evolve.CompareValidation(io.Dir, strings.TrimSpace(*plan))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation compare: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: COMPARED %s %s\n", strings.TrimSpace(*plan), cmp.Verdict)
	fmt.Fprintf(io.Stdout, "baseline: pass-rate %.4f (evaluable %d, infra %d) latency mean %.1fms tokens measured %d (unknown %d)\n",
		cmp.Baseline.PassRate, cmp.Baseline.Evaluable, cmp.Baseline.Infra, cmp.Baseline.LatencyMeanMS, cmp.Baseline.TokensMeasured, cmp.Baseline.UnknownTrials)
	fmt.Fprintf(io.Stdout, "candidate: pass-rate %.4f (evaluable %d, infra %d) latency mean %.1fms tokens measured %d (unknown %d)\n",
		cmp.Candidate.PassRate, cmp.Candidate.Evaluable, cmp.Candidate.Infra, cmp.Candidate.LatencyMeanMS, cmp.Candidate.TokensMeasured, cmp.Candidate.UnknownTrials)
	fmt.Fprintf(io.Stdout, "honesty: %s\n", cmp.Honesty)
	if cmp.Verdict == evolve.RobustInsufficient {
		fmt.Fprintf(io.Stdout, "next: record evaluable trials on both arms, then re-compare\n")
		return 1
	}
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation finalize --plan %s --verdict %s --approved-by <human> ...\n", strings.TrimSpace(*plan), cmp.Verdict)
	return 0
}

// EvolveValidationFinalizeCmd implements `finalize`.
func (io IO) EvolveValidationFinalizeCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation finalize", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	verdict := fs.String("verdict", "", "exploration verdict (required, must equal the frozen verdict)")
	approvedBy := fs.String("approved-by", "", "human approver (required)")
	reason := fs.String("reason", "", "why (required)")
	inherited := fs.String("inherited", "", "adopt|keep|revisit (required)")
	inheritedReason := fs.String("inherited-reason", "", "why (required)")
	commits := fs.String("commits", "", "comma-separated commit pins (required)")
	manifests := fs.String("manifests", "", "comma-separated manifest pins (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || strings.TrimSpace(*verdict) == "" || strings.TrimSpace(*approvedBy) == "" ||
		strings.TrimSpace(*reason) == "" || strings.TrimSpace(*inherited) == "" || strings.TrimSpace(*inheritedReason) == "" ||
		strings.TrimSpace(*commits) == "" || strings.TrimSpace(*manifests) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	err := evolve.FinalizeValidation(io.Dir, strings.TrimSpace(*plan), evolve.ValidationDecision{
		Verdict: strings.TrimSpace(*verdict), By: strings.TrimSpace(*approvedBy),
		Reason: strings.TrimSpace(*reason), Inherited: strings.TrimSpace(*inherited),
		InheritedReason: strings.TrimSpace(*inheritedReason),
	}, evolve.ValidationEvidence{
		Commits:   strings.Split(strings.TrimSpace(*commits), ","),
		Manifests: strings.Split(strings.TrimSpace(*manifests), ","),
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation finalize: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "verdict: FINALIZED %s %s (by %s, inherited %s)\n",
		strings.TrimSpace(*plan), strings.TrimSpace(*verdict), strings.TrimSpace(*approvedBy), strings.TrimSpace(*inherited))
	fmt.Fprintf(io.Stdout, "next: regulus evolve validation report --plan %s\n", strings.TrimSpace(*plan))
	return 0
}

// EvolveValidationReportCmd implements `report`.
func (io IO) EvolveValidationReportCmd(args []string) int {
	fs := flag.NewFlagSet("evolve validation report", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	plan := fs.String("plan", "", "validation id (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*plan) == "" || fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, evolveValidationUsage)
		return 2
	}
	rep, err := evolve.RenderValidationReport(io.Dir, strings.TrimSpace(*plan))
	if err != nil {
		fmt.Fprintf(io.Stderr, "evolve validation report: %v\n", err)
		return 1
	}
	fmt.Fprintln(io.Stdout, rep)
	return 0
}
