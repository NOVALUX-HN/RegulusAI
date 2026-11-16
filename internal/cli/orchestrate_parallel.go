package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F12.2 extraction). The umbrella
// orchestrateCmd lives in cmdx; these stay reachable through it.
func (c CLI) orchestrateParallelCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateParallelCmd(args)
}

func (c CLI) orchestrateOrphansCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).OrchestrateOrphansCmd(args)
}
