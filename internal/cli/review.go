package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"regulus-ai/internal/crap"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/sextant"
	"regulus-ai/internal/spec"
)

// reviewLenses are the six read-only lenses a proportional review may
// apply. Exactly one runs per review; the mapping from risk to lens is
// advisory, never mandatory.
var reviewLenses = []string{"reviewer", "crap", "mutation", "spec", "evidence", "gate"}

// review caps keep the mode in seconds: at most this many files are
// listed, and at most this many bytes per file are read.
const (
	reviewMaxFiles = 50
	reviewMaxBytes = 256 * 1024
)

// reviewCmd implements `regulus review [--task <t>] [--lens <name>]`:
// the F2 proportional review. It freezes the candidate (HEAD + worktree
// status, read-only), classifies risk reusing `task classify` signals,
// applies exactly one lens, and reports. The result is INFORMATIONAL:
// it never blocks (exit 0 on a completed review), never commits, never
// writes receipts — delivery is the human reading stdout. The gauntlet
// and its gates are untouched; full proof stays `gate run`.
func (c CLI) reviewCmd(args []string) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	task := fs.String("task", "", "scope label for evidence-backed lenses (validated, never written)")
	lens := fs.String("lens", "", "force one lens: reviewer|crap|mutation|spec|evidence|gate (default: auto by risk)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus review [--task <t>] [--lens reviewer|crap|mutation|spec|evidence|gate]")
		return 2
	}
	if *task != "" && !validTaskID(*task) {
		fmt.Fprintf(c.Stderr, "review: invalid task id %q (allowed: letters, digits, '.', '_', '-', '/'; no '..')\n", *task)
		return 2
	}
	if *lens != "" && !reviewValidLens(*lens) {
		fmt.Fprintf(c.Stderr, "review: unknown lens %q (want %s)\n", *lens, strings.Join(reviewLenses, "|"))
		return 2
	}
	head, files, deleted, gitOK := reviewFreeze(c)
	class := sextant.Classify(reviewSignals(files, deleted, gitOK))
	risk := reviewRisk(class)
	name, reason := reviewPickLens(c.dir(), files, *lens)
	fmt.Fprintf(c.Stdout, "review: frozen %s %d file(s) (candidate snapshot; read-only, nothing committed)\n",
		reviewShortHead(head, gitOK), len(files))
	fmt.Fprintf(c.Stdout, "review: risk=%s class=%s lens=%s (%s)\n", risk, class, name, reason)
	if scope := strings.TrimSpace(*task); scope != "" {
		fmt.Fprintf(c.Stdout, "review: task=%s\n", scope)
	}
	reviewRunLens(c, name, files, strings.TrimSpace(*task))
	fmt.Fprintln(c.Stdout, "review: verdict INFORMATIONAL — nothing blocked, nothing committed, no gates changed")
	fmt.Fprintln(c.Stdout, "review: full proof stays `regulus gate run <task>`; this review is opt-in and advisory")
	return 0
}

// reviewValidLens reports whether name is one of the six lenses.
func reviewValidLens(name string) bool {
	for _, l := range reviewLenses {
		if l == name {
			return true
		}
	}
	return false
}

// reviewShortHead renders the frozen HEAD for the report: 8 hex chars,
// or a nogit marker when git was unavailable (UnclearRisk escalates).
func reviewShortHead(head string, gitOK bool) string {
	if !gitOK || strings.TrimSpace(head) == "" {
		return "nogit"
	}
	h := strings.TrimSpace(head)
	if len(h) > 8 {
		h = h[:8]
	}
	return "head=" + h
}

// reviewFreeze snapshots the candidate before reading it (RDD freeze):
// HEAD plus the worktree status, parsed like the gauntlet's own
// changed-file probe. Read-only; git failure degrades to UnclearRisk,
// never to an error.
func reviewFreeze(c CLI) (head string, files []string, deleted map[string]bool, gitOK bool) {
	deleted = map[string]bool{}
	head = gitHeadBounded(c.runner())
	ctx, cancel := gitTimeout()
	defer cancel()
	r := c.runner()
	out, _, code, err := r.Run(ctx, "git status --porcelain=v1 -z --untracked-files=normal -- .")
	if err != nil || code != 0 {
		return head, nil, deleted, head != ""
	}
	files, deleted = reviewParseStatus(out)
	return head, files, deleted, true
}

