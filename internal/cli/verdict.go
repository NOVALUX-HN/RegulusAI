package cli

import (
	"flag"
	"fmt"

	"regulus-ai/internal/guardians"
)

// gateVerdictCmd implements `regulus gate verdict <task>`: the core
// consolidates one task's receipts and recorded evaluations into a single
// verdict. Receipts decide; evaluations are shown with their dispute flag
// and counted, never applied.
func (c CLI) gateVerdictCmd(args []string) int {
	fs := flag.NewFlagSet("gate verdict", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus gate verdict <task>")
		return 2
	}
	task := fs.Arg(0)
	if !validTaskID(task) {
		fmt.Fprintf(c.Stderr, "gate verdict: invalid task id %q\n", task)
		return 2
	}
	con, err := guardians.Consolidate(".", task)
	if err != nil {
		fmt.Fprintf(c.Stderr, "gate verdict: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "task: %s\n", con.Task)
	fmt.Fprintln(c.Stdout, "deterministic (receipts decide):")
	for _, g := range con.Gates {
		fmt.Fprintf(c.Stdout, "  layer %d %-16s %s (%s) %s\n", g.Layer, g.Gate, g.Status, g.Reason, g.ReceiptID)
	}
	fmt.Fprintln(c.Stdout, "semantic (evaluations are recorded, never authoritative):")
	if len(con.Evaluations) == 0 {
		fmt.Fprintln(c.Stdout, "  (no evaluations recorded)")
	}
	for _, e := range con.Evaluations {
		flag := "AGREE"
		if e.Disputed {
			flag = "DISPUTE"
		}
		fmt.Fprintf(c.Stdout, "  %s: %s — %s [%s]\n", e.Guardian, e.Status, e.Note, flag)
	}
	fmt.Fprintf(c.Stdout, "verdict: %s (%s)\n", con.Verdict, con.Reason)
	fmt.Fprintf(c.Stdout, "rates: disagreement=%.2f inconclusive_gates=%d inconclusive_evals=%d completeness=%.2f legacy=%d\n",
		con.DisagreementRate, con.GatesInconclusive, con.EvalsInconclusive, con.EvidenceCompleteness, con.Legacy)
	if con.Verdict != "PASS" {
		return 1
	}
	return 0
}
