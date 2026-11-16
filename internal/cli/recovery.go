package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["recovery"] = func(c CLI, args []string) int { return c.recoveryCmd(args) }
}

// Thin forwarders to internal/cmdx (F12.5). Registration stays via
// init so cli.go is untouched and the dispatch pin holds (the pin
// update itself is the parent's, like every F12 task).
func (c CLI) recoveryCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryCmd(args)
}

func (c CLI) recoveryCheckpointCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryCheckpointCmd(args)
}

func (c CLI) recoveryResumeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryResumeCmd(args)
}

func (c CLI) recoveryRetryCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryRetryCmd(args)
}

func (c CLI) recoveryStatusCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryStatusCmd(args)
}

func (c CLI) recoveryRollbackCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryRollbackCmd(args)
}

func (c CLI) recoveryCancelCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).RecoveryCancelCmd(args)
}
