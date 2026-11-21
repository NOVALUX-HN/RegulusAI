// Thin forwarder to internal/cmdx (F13.3). Registration stays
// via init so cli.go is untouched and the dispatch pin holds (the
// pin update itself is the parent's, like every F12 task).
package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["execute"] = func(c CLI, args []string) int { return c.executeCmd(args) }
}

func (c CLI) executeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteCmd(args)
}

func (c CLI) executeRunCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteRunCmd(args)
}

func (c CLI) executeStatusCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteStatusCmd(args)
}

func (c CLI) executeApproveCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteApproveCmd(args)
}

func (c CLI) executeCancelCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteCancelCmd(args)
}

func (c CLI) executeBenchCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecuteBenchCmd(args)
}
