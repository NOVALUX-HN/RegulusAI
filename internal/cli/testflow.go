// `regulus testflow run` (F15.4/NOV-125): smoke, focused and full
// execution of the real Go test suite through the centralized
// executor. Thin CLI: selection, printing and exit code live here;
// discovery, execution, classification and the verdict live in
// internal/testflow (unit-tested without spawning anything).
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"regulus-ai/internal/testflow"
)

func testflowUsage() string {
	return "usage: regulus testflow run --mode smoke|focused|full [--packages a,b] [--skip a,b] [--save P] [--json] [--timeout-s N]"
}

// testflowCmd implements `regulus testflow run`. Exit 0 only on a
// green verdict; 1 on any FAIL/BLOCKED/NOT_RUN, unallowed skip or
// incomplete coverage; 2 on usage errors. A partial run never
// reports green.
func (c CLI) testflowCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, testflowUsage())
		return 0
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(c.Stderr, testflowUsage())
		return 2
	}
	fs := flag.NewFlagSet("testflow run", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	mode := fs.String("mode", "smoke", "smoke | focused | full")
	pkgs := fs.String("packages", "", "comma-separated packages (focused mode)")
	skip := fs.String("skip", "", "comma-separated allowed skips")
	save := fs.String("save", "", "write versioned JSON report to this path")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	timeoutS := fs.Int("timeout-s", 0, "reserved: per-package budget override (not yet honored, must stay 0)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *timeoutS != 0 {
		fmt.Fprintln(c.Stderr, testflowUsage())
		return 2
	}
	split := func(s string) []string {
		var out []string
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	req := testflow.Request{Mode: *mode, Packages: split(*pkgs), Skipped: split(*skip), Save: *save, Regulus: c.Version}
	ctx := context.Background()
	rep, ok := testflow.Run(ctx, req)
	if *asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "testflow: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
	} else {
		fmt.Fprintf(c.Stdout, "testflow %s N=%d\n", rep.Mode, len(rep.Discovered))
		for _, r := range rep.Checks {
			fmt.Fprintf(c.Stdout, "  %-28s %s %s\n", r.Package, r.Outcome, shortTestflowDetail(r.Detail))
		}
		for _, r := range rep.Results {
			fmt.Fprintf(c.Stdout, "  %-28s %s %s\n", r.Package, r.Outcome, shortTestflowDetail(r.Detail))
		}
		if len(rep.Skipped) > 0 {
			fmt.Fprintf(c.Stdout, "  skipped (allowed): %s\n", strings.Join(rep.Skipped, ", "))
		}
	}
	if !ok {
		return 1
	}
	return 0
}

func shortTestflowDetail(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}
