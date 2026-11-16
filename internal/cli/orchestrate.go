package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["orchestrate"] = func(c CLI, args []string) int { return c.orchestrateCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.1 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds.
func (c CLI) orchestrateCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateCmd(args)
}

func (c CLI) orchestrateAgentsCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateAgentsCmd(args)
}

func (c CLI) orchestrateDecideCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateDecideCmd(args)
}

func (c CLI) orchestrateEvaluateCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateEvaluateCmd(args)
}
