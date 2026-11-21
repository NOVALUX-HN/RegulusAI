// Thin forwarders to internal/cmdx (F13.1). Registration stays
// via init so cli.go is untouched and the dispatch pin holds (the
// pin update itself is the parent's, like every F12 task).
package cli

import (
	"regulus-ai/internal/cmdx"
)

func init() {
	cliDispatch["mission"] = func(c CLI, args []string) int { return c.missionCmd(args) }
}

func (c CLI) missionCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionCmd(args)
}

func (c CLI) missionCreateCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionCreateCmd(args)
}

func (c CLI) missionShowCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionShowCmd(args)
}

func (c CLI) missionListCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionListCmd(args)
}

func (c CLI) missionStatusCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionStatusCmd(args)
}

func (c CLI) missionAdvanceCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionAdvanceCmd(args)
}

func (c CLI) missionPauseCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionPauseCmd(args)
}

func (c CLI) missionResumeCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionResumeCmd(args)
}

func (c CLI) missionCancelCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionCancelCmd(args)
}

func (c CLI) missionLinkCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionLinkCmd(args)
}

func (c CLI) missionReopenCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionReopenCmd(args)
}

func (c CLI) missionReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionReportCmd(args)
}

func (c CLI) missionVerifyReportCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionVerifyReportCmd(args)
}
