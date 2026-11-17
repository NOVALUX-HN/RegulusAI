package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["impact"] = func(c CLI, args []string) int { return c.impactCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.8 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds.
func (c CLI) impactCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactCmd(args)
}

func (c CLI) impactInventoryCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactInventoryCmd(args)
}

func (c CLI) impactGraphCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactGraphCmd(args)
}

func (c CLI) impactAffectCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactAffectCmd(args)
}

func (c CLI) impactSuiteCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactSuiteCmd(args)
}

func (c CLI) impactCyclesCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactCyclesCmd(args)
}

func (c CLI) impactBreakingCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactBreakingCmd(args)
}

func (c CLI) impactBenchmarkCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ImpactBenchmarkCmd(args)
}
