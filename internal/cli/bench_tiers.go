package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F11.3 extraction). bench.go
// dispatches unchanged.
func (c CLI) benchTiersCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchTiersCmd(args)
}

func (c CLI) benchTierPlanCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchTierPlanCmd(args)
}

func (c CLI) benchTierReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchTierReportCmd(args)
}
