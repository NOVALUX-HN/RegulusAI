package cli

import (
	"flag"
	"fmt"
	"strings"

	"regulus-ai/internal/guardians"
)

// gateEvaluateCmd implements `regulus gate evaluate`: record one guardian
// assessment against a task's receipts. Validation only — the core never
// lets an evaluation decide anything; consolidation is `gate verdict`.
func (c CLI) gateEvaluateCmd(args []string) int {
	fs := flag.NewFlagSet("gate evaluate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task whose receipts the assessment cites (required)")
	guardian := fs.String("guardian", "", "registry guardian id, e.g. regulus-evidence-auditor (required)")
	status := fs.String("verdict", "", "PASS, FAIL, BLOCKED or INCONCLUSIVE (required)")
	note := fs.String("note", "", "what was judged and why (required)")
	var evidences multiFlag
	fs.Var(&evidences, "evidence", "receipt id or filename backing the assessment (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *task == "" || *guardian == "" || *status == "" || *note == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus gate evaluate --task <t> --guardian <name> --verdict <PASS|FAIL|BLOCKED|INCONCLUSIVE> --note <text> [--evidence <id> ...]")
		return 2
	}
	ev, err := guardians.Evaluate(".", *task, *guardian, *status, *note, evidences)
	if err != nil {
		fmt.Fprintf(c.Stderr, "%v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "recorded: %s judges %s (%d evidence ref(s))\n", ev.Guardian, ev.Status, len(ev.Evidence))
	fmt.Fprintf(c.Stdout, "note: %s\n", ev.Note)
	return 0
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }
