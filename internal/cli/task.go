package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/gates"
	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
)

// taskCmd implements `regulus task ...`: the machine-checkable Sextant
// discipline (Phase 2) — contracts, delegation, progress and core-verified
// completion, all read from the chart.
func (c CLI) taskCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(c.Stdout, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close [--issue NOV-xx] <chart> <id> | regulus task classify [flags]")
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close [--issue NOV-xx] <chart> <id> | regulus task classify [flags]")
		return 2
	}
	if len(args) > 1 && (args[0] == "check" || args[0] == "status" || args[0] == "close") && strings.HasPrefix(args[1], "-") {
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close [--issue NOV-xx] <chart> <id> | regulus task classify [flags]")
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
		fmt.Fprintln(c.Stderr, "usage: regulus task check <chart> | regulus task status <chart> | regulus task close [--issue NOV-xx] <chart> <id> | regulus task classify [flags]")
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
	fmt.Fprintln(c.Stdout, "why: an incomplete chart cannot prove acceptance — BUILD is blocked until check passes")
	fmt.Fprintf(c.Stdout, "next: fix the chart contract/tasks above, then run: regulus task check %s\n", args[0])
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
		fmt.Fprintln(c.Stdout, "why: an incomplete chart cannot prove acceptance — BUILD is blocked until status is clean")
		fmt.Fprintf(c.Stdout, "next: fix the violations above, then run: regulus task check %s\n", args[0])
		return 1
	}
	return 0
}

// validateCloseSkip enforces explicit, recorded traceability skips:
// skipping is only meaningful with --issue and always needs --justified.
// Empty means the flag combination is allowed.
func validateCloseSkip(issue string, skip bool, justified string) string {
	if !skip {
		return ""
	}
	if strings.TrimSpace(issue) == "" {
		return "task close: --skip-traceability needs --issue (nothing to skip without an issue)"
	}
	if strings.TrimSpace(justified) == "" {
		return "task close: --skip-traceability needs --justified <reason> (skips are explicit and recorded, never silent)"
	}
	return ""
}

// taskCloseOptions carries the parsed `task close` flags plus the two
// positional arguments. Splitting the parse out of taskCloseCmd keeps the
// entry point under the CRAP ceiling without changing behavior.
type taskCloseOptions struct {
	issue            string
	justified        string
	implementer      string
	independent      bool
	verifyOnly       bool
	skipTraceability bool
	chart            string
	id               string
}

