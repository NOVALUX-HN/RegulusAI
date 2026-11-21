package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F11.2 extraction). bench.go
// dispatches unchanged; cli no longer imports bench/evolve/executil
// for these paths.
func (c CLI) benchLabCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchLabCmd(args)
}

func (c CLI) benchLabAgentsCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchLabAgentsCmd(args)
}

func (c CLI) benchLabRunCmd(mode string, args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchLabRunCmd(mode, args)
}

func (c CLI) benchLabCompareCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchLabCompareCmd(args)
}

func (c CLI) benchLabReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchLabReportCmd(args)
}
