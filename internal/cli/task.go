package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
)

// taskCmd implements `regulus task ...`: the machine-checkable Sextant
// discipline (Phase 2) — contracts, delegation, progress and core-verified
// completion, all read from the chart.
func (c CLI) taskCmd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close <chart> <id> | regulus task classify [flags]")
		return 2
	}
	switch args[0] {
	case "check":
		return c.taskCheckCmd(args[1:])
	case "status":
		return c.taskStatusCmd(args[1:])
	case "close":
		return c.taskCloseCmd(args[1:])
	case "classify":
		return c.taskClassifyCmd(args[1:])
	default:
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close <chart> <id> | regulus task classify [flags]")
		return 2
	}
}

func readChart(path string) (string, string, error) {
	p := path
	if _, err := os.Stat(p); err != nil {
		alt := filepath.Join("charts", strings.TrimSuffix(path, ".md")+".md")
		if _, err2 := os.Stat(alt); err2 == nil {
			p = alt
		}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", fmt.Errorf("task: cannot read chart %s", path)
	}
	return p, string(data), nil
}

func chartClass(doc string) sextant.Class {
	if class, ok := sextant.ParseClassLine(doc); ok {
		return class
	}
	return sextant.ClassStandard
}

// collectViolations runs every machine check over a chart: spec shape,
// contract coherence, delegation rules, progress and log discipline.
func collectViolations(doc string) []string {
	var out []string
	class := chartClass(doc)
	if _, violations := spec.Check(doc); len(violations) != 0 {
		for _, v := range violations {
			out = append(out, "spec: "+v)
		}
	}
	if _, violations := sextant.ParseContract(doc); len(violations) != 0 {
		out = append(out, violations...)
	}
	tasks, perrs := sextant.ParseTasks(doc)
	for _, e := range perrs {
		out = append(out, e)
	}
	out = append(out, sextant.CheckDelegations(tasks, class)...)
	out = append(out, sextant.CheckProgress(tasks, doc)...)
	out = append(out, sextant.CheckLog(doc)...)
	return out
}

func (c CLI) taskCheckCmd(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart>")
		return 2
	}
	_, doc, err := readChart(args[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "%v\n", err)
		return 1
	}
	violations := collectViolations(doc)
	tasks, _ := sextant.ParseTasks(doc)
	if len(violations) == 0 {
		fmt.Fprintf(c.Stdout, "task check: OK (class %s, %d task(s))\n", chartClass(doc), len(tasks))
		return 0
	}
	for _, v := range violations {
		fmt.Fprintf(c.Stdout, "violation: %s\n", v)
	}
	fmt.Fprintf(c.Stdout, "task check: %d violation(s)\n", len(violations))
	return 1
}

func (c CLI) taskStatusCmd(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.Stderr, "usage: regulus task status <chart>")
		return 2
	}
	_, doc, err := readChart(args[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "%v\n", err)
		return 1
	}
	tasks, perrs := sextant.ParseTasks(doc)
	for _, e := range perrs {
		fmt.Fprintf(c.Stdout, "parse: %s\n", e)
	}
	closed := sextant.ClosedTasks(doc)
	for _, t := range tasks {
		state := string(t.Status)
		if t.Status == sextant.StatusBlocked {
			state = "blocked: " + t.BlockedReason
		}
		if t.Status == sextant.StatusDone && !closed[t.ID] {
			state = "done (UNVERIFIED — run task close)"
		}
		deps := "-"
		if len(t.Depends) != 0 {
			deps = strings.Join(t.Depends, ",")
		}
		fmt.Fprintf(c.Stdout, "%-8s %-28s route=%s depends=%s\n", t.ID, state, t.Route, deps)
	}
	if len(tasks) == 0 {
		fmt.Fprintln(c.Stdout, "(no tasks tracked)")
	}
	violations := collectViolations(doc)
	for _, v := range violations {
		fmt.Fprintf(c.Stdout, "violation: %s\n", v)
	}
	if len(violations) != 0 {
		return 1
	}
	return 0
}

func (c CLI) taskCloseCmd(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(c.Stderr, "usage: regulus task close <chart> <id>")
		return 2
	}
	path, doc, err := readChart(args[0])
	if err != nil {
		fmt.Fprintf(c.Stderr, "%v\n", err)
		return 1
	}
	id := args[1]
	chartBase := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	res := sextant.CheckClose(doc, ".", chartBase, id)
	if !res.Closed {
		fmt.Fprintf(c.Stdout, "task %s cannot close:\n", id)
		for _, u := range res.Unmet {
			fmt.Fprintf(c.Stdout, "  - %s\n", u)
		}
		return 1
	}
	updated := sextant.ApplyClose(doc, id, res)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		fmt.Fprintf(c.Stderr, "task close: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "CLOSED %s (%s): spec OK + %d receipt(s) PASS\n", id, res.Class, len(res.Receipts))
	for _, r := range res.Receipts {
		fmt.Fprintf(c.Stdout, "  evidence: %s\n", r)
	}
	return 0
}

func (c CLI) taskClassifyCmd(args []string) int {
	fs := flag.NewFlagSet("task classify", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	code := fs.Bool("code", false, "the work modifies source")
	destructive := fs.Bool("destructive", false, "deletes, migrates, or irreversibly rewrites data")
	security := fs.Bool("security", false, "touches auth, permissions, secrets, or sandboxes")
	contracts := fs.Bool("contracts", false, "changes an API, CLI, config, or format others consume")
	concurrency := fs.Bool("concurrency", false, "touches concurrency or parallelism")
	delivery := fs.Bool("delivery", false, "touches release, CI, deploy, or dependencies")
	unclear := fs.Bool("unclear", false, "risk could not be determined by a bounded look")
	units := fs.Int("units", 1, "independent behavior units")
	deps := fs.Int("deps", 0, "task dependencies")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus task classify [--code] [--destructive] [--security] [--contracts] [--concurrency] [--delivery] [--unclear] [--units N] [--deps N]")
		return 2
	}
	signals := sextant.Signals{
		TouchesCode: *code, Destructive: *destructive, Security: *security,
		ContractConsumed: *contracts, Concurrency: *concurrency,
		Delivery: *delivery, UnclearRisk: *unclear, Units: *units, Dependencies: *deps,
	}
	class := sextant.Classify(signals)
	req := sextant.Requirements(class)
	fmt.Fprintf(c.Stdout, "class: %s\n", class)
	fmt.Fprintf(c.Stdout, "planning: %s\n", req.Planning)
	fmt.Fprintf(c.Stdout, "specification: %s\n", req.Specification)
	fmt.Fprintf(c.Stdout, "execution: %s\n", req.Execution)
	fmt.Fprintf(c.Stdout, "verification: %s\n", req.Verification)
	fmt.Fprintf(c.Stdout, "documentation: %s\n", req.Documentation)
	fmt.Fprintf(c.Stdout, "completion: %s\n", req.Completion)
	return 0
}
