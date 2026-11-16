package gates

// gate selection and gauntlet orchestration (NOV-67 split; behavior unchanged).

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"regulus-ai/internal/verdict"
)

// Run executes the gauntlet for one task, writing one append-only receipt
// per executed gate into evidenceDir. It stops at the first failure.
// Signature is UNCHANGED (probe calls it directly): policy is evaluated for
// every runner including fakes, and receipts carry the centralized verdict.
// runStamp links one Run to its candidate manifest. Empty means the
// probe path: receipts verify on hashes alone.
type runStamp struct {
	runID        string
	manifestFile string
}

func Run(ctx context.Context, cfg Config, evidenceDir, task, gitHead string, r CmdRunner) (Result, error) {
	return run(ctx, cfg, nil, evidenceDir, task, gitHead, r, runStamp{}, "", "")
}

// SelectGates resolves which gates run: "" and "full" select everything
// (today's behavior), "quick" selects required gates only, and any other
// name must be declared by at least one gate's profiles list — an unknown
// profile is an error, never a silent subset.
func SelectGates(cfg Config, profile string) ([]Gate, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	switch profile {
	case "", ProfileFull:
		return append([]Gate(nil), cfg.Gates...), nil
	case ProfileQuick:
		var out []Gate
		for _, g := range cfg.Gates {
			if g.IsRequired() {
				out = append(out, g)
			}
		}
		return out, nil
	}
	declared := map[string]bool{}
	for _, g := range cfg.Gates {
		for _, p := range g.Profiles {
			declared[p] = true
		}
	}
	if !declared[profile] {
		var names []string
		for p := range declared {
			names = append(names, p)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown profile %q (declared: %s; reserved: quick, full)", profile, strings.Join(names, ", "))
	}
	var out []Gate
	for _, g := range cfg.Gates {
		if g.IsRequired() {
			out = append(out, g)
			continue
		}
		for _, p := range g.Profiles {
			if p == profile {
				out = append(out, g)
				break
			}
		}
	}
	return out, nil
}

// changedFiles lists worktree paths from git status (tracked modifications
// plus untracked files, honoring git's own excludes). ok=false when git is
// unavailable: conditions are then unevaluable.
func changedFiles(ctx context.Context, r CmdRunner) ([]string, bool) {
	gctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, _, code, err := r.Run(gctx, "git status --porcelain=v1 -z --untracked-files=normal -- .")
	if err != nil || code != 0 {
		return nil, false
	}
	var paths []string
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
		paths = append(paths, path)
		if xy[0] == 'R' {
			i++
		}
	}
	return paths, true
}

func matchesAnyGate(g Gate, changed []string) bool {
	for _, c := range changed {
		if matchesAny(c, g.When) {
			return true
		}
	}
	return false
}

func matchesAny(path string, patterns []string) bool {
	for _, p := range patterns {
		if ok, err := filepath.Match(p, path); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(p, filepath.Base(path)); err == nil && ok {
			return true
		}
	}
	return false
}

// runnerFacts maps a runner failure to verdict facts: a run killed for
// exceeding the output cap exhausted its resources, a deadline kill is an
// infrastructure timeout, and anything else never really ran.
func runnerFacts(gctx context.Context, rerr error) verdict.Facts {
	var capErr *outputCapError
	if errors.As(rerr, &capErr) {
		return verdict.Facts{Executed: true, ResourceExhausted: true}
	}
	if gctx.Err() != nil {
		return verdict.Facts{Executed: false, TimedOut: true}
	}
	return verdict.Facts{Executed: false, SpawnError: true}
}

