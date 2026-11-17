// Thin forwarders to internal/cmdx (F13.5). Registration stays
// via init so cli.go is untouched and the dispatch pin holds (the
// pin update itself is the parent's, like every F12 task).
package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["failure"] = func(c CLI, args []string) int { return c.failureCmd(args) }
}

func (c CLI) failureCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).FailureCmd(args)
}

func (c CLI) failureClassifyCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).FailureClassifyCmd(args)
}

func (c CLI) failureDecideCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).FailureDecideCmd(args)
}

func (c CLI) failureStatusCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).FailureStatusCmd(args)
}

func (c CLI) failureExplainCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).FailureExplainCmd(args)
}
