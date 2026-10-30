package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

// evidenceCmd implements `regulus evidence list [--task <t>] | show
// <file>`: the human audit surface (PRD R-G6) — receipts, never diffs.
func (c CLI) evidenceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file>")
		return 2
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("evidence list", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		task := fs.String("task", "", "list only this task's receipts")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>]")
			return 2
		}
		entries, err := evidence.List(".", *task)
		if err != nil {
			fmt.Fprintf(c.Stderr, "evidence list: %v\n", err)
			return 1
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Stdout, "(no receipts yet)")
			return 0
		}
		for _, e := range entries {
			r := e.Receipt
			metric := ""
			if r.MetricValue != nil {
				metric = fmt.Sprintf(" metric=%.2f %s %.2f", *r.MetricValue, r.MetricOperator, *r.MetricThreshold)
			}
			fmt.Fprintf(c.Stdout, "%s layer%d %-12s %-4s exit=%d %dms%s %s\n",
				r.StartedAt.Format(time.RFC3339), r.Layer, r.Gate, r.Verdict, r.ExitCode, r.DurationMS, metric, e.Path)
		}
		return 0
	case "show":
		if len(args) != 2 {
			fmt.Fprintln(c.Stderr, "usage: regulus evidence show <file>")
			return 2
		}
		rec, err := receipts.ReadFile(args[1])
		if err != nil {
			fmt.Fprintf(c.Stderr, "evidence show: cannot read %s: %v\n", args[1], err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "id:          %s\n", rec.ID)
		fmt.Fprintf(c.Stdout, "task:        %s\n", rec.Task)
		fmt.Fprintf(c.Stdout, "gate:        %s (layer %d)\n", rec.Gate, rec.Layer)
		fmt.Fprintf(c.Stdout, "command:     %s\n", rec.Command)
		fmt.Fprintf(c.Stdout, "verdict:     %s (exit %d)\n", rec.Verdict, rec.ExitCode)
		fmt.Fprintf(c.Stdout, "started_at:  %s\n", rec.StartedAt.Format(time.RFC3339))
		fmt.Fprintf(c.Stdout, "duration_ms: %d\n", rec.DurationMS)
		if rec.MetricValue != nil {
			fmt.Fprintf(c.Stdout, "metric:      %.2f %s %.2f\n", *rec.MetricValue, rec.MetricOperator, *rec.MetricThreshold)
		}
		fmt.Fprintf(c.Stdout, "git_head:    %s\n", rec.GitHead)
		fmt.Fprintf(c.Stdout, "stdout (sha256 %s):\n%s\n", rec.StdoutSHA, rec.Stdout)
		fmt.Fprintf(c.Stdout, "stderr (sha256 %s):\n%s\n", rec.StderrSHA, rec.Stderr)
		return 0
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file>")
		return 2
	}
}
