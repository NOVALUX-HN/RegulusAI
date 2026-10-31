package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"regulus-ai/internal/decisions"
)

// decideCmd implements `regulus decide ...`: Decision Intelligence —
// structured technical decisions, assessed (never auto-approved) by the
// core against evidence, policy, and human approval.
func (c CLI) decideCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus decide propose --file <json> | regulus decide assess <id> [--json] | regulus decide resolve <id> --accept|--reject|--inconclusive [--supersede <id>] [--approve-by \"nick: reason\"] [--note <text>] | regulus decide show <id> [--json] | regulus decide list [--state S]")
		return 2
	}
	switch args[0] {
	case "propose":
		return c.decideProposeCmd(args[1:])
	case "assess":
		return c.decideAssessCmd(args[1:])
	case "resolve":
		return c.decideResolveCmd(args[1:])
	case "show":
		return c.decideShowCmd(args[1:])
	case "list":
		return c.decideListCmd(args[1:])
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus decide propose --file <json> | regulus decide assess <id> [--json] | regulus decide resolve <id> --accept|--reject|--inconclusive [--supersede <id>] [--approve-by \"nick: reason\"] [--note <text>] | regulus decide show <id> [--json] | regulus decide list [--state S]")
		return 2
	}
}

func (c CLI) decideProposeCmd(args []string) int {
	fs := flag.NewFlagSet("decide propose", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	file := fs.String("file", "", "decision contract JSON file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *file == "" {
		fmt.Fprintln(c.Stderr, "usage: regulus decide propose --file <json>")
		return 2
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide propose: %v\n", err)
		return 1
	}
	ct, err := decisions.ParseContract(data)
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide propose: %v\n", err)
		return 1
	}
	d, err := decisions.Propose(".", ct)
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide propose: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "proposed %s: %s\n", d.ID, d.State)
	fmt.Fprintf(c.Stdout, "scope: %s | confidence: %s | reversibility: %s\n", d.Contract.Scope, d.Contract.Confidence, d.Contract.Reversibility)
	return 0
}

func splitPositional(args []string) (pos []string, flags []string) {
	seenPos := false
	for _, a := range args {
		if !seenPos && a != "" && a[0] != '-' {
			pos = append(pos, a)
			seenPos = true
			continue
		}
		flags = append(flags, a)
	}
	return pos, flags
}

func acceptedDecisions(root, except string) []decisions.Decision {
	all, err := decisions.List(root)
	if err != nil {
		return nil
	}
	var out []decisions.Decision
	for _, d := range all {
		if d.State == decisions.StateAccepted && d.ID != except {
			out = append(out, d)
		}
	}
	return out
}

