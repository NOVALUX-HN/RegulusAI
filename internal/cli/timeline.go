package cli

import (
	"regulus-ai/internal/cmdx"
)

// Thin forwarders to internal/cmdx (F11/F12 extraction): cli keeps
// the dispatch surface byte-identical while the implementations live
// in cmdx, so cli no longer imports observability for these paths.

func (c CLI) traceEmitCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).TraceEmitCmd(args)
}

func (c CLI) traceTimelineCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).TraceTimelineCmd(args)
}

func (c CLI) tracePruneExecCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).TracePruneExecCmd(args)
}
