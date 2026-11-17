package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["verify"] = func(c CLI, args []string) int { return c.verifyCmd(args) }
}

// Thin forwarders to internal/cmdx (F13.4 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds
// (the parent updates run_split_test.go).
func (c CLI) verifyCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyCmd(args)
}

func (c CLI) verifyPlanCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyPlanCmd(args)
}

func (c CLI) verifyRunCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyRunCmd(args)
}

func (c CLI) verifyReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyReportCmd(args)
}

func (c CLI) verifyMatrixCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyMatrixCmd(args)
}

func (c CLI) verifyRerunCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).VerifyRerunCmd(args)
}
