package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["quality"] = func(c CLI, args []string) int { return c.qualityCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.4 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds.
func (c CLI) qualityCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).QualityCmd(args)
}

func (c CLI) qualityBaselineCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).QualityBaselineCmd(args)
}

func (c CLI) qualityCheckCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).QualityCheckCmd(args)
}

func (c CLI) qualityAlertsCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).QualityAlertsCmd(args)
}
