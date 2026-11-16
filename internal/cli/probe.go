package cli

import (
	"io"

	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx. cli.go dispatches to probeCmd
// unchanged; cli no longer imports internal/probe for these paths.
func (c CLI) probeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ProbeCmd(args)
}

func (c CLI) probeContrastCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ProbeContrastCmd(args)
}

func (c CLI) probeComposeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ProbeComposeCmd(args)
}

func (c CLI) probeUsage(w io.Writer) {
	cmdx.New(c.Stdout, w, c.dir(), c.Version, c.runner()).ProbeUsage(w)
}