func run(ctx context.Context, cfg Config, cfgBytes []byte, evidenceDir, task, gitHead string, r CmdRunner, stamp runStamp, profile, root string) (Result, error) {
	setup, err := selectOrdered(cfg, profile)
	if err != nil {
		return Result{}, err
	}
	ordered, max, dockerImage, execEnvUnset := setup.ordered, setup.max, setup.dockerImage, setup.execEnvUnset

	// Execution conditions are evaluated once per run against the worktree.
	// Required gates always run: when-conditions only ever skip optional
	// gates, and unevaluable conditions fail open with a receipt note.
	var changed []string
	changedOK := false
	needsChanged := false
	for _, g := range ordered {
		if len(g.When) > 0 {
			needsChanged = true
			break
		}
	}
	if needsChanged {
		changed, changedOK = changedFiles(ctx, r)
	}

	var res Result
	for _, g := range ordered {
		if skip, reason := computeSkip(g, changed, changedOK); skip {
			res.Outcomes = append(res.Outcomes, Outcome{Name: g.Name, Layer: g.Layer, Verdict: "skip", Status: "INCONCLUSIVE", Reason: "insufficient_evidence", Skipped: true, SkipReason: reason})
			res.Skipped = append(res.Skipped, g.Name)
			continue
		}
		condNote := whenNote(g, changed, changedOK)
		timeout := effectiveTimeout(g.TimeoutSec, max)
		refused, refuseNote, classification := refusePolicy(g, cfg)

		// Coverage cache (T5): an unchanged config+HEAD reuses the
		// existing coverprofile instead of recomputing it. The
		// synthetic outcome still writes an honest PASS receipt.
		if !refused {
			if cached, ok := coverageHit(root, cfgBytes, gitHead, g); ok {
				metricValue, metricThreshold, metricOp, _, metricNote, facts := evalMetric(g, cached.stdout, cached.code, cached.rerr, refused, cached.facts)
				stop, err := adjudicateAndWrite(g, &res, evidenceDir, task, gitHead, stamp, timeout, classification, cached, metricValue, metricThreshold, metricOp, condNote, metricNote, facts)
				if err != nil {
					return res, err
				}
				if stop {
					return res, nil
				}
				continue
			}
		}

		ex := execGate(ctx, g, r, timeout, dockerImage, execEnvUnset, setup.inheritEnv, setup.envAllow, refused, refuseNote)

		metricValue, metricThreshold, metricOp, metricFailed, metricNote, facts := evalMetric(g, ex.stdout, ex.code, ex.rerr, refused, ex.facts)

		stop, err := adjudicateAndWrite(g, &res, evidenceDir, task, gitHead, stamp, timeout, classification, ex, metricValue, metricThreshold, metricOp, condNote, metricNote, facts)
		if err != nil {
			return res, err
		}
		if stop {
			return res, nil
		}
		// The gate genuinely recomputed coverage and passed: pin the
		// config+HEAD key so the next identical run reuses the file.
		if !refused && ex.rerr == nil && ex.code == 0 && !metricFailed {
			if cover, ok := coverageProfilePath(g.Command); ok {
				WriteCoverageCache(root, cfgBytes, gitHead, cover)
			}
		}
	}
	res.Status = string(verdict.StatusPass)
	res.Reason = string(verdict.ReasonOK)
	return res, nil
}

func isShRunner(r CmdRunner) bool {
	_, ok1 := r.(ShRunner)
	if ok1 {
		return true
	}
	_, ok2 := r.(*ShRunner)
	return ok2
}

func asShRunner(r CmdRunner) (ShRunner, bool) {
	if v, ok := r.(ShRunner); ok {
		return v, true
	}
	if v, ok := r.(*ShRunner); ok && v != nil {
		return *v, true
	}
	return ShRunner{}, false
}

// GitHeadTimeout bounds every `git rev-parse --verify HEAD` probe (S3
// hygiene): git must never hang the harness on a wedged repository.
const GitHeadTimeout = 30 * time.Second

// GitHead returns the current HEAD hash, or "" outside a repository.
// It respects ctx deadlines without hanging: an already-expired ctx
// returns "" without invoking the runner, and a ctx without a deadline
// is bounded by GitHeadTimeout so a wedged git cannot wedge the caller.
func GitHead(ctx context.Context, r CmdRunner) string {
	if ctx.Err() != nil {
		return ""
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, GitHeadTimeout)
		defer cancel()
	}
	out, _, _, err := r.Run(ctx, "git rev-parse --verify HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

var metricNumberRe = regexp.MustCompile(`[-+]?[0-9]*\.?[0-9]+`)

// extractMetric pulls one number out of a metric gate's stdout. Both modes
// read the last non-empty line; the percent mode strips a trailing "%"
// first. Within that line the LAST numeric token wins.
func extractMetric(extract, stdout string) (float64, error) {
	lines := strings.Split(stdout, "\n")
	var last string
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			last = s
			break
		}
	}
	if last == "" {
		return 0, errors.New("could not extract a metric: the command printed nothing")
	}
	if extract == ExtractLastLinePercent {
		last = strings.TrimSuffix(last, "%")
	}
	matches := metricNumberRe.FindAllString(last, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	v, err := strconv.ParseFloat(matches[len(matches)-1], 64)
	if err != nil {
		return 0, fmt.Errorf("could not extract a metric from %q", last)
	}
	return v, nil
}
