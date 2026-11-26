// `regulus usefulness` — the command that makes the F18.3 measurement real.
//
// It reads a batch of reuse events and reports the comparison against the
// no-reuse baseline. Where the batch comes from is the caller's business: this
// command does not invent a recorder or a second store, it computes the number
// the phase asks for and refuses to claim a comparison when one side is empty.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"regulus-ai/internal/almanac"
)

// UsefulnessCmd implements `regulus usefulness`.
func (io IO) UsefulnessCmd(args []string) int {
	fs := flag.NewFlagSet("usefulness", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	eventsPath := fs.String("events", "", "path to a JSON array of reuse events (required)")
	asJSON := fs.Bool("json", false, "print the summary as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *eventsPath == "" {
		fmt.Fprintln(io.Stderr, "usage: regulus usefulness --events <file.json> [--json]")
		return 2
	}

	data, err := os.ReadFile(*eventsPath)
	if err != nil {
		fmt.Fprintf(io.Stderr, "usefulness: %v\n", err)
		return 1
	}
	var events []almanac.ReuseEvent
	if err := json.Unmarshal(data, &events); err != nil {
		fmt.Fprintf(io.Stderr, "usefulness: %s is not a reuse-event array: %v\n", *eventsPath, err)
		return 1
	}
	u := almanac.Summarize(events)

	if *asJSON {
		out, merr := json.MarshalIndent(u, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "usefulness: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(out))
		return 0
	}

	fmt.Fprintf(io.Stdout, "usefulness runs=%d reused=%d baseline=%d reuse_rate=%d%%\n",
		u.Runs, u.Reused, u.Baseline, u.ReuseRatePct)
	fmt.Fprintf(io.Stdout, "  success with reuse:    %d/%d (%d%%)\n", u.SuccessWithReuse, u.Reused, u.SuccessRateWithPct)
	fmt.Fprintf(io.Stdout, "  success without reuse: %d/%d (%d%%)\n", u.SuccessWithoutReuse, u.Baseline, u.SuccessRateWithoutPct)
	if !u.Comparable {
		fmt.Fprintln(io.Stdout, "  comparison: not claimed (one group is empty, so there is no baseline)")
		return 0
	}
	fmt.Fprintf(io.Stdout, "  mean retries with reuse:    %.3f\n", float64(u.MeanRetriesWithMilli)/1000)
	fmt.Fprintf(io.Stdout, "  mean retries without reuse: %.3f\n", float64(u.MeanRetriesWithoutMilli)/1000)
	fmt.Fprintf(io.Stdout, "  retry delta (with - without): %.3f\n", float64(u.RetryDeltaMilli)/1000)
	return 0
}
