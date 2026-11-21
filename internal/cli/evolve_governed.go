package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F12.7 governed cycle). The dispatch
// pin and usage line in library.go/cli.go stay with the parent: these
// forwarders only expose the cycle verbs through the existing
// `evolve validation` entry point.
func (c CLI) evolveValidationCycleDeclareCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleDeclareCmd(args)
}

func (c CLI) evolveValidationCycleProposeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleProposeCmd(args)
}

func (c CLI) evolveValidationCycleLinkCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleLinkCmd(args)
}

func (c CLI) evolveValidationCycleCompareCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleCompareCmd(args)
}

func (c CLI) evolveValidationCycleReviewCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleReviewCmd(args)
}

func (c CLI) evolveValidationCycleApproveCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleApproveCmd(args)
}

func (c CLI) evolveValidationCycleCanaryCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleCanaryCmd(args)
}

func (c CLI) evolveValidationCycleMonitorCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleMonitorCmd(args)
}

func (c CLI) evolveValidationCyclePromoteCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCyclePromoteCmd(args)
}

func (c CLI) evolveValidationCycleRollbackCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleRollbackCmd(args)
}

func (c CLI) evolveValidationCycleReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveCycleReportCmd(args)
}
