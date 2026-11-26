// `regulus duplication` - the command that makes internal/duplication real.
//
// Report-only by design: it prints findings and exits 0 whether it finds clones or
// not. A duplication budget is a judgement call, and turning this into a gate would
// be exactly the arbitrary threshold F20 excludes.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"text/tabwriter"

	"regulus-ai/internal/duplication"
)

// DuplicationCmd implements `regulus duplication`.
func (io IO) DuplicationCmd(args []string) int {
	fs := flag.NewFlagSet("duplication", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	root := fs.String("root", ".", "directory to analyse")
	minTokens := fs.Int("min-tokens", duplication.DefaultMinTokens, "normalised window size")
	includeTests := fs.Bool("include-tests", false, "include _test.go files")
	limit := fs.Int("limit", 50, "maximum pairs to print")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, "usage: regulus duplication [--json] [--root <dir>] [--min-tokens N] [--include-tests] [--limit N]")
		return 2
	}

	rep, err := duplication.Analyze(*root, duplication.Options{
		MinTokens: *minTokens, IncludeTests: *includeTests, Limit: *limit,
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "duplication: %v\n", err)
		return 1
	}

	if *asJSON {
		data, merr := json.MarshalIndent(rep, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "duplication: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(data))
		return 0
	}

	fmt.Fprintf(io.Stdout, "structural duplication: %d pairs across %d functions in %d files (window %d tokens)\n",
		len(rep.Clones), rep.Functions, rep.Files, rep.MinTokens)
	if len(rep.Clones) == 0 {
		fmt.Fprintln(io.Stdout, "no structural clone found at this window")
		return 0
	}
	fmt.Fprintln(io.Stdout)
	tw := tabwriter.NewWriter(io.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "A\tB\tWINDOWS")
	for _, c := range rep.Clones {
		fmt.Fprintf(tw, "%s:%d %s\t%s:%d %s\t%d\n",
			c.A.File, c.A.Line, c.A.Func, c.B.File, c.B.Line, c.B.Func, c.Windows)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(io.Stderr, "duplication: %v\n", err)
		return 1
	}
	if rep.Truncated {
		fmt.Fprintf(io.Stdout, "\n(truncated at --limit %d: more pairs exist)\n", *limit)
	}
	fmt.Fprintf(io.Stdout, "\nWINDOWS counts overlapping windows, so it measures how much code the pair\n"+
		"shares, not how many distinct blocks it repeats. Report only: this command\n"+
		"never blocks a gate. See docs/quality-tooling-decisions.md.\n")
	return 0
}
