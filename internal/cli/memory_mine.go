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
	series, stats, total, err := mineSeries(c.dir(), *domain)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	decisions, err := envfacts.Plan(c.dir(), series)
	if err != nil {
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stderr, trustBanner)
	if !*apply {
		printMinePlan(c, decisions, stats, *domain, total)
		return 0
	}
	results, err := envfacts.Apply(c.dir(), decisions, time.Now().UTC())
	if err != nil {
		// H3 per-decision report: Apply keeps the successes and
		// aggregates each failure ("apply: m/n failed: ..."). Show what
		// landed before exiting non-zero with the failure list.
		if len(results) > 0 {
			printMineResults(c, results, stats, *domain, total)
		}
		fmt.Fprintf(c.Stderr, "memory mine: %v\n", err)
		return 1
	}
	printMineResults(c, results, stats, *domain, total)
	for _, r := range results {
		if r.FactID != "" || r.NoteID != "" {
			c.refreshIndexBestEffort("mine")
			break
		}
	}
	return 0
}

// mineSeries scans receipts and traces and keeps the requested domain.
// Stats stay global (whole-tree coverage); total is the pre-filter
// series count so the scan line can warn on domain typos.
func mineSeries(root, domain string) ([]envfacts.Series, envfacts.Stats, int, error) {
	rs, rst, err := envfacts.ReceiptSeries(root)
	if err != nil {
		return nil, envfacts.Stats{}, 0, err
	}
	ts, tst, err := envfacts.TraceSeries(root)
	if err != nil {
		return nil, envfacts.Stats{}, 0, err
	}
	stats := envfacts.Stats{
		Receipts:             rst.Receipts,
		ReceiptsUnattributed: rst.ReceiptsUnattributed,
		TraceSessions:        tst.TraceSessions,
		TraceUnattributed:    tst.TraceUnattributed,
		Corrupt:              rst.Corrupt + tst.Corrupt,
	}
	series := append(rs, ts...)
	total := len(series)
	if domain == "" {
		return series, stats, total, nil
	}
	kept := series[:0]
	for _, s := range series {
		if s.Key.Domain == domain {
			kept = append(kept, s)
		}
	}
	return kept, stats, total, nil
}

// printMinePlan renders the dry run: what --apply would write.
func printMinePlan(c CLI, decisions []envfacts.Decision, stats envfacts.Stats, domain string, total int) {
	counts := map[string]int{}
	for _, d := range decisions {
		counts[d.Action]++
		switch d.Action {
		case envfacts.ActionMine:
			failing := envfacts.FailingSessions(d.Series)
			fmt.Fprintf(c.Stdout, "would-mine %s — %s (%d failing sessions, confidence medium)\n",
				d.Series.Key.Topic(), envfacts.CueFor(d.Series.Key, failing), failing)
		case envfacts.ActionDiscard:
			fmt.Fprintf(c.Stdout, "would-discard %s — %s\n",
				strings.Join(d.LiveFactIDs, ", "), d.Reason)
		default:
			fmt.Fprintf(c.Stdout, "skip %s: %s\n", d.Series.Key.Topic(), d.Reason)
		}
	}
	fmt.Fprintf(c.Stdout, "mine: %d to mine, %d to discard, %d skipped (dry run — pass --apply to write)\n",
		counts[envfacts.ActionMine], counts[envfacts.ActionDiscard], counts[envfacts.ActionSkip])
	printMineScan(c, stats, domain, len(decisions), total)
}

// printMineResults renders an applied run with the written ids. H3: a
// discard with an empty note id is an idempotent no-op (the topic had no
// live facts left), reported as a skip so a re-applied discard never
// counts as newly discarded.
func printMineResults(c CLI, results []envfacts.Result, stats envfacts.Stats, domain string, total int) {
	counts := map[string]int{}
	for _, r := range results {
		action := r.Decision.Action
		if action == envfacts.ActionDiscard && r.NoteID == "" {
			action = envfacts.ActionSkip
		}
		counts[action]++
		switch r.Decision.Action {
		case envfacts.ActionMine:
			fmt.Fprintf(c.Stdout, "mined %s %s\n", r.FactID, r.Decision.Series.Key.Topic())
		case envfacts.ActionDiscard:
			if r.NoteID == "" {
				fmt.Fprintf(c.Stdout, "skip %s: already discarded\n", r.Decision.Series.Key.Topic())
				break
			}
			fmt.Fprintf(c.Stdout, "discarded %s (note %s)\n",
				strings.Join(r.Decision.LiveFactIDs, ", "), r.NoteID)
		default:
			fmt.Fprintf(c.Stdout, "skip %s: %s\n", r.Decision.Series.Key.Topic(), r.Decision.Reason)
		}
	}
	fmt.Fprintf(c.Stdout, "mine: %d mined, %d discarded, %d skipped\n",
		counts[envfacts.ActionMine], counts[envfacts.ActionDiscard], counts[envfacts.ActionSkip])
	printMineScan(c, stats, domain, len(results), total)
}

// printMineScan reports the scan coverage: unattributed data never
// forms domain facts, and the count says so. Corrupt documents skipped
// by the tolerant H2 readers are reported as well: they were warned
// about on stderr as path:line and never aborted the scan. Stats stay
// global when --domain filters; a filter matching nothing warns with
// the global series total so typos are visible.
func printMineScan(c CLI, stats envfacts.Stats, domain string, matched, total int) {
	fmt.Fprintf(c.Stdout, "scanned %d receipt(s) (%d unattributed), %d trace session(s) (%d unattributed entries), %d corrupt skipped\n",
		stats.Receipts, stats.ReceiptsUnattributed, stats.TraceSessions, stats.TraceUnattributed, stats.Corrupt)
	if domain != "" && matched == 0 {
		fmt.Fprintf(c.Stdout, "domain %q matched 0 series (global scanned %d series)\n", domain, total)
	}
}
