// Thin forwarders to internal/cmdx (F13.6). Registration stays
// via init so cli.go is untouched and the dispatch pin holds (the
// pin update itself is the parent's, like every F12 task).
package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["govern"] = func(c CLI, args []string) int { return c.governCmd(args) }
}

func (c CLI) governCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernCmd(args)
}

func (c CLI) governMatrixCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernMatrixCmd(args)
}

func (c CLI) governAuthorizeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernAuthorizeCmd(args)
}

func (c CLI) governRequestCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernRequestCmd(args)
}

func (c CLI) governApproveCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernApproveCmd(args)
}

func (c CLI) governCheckCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernCheckCmd(args)
}

func (c CLI) governShowCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernShowCmd(args)
}

func (c CLI) governListCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernListCmd(args)
}

func (c CLI) governAuditCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernAuditCmd(args)
}

func (c CLI) governPolicyCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).GovernPolicyCmd(args)
}