// reviewParseStatus parses NUL-separated porcelain v1 entries into
// paths plus a deleted set. Renames consume their follow-up entry, as
// in the gauntlet probe.
func reviewParseStatus(out string) ([]string, map[string]bool) {
	var files []string
	deleted := map[string]bool{}
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		e := parts[i]
		if len(e) < 4 {
			continue
		}
		xy, path := e[:2], e[3:]
		if path == "" || xy == "!!" {
			continue
		}
		files = append(files, path)
		if strings.Contains(xy, "D") {
			deleted[path] = true
		}
		if xy[0] == 'R' {
			i++
		}
	}
	return files, deleted
}

// reviewSignals maps the frozen file list to the existing `task
// classify` signals, so review risk reuses the Sextant scale instead of
// inventing one. UnclearRisk escalates when git was unavailable.
func reviewSignals(files []string, deleted map[string]bool, gitOK bool) sextant.Signals {
	var s sextant.Signals
	s.Units = 1
	if !gitOK {
		s.UnclearRisk = true
	}
	for _, f := range files {
		lower := strings.ToLower(filepath.ToSlash(f))
		if reviewIsCode(lower) {
			s.TouchesCode = true
		}
		if deleted[f] || strings.Contains(lower, "migration") || strings.Contains(lower, "migrate") {
			s.Destructive = true
		}
		if reviewContainsAny(lower, []string{"auth", "secur", "secret", "token", "crypto", "sandbox", "perm"}) {
			s.Security = true
		}
		if reviewContainsAny(lower, []string{"api/", "cli/", "contract", "openapi", "proto", "schema", "config"}) {
			s.ContractConsumed = true
		}
		if reviewContainsAny(lower, []string{"concurr", "lock", "worker", "pool", "sync", "channel", "goroutine"}) {
			s.Concurrency = true
		}
		if reviewIsDelivery(lower) {
			s.Delivery = true
		}
	}
	if s.TouchesCode && reviewGoTouchesConcurrency(files) {
		s.Concurrency = true
	}
	return s
}

// reviewIsCode reports source extensions the review treats as code.
func reviewIsCode(lower string) bool {
	for _, ext := range []string{".go", ".ts", ".js", ".py", ".rs", ".java"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// reviewIsDelivery reports release-surface paths: CI, manifests,
// containers and dependency pins.
func reviewIsDelivery(lower string) bool {
	if strings.HasPrefix(lower, ".github/") {
		return true
	}
	if lower == "regulus.json" || lower == "opencode.json" {
		return true
	}
	for _, mark := range []string{"dockerfile", "makefile", "go.mod", "go.sum", "package.json", "deploy", "/ci/", "/cd/"} {
		if strings.Contains(lower, mark) {
			return true
		}
	}
	return false
}

// reviewContainsAny reports whether s holds any marker.
func reviewContainsAny(s string, marks []string) bool {
	for _, m := range marks {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// reviewGoTouchesConcurrency scans capped Go sources for concurrency
// primitives. Bounded like every review read: skipped files simply do
// not vote.
func reviewGoTouchesConcurrency(files []string) bool {
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f), ".go") {
			continue
		}
		data, err := reviewCappedRead(f)
		if err != nil {
			continue
		}
		for _, mark := range []string{"go func", "sync.", "chan ", "<-"} {
			if strings.Contains(string(data), mark) {
				return true
			}
		}
	}
	return false
}

// reviewRisk maps the Sextant class to the review's risk word. Medium
// is the proportional sweet spot (seconds, one lens); elevated and
// high still report one lens but point at the full gauntlet.
func reviewRisk(class sextant.Class) string {
	switch class {
	case sextant.ClassCritical:
		return "high"
	case sextant.ClassComplex:
		return "elevated"
	case sextant.ClassStandard:
		return "medium"
	default:
		return "low"
	}
}

// reviewPickLens selects exactly one lens. An explicit --lens always
// wins (opt-in choice); otherwise the touched tree decides, docs and
// config first, code last with reviewer as the default.
func reviewPickLens(root string, files []string, override string) (string, string) {
	if override != "" {
		return override, "explicit --lens"
	}
	if len(files) == 0 {
		return "reviewer", "clean tree — reviewer confirms emptiness"
	}
	if reviewAllMatch(files, reviewIsDoc) {
		return "spec", "docs-only change"
	}
	if reviewAllMatch(files, reviewIsGateConfig) {
		return "gate", "gate-config-only change"
	}
	if reviewAllMatch(files, reviewIsEvidencePath) {
		return "evidence", "receipts-only change"
	}
	if reviewTestHeavy(files) {
		return "mutation", "test-heavy change"
	}
	if reviewHasGo(files) && reviewCoverageExists(root) {
		return "crap", "go change with coverage at hand"
	}
	return "reviewer", "code change — senior read of touched files"
}

// reviewAllMatch reports whether every file satisfies pred.
func reviewAllMatch(files []string, pred func(string) bool) bool {
	for _, f := range files {
		if !pred(strings.ToLower(filepath.ToSlash(f))) {
			return false
		}
	}
	return true
}

// reviewIsDoc reports prose extensions the spec lens can judge.
func reviewIsDoc(lower string) bool {
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt")
}

// reviewIsGateConfig reports harness-config paths the gate lens lists.
func reviewIsGateConfig(lower string) bool {
	if lower == "regulus.json" || lower == "opencode.json" {
		return true
	}
	if strings.HasPrefix(lower, ".github/") {
		return true
	}
	base := lower
	if i := strings.LastIndex(lower, "/"); i >= 0 {
		base = lower[i+1:]
	}
	return base == "dockerfile" || base == "makefile" || base == "go.mod" || base == "go.sum"
}

// reviewIsEvidencePath reports receipt-tree paths the evidence lens audits.
func reviewIsEvidencePath(lower string) bool {
	return strings.HasPrefix(lower, ".regulus/evidence/")
}

// reviewHasGo reports any Go source in the freeze.
func reviewHasGo(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".go") {
			return true
		}
	}
	return false
}

