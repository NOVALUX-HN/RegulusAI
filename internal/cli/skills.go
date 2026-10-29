package cli

import (
	"fmt"

	"regulus-ai/internal/skills"
)

// skillsCmd implements `regulus skills list`: the trigger registry —
// every skill with its one-line trigger, so agents (and the context
// hook) know what each skill fires on.
func (c CLI) skillsCmd(args []string) int {
	if len(args) == 0 || args[0] != "list" || len(args) != 1 {
		if len(args) > 0 && args[0] != "list" {
			fmt.Fprintf(c.Stderr, "skills: unknown subcommand %q\n", args[0])
		}
		fmt.Fprintln(c.Stderr, "usage: regulus skills list")
		return 2
	}
	all, err := skills.List(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "skills list: %v\n", err)
		return 1
	}
	if len(all) == 0 {
		fmt.Fprintln(c.Stdout, "(no skills registered)")
		return 0
	}
	for _, s := range all {
		if s.Description != "" {
			fmt.Fprintf(c.Stdout, "%s — %s\n", s.Name, s.Description)
		} else {
			fmt.Fprintln(c.Stdout, s.Name)
		}
	}
	return 0
}
