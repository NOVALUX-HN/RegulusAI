package gates

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/receipts"
	"regulus-ai/internal/verdict"
)

// runSetup is the shared budget of one gauntlet run: gates in layer order
// plus the execution policy they share.
type runSetup struct {
	ordered      []Gate
	max          int
	dockerImage  string
	execEnvUnset []string
}

// selectOrdered resolves which gates run in layer order plus the execution
// budget they share. Pure selection: no worktree reads, no execution.
func selectOrdered(cfg Config, profile string) (runSetup, error) {
	selected, err := SelectGates(cfg, profile)
	if err != nil {
		return runSetup{}, err
	}
	ordered := append([]Gate(nil), selected...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Layer < ordered[j].Layer })
	setup := runSetup{ordered: ordered, max: maxTimeout(cfg)}
	if cfg.Execution != nil {
		setup.dockerImage = cfg.Execution.DockerImage
		setup.execEnvUnset = cfg.Execution.EnvUnset
	}
	return setup, nil
}

// computeSkip reports whether an optional gate skips for lack of matching
// changed files. Required gates never skip: mandatory policy can never be
// omitted by omission.
func computeSkip(g Gate, changed []string, changedOK bool) (bool, string) {
	if !g.IsRequired() && len(g.When) > 0 && changedOK && !matchesAnyGate(g, changed) {
		return true, fmt.Sprintf("no changed files match when %s", strings.Join(g.When, ", "))
	}
	return false, ""
}

// whenNote records why a gate ran despite its execution conditions:
// unevaluable conditions fail open, required gates run even unmatched.
func whenNote(g Gate, changed []string, changedOK bool) string {
	if len(g.When) == 0 {
		return ""
	}
	if !changedOK {
		return "when: execution conditions unevaluable (git unavailable), ran unconditionally"
	}
	if g.IsRequired() && !matchesAnyGate(g, changed) {
		return fmt.Sprintf("when: no changed files match %s, ran anyway (required gate is mandatory)", strings.Join(g.When, ", "))
	}
	return ""
}

// refusePolicy enforces the execution policy before any runner runs:
// destructive commands refuse without opt-in, and an allowlist refuses
// anything it does not name. Returns the classification for the receipt.
func refusePolicy(g Gate, cfg Config) (refused bool, note, classification string) {
	classification = Classify(g.Command)
	if classification == ClassificationDestructive && !(cfg.Execution != nil && cfg.Execution.AllowDestructive) {
		return true, "policy: destructive command refused", classification
	}
	if !refused && cfg.Execution != nil && len(cfg.Execution.Allowlist) > 0 {
		ft := firstToken(g.Command)
		allowed := false
		for _, a := range cfg.Execution.Allowlist {
			if ft == a {
				allowed = true
				break
			}
		}
		if !allowed {
			return true, "policy: command not in allowlist", classification
		}
	}
	return false, "", classification
}

// appendGateNote appends a runner/metric/condition note to stderr,
// preserving the empty-vs-nonempty receipt wording exactly.
func appendGateNote(stderr, note string) string {
	if stderr == "" {
		return note
	}
	return stderr + "\n" + note
}

// execResult is one gate's raw execution facts before metric evaluation.
type execResult struct {
	stdout    string
	stderr    string
	code      int
	exit      int
	rerr      error
	cpuMs     int64
	isolation string
	start     time.Time
	dur       time.Duration
	facts     verdict.Facts
}

// execGate runs one gate through the ShRunner fast path or the generic
// runner, mapping runner failures to verdict facts. Refused gates never
// touch a runner: they carry the policy note with a zero duration.
func execGate(ctx context.Context, g Gate, r CmdRunner, timeout int, dockerImage string, execEnvUnset []string, refused bool, refuseNote string) execResult {
	if refused {
		isolation := IsolationUnknown
		if isShRunner(r) {
			isolation = IsolationHostContained
		}
		return execResult{
			code: -1, exit: -1, stderr: refuseNote,
			isolation: isolation, start: time.Now().UTC(),
			facts: verdict.Facts{Refused: true},
		}
	}
	if sh, ok := asShRunner(r); ok {
		if isMutationGate(g) {
			return execMutationSh(ctx, g, sh, timeout, dockerImage, execEnvUnset)
		}
		mergedEnv := append(append([]string(nil), sh.EnvUnset...), execEnvUnset...)
		shGate := ShRunner{Dir: sh.Dir, EnvUnset: mergedEnv}
		gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		start := time.Now().UTC()
		so, se, c, cpu, iso, err := shGate.runWithStats(gctx, g.Command, sh.Dir, dockerImage, g.Isolate)
		dur := time.Since(start)
		cancel()
		out := execResult{stdout: so, stderr: se, code: c, cpuMs: cpu, isolation: iso, rerr: err, start: start, dur: dur, exit: c}
		if err != nil {
			if out.exit == 0 {
				out.exit = -1
			}
			out.stderr = appendGateNote(out.stderr, "gate could not run: "+err.Error())
			out.facts = runnerFacts(gctx, err)
		} else if c != 0 {
			out.facts = verdict.Facts{Executed: true, ExitCode: c}
		} else {
			out.facts = verdict.Facts{Executed: true, ExitCode: 0}
		}
		return out
	}
	gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	start := time.Now().UTC()
	so, se, c, err := r.Run(gctx, g.Command)
	dur := time.Since(start)
	cancel()
	out := execResult{stdout: so, stderr: se, code: c, rerr: err, start: start, dur: dur, exit: c, isolation: IsolationUnknown}
	if err != nil {
		if out.exit == 0 {
			out.exit = -1
		}
		out.stderr = appendGateNote(out.stderr, "gate could not run: "+err.Error())
		out.facts = runnerFacts(gctx, err)
	} else if c != 0 {
		out.facts = verdict.Facts{Executed: true, ExitCode: c}
	} else {
		out.facts = verdict.Facts{Executed: true, ExitCode: 0}
	}
	return out
}

