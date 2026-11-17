// Close traceability (NOV-64, S4): Done must be backed by linked
// evidence, not by the implementer's word. CheckTraceability verifies
// that closing an issue cites at least one commit, that multi-issue
// commits carry an explicit justification, that the linked diffs stay
// inside the chart scope, and — for P0 — that an independent review
// verdict exists. It reuses the NOV-32 scope enforcement and the T3-02
// guardian log; it creates no parallel mechanism.
//
// Only enforced when the caller passes --issue: plain closes keep
// their existing behavior.
package sextant

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"regulus-ai/internal/guardians"
	"regulus-ai/internal/pxexec"
)

// TraceOptions names the issue being closed and its provenance.
type TraceOptions struct {
	Issue             string // e.g. NOV-64 (required)
	Justified         string // reason covering every multi-issue commit (flag-level)
	Implementer       string // who did the work (git author, worker profile)
	IndependentReview bool   // P0: require a review verdict by someone else
	ChartBase         string // evidence task prefix, like CheckClose
	TaskID            string // chart task id for the evaluations log
	Scope             Scope  // chart scope; undeclared disables the diff check
}

var issueRefRe = regexp.MustCompile(`NOV-\d+`)
var justifiedRe = regexp.MustCompile(`(?im)^justified:\s*(.+)$`)

// TraceCommit is one linked commit: its identity plus every other
// issue it cites (multi-issue needs justification).
type TraceCommit struct {
	SHA    string
	Author string
	Email  string
	Others []string
}

// CheckTraceability verifies issue→commit→scope→review linkage.
// Empty means the closure is auditable; every entry names the gap.
func CheckTraceability(root string, opts TraceOptions) []string {
	issue := strings.ToUpper(strings.TrimSpace(opts.Issue))
	if issue == "" {
		return []string{"traceability: no --issue given"}
	}
	commits, err := linkedCommits(root, issue)
	if err != nil {
		return []string{fmt.Sprintf("traceability: %v", err)}
	}
	if len(commits) == 0 {
		return []string{fmt.Sprintf("traceability: no commit cites %s (close needs at least one linked commit)", issue)}
	}
	var unmet []string
	for _, c := range commits {
		if len(c.Others) != 0 && opts.Justified == "" && !hasJustifiedTrailer(root, c.SHA) {
			unmet = append(unmet, fmt.Sprintf("traceability: commit %s cites %s without justification (--justified or Justified: trailer)", shortSHA(c.SHA), strings.Join(c.Others, ", ")))
		}
		if !opts.Scope.Declared {
			continue
		}
		files, err := commitFiles(root, c.SHA)
		if err != nil {
			unmet = append(unmet, fmt.Sprintf("traceability: commit %s diff unreadable: %v", shortSHA(c.SHA), err))
			continue
		}
		if err := opts.Scope.CheckScopeFilesIn(root, files); err != nil {
			unmet = append(unmet, fmt.Sprintf("traceability: commit %s leaves the chart scope: %v", shortSHA(c.SHA), err))
		}
	}
	if opts.IndependentReview {
		if u := checkIndependentReview(root, opts); u != "" {
			unmet = append(unmet, u)
		}
	}
	sort.Strings(unmet)
	return unmet
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// git runs git -C root and returns trimmed stdout. The error preserves
// the "git ARGS: exit status N (STDERR)" form.
func git(root string, args ...string) (string, error) {
	res := pxexec.Run(context.Background(), pxexec.Spec{
		Name: "git", Args: append([]string{"-C", root}, args...),
	})
	if res.Outcome == pxexec.OutcomePass {
		return strings.TrimRight(res.Stdout, "\n"), nil
	}
	if res.ErrClass == pxexec.ErrFail {
		return "", fmt.Errorf("git %s: exit status %d (%s)", strings.Join(args, " "), res.Exit, strings.TrimSpace(res.Stderr))
	}
	return "", fmt.Errorf("git %s: %s (%s)", strings.Join(args, " "), res.Detail, strings.TrimSpace(res.Stderr))
}

// linkedCommits lists commits citing the issue, plus the other issues
// each one cites.
func linkedCommits(root, issue string) ([]TraceCommit, error) {
	raw, err := git(root, "log", "--format=%H%x00%an%x00%ae%x00%s%x00%b%x1e", "--grep="+issue)
	if err != nil || strings.TrimSpace(raw) == "" {
		if err != nil {
			return nil, err
		}
		return nil, nil
	}
	var out []TraceCommit
	for _, rec := range strings.Split(raw, "\x1e") {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x00")
		if len(f) < 5 {
			continue
		}
		seen := map[string]bool{}
		var others []string
		for _, id := range issueRefRe.FindAllString(f[3]+"\n"+f[4], -1) {
			id = strings.ToUpper(id)
			if id != issue && !seen[id] {
				seen[id] = true
				others = append(others, id)
			}
		}
		sort.Strings(others)
		out = append(out, TraceCommit{SHA: f[0], Author: f[1], Email: f[2], Others: others})
	}
	return out, nil
}

func hasJustifiedTrailer(root, sha string) bool {
	raw, err := git(root, "log", "-1", "--format=%B", sha)
	if err != nil {
		return false
	}
	return justifiedRe.MatchString(raw)
}

// commitFiles lists the files one commit touches (-m splits merges).
func commitFiles(root, sha string) ([]string, error) {
	raw, err := git(root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-m", sha)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(raw, "\n") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, filepath.FromSlash(f))
		}
	}
	return files, nil
}

// checkIndependentReview requires one recorded evaluation by someone
// other than the implementer (guardian registry membership was
// already enforced at record time by T3-02; here the rule is only
// independence). An empty implementer refuses: independence cannot
// be judged against nobody.
func checkIndependentReview(root string, opts TraceOptions) string {
	impl := normalizeReviewer(opts.Implementer)
	if impl == "" {
		return "traceability: P0 independent review needs --implementer (independence cannot be judged against nobody)"
	}
	var evals []guardians.Evaluation
	found := false
	for _, task := range []string{opts.ChartBase + "/" + opts.TaskID, opts.TaskID} {
		data, err := readEvaluations(root, task)
		if err != nil || !data.found {
			continue
		}
		found = true
		evals = append(evals, data.evals...)
	}
	if !found {
		return fmt.Sprintf("traceability: P0 needs an independent review verdict for %s/%s (no evaluations log)", opts.ChartBase, opts.TaskID)
	}
	for _, e := range evals {
		if strings.TrimSpace(e.Note) == "" {
			continue
		}
		if normalizeReviewer(e.Guardian) != impl {
			return ""
		}
	}
	return fmt.Sprintf("traceability: P0 needs a review by someone other than %q (reviewer equal to implementer refuses)", opts.Implementer)
}

func normalizeReviewer(id string) string {
	id = strings.TrimSpace(strings.TrimPrefix(id, "@"))
	return strings.ToLower(id)
}

type evalLog struct {
	found bool
	evals []guardians.Evaluation
}

// readEvaluations loads one task's guardian log; missing file means
// no log, malformed lines fail the read (never silently skipped).
func readEvaluations(root, task string) (evalLog, error) {
	p := guardians.EvaluationsFile(root, task)
	data, err := os.ReadFile(p)
	if err != nil {
		return evalLog{}, err
	}
	var out []guardians.Evaluation
	for i, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var e guardians.Evaluation
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			return evalLog{}, fmt.Errorf("evaluations log line %d malformed: %v", i+1, err)
		}
		out = append(out, e)
	}
	return evalLog{found: true, evals: out}, nil
}
