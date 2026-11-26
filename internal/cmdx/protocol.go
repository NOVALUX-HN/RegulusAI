// `regulus protocol` - the command that makes internal/protocol real.
//
// It prints the engineering protocol as it is actually held: which practices
// block, which only advise, and which are a human call. It refuses to print a
// table that fails validation, so a broken protocol is a failure and never a
// comfortable default.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"regulus-ai/internal/protocol"
)

// ProtocolCmd implements `regulus protocol`.
func (io IO) ProtocolCmd(args []string) int {
	fs := flag.NewFlagSet("protocol", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the protocol as JSON")
	only := fs.String("enforcement", "", "filter by enforced|advisory|judgement")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus protocol [--json] [--enforcement enforced|advisory|judgement]")
		return 2
	}

	all := protocol.All()
	if errs := protocol.Validate(all); len(errs) != 0 {
		for _, e := range errs {
			fmt.Fprintf(io.Stderr, "protocol: %s\n", e)
		}
		return 1
	}

	selected := all
	if *only != "" {
		selected = nil
		for _, p := range all {
			if string(p.Enforcement) == *only {
				selected = append(selected, p)
			}
		}
		if len(selected) == 0 {
			fmt.Fprintf(io.Stderr, "protocol: no practice is held as %q\n", *only)
			return 2
		}
	}

	if *asJSON {
		data, err := json.MarshalIndent(selected, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "protocol: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(data))
		return 0
	}

	counts := map[protocol.Enforcement]int{}
	for _, p := range all {
		counts[p.Enforcement]++
	}
	if *only == "" {
		fmt.Fprintf(io.Stdout, "engineering protocol: %d enforced, %d advisory, %d judgement\n\n",
			counts[protocol.Enforced], counts[protocol.Advisory], counts[protocol.Judgement])
	} else {
		// A filtered view reports the filtered count. Printing the totals here
		// made the header disagree with the rows underneath it.
		fmt.Fprintf(io.Stdout, "engineering protocol: showing %d of %d held as %s\n\n",
			len(selected), len(all), *only)
	}

	tw := tabwriter.NewWriter(io.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tHELD AS\tWHERE\tNOTE")
	for _, p := range selected {
		where := "-"
		if p.File != "" {
			where = p.File
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, p.Enforcement, where, p.Note)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(io.Stderr, "protocol: %v\n", err)
		return 1
	}
	if *only == "" {
		fmt.Fprintf(io.Stdout, "\n%s\n",
			strings.TrimSpace("A judgement entry is not an oversight: it is a practice with no\n"+
				"deterministic oracle. docs/engineering-protocol.md explains each one."))
	}
	return 0
}
