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
	"io"
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
	req, asJSON, code, done := parseTestflowArgs(args, c.Stdout, c.Stderr, c.Version)
	if done {
		return code
	}
	rep, ok := testflow.Run(context.Background(), req)
	if rc := printTestflowReport(c.Stdout, c.Stderr, rep, asJSON); rc != 0 {
		return rc
	}
	if !ok {
		return 1
	}
	return 0
}

// splitCSV splits a comma-separated flag value, trimming spaces.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseTestflowArgs validates `testflow run` flags without executing.
// Returns done=true when the caller must return code immediately
// (help or usage error, message already printed).
func parseTestflowArgs(args []string, stdout, stderr io.Writer, version string) (testflow.Request, bool, int, bool) {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, testflowUsage())
		return testflow.Request{}, false, 0, true
	}
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, testflowUsage())
		return testflow.Request{}, false, 2, true
	}
	fs := flag.NewFlagSet("testflow run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "smoke", "smoke | focused | full")
	pkgs := fs.String("packages", "", "comma-separated packages (focused mode)")
	skip := fs.String("skip", "", "comma-separated allowed skips")
	save := fs.String("save", "", "write versioned JSON report to this path")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	timeoutS := fs.Int("timeout-s", 0, "reserved")
	if err := fs.Parse(args[1:]); err != nil {
		return testflow.Request{}, false, 2, true
	}
	if fs.NArg() != 0 || *timeoutS != 0 {
		fmt.Fprintln(stderr, testflowUsage())
		return testflow.Request{}, false, 2, true
	}
	req := testflow.Request{Mode: *mode, Packages: splitCSV(*pkgs), Skipped: splitCSV(*skip), Save: *save, Regulus: version}
	return req, *asJSON, 0, false
}

// printTestflowReport prints one report as JSON or text.
// Returns 1 only on JSON marshal failure, never on verdict.
func printTestflowReport(stdout, stderr io.Writer, rep testflow.Report, asJSON bool) int {
	if asJSON {
		raw, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "testflow: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(raw))
		return 0
	}
	fmt.Fprintf(stdout, "testflow %s N=%d\n", rep.Mode, len(rep.Discovered))
	for _, r := range rep.Checks {
		fmt.Fprintf(stdout, "  %-28s %s %s\n", r.Package, r.Outcome, shortTestflowDetail(r.Detail))
	}
	for _, r := range rep.Results {
		fmt.Fprintf(stdout, "  %-28s %s %s\n", r.Package, r.Outcome, shortTestflowDetail(r.Detail))
	}
	if len(rep.Skipped) > 0 {
		fmt.Fprintf(stdout, "  skipped (allowed): %s\n", strings.Join(rep.Skipped, ", "))
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
