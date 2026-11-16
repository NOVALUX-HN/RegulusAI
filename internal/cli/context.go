package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["context"] = func(c CLI, args []string) int { return c.contextCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.3 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds.
func (c CLI) contextCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ContextCmd(args)
}

func (c CLI) contextInventoryCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ContextInventoryCmd(args)
}

func (c CLI) contextSelectCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ContextSelectCmd(args)
}