// reviewTestHeavy reports Go changes dominated by test files.
func reviewTestHeavy(files []string) bool {
	goFiles, testFiles := 0, 0
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f), ".go") {
			continue
		}
		goFiles++
		if strings.HasSuffix(strings.ToLower(f), "_test.go") {
			testFiles++
		}
	}
	return goFiles > 0 && testFiles*2 >= goFiles
}

// reviewCoverageExists reports whether the gauntlet's coverage profile
// is on disk for the crap lens to reuse.
func reviewCoverageExists(root string) bool {
	fi, err := os.Stat(filepath.Join(root, ".regulus", "coverage.out"))
	return err == nil && !fi.IsDir()
}

// reviewRunLens dispatches the single selected lens. Lens failures
// degrade to honest notes — a lens never fails the review.
func reviewRunLens(c CLI, name string, files []string, task string) {
	switch name {
	case "reviewer":
		reviewRunReviewer(c, files)
	case "crap":
		reviewRunCrap(c)
	case "mutation":
		reviewRunMutation(c, task)
	case "spec":
		reviewRunSpec(c, files)
	case "evidence":
		reviewRunEvidence(c, task)
	case "gate":
		reviewRunGate(c)
	}
}

// reviewCappedRead reads at most reviewMaxBytes of a file.
func reviewCappedRead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, reviewMaxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

// reviewRunReviewer is the senior read over the frozen files: sizes,
// markers that deserve a second look, and the Necessity Ladder
// checklist. Read-only.
func reviewRunReviewer(c CLI, files []string) {
	fmt.Fprintln(c.Stdout, "lens reviewer: senior read of touched files (not just the diff)")
	shown := files
	truncated := false
	if len(shown) > reviewMaxFiles {
		shown = shown[:reviewMaxFiles]
		truncated = true
	}
	totalLines := 0
	marked := map[string][]string{}
	for _, f := range shown {
		data, err := reviewCappedRead(f)
		if err != nil {
			fmt.Fprintf(c.Stdout, "  %s: unreadable (%v)\n", f, err)
			continue
		}
		lines := strings.Count(string(data), "\n")
		totalLines += lines
		fmt.Fprintf(c.Stdout, "  %s: %d line(s), %d B\n", f, lines, len(data))
		for _, m := range []string{"TODO", "FIXME", "XXX", "HACK", "panic(", "os.Exit("} {
			if strings.Contains(string(data), m) {
				marked[f] = append(marked[f], m)
			}
		}
	}
	if truncated {
		fmt.Fprintf(c.Stdout, "  ... and %d more file(s) (capped at %d)\n", len(files)-reviewMaxFiles, reviewMaxFiles)
	}
	if len(marked) == 0 {
		fmt.Fprintln(c.Stdout, "  markers: none (no TODO/FIXME/XXX/HACK/panic/os.Exit in capped reads)")
	} else {
		names := make([]string, 0, len(marked))
		for f := range marked {
			names = append(names, f)
		}
		sort.Strings(names)
		for _, f := range names {
			fmt.Fprintf(c.Stdout, "  markers %s: %s (second look)\n", f, strings.Join(marked[f], ", "))
		}
	}
	fmt.Fprintf(c.Stdout, "  necessity: %d file(s), ~%d lines in view — cut what the task does not need\n", len(files), totalLines)
	fmt.Fprintln(c.Stdout, "  never cut: validation at trust boundaries, error handling, security, accessibility")
	for _, f := range reviewUntestedGo(files) {
		fmt.Fprintf(c.Stdout, "  check: %s has no test companion in tree\n", f)
	}
}

