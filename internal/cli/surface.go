package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/surface"
)

// surfaceCmd implements `regulus surface propose|list|apply` — the
// mechanical gate for harness-surface edits (PRD R-E3).
func (c CLI) surfaceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus surface propose --target <f> --content <c> --reason <r> | list | apply --id <id> --guardian <name>")
		return 2
	}
	switch args[0] {
	case "propose":
		fs := flag.NewFlagSet("surface propose", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		target := fs.String("target", "", "the surface file to edit (required; must be in the whitelist)")
		content := fs.String("content", "", "the text to append (required)")
		reason := fs.String("reason", "", "why — cite the lesson or evidence (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *target == "" || *content == "" || *reason == "" || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus surface propose --target <f> --content <c> --reason <r>")
			return 2
		}
		e, err := surface.Propose(".", *target, *content, *reason, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface propose: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "proposed: %s → %s\n", e.ID, e.Target)
		fmt.Fprintln(c.Stdout, "guardian check, then: regulus surface apply --id "+e.ID+" --guardian <name>")
		return 0
	case "list":
		edits, err := surface.List(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface list: %v\n", err)
			return 1
		}
		if len(edits) == 0 {
			fmt.Fprintln(c.Stdout, "(no surface edits)")
			return 0
		}
		for _, e := range edits {
			fmt.Fprintf(c.Stdout, "%s %s %s (%s)%s\n", e.ID, e.Status, e.Target, e.Reason, guardianSuffix(e))
		}
		return 0
	case "apply":
		fs := flag.NewFlagSet("surface apply", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		id := fs.String("id", "", "the proposal id (required)")
		guardian := fs.String("guardian", "", "who validated (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *id == "" || *guardian == "" || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus surface apply --id <id> --guardian <name>")
			return 2
		}
		e, err := surface.Apply(".", *id, *guardian, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(c.Stderr, "surface apply: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "applied: %s → %s (guardian %s)\n", e.ID, e.Target, e.Guardian)
		return 0
	default:
		fmt.Fprintf(c.Stderr, "surface: unknown subcommand %q\n", args[0])
		return 2
	}
}

func guardianSuffix(e surface.Edit) string {
	if e.Guardian == "" {
		return ""
	}
	return " — guardian " + e.Guardian
}
