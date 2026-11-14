package cli

import (
	"flag"
	"fmt"

	"regulus-ai/internal/crap"
)

// gateCmd implements `regulus gate run [--profile NAME] [--dry-run] <task>`:
// run the layered gauntlet and write one append-only receipt per executed
// gate (PRD R-G2, R-G4). --dry-run prints the execution plan and writes
// nothing. --profile selects required gates plus the profile's members
// (quick = required only, full = everything, the default).
//
// gateCmd is a thin orchestrator: parseGateRunArgs validates flags, task
// and config; renderDryRunPlan prints the plan (no preflight: it writes
// nothing); execAndReport preflights binaries then runs the gauntlet.
// Behavior, messages and exit codes are unchanged.
func (c CLI) gateCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task> | regulus gate evaluate ... | regulus gate verdict <task>")
		return 2
	}
	switch args[0] {
	case "evaluate":
		return c.gateEvaluateCmd(args[1:])
	case "verdict":
		return c.gateVerdictCmd(args[1:])
	case "run":
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus gate run [--profile NAME] [--dry-run] <task> | regulus gate evaluate ... | regulus gate verdict <task>")
		return 2
	}
	plan, code, ok := c.parseGateRunArgs(args[1:])
	if !ok {
		return code
	}
	if plan.dryRun {
		return c.renderDryRunPlan(plan)
	}
	return c.execAndReport(plan)
}

// crapCmd implements `regulus crap --profile <coverprofile> [--max N]
// [--max-critical C] [--max-standard S] [--top N] [--hotspot-report P]
// [--exceptions E]`: the layer-7 gauntlet gate (PRD R-G2) — Uncle Bob's
// CRAP formula over a go cover profile. Core routes fail at
// --max-critical, the rest at --max-standard; the global --max stays as
// the ceiling. Waived functions (docs/quality-exceptions.md) discount
// every Max: listed but never failing.
func (c CLI) crapCmd(args []string) int {
	fs := flag.NewFlagSet("crap", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	profile := fs.String("profile", "", "go cover profile path (required)")
	max := fs.Float64("max", 30, "fail if the effective (waivers discounted) max CRAP exceeds this ceiling")
	maxCrit := fs.Float64("max-critical", 30, "fail if any core-route function's CRAP exceeds this (cmd/*, internal/decisions|gates|receipts|evidence/*)")
	maxStd := fs.Float64("max-standard", 50, "fail if any off-core function's CRAP exceeds this")
	top := fs.Int("top", 10, "how many worst functions to print")
	hotspot := fs.String("hotspot-report", "", "write JSON hotspot report {generated_at, profile, profile_sha256, top, functions} to this path")
	exceptions := fs.String("exceptions", "docs/quality-exceptions.md", "waiver file (file:func motivo expira aprueba); waived functions discount the Max")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus crap --profile <coverprofile> [--max N] [--max-critical C] [--max-standard S] [--top N] [--hotspot-report P] [--exceptions E]")
		return 2
	}
	rep, err := crap.Analyze(c.dir(), *profile)
	if err != nil {
		fmt.Fprintf(c.Stderr, "crap: %v\n", err)
		return 1
	}
	waived := loadWaivers(c.dir(), *exceptions)
	full, short, when := sourceInfo(c.dir(), *profile)
	sha := short
	if sha == "" {
		sha = "unknown"
	}
	fmt.Fprintf(c.Stdout, "source: %s sha256:%s generated_at:%s\n", *profile, sha, when)
	var hot []hotspotFunc
	for i, f := range rep.Functions {
		if i >= *top {
			break
		}
		tier := auditTierFor(f, *maxCrit, *maxStd, *max)
		if waived[crap.WaiverKey(f.File, f.Name)] {
			tier = "waived"
		}
		suffix := ""
		if tier == "waived" {
			suffix = " [waived]"
		}
		fmt.Fprintf(c.Stdout, "%s:%d %s (complexity %d, coverage %s) CRAP %.2f%s\n",
			f.File, f.StartLine, f.Name, f.Complexity, coverageLabel(f), f.CRAP, suffix)
		hot = append(hot, hotspotFunc{File: f.File, Func: f.Name, CX: f.Complexity, Coverage: coveragePtr(f), CRAP: f.CRAP, Tier: tier})
	}
	if *hotspot != "" {
		if err := writeHotspotReport(*hotspot, *profile, full, *top, hot); err != nil {
			fmt.Fprintf(c.Stderr, "crap: %v\n", err)
			return 1
		}
	}
	eff := rep.EffectiveMax(waived)
	critMax, stdMax := rep.TierMaxima(waived)
	fmt.Fprintf(c.Stdout, "CRAP max: %.2f\n", eff)
	if critMax > *maxCrit {
		fmt.Fprintf(c.Stderr, "crap: critical max CRAP %.2f exceeds --max-critical %.2f — cover or split the worst core-route functions\n", critMax, *maxCrit)
		return 1
	}
	if stdMax > *maxStd {
		fmt.Fprintf(c.Stderr, "crap: standard max CRAP %.2f exceeds --max-standard %.2f — cover or split the worst functions\n", stdMax, *maxStd)
		return 1
	}
	if eff > *max {
		fmt.Fprintf(c.Stderr, "crap: max CRAP %.2f exceeds %.2f — cover or split the worst functions\n", eff, *max)
		return 1
	}
	return 0
}
