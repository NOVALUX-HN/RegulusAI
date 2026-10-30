package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/library"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/reflect"
)

// reflectCmd implements `regulus reflect --domain --task --cue --lesson
// [--dimension] [--confidence] <feature-doc>`: admit an evidence-gated lesson (PRD
// R-E1/R-E2).
func (c CLI) reflectCmd(args []string) int {
	fs := flag.NewFlagSet("reflect", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "lesson domain (required)")
	dimension := fs.String("dimension", "", "inv (how the agent works) | dom (what a practitioner knows)")
	task := fs.String("task", "", "task id whose receipts are the evidence (required)")
	cue := fs.String("cue", "", "when this pattern appears (required)")
	lesson := fs.String("lesson", "", "what to do (required)")
	confidence := fs.String("confidence", "low", "low | medium | high")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || *cue == "" || *lesson == "" || fs.NArg() != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus reflect --domain <d> --task <t> --cue <c> --lesson <l> [--dimension inv|dom] [--confidence low|medium|high] <feature-doc>")
		fmt.Fprintln(c.Stderr, "lesson guidance: anchor workspace facts ('in this workspace, ...'), keep procedure transferable across projects, prefer skills/scripts over always-on prose")
		return 2
	}
	r := c.Runner
	if r == nil {
		r = gates.ShRunner{}
	}
	head := gates.GitHead(context.Background(), r)
	l, err := reflect.Run(".", reflect.Input{
		FeatureDoc: fs.Arg(0), Domain: *domain, Dimension: *dimension, Task: *task, Cue: *cue, Lesson: *lesson,
		Confidence: *confidence, Now: time.Now().UTC(),
	}, head)
	if err != nil {
		fmt.Fprintf(c.Stderr, "reflect: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "candidate proposed: %s\n", l.ID)
	fmt.Fprintf(c.Stdout, "guardian check, then: regulus library admit --domain %s --candidate %s --guardian <name>\n", l.Domain, l.ID)
	return 0
}

// libraryCmd implements `regulus library list|candidates|show|domains
// [--domain] | admit --domain --candidate --guardian |
// forget|retain --domain --task` — dispatched to
// one handler per subcommand.
func (c CLI) libraryCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library list|candidates|show|domains|forget|retain [--domain <name>] | regulus library admit --domain <d> --candidate <id> --guardian <name> | regulus library forget|retain --domain <d> --task <t>")
		return 2
	}
	switch args[0] {
	case "domains":
		return c.libraryDomainsCmd(args[1:])
	case "list":
		return c.libraryListCmd(args[1:])
	case "candidates":
		return c.libraryCandidatesCmd(args[1:])
	case "show":
		return c.libraryShowCmd(args[1:])
	case "admit":
		return c.libraryAdmitCmd(args[1:])
	case "forget":
		return c.libraryForgetCmd(args[1:])
	case "retain":
		return c.libraryRetainCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "library: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus library list|candidates|show|domains|forget|retain [--domain <name>] | regulus library admit --domain --candidate --guardian | regulus library forget|retain --domain <d> --task <t>")
		return 2
	}
}

// libraryDomainsCmd implements `regulus library domains`: one domain
// per line, sorted; empty (exit 0, no output) when no library exists.
// The OpenCode plugin reads it to auto-detect the lesson domain.
func (c CLI) libraryDomainsCmd(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library domains")
		return 2
	}
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "library domains: %v\n", err)
		return 1
	}
	for _, d := range domains {
		fmt.Fprintln(c.Stdout, d)
	}
	return 0
}

// libraryListCmd implements `regulus library list [--domain <name>]`.
func (c CLI) libraryListCmd(args []string) int {
	fs := flag.NewFlagSet("library list", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "only this domain's lessons")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library list [--domain <name>]")
		return 2
	}
	if *domain != "" {
		lessons, err := library.List(".", *domain)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library list: %v\n", err)
			return 1
		}
		c.printLessons(*domain, lessons)
		return 0
	}
	domains, err := library.Domains(".")
	if err != nil {
		fmt.Fprintf(c.Stderr, "library list: %v\n", err)
		return 1
	}
	if len(domains) == 0 {
		fmt.Fprintln(c.Stdout, "(no lessons admitted yet)")
		return 0
	}
	for _, d := range domains {
		lessons, err := library.List(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library list: %v\n", err)
			return 1
		}
		c.printLessons(d, lessons)
	}
	return 0
}

