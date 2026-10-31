package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"regulus-ai/internal/almanac"
)

// memoryVerifyCmd implements `regulus memory verify <id> --receipt <path>`:
// mark an observation VERIFIED against cited evidence (or ACCEPTED with
// --accept). Verification needs evidence; agents never self-promote.
func (c CLI) memoryVerifyCmd(args []string) int {
	fs := flag.NewFlagSet("memory verify", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	receipt := fs.String("receipt", "", "evidence file backing the verification (required)")
	by := fs.String("by", "", "who verified (default: reviewer)")
	accept := fs.Bool("accept", false, "promote to ACCEPTED instead of VERIFIED")
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 || *receipt == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory verify <id> --receipt <path> [--by <who>] [--accept]")
		return 2
	}
	id := pos[0]
	ref := *receipt
	if !filepath.IsAbs(ref) {
		ref = filepath.Join(".", ref)
	}
	if fi, err := os.Stat(ref); err != nil || fi.IsDir() {
		fmt.Fprintf(c.Stderr, "memory verify: missing evidence %q\n", *receipt)
		return 1
	}
	who := *by
	if who == "" {
		who = "reviewer"
	}
	to := almanac.StatusVerified
	if *accept {
		to = almanac.StatusAccepted
	}
	current, err := almanac.EffectiveStatus(".", id)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory verify: %v\n", err)
		return 1
	}
	if current == to {
		fmt.Fprintf(c.Stdout, "%s is already %s\n", id, to)
		return 0
	}
	note := "verified against " + *receipt
	if *accept {
		note = "accepted with evidence " + *receipt
	}
	if err := almanac.RecordStatus(".", id, to, who, note, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory verify: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "%s: %s -> %s by %s\n", id, current, to, who)
	return 0
}

// memoryRejectCmd implements `regulus memory reject <id> --reason ...`:
// refuse knowledge with a recorded reason. Terminal, like all goodbyes.
func (c CLI) memoryRejectCmd(args []string) int {
	fs := flag.NewFlagSet("memory reject", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	reason := fs.String("reason", "", "why this knowledge is refused (required)")
	by := fs.String("by", "", "who rejects (default: reviewer)")
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 || *reason == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus memory reject <id> --reason <text> [--by <who>]")
		return 2
	}
	id := pos[0]
	who := *by
	if who == "" {
		who = "reviewer"
	}
	current, err := almanac.EffectiveStatus(".", id)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory reject: %v\n", err)
		return 1
	}
	if err := almanac.RecordStatus(".", id, almanac.StatusRejected, who, *reason, time.Now().UTC()); err != nil {
		fmt.Fprintf(c.Stderr, "memory reject: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "%s: %s -> REJECTED by %s\n", id, current, who)
	return 0
}

// memoryStatusCmd implements `regulus memory status <id>`: effective state
// plus its audit trail.
func (c CLI) memoryStatusCmd(args []string) int {
	fs := flag.NewFlagSet("memory status", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory status <id>")
		return 2
	}
	id := pos[0]
	st, err := almanac.EffectiveStatus(".", id)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory status: %v\n", err)
		return 1
	}
	sys := ""
	if all, err := almanac.All("."); err == nil {
		for _, o := range all {
			if o.ID == id {
				sys = almanac.SystemOf(o.Type)
			}
		}
	}
	fmt.Fprintf(c.Stdout, "%s: %s (system %s)\n", id, st, sys)
	hist, err := almanac.StatusHistory(".", id)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory status: %v\n", err)
		return 1
	}
	for _, h := range hist {
		fmt.Fprintf(c.Stdout, "  %s %s -> %s by %s: %s\n", h.At.Format(time.RFC3339), h.From, h.To, h.By, h.Note)
	}
	return 0
}

// memoryCheckCmd implements `regulus memory check`: live contradictions,
// retired knowledge still presented as current, and expired-but-active
// observations. Exit 1 names every problem; exit 0 certifies clean.
func (c CLI) memoryCheckCmd(args []string) int {
	fs := flag.NewFlagSet("memory check", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory check")
		return 2
	}
	now := time.Now().UTC()
	problems := 0
	conflicts, err := almanac.Contradictions(".", now)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory check: %v\n", err)
		return 1
	}
	for _, cf := range conflicts {
		fmt.Fprintf(c.Stdout, "CONTRADICTION: %s conflicts_with %s (both active)\n", cf.A, cf.B)
		problems++
	}
	all, err := almanac.All(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory check: %v\n", err)
		return 1
	}
	sup, _ := almanac.SupersededIDs(".")
	inv, _ := almanac.InvalidatedIDs(".")
	for _, o := range all {
		st, err := almanac.EffectiveStatus(".", o.ID)
		if err != nil {
			continue
		}
		if almanac.IsExpired(&o, now) && st != almanac.StatusRejected && !sup[o.ID] && !inv[o.ID] {
			fmt.Fprintf(c.Stdout, "STALE: %s expired but reads %s\n", o.ID, st)
			problems++
		}
	}
	if problems == 0 {
		fmt.Fprintln(c.Stdout, "memory check: clean (no live contradictions, no stale-but-active knowledge)")
		return 0
	}
	fmt.Fprintf(c.Stdout, "memory check: %d problem(s)\n", problems)
	return 1
}
