package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

// evidenceCmd implements `regulus evidence list [--task <t>] | show
// <file> | verify [--task <t>]`: the human audit surface (PRD R-G6) —
// receipts, never diffs — plus the Phase 1 integrity verifier.
func (c CLI) evidenceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file> | regulus evidence verify [--task <task>]")
		return 2
	}
	switch args[0] {
	case "verify":
		return c.evidenceVerifyCmd(args[1:])
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
		if rec.Schema != 0 || rec.Status != "" {
			fmt.Fprintf(c.Stdout, "status:      %s (%s)\n", rec.Status, rec.Reason)
			fmt.Fprintf(c.Stdout, "schema:      %d\n", rec.Schema)
		}
		if rec.RunID != "" {
			fmt.Fprintf(c.Stdout, "run_id:      %s\n", rec.RunID)
		}
		if rec.Manifest != "" {
			fmt.Fprintf(c.Stdout, "manifest:    %s\n", rec.Manifest)
		}
		if rec.CPUTimeMS != 0 {
			fmt.Fprintf(c.Stdout, "cpu_time_ms: %d\n", rec.CPUTimeMS)
		}
		if rec.Isolation != "" {
			fmt.Fprintf(c.Stdout, "isolation:   %s\n", rec.Isolation)
		}
		if rec.Classification != "" {
			fmt.Fprintf(c.Stdout, "class:       %s\n", rec.Classification)
		}
		fmt.Fprintf(c.Stdout, "stdout (sha256 %s):\n%s\n", rec.StdoutSHA, rec.Stdout)
		fmt.Fprintf(c.Stdout, "stderr (sha256 %s):\n%s\n", rec.StderrSHA, rec.Stderr)
		return 0
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file> | regulus evidence verify [--task <task>]")
		return 2
	}
}

// evidenceVerifyCmd implements `regulus evidence verify [--task <task>]`:
// recompute every content hash, check versions and manifest links, and
// quarantine corrupt files into corrupt/ (moved, never deleted).
func (c CLI) evidenceVerifyCmd(args []string) int {
	fs := flag.NewFlagSet("evidence verify", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "verify only this task's receipts")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence verify [--task <task>]")
		return 2
	}
	rep, err := evidence.Verify(".", *task, true)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evidence verify: %v\n", err)
		return 1
	}
	scope := "all tasks"
	if *task != "" {
		scope = "task " + *task
	}
	fmt.Fprintf(c.Stdout, "evidence verify (%s): %d checked, %d passed", scope, rep.Checked, rep.Passed)
	if rep.Legacy > 0 {
		fmt.Fprintf(c.Stdout, " (%d legacy, hash-checked only)", rep.Legacy)
	}
	if rep.Manifests > 0 {
		fmt.Fprintf(c.Stdout, " [%d manifest(s)]", rep.Manifests)
	}
	fmt.Fprintln(c.Stdout)
	for _, f := range rep.Failed {
		fmt.Fprintf(c.Stdout, "FAIL %s: %s — %s\n", f.File, f.Reason, f.Detail)
	}
	for _, q := range rep.Quarantined {
		fmt.Fprintf(c.Stdout, "quarantined: %s\n", q)
	}
	if len(rep.Failed) != 0 {
		return 1
	}
	return 0
}