// parseTaskCloseOptions parses flags and positionals, printing the exact
// legacy usage strings on Stderr. ok=false means the caller returns code.
func (c CLI) parseTaskCloseOptions(args []string) (taskCloseOptions, int, bool) {
	var opts taskCloseOptions
	fs := flag.NewFlagSet("task close", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	issue := fs.String("issue", "", "Linear issue closed (e.g. NOV-64): require linked commit(s), scope-clean diffs and, for P0, an independent review")
	justified := fs.String("justified", "", "reason covering multi-issue commits (or put a Justified: trailer in the commit)")
	implementer := fs.String("implementer", "", "who did the work (compared against the reviewer for P0 independence)")
	independent := fs.Bool("independent-review", false, "P0: require a recorded review verdict by someone other than --implementer")
	verifyOnly := fs.Bool("verify-only", false, "run all close checks without writing: exit 1 on any unmet item, nothing is modified (CI path)")
	skipTraceability := fs.Bool("skip-traceability", false, "bypass the --issue traceability check (requires --justified with a recorded reason; CI never uses it)")
	if err := fs.Parse(args); err != nil {
		return opts, 2, false
	}
	rest := fs.Args()
	if len(rest) != 2 {
		fmt.Fprintln(c.Stderr, "usage: regulus task close [--issue NOV-xx] [--justified reason] [--implementer id] [--independent-review] [--verify-only] [--skip-traceability] <chart> <id>")
		return opts, 2, false
	}
	if msg := validateCloseSkip(*issue, *skipTraceability, *justified); msg != "" {
		fmt.Fprintln(c.Stderr, msg)
		return opts, 2, false
	}
	opts.issue = *issue
	opts.justified = *justified
	opts.implementer = *implementer
	opts.independent = *independent
	opts.verifyOnly = *verifyOnly
	opts.skipTraceability = *skipTraceability
	opts.chart = rest[0]
	opts.id = rest[1]
	return opts, 0, true
}

// verifyTaskClose runs every close check: the pure CheckClose, the S11/NOV-32
// worktree scope enforcement (needs git I/O, so it lives here rather than
// in pure CheckClose), and the NOV-64/NOV-78 issue traceability linkage.
func (c CLI) verifyTaskClose(doc, chartPath, chartBase, id string, opts taskCloseOptions) sextant.CloseResult {
	res := sextant.CheckClose(doc, c.dir(), chartBase, id)
	// S11/NOV-32: tasks inherit the chart scope. The worktree check
	// needs git I/O, so it runs here (not in pure CheckClose): any file
	// outside the declared Scope blocks the close by name.
	if unmet := c.scopeUnmet(doc, chartPath); len(unmet) != 0 {
		res.Unmet = append(res.Unmet, unmet...)
		res.Closed = false
	}
	// NOV-64: with --issue, the close additionally needs auditable
	// linkage: commit(s) citing the issue, scope-clean diffs and, for
	// P0, an independent review verdict. NOV-78: the check runs by
	// default; --skip-traceability bypasses it only with an explicit,
	// recorded --justified reason (never silently; CI never skips).
	if opts.issue != "" && !opts.skipTraceability {
		sc, serr := sextant.ParseScope(doc)
		if serr != nil {
			res.Unmet = append(res.Unmet, fmt.Sprintf("scope: %v", serr))
			res.Closed = false
		} else if unmet := sextant.CheckTraceability(c.dir(), sextant.TraceOptions{
			Issue: opts.issue, Justified: opts.justified, Implementer: opts.implementer,
			IndependentReview: opts.independent, ChartBase: chartBase, TaskID: id, Scope: sc,
		}); len(unmet) != 0 {
			res.Unmet = append(res.Unmet, unmet...)
			res.Closed = false
		}
	}
	return res
}

// finishTaskClose reports the verification outcome and, unless --verify-only,
// persists the close. Messages and exit codes are byte-identical to the
// legacy monolith: the skip notice, the cannot-close block, the verify-only
// report, the chart rewrite, the CLOSED evidence list.
func (c CLI) finishTaskClose(opts taskCloseOptions, chartPath, doc, chartBase string, res sextant.CloseResult) int {
	if opts.skipTraceability {
		fmt.Fprintf(c.Stdout, "traceability skipped (--skip-traceability): %s\n", opts.justified)
	}
	if !res.Closed {
		fmt.Fprintf(c.Stdout, "task %s cannot close:\n", opts.id)
		for _, u := range res.Unmet {
			fmt.Fprintf(c.Stdout, "  - %s\n", u)
		}
		fmt.Fprintln(c.Stdout, "why: self-report closes nothing — close needs contract, receipts PASS, and shut dependencies")
		fmt.Fprintf(c.Stdout, "next: run regulus task check %s, then regulus gate run %s/%s, then retry close\n", opts.chart, chartBase, opts.id)
		return 1
	}
	// NOV-78: --verify-only runs every check above and reports, but
	// writes nothing: no chart rewrite, no receipt, no memory note.
	// CI calls this per close; exit 1 names each gap (fail-closed).
	if opts.verifyOnly {
		if !res.Closed {
			fmt.Fprintf(c.Stdout, "task %s would NOT close:\n", opts.id)
			for _, u := range res.Unmet {
				fmt.Fprintf(c.Stdout, "  - %s\n", u)
			}
			return 1
		}
		fmt.Fprintf(c.Stdout, "task %s verifies: spec OK + %d receipt(s) PASS (nothing written)\n", opts.id, len(res.Receipts))
		return 0
	}
	updated := sextant.ApplyClose(doc, opts.id, res)
	if err := os.WriteFile(chartPath, []byte(updated), 0o644); err != nil {
		fmt.Fprintf(c.Stderr, "task close: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "CLOSED %s (%s): spec OK + %d receipt(s) PASS\n", opts.id, res.Class, len(res.Receipts))
	for _, r := range res.Receipts {
		fmt.Fprintf(c.Stdout, "  evidence: %s\n", r)
	}
	// M3 auto-save: a side effect only — the close already succeeded, and
	// this call never changes its verdict.
	c.autoSaveCloseNote(chartBase, opts.id, doc, res)
	return 0
}

func (c CLI) taskCloseCmd(args []string) int {
	opts, code, ok := c.parseTaskCloseOptions(args)
	if !ok {
		return code
	}
	path, doc, err := readChart(opts.chart)
	if err != nil {
		fmt.Fprintf(c.Stderr, "%v\n", err)
		return 1
	}
	chartBase := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	res := c.verifyTaskClose(doc, path, chartBase, opts.id, opts)
	return c.finishTaskClose(opts, path, doc, chartBase, res)
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

// scopeUnmet enforces the chart scope at close (S11/NOV-32): tasks
// inherit the chart's Scope, so any worktree file outside the declared
// Allow/Deny boundary blocks the close by name. No declared scope means
// no enforcement (legacy charts keep working); an unevaluable worktree
// (git unavailable) fails open like the gate when-conditions, never
// blocking the close on missing tooling.
func (c CLI) scopeUnmet(doc, chartPath string) []string {
	sc, err := sextant.ParseScope(doc)
	if err != nil {
		return []string{fmt.Sprintf("%v — ajusta el Scope del chart (REPLAN con motivo) o mueve el cambio dentro del scope", err)}
	}
	if !sc.Declared {
		return nil
	}
	files, ok := c.scopeWorktreeFiles(chartPath)
	if !ok {
		return nil
	}
	if err := sc.CheckScopeFilesIn(c.dir(), files); err != nil {
		return []string{fmt.Sprintf("%v — ajusta el Scope del chart (REPLAN con motivo) o mueve el cambio dentro del scope", err)}
	}
	return nil
}

// scopeWorktreeFiles lists the real worktree: tracked changes against
// HEAD plus untracked files (honoring git's own excludes). The chart
// itself is dropped: it is the authorization artifact close rewrites,
// not worker output, so its own path never counts against the scope.
func (c CLI) scopeWorktreeFiles(chartPath string) ([]string, bool) {
	ctx, cancel := gitTimeout()
	defer cancel()
	r := c.scopeRunner()
	var files []string
	ok := false
	if out, _, code, err := r.Run(ctx, "git diff --name-only HEAD"); err == nil && code == 0 {
		ok = true
		files = append(files, splitScopeLines(out)...)
	}
	if out, _, code, err := r.Run(ctx, "git ls-files --others --exclude-standard"); err == nil && code == 0 {
		ok = true
		files = append(files, splitScopeLines(out)...)
	}
	if !ok {
		return nil, false
	}
	return dropScopeChartFile(files, scopeChartRel(c.dir(), chartPath)), true
}

// scopeRunner pins the real shell runner to the project root (git lists
// root-relative paths); injected doubles run untouched.
func (c CLI) scopeRunner() gates.CmdRunner {
	root := c.dir()
	if sh, ok := c.Runner.(gates.ShRunner); ok {
		sh.Dir = root
		return sh
	}
	if shp, ok := c.Runner.(*gates.ShRunner); ok && shp != nil {
		cp := *shp
		cp.Dir = root
		return cp
	}
	if c.Runner != nil {
		return c.Runner
	}
	return gates.ShRunner{Dir: root}
}

func splitScopeLines(out string) []string {
	var files []string
	for _, line := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			files = append(files, t)
		}
	}
	return files
}

// scopeChartRel names the chart repo-relative so the close can exempt
// its own authorization file; "" exempts nothing.
func scopeChartRel(root, chartPath string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	absChart, err := filepath.Abs(chartPath)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(absRoot, absChart)
	if err != nil || rel == "" || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel) {
		return ""
	}
	return rel
}

func dropScopeChartFile(files []string, chartRel string) []string {
	if chartRel == "" {
		return files
	}
	want := strings.ToLower(filepath.ToSlash(chartRel))
	out := make([]string, 0, len(files))
	for _, f := range files {
		if strings.ToLower(strings.TrimSpace(f)) == want {
			continue
		}
		out = append(out, f)
	}
	return out
}
