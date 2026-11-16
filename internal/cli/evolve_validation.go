package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F11.6 extraction). evolveCmd in
// library.go dispatches unchanged.
func (c CLI) evolveValidationCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationCmd(args)
}

func (c CLI) evolveValidationDeclareCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationDeclareCmd(args)
}

func (c CLI) evolveValidationRunCmd(args []string, holdout bool) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationRunCmd(args, holdout)
}

func (c CLI) evolveValidationControlCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationControlCmd(args)
}

func (c CLI) evolveValidationDiscardCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationDiscardCmd(args)
}

func (c CLI) evolveValidationLinkCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationLinkCmd(args)
}

func (c CLI) evolveValidationCompareCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationCompareCmd(args)
}

func (c CLI) evolveValidationFinalizeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationFinalizeCmd(args)
}

func (c CLI) evolveValidationReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).EvolveValidationReportCmd(args)
}
