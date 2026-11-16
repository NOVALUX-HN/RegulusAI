package gates

import (
	"context"
	"fmt"
	"os"
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
	inheritEnv   bool
	envAllow     []string
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
		setup.inheritEnv = cfg.Execution.InheritEnv
		setup.envAllow = cfg.Execution.EnvAllow
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

// executionCtxKey carries the correlated execution id (NOV-34) from
// the invocation to the trace hook so the hook's trace records land on
// the same id as the gate run's receipts. It travels in the context
// (not the config) for the same reason consent does: the run mints the
// runID in one place while the hook records traces in another, and the
// config must not change per invocation.
type executionCtxKey struct{}

// ContextWithExecutionID returns ctx carrying the execution id the
// trace hook attributes its records to. Empty means no correlation
// (probe path, legacy hook): records stay unattributed, exactly as
// before.
func ContextWithExecutionID(ctx context.Context, executionID string) context.Context {
	return context.WithValue(ctx, executionCtxKey{}, executionID)
}

// ExecutionIDFromContext reports the execution id carried by ctx, or
// "" when none was set.
func ExecutionIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(executionCtxKey{}).(string)
	return id
}

// ResolveExecutionID resolves the execution id to attribute a gate's
// trace record: the explicit context value wins (hook-supplied, e.g. a
// resumed execution), else the run's runID, else "" (probe path runs
// without a manifest, so there is nothing to correlate — legacy calls
// without an id keep working unchanged). The hook passes the resolved
// id as the trace record's execution_id (record-batch NDJSON carries it
// with no flag change; single-record passes --execution-id only when
// its context carries one), and the observability projections join
// execution_id==RunID.
func ResolveExecutionID(ctx context.Context, runID string) string {
	if id := ExecutionIDFromContext(ctx); id != "" {
		return id
	}
	return runID
}

// consentCtxKey carries per-invocation operator consent (`gate run --yes`)
// from the CLI to the executor so executed receipts record it. Consent
// travels in the context (not the config) because adjudicateAndWrite
// signs receipts without seeing the invocation flags.
type consentCtxKey struct{}

// ContextWithConsent returns ctx carrying operator consent for receipts.
func ContextWithConsent(ctx context.Context, consent bool) context.Context {
	return context.WithValue(ctx, consentCtxKey{}, consent)
}

// consentFromContext reports whether ctx carries operator consent.
func consentFromContext(ctx context.Context) bool {
	consent, _ := ctx.Value(consentCtxKey{}).(bool)
	return consent
}

// refusePolicy enforces the execution policy before any runner runs:
// destructive commands refuse without opt-in, a non-empty allowlist
// refuses anything it does not name, and (S6) a `gate run` with no
// allowlist configured denies by default unless --yes consented for
// this invocation. Returns the classification for the receipt.
func refusePolicy(g Gate, cfg Config) (refused bool, note, classification string) {
	classification = Classify(g.Command)
	if classification == ClassificationDestructive && !(cfg.Execution != nil && cfg.Execution.AllowDestructive) {
		return true, "policy: destructive command refused", classification
	}
	if !refused && cfg.Execution != nil && len(cfg.Execution.Allowlist) > 0 {
		if allowlistRefused(g.Command, cfg.Execution.Allowlist) {
			return true, "policy: command not in allowlist", classification
		}
		return false, "", classification
	}
	// S6 deny-by-default: a `gate run` (EnforceDenyByDefault) with no
	// allowlist refuses every gate unless --yes consented for this
	// invocation (ExplicitYes). Programmatic use (flag unset) keeps the
	// historical allowlist-only behavior. CI note: CI runners have no
	// interactive fallback, so without --yes the refusal below is
	// unconditional there too — scripts must pass --yes or declare an
	// allowlist.
	if cfg.Execution != nil && cfg.Execution.EnforceDenyByDefault && len(cfg.Execution.Allowlist) == 0 && !cfg.Execution.ExplicitYes {
		return true, "policy: no allowlist configured (deny by default; re-run with --yes or add execution.allowlist)", classification
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
	// secrets are redacted from stdout/stderr before the receipt is
	// written (sha matches stored bytes by construction).
	secrets []string
}

// execGate runs one gate through the ShRunner fast path or the generic
// runner, mapping runner failures to verdict facts. Refused gates never
// touch a runner: they carry the policy note with a zero duration.
func execGate(ctx context.Context, g Gate, r CmdRunner, timeout int, dockerImage string, execEnvUnset []string, inheritEnv bool, envAllow []string, refused bool, refuseNote string) execResult {
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
	// S6: explicit --yes consent rides the context so the receipt records
	// it; refused gates never run, so they carry no consent note.
	consentNote := ""
	if consentFromContext(ctx) {
		consentNote = "consent:true (operator passed --yes)"
	}
	if sh, ok := asShRunner(r); ok {
		if isMutationGate(g) {
			return execMutationSh(ctx, g, sh, timeout, dockerImage, execEnvUnset, inheritEnv, envAllow)
		}
		mergedEnv := append(append([]string(nil), sh.EnvUnset...), execEnvUnset...)
		// NOV-81: allowlist follows the runner; FullEnv is the union of
		// the runner flag and the config escape hatch.
		shGate := ShRunner{
			Dir:      sh.Dir,
			EnvUnset: mergedEnv,
			Allow:    append(append([]string(nil), sh.Allow...), envAllow...),
			FullEnv:  sh.FullEnv || inheritEnv,
		}
		gctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		start := time.Now().UTC()
		so, se, c, cpu, iso, err := shGate.runWithStats(gctx, g.Command, sh.Dir, dockerImage, g.Isolate)
		dur := time.Since(start)
		cancel()
		out := execResult{stdout: so, stderr: se, code: c, cpuMs: cpu, isolation: iso, rerr: err, start: start, dur: dur, exit: c, secrets: SecretValues(os.Environ(), mergedEnv)}
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
		if consentNote != "" {
			out.stderr = appendGateNote(out.stderr, consentNote)
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
	if consentNote != "" {
		out.stderr = appendGateNote(out.stderr, consentNote)
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
	// NOV-81: redact before Capture so the stored sha matches the
	// stored (redacted) bytes; verification never sees raw secrets.
	rec.Stdout, rec.StdoutSHA = receipts.Capture(receipts.Redact(ex.stdout, ex.secrets))
	rec.Stderr, rec.StderrSHA = receipts.Capture(receipts.Redact(stderr, ex.secrets))
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