// libraryCandidatesCmd implements `regulus library candidates
// [--domain <name>]`.
func (c CLI) libraryCandidatesCmd(args []string) int {
	fs := flag.NewFlagSet("library candidates", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "only this domain's candidates")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library candidates [--domain <name>]")
		return 2
	}
	var domains []string
	var err error
	if *domain != "" {
		domains = []string{*domain}
	} else {
		domains, err = library.Domains(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "library candidates: %v\n", err)
			return 1
		}
	}
	any := false
	for _, d := range domains {
		cands, err := library.Candidates(".", d)
		if err != nil {
			fmt.Fprintf(c.Stderr, "library candidates: %v\n", err)
			return 1
		}
		if len(cands) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(c.Stdout, "%s (pending):\n", d)
		for _, l := range cands {
			fmt.Fprintf(c.Stdout, "  %s  %s  (%s, %d receipt(s))\n", l.ID, l.Cue, l.Confidence, len(l.Evidence))
		}
	}
	if !any {
		fmt.Fprintln(c.Stdout, "(no candidates pending)")
	}
	return 0
}

// libraryShowCmd implements `regulus library show --domain <name>` —
// the domain INDEX, verbatim, for agent consultation.
func (c CLI) libraryShowCmd(args []string) int {
	fs := flag.NewFlagSet("library show", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the domain index to print (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library show --domain <name>")
		return 2
	}
	data, err := os.ReadFile(library.IndexFile(".", *domain))
	if os.IsNotExist(err) {
		fmt.Fprintln(c.Stdout, "(no lessons admitted yet)")
		return 0
	}
	if err != nil {
		fmt.Fprintf(c.Stderr, "library show: %v\n", err)
		return 1
	}
	fmt.Fprint(c.Stdout, string(data))
	return 0
}

// libraryAdmitCmd implements `regulus library admit --domain --candidate
// --guardian` — the second phase of lesson admission.
func (c CLI) libraryAdmitCmd(args []string) int {
	fs := flag.NewFlagSet("library admit", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "candidate's domain (required)")
	candidate := fs.String("candidate", "", "candidate id (required)")
	guardian := fs.String("guardian", "", "who validated the admission (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *candidate == "" || *guardian == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library admit --domain <d> --candidate <id> --guardian <name>")
		return 2
	}
	head := gates.GitHead(context.Background(), c.runner())
	l, err := reflect.Admit(".", *domain, *candidate, *guardian, head, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "library admit: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "lesson admitted: %s (guardian %s)\n", l.ID, l.Guardian)
	fmt.Fprintf(c.Stdout, "index: %s\n", library.IndexFile(".", l.Domain))
	return 0
}

// libraryForgetCmd implements `regulus library forget --domain <d>
// --task <t>`: drop one task snapshot, then rebuild the INDEX.
// Forgetting an unknown task is a no-op success.
func (c CLI) libraryForgetCmd(args []string) int {
	fs := flag.NewFlagSet("library forget", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the snapshot's domain (required)")
	task := fs.String("task", "", "the task snapshot to drop (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library forget --domain <d> --task <t>")
		return 2
	}
	if err := library.Forget(".", *domain, *task); err != nil {
		fmt.Fprintf(c.Stderr, "library forget: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "forgotten: harnesses/%s/ (domain %s)\n", receipts.Slug(*task), *domain)
	return 0
}

// libraryRetainCmd implements `regulus library retain --domain <d>
// --task <t>`: keep only lessons whose task sorts strictly before <t>,
// drop <t> and everything after it, then rebuild the INDEX.
func (c CLI) libraryRetainCmd(args []string) int {
	fs := flag.NewFlagSet("library retain", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	domain := fs.String("domain", "", "the domain to prune (required)")
	task := fs.String("task", "", "keep tasks strictly before this one (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *domain == "" || *task == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus library retain --domain <d> --task <t>")
		return 2
	}
	if err := library.Retain(".", *domain, *task); err != nil {
		fmt.Fprintf(c.Stderr, "library retain: %v\n", err)
		return 1
	}
	lessons, err := library.List(".", *domain)
	if err != nil {
		fmt.Fprintf(c.Stderr, "library retain: %v\n", err)
		return 1
	}
	kept := make([]string, 0, len(lessons))
	for _, l := range lessons {
		if l.Task < *task {
			kept = append(kept, l.Task)
		}
	}
	if len(kept) == 0 {
		fmt.Fprintf(c.Stdout, "retained: (no tasks before %s in domain %s)\n", *task, *domain)
		return 0
	}
	fmt.Fprintf(c.Stdout, "retained: %s (domain %s)\n", strings.Join(kept, ", "), *domain)
	return 0
}

func (c CLI) printLessons(domain string, lessons []library.Lesson) {
	if len(lessons) == 0 {
		fmt.Fprintf(c.Stdout, "%s: (no lessons admitted yet)\n", domain)
		return
	}
	fmt.Fprintf(c.Stdout, "%s:\n", domain)
	for _, l := range lessons {
		fmt.Fprintf(c.Stdout, "  %s  %s  (%s, %d receipt(s))\n", l.ID, l.Cue, l.Confidence, len(l.Evidence))
	}
}

// budgetCmd implements `regulus budget show | record --kind probe`.
func (c CLI) budgetCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus budget show | regulus budget record --kind probe --detail <text>")
		return 2
	}
	switch args[0] {
	case "show":
		if len(args) != 1 {
			fmt.Fprintln(c.Stderr, "usage: regulus budget show")
			return 2
		}
		entries, err := budget.Show(".")
		if err != nil {
			fmt.Fprintf(c.Stderr, "budget show: %v\n", err)
			return 1
		}
		if len(entries) == 0 {
			fmt.Fprintln(c.Stdout, "(no budget entries)")
			return 0
		}
		for _, e := range entries {
			detail := ""
			if e.Detail != "" {
				detail = " (" + e.Detail + ")"
			}
			fmt.Fprintf(c.Stdout, "%s %s %s %s %s%s\n", e.Timestamp.Format(time.RFC3339), e.Kind, e.Task, e.Domain, e.Outcome, detail)
		}
		return 0
	case "record":
		fs := flag.NewFlagSet("budget record", flag.ContinueOnError)
		fs.SetOutput(c.Stderr)
		kind := fs.String("kind", "", "probe (required; reflect runs are recorded by regulus reflect itself)")
		detail := fs.String("detail", "", "what was probed")
		task := fs.String("task", "", "task id")
		domain := fs.String("domain", "", "domain")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *kind != budget.KindProbe || fs.NArg() != 0 {
			fmt.Fprintln(c.Stderr, "budget record: only probe runs are recorded manually (reflect runs are recorded by regulus reflect)")
			fmt.Fprintln(c.Stderr, "usage: regulus budget record --kind probe [--detail <text>] [--task <t>] [--domain <d>]")
			return 2
		}
		now := time.Now().UTC()
		cap := reflect.EvolutionCap(".")
		today, err := budget.TodayEvolutionCount(".", now)
		if err != nil {
			fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
			return 1
		}
		if today >= cap {
			if err := budget.Record(".", budget.Entry{Kind: budget.KindProbe, Timestamp: now, Task: *task, Domain: *domain, Outcome: budget.OutcomeRefusedCap, Detail: fmt.Sprintf("cap %d reached", cap)}); err != nil {
				fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
				return 1
			}
			fmt.Fprintf(c.Stderr, "budget record: evolution cap reached: %d run(s) today, cap %d\n", today, cap)
			return 1
		}
		if err := budget.Record(".", budget.Entry{Kind: budget.KindProbe, Timestamp: now, Task: *task, Domain: *domain, Outcome: budget.OutcomeProbeRun, Detail: *detail}); err != nil {
			fmt.Fprintf(c.Stderr, "budget record: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, "probe recorded")
		return 0
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus budget show | regulus budget record --kind probe")
		return 2
	}
}

// evolveCmd implements `regulus evolve propose`: system-driven
// proposal evidence (V5.7 chart charts/v0.5-evolve-propose.md).
// `propose` assembles the evidence pack; authoring stays with the
// agent/guardian. Assembly is read-only and records nothing.
func (c CLI) evolveCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evolve propose --task <t> --domain <d>")
		return 2
	}
	switch args[0] {
	case "propose":
		return c.evolveProposeCmd(args[1:])
	default:
		fmt.Fprintf(c.Stderr, "evolve: unknown subcommand %q\n", args[0])
		fmt.Fprintln(c.Stderr, "usage: regulus evolve propose --task <t> --domain <d>")
		return 2
	}
}

// evolveProposeCmd implements `regulus evolve propose --task --domain`:
// prints the evidence pack to stdout; exit 0 pack, 1 refusal (no
// evidence), 2 usage. Records nothing.
func (c CLI) evolveProposeCmd(args []string) int {
	fs := flag.NewFlagSet("evolve propose", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "task id whose receipts are the evidence (required)")
	domain := fs.String("domain", "", "lesson domain for the INDEX section (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *task == "" || *domain == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus evolve propose --task <t> --domain <d>")
		return 2
	}
	pack, err := evolve.Assemble(".", *task, *domain)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve propose: %v\n", err)
		return 1
	}
	fmt.Fprintln(c.Stdout, pack)
	return 0
}