// reviewUntestedGo lists touched non-test Go files with no _test.go
// sibling on disk.
func reviewUntestedGo(files []string) []string {
	var out []string
	for _, f := range files {
		lower := strings.ToLower(f)
		if !strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, "_test.go") {
			continue
		}
		dir := filepath.Dir(f)
		base := strings.TrimSuffix(filepath.Base(f), ".go")
		candidate := filepath.Join(dir, base+"_test.go")
		if _, err := os.Stat(candidate); err != nil {
			out = append(out, f)
		}
	}
	return out
}

// reviewRunSpec checks changed charts with the same spec.Check the
// gauntlet's spec gate uses. Charts untouched means nothing judged.
func reviewRunSpec(c CLI, files []string) {
	fmt.Fprintln(c.Stdout, "lens spec: acceptance quality of changed charts")
	var charts []string
	for _, f := range files {
		slashed := filepath.ToSlash(f)
		if strings.HasSuffix(strings.ToLower(f), ".md") && (strings.HasPrefix(slashed, "charts/") || strings.HasPrefix(slashed, "./charts/")) {
			charts = append(charts, f)
		}
	}
	if len(charts) == 0 {
		fmt.Fprintln(c.Stdout, "  no charts touched — nothing judged (full gate: `spec check charts/`)")
		return
	}
	for _, ch := range charts {
		data, err := os.ReadFile(ch)
		if err != nil {
			fmt.Fprintf(c.Stdout, "  %s: unreadable (%v)\n", ch, err)
			continue
		}
		n, violations := spec.Check(string(data))
		if len(violations) == 0 {
			fmt.Fprintf(c.Stdout, "  %s: spec OK (%d scenario(s))\n", ch, n)
			continue
		}
		fmt.Fprintf(c.Stdout, "  %s: %d violation(s)\n", ch, len(violations))
		for _, v := range violations {
			fmt.Fprintf(c.Stdout, "    - %s\n", v)
		}
	}
}

// reviewRunCrap reuses the gauntlet coverage profile over touched Go
// files. Without a profile it says so instead of inventing numbers.
func reviewRunCrap(c CLI) {
	fmt.Fprintln(c.Stdout, "lens crap: CRAP risk of touched functions (informative; gate enforces max 30)")
	profile := filepath.Join(c.dir(), ".regulus", "coverage.out")
	if _, err := os.Stat(profile); err != nil {
		fmt.Fprintln(c.Stdout, "  no coverage profile on disk — run the gauntlet (layers 4+7) for CRAP numbers")
		return
	}
	rep, err := crap.Analyze(c.dir(), profile)
	if err != nil {
		fmt.Fprintf(c.Stdout, "  crap unavailable: %v\n", err)
		return
	}
	fmt.Fprintf(c.Stdout, "  functions scored: %d, worst CRAP %.2f\n", len(rep.Functions), rep.Max)
	shown := 0
	for _, fn := range rep.Functions {
		if shown >= 5 {
			break
		}
		fmt.Fprintf(c.Stdout, "  %s:%d %s CRAP %.2f\n", fn.File, fn.StartLine, fn.Name, fn.CRAP)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(c.Stdout, "  no functions scored")
	}
}

