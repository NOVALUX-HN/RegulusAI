// Verification test runner (F15.4/NOV-125): smoke, focused and full
// execution of the real Go test suite through the centralized
// executor — never a direct test binary, never a bare `go test`
// the runner does not observe. Every package gets a verdict;
// full mode fails on anything but all-green.
//
// Reuse, never duplication:
//   - launches: pxexec only (go list, go build, go vet, go test),
//     with per-package timeouts and BLOCKED classification.
//   - discovery: `go list -f` with test-file presence, so zero-test
//     packages can never count as passed.
//   - report: versioned JSON (commit, host, versions, mechanism,
//     discovered/executed counts, per-package results).
package testflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/pxexec"
)

// ReportVersion versions the execution report.
const ReportVersion = "testflow-report-v1"

// Modes: smoke (curated fast core), focused (explicit packages),
// full (every discovered package).
const (
	ModeSmoke   = "smoke"
	ModeFocused = "focused"
	ModeFull    = "full"
)

// Outcomes reuse the executor vocabulary.
const (
	OutcomePass    = pxexec.OutcomePass
	OutcomeFail    = pxexec.OutcomeFail
	OutcomeBlocked = pxexec.OutcomeBlocked
	OutcomeSkipped = pxexec.OutcomeSkipped
	OutcomeNotRun  = pxexec.OutcomeNotRun
)

// SmokePackages is the curated fast core: deterministic, no
// multi-process orchestration, sub-minute even cold.
var SmokePackages = []string{
	"regulus-ai/internal/pxexec",
}

// PerPackageTimeout bounds one package (slow suites must still
// finish); ListTimeout bounds discovery; BuildTimeout bounds the
// separate build check.
const (
	ListTimeout    = 60 * time.Second
	BuildTimeout   = 600 * time.Second
	PackageTimeout = 600 * time.Second
)

// PackageResult is one package verdict with its evidence.
type PackageResult struct {
	Package    string
	Outcome    string
	Detail     string
	DurationMS int64
}

// Report is the versioned execution record. Skipped holds only
// caller-allowed skips; anything else missing from Results is
// NOT_RUN and fails full mode.
type Report struct {
	Version    string
	Mode       string
	Commit     string
	Host       string
	GoVersion  string
	Regulus    string
	At         string
	Discovered []string
	Skipped    []string
	Results    []PackageResult
	Checks     []PackageResult
}

// Request selects the run: mode, explicit packages for focused,
// allowed skips, output cap per package, save path ("" = no save).
type Request struct {
	Mode     string
	Packages []string
	Skipped  []string
	Save     string
	Regulus  string
}

// Run executes the requested flow and returns the report plus the
// process verdict: true only on all-green (full additionally
// requires complete coverage and no unallowed skips).
func Run(ctx context.Context, req Request) (Report, bool) {
	rep := Report{Version: ReportVersion, Mode: req.Mode, Regulus: req.Regulus,
		Host: hostname(), At: time.Now().UTC().Format(time.RFC3339)}
	rep.GoVersion = goVersion(ctx)
	rep.Commit = gitHead(ctx)
	allowed := map[string]bool{}
	for _, s := range req.Skipped {
		allowed[strings.TrimSpace(s)] = true
	}
	var pkgs []string
	switch req.Mode {
	case ModeSmoke:
		pkgs = append([]string{}, SmokePackages...)
	case ModeFocused:
		pkgs = append([]string{}, req.Packages...)
		if len(pkgs) == 0 {
			return rep, false
		}
	case ModeFull:
		var err error
		pkgs, err = discover(ctx)
		if err != nil {
			rep.Results = append(rep.Results, PackageResult{Package: "(discovery)", Outcome: OutcomeNotRun, Detail: err.Error()})
			return rep, false
		}
	default:
		return rep, false
	}
	rep.Discovered = append([]string{}, pkgs...)
	for _, p := range pkgs {
		if allowed[p] {
			rep.Skipped = append(rep.Skipped, p)
			continue
		}
		rep.Results = append(rep.Results, runPackage(ctx, p))
	}
	rep.Checks = runChecks(ctx)
	if req.Save != "" {
		_ = saveReport(req.Save, rep)
	}
	return rep, Verdict(rep, allowed)
}

// Verdict decides: every executed package PASS, every check PASS,
// no unallowed skips, and full coverage (executed + allowed-skipped
// == discovered). A blocked launch or an unrun package is never green.
func Verdict(rep Report, allowed map[string]bool) bool {
	if len(rep.Results) == 0 && len(rep.Skipped) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range rep.Results {
		seen[r.Package] = true
		if r.Outcome != OutcomePass {
			return false
		}
	}
	for _, c := range rep.Checks {
		if c.Outcome != OutcomePass {
			return false
		}
	}
	for _, d := range rep.Discovered {
		if allowed[d] {
			continue
		}
		if !seen[d] {
			return false
		}
	}
	return true
}