// evalMetric evaluates a metric gate whose command exited 0: the command
// must exit 0 AND the extracted metric must satisfy the operator. Returns
// the receipt fields, a stderr note when the metric fails, and the facts
// to adjudicate (metric violations override the exit-code facts).
func evalMetric(g Gate, stdout string, code int, rerr error, refused bool, facts verdict.Facts) (value, threshold *float64, op string, failed bool, note string, outFacts verdict.Facts) {
	outFacts = facts
	if refused || rerr != nil || code != 0 || g.Metric == nil {
		return nil, nil, "", false, "", facts
	}
	op = g.Metric.Operator
	v, err := extractMetric(g.Metric.Extract, stdout)
	if err != nil {
		return nil, nil, op, true, "metric: " + err.Error(), verdict.Facts{Executed: true, ExitCode: 0, MetricViolated: true}
	}
	value = &v
	th := g.Metric.Threshold
	threshold = &th
	ok := (g.Metric.Operator == OpAtLeast && v >= th) ||
		(g.Metric.Operator == OpAtMost && v <= th)
	if !ok {
		return value, threshold, op, true, fmt.Sprintf("metric %.2f does not meet %s %.2f", v, g.Metric.Operator, th), verdict.Facts{Executed: true, ExitCode: 0, MetricViolated: true}
	}
	return value, threshold, op, false, "", facts
}

// adjudicateAndWrite adjudicates one executed gate, appends the metric and
// condition notes, writes its receipt, and folds the outcome into res.
// Returns stop=true when a halt failure ends the gauntlet; advisory
// failures record honestly and continue.
func adjudicateAndWrite(g Gate, res *Result, evidenceDir, task, gitHead string, stamp runStamp, timeout int, classification string, ex execResult, metricValue, metricThreshold *float64, metricOp, condNote, metricNote string, facts verdict.Facts) (bool, error) {
	v := verdict.Adjudicate(facts)
	legacy := v.Legacy()
	stderr := ex.stderr
	if metricNote != "" {
		stderr = appendGateNote(stderr, metricNote)
	}
	rec := receipts.Receipt{
		Task: task, Gate: g.Name, Layer: g.Layer, Command: g.Command,
		ExitCode: ex.exit, Verdict: legacy, StartedAt: ex.start,
		DurationMS: ex.dur.Milliseconds(), GitHead: gitHead,
		MetricValue: metricValue, MetricThreshold: metricThreshold, MetricOperator: metricOp,
		Status: string(v.Status), Reason: string(v.Reason),
		RunID: stamp.runID, Manifest: stamp.manifestFile,
		CPUTimeMS: ex.cpuMs, Isolation: ex.isolation,
		Classification: classification, Schema: receipts.ReceiptSchemaVersion,
	}
	if condNote != "" {
		stderr = appendGateNote(stderr, condNote)
	}
	rec.Stdout, rec.StdoutSHA = receipts.Capture(ex.stdout)
	rec.Stderr, rec.StderrSHA = receipts.Capture(stderr)
	path, err := receipts.Write(evidenceDir, rec)
	if err != nil {
		return false, fmt.Errorf("gate %s: writing receipt: %w", g.Name, err)
	}
	recID := ""
	if rr, rerr := receipts.ReadFile(path); rerr == nil {
		recID = rr.ID
	}
	res.Executed++
	res.Receipts = append(res.Receipts, path)
	res.Outcomes = append(res.Outcomes, Outcome{Name: g.Name, Layer: g.Layer, Verdict: legacy, DurationMS: ex.dur.Milliseconds(), Status: string(v.Status), Reason: string(v.Reason), ReceiptPath: path, ReceiptID: recID, TimeoutSec: timeout, Advisory: g.EffectiveOnFail() == OnFailAdvisory})
	if legacy == receipts.VerdictFail {
		if g.EffectiveOnFail() == OnFailAdvisory {
			return false, nil
		}
		res.Failed = true
		res.FailedGate = g.Name
		res.Status = string(v.Status)
		res.Reason = string(v.Reason)
		return true, nil
	}
	res.Passed++
	return false, nil
}