// reviewRunEvidence audits the task's receipts the way the evidence
// gate would: counts by verdict. Without a task it scopes honestly.
func reviewRunEvidence(c CLI, task string) {
	fmt.Fprintln(c.Stdout, "lens evidence: receipts behind the claim (informative; auditor decides)")
	if task == "" {
		entries, err := evidence.List(c.dir(), "")
		if err != nil {
			fmt.Fprintf(c.Stdout, "  evidence unavailable: %v\n", err)
			return
		}
		fmt.Fprintf(c.Stdout, "  %d receipt(s) on disk across tasks — pass --task <t> to scope\n", len(entries))
		return
	}
	entries, err := evidence.List(c.dir(), task)
	if err != nil {
		fmt.Fprintf(c.Stdout, "  evidence unavailable: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Fprintf(c.Stdout, "  no receipts for task %s yet\n", task)
		return
	}
	pass, fail := 0, 0
	for _, e := range entries {
		if strings.EqualFold(e.Receipt.Verdict, "pass") {
			pass++
		} else {
			fail++
		}
	}
	fmt.Fprintf(c.Stdout, "  task %s: %d receipt(s), %d pass / %d non-pass\n", task, len(entries), pass, fail)
}

// reviewRunGate lists the gauntlet the change would face, without
// running it: layers, cost surface, mandatory policy.
func reviewRunGate(c CLI) {
	fmt.Fprintln(c.Stdout, "lens gate: the gauntlet this candidate would face (listed, not run)")
	cfg, err := gates.Load(filepath.Join(c.dir(), "regulus.json"))
	if err != nil {
		fmt.Fprintf(c.Stdout, "  gates unavailable: %v (not configured here)\n", err)
		return
	}
	if len(cfg.Gates) == 0 {
		fmt.Fprintln(c.Stdout, "  no gates configured")
		return
	}
	required := 0
	for _, g := range cfg.Gates {
		if g.IsRequired() {
			required++
		}
	}
	fmt.Fprintf(c.Stdout, "  %d gate(s), %d required; required always run under every profile\n", len(cfg.Gates), required)
	ordered := append([]gates.Gate(nil), cfg.Gates...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
	for _, g := range ordered {
		fmt.Fprintf(c.Stdout, "  layer %d %-16s %s\n", g.Layer, g.Name, g.Command)
	}
}

// reviewMutationPolicy states the mutation gate command from the local
// harness config, if one is configured here.
func reviewMutationPolicy(c CLI) {
	cfg, err := gates.Load(filepath.Join(c.dir(), "regulus.json"))
	if err != nil {
		return
	}
	for _, g := range cfg.Gates {
		if g.Name == "mutation" {
			fmt.Fprintf(c.Stdout, "  policy: %s\n", g.Command)
			return
		}
	}
}

// reviewLatestMutation finds the task's newest mutation receipt, if any.
func reviewLatestMutation(root, task string) (id, verdict string, efficacy *float64, found bool, listErr error) {
	entries, err := evidence.List(root, task)
	if err != nil {
		return "", "", nil, false, err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Receipt.Gate == "mutation" {
			r := entries[i].Receipt
			return r.ID, r.Verdict, r.MetricValue, true, nil
		}
	}
	return "", "", nil, false, nil
}

// reviewRunMutation states the mutation policy and the task's latest
// mutation receipt. Gremlins takes minutes, so the lens never runs it:
// it points at the floor the gauntlet enforces.
func reviewRunMutation(c CLI, task string) {
	fmt.Fprintln(c.Stdout, "lens mutation: efficacy floor for this candidate (stated, not run)")
	reviewMutationPolicy(c)
	if task == "" {
		fmt.Fprintln(c.Stdout, "  pass --task <t> to scope the latest mutation receipt")
		return
	}
	id, verdict, efficacy, found, err := reviewLatestMutation(c.dir(), task)
	if err != nil {
		fmt.Fprintf(c.Stdout, "  evidence unavailable: %v\n", err)
		return
	}
	if !found {
		fmt.Fprintf(c.Stdout, "  no mutation receipt for task %s yet — the gauntlet owns the verdict\n", task)
		return
	}
	fmt.Fprintf(c.Stdout, "  latest mutation receipt: %s verdict=%s\n", id, verdict)
	if efficacy != nil {
		fmt.Fprintf(c.Stdout, "  efficacy: %.1f\n", *efficacy)
	}
}
