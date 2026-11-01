package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/envfacts"
)

// memoryMineCmd implements `regulus memory mine [--domain <d>] [--apply]`:
// the M4 envfacts miner. It folds gauntlet receipts (same gate failing)
// and session traces (same tool erroring) into per-domain series; a
// series failing in ≥3 sessions is mined as a domain lesson (flowing to
// the existing per-domain injection with zero model calls), and a mined
// series whose recent window runs clean is discarded as contradicted.
// Dry run by default; --apply admits and discards.
func (c CLI) memoryMineCmd(args []string) int {
	fs := flag.NewFlagSet("memory mine", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "only mine facts of this domain (default: every domain)")
	apply := fs.Bool("apply", false, "admit mined facts and discard contradicted ones (default: dry run)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus memory mine [--domain <d>] [--apply]")
		return 2
	}
	series, stats, err := mineSeries(*domain)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	decisions, err := envfacts.Plan(".", series)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stderr, trustBanner)
	if !*apply {
		printMinePlan(c, decisions, stats)
		return 0
	}
	results, err := envfacts.Apply(".", decisions, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	printMineResults(c, results, stats)
	if mineWrote(results) {
		c.refreshIndexBestEffort("mine")
	}
	return 0
}

// mineSeries scans receipts and traces and keeps the requested domain.
func mineSeries(domain string) ([]envfacts.Series, envfacts.Stats, error) {
	rs, rst, err := envfacts.ReceiptSeries(".")
	if err != nil {
		return nil, envfacts.Stats{}, err
	}
	ts, tst, err := envfacts.TraceSeries(".")
	if err != nil {
		return nil, envfacts.Stats{}, err
	}
	stats := envfacts.Stats{
		Receipts:             rst.Receipts,
		ReceiptsUnattributed: rst.ReceiptsUnattributed,
		TraceSessions:        tst.TraceSessions,
		TraceUnattributed:    tst.TraceUnattributed,
	}
	series := append(rs, ts...)
	if domain == "" {
		return series, stats, nil
	}
	kept := series[:0]
	for _, s := range series {
		if s.Key.Domain == domain {
			kept = append(kept, s)
		}
	}
	return kept, stats, nil
}

// printMinePlan renders the dry run: what --apply would write.
func printMinePlan(c CLI, decisions []envfacts.Decision, stats envfacts.Stats) {
	counts := printDecisions(c, decisions)
	fmt.Fprintf(c.Stdout, "mine: %d to mine, %d to discard, %d skipped (dry run — pass --apply to write)\n",
		counts[envfacts.ActionMine], counts[envfacts.ActionDiscard], counts[envfacts.ActionSkip])
	printMineScan(c, stats)
}

// printMineResults renders an applied run with the written ids.
func printMineResults(c CLI, results []envfacts.Result, stats envfacts.Stats) {
	counts := map[string]int{}
	for _, r := range results {
		counts[r.Decision.Action]++
		switch r.Decision.Action {
		case envfacts.ActionMine:
			fmt.Fprintf(c.Stdout, "mined %s %s\n", r.FactID, r.Decision.Series.Key.Topic())
		case envfacts.ActionDiscard:
			fmt.Fprintf(c.Stdout, "discarded %s (note %s)\n",
				strings.Join(r.Decision.LiveFactIDs, ", "), r.NoteID)
		default:
			fmt.Fprintf(c.Stdout, "skip %s: %s\n", r.Decision.Series.Key, r.Decision.Reason)
		}
	}
	fmt.Fprintf(c.Stdout, "mine: %d mined, %d discarded, %d skipped\n",
		counts[envfacts.ActionMine], counts[envfacts.ActionDiscard], counts[envfacts.ActionSkip])
	printMineScan(c, stats)
}

// printDecisions prints one dry-run line per decision and returns
// per-action counts.
func printDecisions(c CLI, decisions []envfacts.Decision) map[string]int {
	counts := map[string]int{}
	for _, d := range decisions {
		counts[d.Action]++
		switch d.Action {
		case envfacts.ActionMine:
			failing := envfacts.FailingSessions(d.Series)
			fmt.Fprintf(c.Stdout, "would-mine %s — %s (%d failing sessions, confidence %s)\n",
				d.Series.Key.Topic(), envfacts.CueFor(d.Series.Key, failing),
				failing, envfacts.ConfidenceFor(failing))
		case envfacts.ActionDiscard:
			fmt.Fprintf(c.Stdout, "would-discard %s — %s\n",
				strings.Join(d.LiveFactIDs, ", "), d.Reason)
		default:
			fmt.Fprintf(c.Stdout, "skip %s: %s\n", d.Series.Key, d.Reason)
		}
	}
	return counts
}

// mineWrote reports whether the applied run changed the log.
func mineWrote(results []envfacts.Result) bool {
	for _, r := range results {
		if r.FactID != "" || r.NoteID != "" {
			return true
		}
	}
	return false
}

// printMineScan reports the scan coverage: unattributed data never
// forms domain facts, and the count says so.
func printMineScan(c CLI, stats envfacts.Stats) {
	fmt.Fprintf(c.Stdout, "scanned %d receipt(s) (%d unattributed), %d trace session(s) (%d unattributed entries)\n",
		stats.Receipts, stats.ReceiptsUnattributed, stats.TraceSessions, stats.TraceUnattributed)
}