// discover lists test-bearing packages via `go list`.
func discover(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, ListTimeout)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "go",
		Args:    []string{"list", "-f", "{{.ImportPath}}|{{if .TestGoFiles}}yes{{else}}no{{end}}", "./internal/..."},
		Timeout: ListTimeout})
	if res.Outcome != pxexec.OutcomePass {
		return nil, fmt.Errorf("package discovery %s: %s", res.Outcome, res.Detail)
	}
	var pkgs []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// <importpath>|yes — the pipe separator survives multi-file
		// TestGoFiles lists, which the old space-split dropped.
		if parts := strings.SplitN(line, "|", 2); len(parts) == 2 && parts[1] == "yes" {
			pkgs = append(pkgs, parts[0])
		}
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

// classifyGoTest is the pure verdict core: executor outcome plus
// go output. Kept pure so the runner's own tests pin it without
// spawning anything.
func classifyGoTest(outcome, out string) (string, string) {
	switch outcome {
	case pxexec.OutcomePass:
		if strings.Contains(out, "no test files") || (strings.Contains(out, "no tests to run") && !strings.Contains(out, "ok ")) {
			return OutcomeNotRun, "zero tests executed: not a pass"
		}
		return OutcomePass, firstLine(out)
	case pxexec.OutcomeBlocked:
		return OutcomeBlocked, "execution blocked by policy"
	case pxexec.OutcomeSkipped:
		return OutcomeSkipped, "skipped by caller"
	default:
		return OutcomeFail, firstLine(out)
	}
}

// runPackage executes one package suite and classifies it. A test
// binary blocked by policy is BLOCKED (never PASS, never FAIL);
// an empty run (zero tests discovered by go, e.g. [no test files])
// refuses the PASS.
func runPackage(ctx context.Context, pkg string) PackageResult {
	start := time.Now()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "go",
		Args: []string{"test", "-count=1", pkg}, Timeout: PackageTimeout})
	out := res.Combined
	dur := time.Since(start).Milliseconds()
	outcome, detail := classifyGoTest(res.Outcome, out)
	if outcome != OutcomePass {
		// Tail for diagnosis: the first line says what, the tail says why.
		if tail := lastLines(out, 20); tail != "" && tail != detail {
			if len(tail) > 2000 {
				tail = tail[len(tail)-2000:]
			}
			detail += "\n[tail]\n" + tail
		}
	}
	if res.Outcome == pxexec.OutcomeBlocked {
		detail += ": " + res.BlockHint
	}
	if res.Outcome != pxexec.OutcomePass && res.Outcome != pxexec.OutcomeBlocked {
		detail += " [" + res.ErrClass + "]"
	}
	return PackageResult{Package: pkg, Outcome: outcome, Detail: detail, DurationMS: dur}
}

// runChecks runs the non-test gates as separate entries: build and
// vet are evidence, never substitutes for executed tests.
func runChecks(ctx context.Context) []PackageResult {
	var out []PackageResult
	for _, c := range []struct {
		name string
		args []string
	}{
		{"build ./...", []string{"build", "./..."}},
		{"vet ./internal/pxexec", []string{"vet", "./internal/pxexec/"}},
	} {
		start := time.Now()
		res := pxexec.Run(ctx, pxexec.Spec{Name: "go", Args: c.args, Timeout: BuildTimeout})
		pr := PackageResult{Package: "(check) " + c.name, DurationMS: time.Since(start).Milliseconds()}
		if res.Outcome == pxexec.OutcomePass {
			pr.Outcome, pr.Detail = OutcomePass, "exit 0"
		} else {
			pr.Outcome, pr.Detail = OutcomeFail, res.Outcome+": "+res.Detail
		}
		out = append(out, pr)
	}
	return out
}

func goVersion(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "go", Args: []string{"version"}, Timeout: 30 * time.Second})
	if res.Outcome != pxexec.OutcomePass {
		return "unknown"
	}
	return strings.TrimSpace(res.Stdout)
}

func gitHead(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	res := pxexec.Run(ctx, pxexec.Spec{Name: "git", Args: []string{"rev-parse", "--short", "HEAD"}, Timeout: 30 * time.Second})
	if res.Outcome != pxexec.OutcomePass {
		return "unknown"
	}
	return strings.TrimSpace(res.Stdout)
}

// saveReport persists the versioned execution record. A save
// failure is reported, never silent, and never flips the verdict.
func saveReport(path string, rep Report) error {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func hostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return runtime.GOOS + "-unknown"
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