func (c CLI) decideAssessCmd(args []string) int {
	fs := flag.NewFlagSet("decide assess", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the assessment pipeline as JSON")
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus decide assess [--json] <id>")
		return 2
	}
	d, err := decisions.Load(".", pos[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide assess: %v\n", err)
		return 1
	}
	p := decisions.Assess(".", d, acceptedDecisions(".", d.ID))
	if *asJSON {
		raw, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "decide assess: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "decision: %s (%s)\n", d.ID, d.State)
	fmt.Fprintf(c.Stdout, "PERCEPTION: %s\n", p.Perception)
	resolved := 0
	for _, r := range p.Evidence {
		if r.Resolved {
			resolved++
		}
	}
	fmt.Fprintf(c.Stdout, "EVIDENCE: %d/%d refs resolve\n", resolved, len(p.Evidence))
	for _, r := range p.Evidence {
		mark := "ok"
		if !r.Resolved {
			mark = "UNRESOLVED"
		}
		fmt.Fprintf(c.Stdout, "  [%s] %s %s\n", mark, r.Kind, r.Raw)
	}
	fmt.Fprintf(c.Stdout, "POLICY: evidence_ok=%v blocking=%v critical=%v obsolete_by=%s task=%s\n",
		p.Policy.EvidenceOK, p.Policy.HasBlocking, p.Policy.Critical, orDash(p.Policy.ObsoleteBy), orDash(p.Policy.TaskVerdict))
	fmt.Fprintln(c.Stdout, "ASSESSMENT:")
	for _, cr := range p.Criteria {
		fmt.Fprintf(c.Stdout, "  [%s] %s — %s\n", strings.ToUpper(cr.Verdict), cr.Name, cr.Note)
	}
	if len(p.Findings) == 0 {
		fmt.Fprintln(c.Stdout, "findings: none")
	} else {
		fmt.Fprintln(c.Stdout, "findings:")
		for _, f := range p.Findings {
			fmt.Fprintf(c.Stdout, "  [%s] %s: %s\n", f.Severity, f.Code, f.Detail)
		}
	}
	abs := ""
	if p.Abstained {
		abs = " (the system abstains)"
	}
	fmt.Fprintf(c.Stdout, "RECOMMENDATION: %s — %s%s\n", p.Recommendation, p.Rationale, abs)
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (c CLI) decideResolveCmd(args []string) int {
	fs := flag.NewFlagSet("decide resolve", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	accept := fs.Bool("accept", false, "accept the decision (policy permitting)")
	reject := fs.Bool("reject", false, "reject the decision (needs --note)")
	inconclusive := fs.Bool("inconclusive", false, "abstain (needs --note)")
	supersede := fs.String("supersede", "", "accepted decision id this accept atomically supersedes")
	approveBy := fs.String("approve-by", "", "human approval as \"nick: reason\"")
	note := fs.String("note", "", "rationale for reject/inconclusive")
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus decide resolve [--accept|--reject|--inconclusive] [--supersede <id>] [--approve-by \"nick: reason\"] [--note <text>] <id>")
		return 2
	}
	d, err := decisions.Resolve(".", pos[0], decisions.ResolveOptions{
		Accept: *accept, Reject: *reject, Inconclusive: *inconclusive,
		Supersede: *supersede, ApproveBy: *approveBy, Note: *note,
	})
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide resolve: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "resolved %s: %s\n", d.ID, d.State)
	last := d.History[len(d.History)-1]
	fmt.Fprintf(c.Stdout, "by %s: %s\n", last.By, last.Note)
	return 0
}

func (c CLI) decideShowCmd(args []string) int {
	fs := flag.NewFlagSet("decide show", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	asJSON := fs.Bool("json", false, "print the raw decision document")
	pos, flagArgs := splitPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus decide show [--json] <id>")
		return 2
	}
	d, err := decisions.Load(".", pos[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide show: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(d, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "decide show: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(c.Stdout, "id:          %s\n", d.ID)
	fmt.Fprintf(c.Stdout, "state:       %s\n", d.State)
	fmt.Fprintf(c.Stdout, "scope:       %s\n", d.Contract.Scope)
	fmt.Fprintf(c.Stdout, "problem:     %s\n", d.Contract.Problem)
	fmt.Fprintf(c.Stdout, "proposal:    %s\n", d.Contract.Proposal)
	fmt.Fprintf(c.Stdout, "confidence:  %s | reversibility: %s\n", d.Contract.Confidence, d.Contract.Reversibility)
	fmt.Fprintln(c.Stdout, "history:")
	for _, h := range d.History {
		fmt.Fprintf(c.Stdout, "  %s %s %s -> %s by %s: %s\n", h.At, h.Kind, h.From, h.To, h.By, h.Note)
	}
	if warns := decisions.CheckIntegrity(d); len(warns) != 0 {
		fmt.Fprintln(c.Stdout, "integrity warnings:")
		for _, w := range warns {
			fmt.Fprintf(c.Stdout, "  ! %s\n", w)
		}
		return 1
	}
	return 0
}

func (c CLI) decideListCmd(args []string) int {
	fs := flag.NewFlagSet("decide list", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	state := fs.String("state", "", "filter by state")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus decide list [--state S]")
		return 2
	}
	all, err := decisions.List(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "decide list: %v\n", err)
		return 1
	}
	if len(all) == 0 {
		fmt.Fprintln(c.Stdout, "(no decisions)")
		return 0
	}
	for _, d := range all {
		if *state != "" && d.State != *state {
			continue
		}
		problem := d.Contract.Problem
		if len(problem) > 60 {
			problem = problem[:57] + "..."
		}
		fmt.Fprintf(c.Stdout, "%-16s %-12s %-20s %s\n", d.ID, d.State, d.Contract.Scope, problem)
	}
	return 0
}
