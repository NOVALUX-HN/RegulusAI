package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarder to internal/cmdx (F11.7 extraction). bench.go
// dispatches unchanged.
func (c CLI) benchHostCapabilitiesCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchHostCapabilitiesCmd(args)
}
