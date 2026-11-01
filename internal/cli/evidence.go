package cli

import (
	"flag"
	"fmt"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
)

// evidenceCmd implements `regulus evidence list [--task <t>] | show
// <file> | verify [--task <t>] | prune --task <t> [--keep N] [--yes] [--force]`:
// the human audit surface (PRD R-G6) — receipts, never diffs — plus the
// Phase 1 integrity verifier and the retention-gc pruner.
func (c CLI) evidenceCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file> | regulus evidence verify [--task <task>] | regulus evidence prune --task <task> [--keep N] [--yes] [--force]")
		return 2
	}
	switch args[0] {
	case "prune":
		return c.evidencePruneCmd(args[1:])
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
		// NOTE (S2 jail): <file> is an arbitrary path BY DESIGN — the
		// auditor points at any receipt file to inspect it (read-only
		// via receipts.ReadFile). It is not a task id and must not be
		// jailed into the evidence dir; the jail applies to task
		// resolution (List/Plan/Verify), not to this explicit path.
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
		fmt.Fprintln(c.Stderr, "usage: regulus evidence list [--task <task>] | regulus evidence show <file> | regulus evidence verify [--task <task>] | regulus evidence prune --task <task> [--keep N] [--yes] [--force]")
		return 2
	}
}

// evidencePruneCmd implements `regulus evidence prune --task <slug>
// [--keep N] [--yes] [--force]`: retention-gc S1. Without --yes it is a
// dry run (lists, deletes nothing); with --yes it deletes receipts
// oldest-first keeping the N newest, never manifests, never corrupt/.
// A task marked CLOSED in charts/ refuses without --force.
func (c CLI) evidencePruneCmd(args []string) int {
	fs := flag.NewFlagSet("evidence prune", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "prune only this task's receipts")
	keep := fs.Int("keep", 0, "keep the N newest receipts")
	yes := fs.Bool("yes", false, "actually delete (without --yes this is a dry run)")
	force := fs.Bool("force", false, "allow pruning evidence of a closed task")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *keep < 0 || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evidence prune --task <task> [--keep N] [--yes] [--force]")
		return 2
	}
	toDelete, _, manifests, err := evidence.Plan(".", *task, *keep)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evidence prune: %v\n", err)
		return 1
	}
	closed, err := evidence.IsClosed(".", *task)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evidence prune: %v\n", err)
		return 1
	}
	slug := receipts.Slug(*task)
	if closed && !*force {
		fmt.Fprintf(c.Stderr, "refused: evidence of closed task %s — re-run with --force and explicit authorization\n", slug)
		return 1
	}
	if closed && *force {
		fmt.Fprintf(c.Stderr, "warning: pruning closed task %s with --force (explicit authorization)\n", slug)
	}
	if !*yes {
		fmt.Fprintf(c.Stdout, "dry run: %d receipt(s) would be removed, %d manifest(s) kept\n", len(toDelete), manifests)
		for _, p := range toDelete {
			fmt.Fprintln(c.Stdout, p)
		}
		return 0
	}
	n, err := evidence.Delete(toDelete)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evidence prune: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "pruned %d\n", n)
	return 0
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
