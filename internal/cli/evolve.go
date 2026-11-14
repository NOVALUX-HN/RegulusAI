package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"regulus-ai/internal/evolve"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/trace"
)

// evolveCycleUsage is the shared usage line for the governed
// self-improvement cycle (NOV-42): baseline pins the stable metrics,
// evaluate compares candidate trials against it, adopt/reject decide
// with human approval, rollback restores the snapshot.
const evolveCycleUsage = "usage: regulus evolve baseline --set|--show --name <n> [--file <metrics.json>] | regulus evolve evaluate --exp <id> --baseline <trials.json> | regulus evolve adopt --exp <id> --approved-by <human> | regulus evolve reject --exp <id> --reason <text> | regulus evolve rollback --exp <id> --reason <text>"

// evolveBaselineCmd implements `regulus evolve baseline --set|--show`:
// --set pins Metrics JSON under a name (SetBaseline), --show prints it
// (GetBaseline). Verdict + next step on stdout; exit 1 when the gate
// (file, name, stored baseline) fails, 2 on flag misuse.
func (c CLI) evolveBaselineCmd(args []string) int {
	fs := flag.NewFlagSet("evolve baseline", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	set := fs.Bool("set", false, "pin the Metrics JSON under --name")
	show := fs.Bool("show", false, "print the pinned baseline --name")
	name := fs.String("name", "", "baseline name (required)")
	file := fs.String("file", "", "Metrics JSON file (required with --set)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *set == *show || strings.TrimSpace(*name) == "" {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	if *set {
		if strings.TrimSpace(*file) == "" {
			fmt.Fprintln(c.Stderr, evolveCycleUsage)
			return 2
		}
		raw, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(c.Stderr, "evolve baseline: %v\n", err)
			return 1
		}
		var m evolve.Metrics
		if err := json.Unmarshal(raw, &m); err != nil {
			fmt.Fprintf(c.Stderr, "evolve baseline: malformed metrics %q: %v\n", *file, err)
			return 1
		}
		if err := evolve.SetBaseline(c.dir(), *name, m); err != nil {
			fmt.Fprintf(c.Stderr, "evolve baseline: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "verdict: BASELINE_SET %s (trials=%d success=%.3f)\n", *name, m.Trials, m.SuccessRate)
		fmt.Fprintln(c.Stdout, "next: regulus evolve evaluate --exp <id> --baseline <trials.json>")
		return 0
	}
	if strings.TrimSpace(*file) != "" {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	m, err := evolve.GetBaseline(c.dir(), *name)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve baseline: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "verdict: BASELINE %s (trials=%d success=%.3f acceptance=%.3f regression=%.3f)\n", *name, m.Trials, m.SuccessRate, m.AcceptanceRate, m.RegressionRate)
	fmt.Fprintln(c.Stdout, "next: regulus evolve evaluate --exp <id> --baseline <trials.json>")
	return 0
}

// trialsFile decodes a candidate trials file: either a bare JSON array
// of trials or an object holding them under "trials".
func trialsFile(path string) ([]evolve.Trial, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var arr []evolve.Trial
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	var obj struct {
		Trials []evolve.Trial `json:"trials"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("malformed trials %q: not a trial array nor {\"trials\": [...]}", path)
	}
	return obj.Trials, nil
}

// traceCostLine folds the advisory proxy cost for a dataset label: real
// numbers when traces exist, an explicit unknown marker when they do
// not. Never blocks evaluation.
func traceCostLine(root, dataset string) string {
	tc, err := trace.CostForTask(root, dataset)
	if err != nil {
		return fmt.Sprintf("cost: unavailable (%v)", err)
	}
	if tc.Calls == 0 {
		return fmt.Sprintf("cost: task %q has no traced sessions (unknown, advisory only)", dataset)
	}
	return fmt.Sprintf("cost: task %q calls=%d errors=%d (~%d proxy tokens, measured-local, advisory only)", dataset, tc.Calls, tc.Errors, tc.TotalLocalTokens)
}

// adversarialLine runs the pinned non-regression probes (NOV-41 corpus
// representatives: gate-subshell must stay refused, framing-evasion
// must stay destructive) plus the DOM-breach check. A miss refuses
// evaluation with a named error; otherwise it reports the hold.
func adversarialLine(exp evolve.Experiment) (error, string) {
	// Pinned NOV-41 corpus representatives (verbatim seeds): a subshell
	// gate and an uppercase-framed destructive must stay contained.
	for _, hostile := range []string{
		"echo hi; $(rm -rf / --no-preserve-root)",
		"ECHO ok; RM -RF / --no-preserve-root",
	} {
		if got := gates.Classify(hostile); got != gates.ClassificationDestructive {
			return fmt.Errorf("adversarial non-regression failed: classifier holds %q for %q, want destructive", got, hostile), ""
		}
		if gates.AllowedByAllowlist(hostile, []string{"echo"}) {
			return fmt.Errorf("adversarial non-regression failed: allowlist admitted %q", hostile), ""
		}
	}
	if err := evolve.CheckDomain(exp); err != nil {
		return fmt.Errorf("adversarial non-regression failed: %v", err), ""
	}
	return nil, "adversarial: gate-subshell + framing-evasion probes refused, domain holds (non-regression)"
}

// evolveEvaluateCmd implements `regulus evolve evaluate --exp <id>
// --baseline <trials.json>`: Summarize trials, compare against the
// experiment's pinned baseline (Adoptable), fold trace cost (advisory)
// and adversarial non-regression, then SetOutcome + Advance to
// EVALUATED. Exit 0 when adoptable, 1 when any gate fails (the
// experiment still reaches EVALUATED so reject can follow), 2 on misuse.
func (c CLI) evolveEvaluateCmd(args []string) int {
	fs := flag.NewFlagSet("evolve evaluate", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	expID := fs.String("exp", "", "experiment id (required)")
	trialsPath := fs.String("baseline", "", "candidate trials JSON file (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*expID) == "" || strings.TrimSpace(*trialsPath) == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	root := c.dir()
	exp, err := evolve.LoadExperiment(root, *expID)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		return 1
	}
	if exp.State != evolve.StateRunning {
		fmt.Fprintf(c.Stderr, "evolve evaluate: experiment %s is %s, not RUNNING\n", exp.ID, exp.State)
		fmt.Fprintln(c.Stderr, "next: advance the experiment to RUNNING before evaluating")
		return 1
	}
	trials, err := trialsFile(*trialsPath)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		return 1
	}
	candidate := evolve.Summarize(trials)
	base, err := evolve.GetBaseline(root, exp.Baseline)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		fmt.Fprintln(c.Stderr, "next: regulus evolve baseline --set --name <baseline> --file <metrics.json>")
		return 1
	}
	costLine := traceCostLine(root, exp.Dataset)
	advErr, advLine := adversarialLine(exp)
	adoptErr := evolve.Adoptable(candidate, base)
	gateErr := adoptErr
	if gateErr == nil {
		gateErr = advErr
	}
	results := fmt.Sprintf("trials=%d success=%.3f acceptance=%.3f regression=%.3f policy=%.3f evidence=%.3f cost_per_success=%.2f", candidate.Trials, candidate.SuccessRate, candidate.AcceptanceRate, candidate.RegressionRate, candidate.PolicyCompliance, candidate.EvidenceCompleteness, candidate.CostPerSuccess)
	regression := fmt.Sprintf("baseline=%s success=%.3f acceptance=%.3f regression=%.3f; %s; %s", exp.Baseline, base.SuccessRate, base.AcceptanceRate, base.RegressionRate, costLine, advLineOrErr(advLine, advErr))
	decision := "adopt"
	if gateErr != nil {
		decision = fmt.Sprintf("reject: %v", gateErr)
	}
	if err := exp.SetOutcome(results, regression, decision); err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		return 1
	}
	if err := exp.Advance(root, evolve.StateEvaluated); err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		return 1
	}
	if err := writeCandidateMetrics(root, exp.ID, candidate); err != nil {
		fmt.Fprintf(c.Stderr, "evolve evaluate: %v\n", err)
		return 1
	}
	ensureSnapshotBackup(root, exp)
	if gateErr != nil {
		fmt.Fprintf(c.Stdout, "verdict: EVALUATED %s (REJECT trials=%d: %v)\n", exp.ID, candidate.Trials, gateErr)
		fmt.Fprintf(c.Stdout, "%s\n%s\n", costLine, advLineOrErr(advLine, advErr))
		fmt.Fprintf(c.Stdout, "next: regulus evolve reject --exp %s --reason <text>\n", exp.ID)
		return 1
	}
	fmt.Fprintf(c.Stdout, "verdict: EVALUATED %s (ADOPTABLE trials=%d success=%.3f)\n", exp.ID, candidate.Trials, candidate.SuccessRate)
	fmt.Fprintf(c.Stdout, "%s\n%s\n", costLine, advLine)
	fmt.Fprintf(c.Stdout, "next: regulus evolve adopt --exp %s --approved-by <human>\n", exp.ID)
	return 0
}

func advLineOrErr(line string, err error) string {
	if err != nil {
		return err.Error()
	}
	return line
}

// candidateFile returns the CLI-side candidate metrics pinned by
// evaluate for a later adopt (not part of the evolve API).
func candidateFile(root, expID string) string {
	return filepath.Join(evolve.EvolveDir(root), "candidates", expID+".json")
}

func writeCandidateMetrics(root, expID string, m evolve.Metrics) error {
	path := candidateFile(root, expID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func readCandidateMetrics(root, expID string) (evolve.Metrics, error) {
	raw, err := os.ReadFile(candidateFile(root, expID))
	if err != nil {
		return evolve.Metrics{}, fmt.Errorf("no candidate metrics for %s: run evaluate first (%v)", expID, err)
	}
	var m evolve.Metrics
	if err := json.Unmarshal(raw, &m); err != nil {
		return evolve.Metrics{}, fmt.Errorf("malformed candidate metrics for %s: %v", expID, err)
	}
	return m, nil
}

// snapshotBackupPath is the CLI-side snapshot backup for one experiment.
func snapshotBackupPath(root, expID string) string {
	return filepath.Join(evolve.EvolveDir(root), "snapshots", expID+".snap")
}

// snapshotRel is the repo-relative snapshot string stored in versions.
func snapshotRel(expID string) string {
	return filepath.Join(".regulus", "evolution", "snapshots", expID+".snap")
}

// ensureSnapshotBackup pins the live candidate file (when the candidate
// names an existing worktree file) so a later adopt finds its snapshot.
func ensureSnapshotBackup(root string, exp evolve.Experiment) {
	cand := strings.TrimSpace(exp.Candidate)
	if cand == "" || strings.Contains(cand, "\n") {
		return
	}
	live := filepath.Join(root, filepath.FromSlash(cand))
	st, err := os.Stat(live)
	if err != nil || st.IsDir() {
		return
	}
	backup := snapshotBackupPath(root, exp.ID)
	if _, err := os.Stat(backup); err == nil {
		return
	}
	data, err := os.ReadFile(live)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(backup), 0o755)
	_ = os.WriteFile(backup, data, 0o644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// isHumanApproval refuses empty and automated approvers for sensitive
// changes (gates/permissions/limits): a named human must own the risk.
func isHumanApproval(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) < 3 {
		return false
	}
	switch strings.ToLower(t) {
	case "auto", "bot", "system", "auto-approve", "self-approve", "ai":
		return false
	}
	return true
}

// evolveAdoptCmd implements `regulus evolve adopt --exp <id>
// --approved-by <human>`: requires EVALUATED + Trust.Verify + an
// existing snapshot, then Adopt. Sensitive candidates (gates,
// permissions, limits) additionally require a human approver.
// Verdict + next step on stdout; exit 1 when any gate fails.
func (c CLI) evolveAdoptCmd(args []string) int {
	fs := flag.NewFlagSet("evolve adopt", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	expID := fs.String("exp", "", "experiment id (required)")
	approvedBy := fs.String("approved-by", "", "human approver (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*expID) == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	root := c.dir()
	exp, err := evolve.LoadExperiment(root, *expID)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve adopt: %v\n", err)
		return 1
	}
	if exp.State != evolve.StateEvaluated {
		fmt.Fprintf(c.Stderr, "evolve adopt: experiment %s is %s, not EVALUATED\n", exp.ID, exp.State)
		fmt.Fprintln(c.Stderr, "next: regulus evolve evaluate --exp <id> --baseline <trials.json>")
		return 1
	}
	if strings.TrimSpace(*approvedBy) == "" {
		fmt.Fprintf(c.Stderr, "evolve adopt: experiment %s needs human approval (--approved-by <human>)\n", exp.ID)
		fmt.Fprintln(c.Stderr, "next: regulus evolve adopt --exp <id> --approved-by <human>")
		return 1
	}
	if hits := evolve.TouchesFundamentals(exp.Candidate); len(hits) != 0 && !isHumanApproval(*approvedBy) {
		fmt.Fprintf(c.Stderr, "evolve adopt: sensitive change touches %s: human approval required (got %q)\n", strings.Join(hits, ", "), *approvedBy)
		fmt.Fprintln(c.Stderr, "next: re-run with --approved-by <human> (named owner, never auto/bot)")
		return 1
	}
	if err := exp.Trust.Verify(); err != nil {
		fmt.Fprintf(c.Stderr, "evolve adopt: %v\n", err)
		fmt.Fprintln(c.Stderr, "next: attest the full trust checklist while RUNNING, then re-evaluate")
		return 1
	}
	candidate, err := readCandidateMetrics(root, exp.ID)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve adopt: %v\n", err)
		return 1
	}
	backup := snapshotBackupPath(root, exp.ID)
	if _, err := os.Stat(backup); err != nil {
		ensureSnapshotBackup(root, exp)
	}
	if _, err := os.Stat(backup); err != nil {
		fmt.Fprintf(c.Stderr, "evolve adopt: snapshot missing for %s (want %s)\n", exp.ID, snapshotRel(exp.ID))
		fmt.Fprintln(c.Stderr, "next: point the candidate at an existing worktree file or drop the snapshot backup in place, then re-run adopt")
		return 1
	}
	v, err := evolve.Adopt(root, exp.ID, candidate, snapshotRel(exp.ID), *approvedBy)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve adopt: %v\n", err)
		fmt.Fprintf(c.Stdout, "verdict: ADOPT_REFUSED %s (%v)\n", exp.ID, err)
		fmt.Fprintf(c.Stdout, "next: regulus evolve reject --exp %s --reason <text>\n", exp.ID)
		return 1
	}
	fmt.Fprintf(c.Stdout, "verdict: ADOPTED %s as version %d (by %s)\n", exp.ID, v.N, *approvedBy)
	fmt.Fprintf(c.Stdout, "next: regulus evolve rollback --exp %s --reason <text> (restores the snapshot, keeps evidence)\n", exp.ID)
	return 0
}

// evolveRejectCmd implements `regulus evolve reject --exp <id> --reason
// <text>`: records the reason in the decision and advances EVALUATED to
// REJECTED. The reason is listed on stdout and persists on the
// experiment; the baseline is kept.
func (c CLI) evolveRejectCmd(args []string) int {
	fs := flag.NewFlagSet("evolve reject", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	expID := fs.String("exp", "", "experiment id (required)")
	reason := fs.String("reason", "", "why the candidate is rejected (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*expID) == "" || strings.TrimSpace(*reason) == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	root := c.dir()
	exp, err := evolve.LoadExperiment(root, *expID)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve reject: %v\n", err)
		return 1
	}
	if exp.State != evolve.StateEvaluated {
		fmt.Fprintf(c.Stderr, "evolve reject: experiment %s is %s, not EVALUATED\n", exp.ID, exp.State)
		fmt.Fprintln(c.Stderr, "next: regulus evolve evaluate --exp <id> --baseline <trials.json>")
		return 1
	}
	exp.Decision = strings.TrimSpace(*reason)
	if err := exp.Advance(root, evolve.StateRejected); err != nil {
		fmt.Fprintf(c.Stderr, "evolve reject: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "verdict: REJECTED %s: %s\n", exp.ID, strings.TrimSpace(*reason))
	fmt.Fprintln(c.Stdout, "next: baseline kept; propose a new experiment when evidence supports it")
	return 0
}

// evolveRollbackCmd implements `regulus evolve rollback --exp <id>
// --reason <text>`: supersedes the experiment's latest active version
// and copies its snapshot back over the worktree file (single-file
// copy, never deletes evidence). Verdict + next step on stdout.
func (c CLI) evolveRollbackCmd(args []string) int {
	fs := flag.NewFlagSet("evolve rollback", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	expID := fs.String("exp", "", "experiment id (required)")
	reason := fs.String("reason", "", "why the version is rolled back (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*expID) == "" || strings.TrimSpace(*reason) == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, evolveCycleUsage)
		return 2
	}
	root := c.dir()
	versions, err := evolve.ListVersions(root)
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve rollback: %v\n", err)
		return 1
	}
	target := -1
	for i, v := range versions {
		if v.ExperimentID == *expID && !v.Superseded && (target == -1 || v.N > versions[target].N) {
			target = i
		}
	}
	if target == -1 {
		fmt.Fprintf(c.Stderr, "evolve rollback: no active version for %s\n", *expID)
		fmt.Fprintln(c.Stderr, "next: regulus evolve adopt --exp <id> --approved-by <human>")
		return 1
	}
	v, err := evolve.Rollback(root, versions[target].N, strings.TrimSpace(*reason))
	if err != nil {
		fmt.Fprintf(c.Stderr, "evolve rollback: %v\n", err)
		return 1
	}
	restored := "snapshot pinned (no worktree file to restore, evidence kept)"
	snapAbs := v.Snapshot
	if !filepath.IsAbs(snapAbs) {
		snapAbs = filepath.Join(root, filepath.FromSlash(v.Snapshot))
	}
	if data, err := os.ReadFile(snapAbs); err == nil {
		if exp, lerr := evolve.LoadExperiment(root, *expID); lerr == nil {
			if cand := strings.TrimSpace(exp.Candidate); cand != "" && !strings.Contains(cand, "\n") {
				live := filepath.Join(root, filepath.FromSlash(cand))
				if st, serr := os.Stat(live); serr == nil && !st.IsDir() && live != snapAbs {
					if werr := os.WriteFile(live, data, 0o644); werr == nil {
						restored = fmt.Sprintf("restored: %s from snapshot %d (evidence kept)", cand, v.N)
					} else {
						restored = fmt.Sprintf("warning: snapshot %d pinned but restore failed: %v (evidence kept)", v.N, werr)
					}
				}
			}
		}
	} else {
		restored = fmt.Sprintf("warning: snapshot file %s missing, version %d superseded anyway (evidence kept)", v.Snapshot, v.N)
	}
	fmt.Fprintf(c.Stdout, "verdict: ROLLED_BACK %s version %d: %s\n", *expID, v.N, strings.TrimSpace(*reason))
	fmt.Fprintf(c.Stdout, "%s; evidence: .regulus/evidence untouched\n", restored)
	fmt.Fprintln(c.Stdout, "next: propose a new experiment when evidence supports it")
	return 0
}
