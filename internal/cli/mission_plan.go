// Thin forwarders to internal/cmdx (F13.2). No new top-level
// command: plan|replan|explain ride the existing `mission`
// dispatch, so the dispatch pin holds and this file imports only
// cmdx.
package cli

import (
	"regulus-ai/internal/cmdx"
)

func (c CLI) missionPlanCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionPlanCmd(args)
}

func (c CLI) missionReplanCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionReplanCmd(args)
}

func (c CLI) missionExplainCmd(args []string) int {
	return cmdx.New(c.Stdout, c.Stderr, c.dir(), c.Version, c.runner()).MissionExplainCmd(args)
}
