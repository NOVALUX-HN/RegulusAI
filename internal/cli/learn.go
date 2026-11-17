package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["learn"] = func(c CLI, args []string) int { return c.learnCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.6 extraction). Registration
// stays via init so cli.go is untouched and the dispatch pin holds.
func (c CLI) learnCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnCmd(args)
}

func (c CLI) learnSaveCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnSaveCmd(args)
}

func (c CLI) learnSearchCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnSearchCmd(args)
}

func (c CLI) learnVerifyCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnVerifyCmd(args)
}

func (c CLI) learnInvalidateCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnInvalidateCmd(args)
}

func (c CLI) learnResolveCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnResolveCmd(args)
}

func (c CLI) learnMetricsCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).LearnMetricsCmd(args)
}
