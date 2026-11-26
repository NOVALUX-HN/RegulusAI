package cli

import "regulus-ai/internal/cmdx"

func init() {
	cliDispatch["exec-backend"] = func(c CLI, args []string) int {
		return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).ExecBackendCmd(args)
	}
}
