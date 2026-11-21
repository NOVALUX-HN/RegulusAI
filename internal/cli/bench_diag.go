package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarder to internal/cmdx (F11.4 extraction). bench.go
// dispatches unchanged.
func (c CLI) benchDiagCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).BenchDiagCmd(args)
}
