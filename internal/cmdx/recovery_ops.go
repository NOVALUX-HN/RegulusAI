// Recovery operators (F12.5/NOV-100): retry within budget,
// status, rollback and cancel, plus the shared flag/store/trace
// helpers the recovery handlers use.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"regulus-ai/internal/observability"
	"regulus-ai/internal/recovery"
)

// recoveryCheckpointIO carries the parsed k=v flag maps.
type recoveryCheckpointIO struct {
	inputs     map[string]string
	outputs    map[string]string
	artifacts  map[string]string
	compensate []recovery.Compensation
}

// parseRecoveryCheckpointIO parses the four k=v list flags. Values
// split on the first "="; empty keys, missing "=" and oversized
// payloads refuse before anything is recorded.
func parseRecoveryCheckpointIO(inputs, outputs, artifacts, compensate string, stderr interface{ Write([]byte) (int, error) }) (recoveryCheckpointIO, int, bool) {
	var out recoveryCheckpointIO
	var err error
	if out.inputs, err = parseRecoveryKV(inputs, 64); err != nil {
		fmt.Fprintf(stderr, "recovery checkpoint: bad --input: %v\n", err)
		return out, 1, false
	}
	if out.outputs, err = parseRecoveryKV(outputs, 64); err != nil {
		fmt.Fprintf(stderr, "recovery checkpoint: bad --output: %v\n", err)
		return out, 1, false
	}
	if out.artifacts, err = parseRecoveryKV(artifacts, 64); err != nil {
		fmt.Fprintf(stderr, "recovery checkpoint: bad --artifact: %v\n", err)
		return out, 1, false
	}
	comp, err := parseRecoveryKV(compensate, 32)
	if err != nil {
		fmt.Fprintf(stderr, "recovery checkpoint: bad --compensate: %v\n", err)
		return out, 1, false
	}
	for k, desc := range comp {
		if strings.TrimSpace(desc) == "" {
			fmt.Fprintf(stderr, "recovery checkpoint: bad --compensate: empty description for %q\n", k)
			return out, 1, false
		}
		out.compensate = append(out.compensate, recovery.Compensation{Key: k, Desc: desc})
	}
	return out, 0, true
}

// parseRecoveryKV parses comma-separated k=v pairs (first "="
// splits). Empty input yields nil; keys are trimmed and must be
// non-empty; at most max pairs guard hostile flags.
func parseRecoveryKV(s string, max int) (map[string]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok || k == "" {
			return nil, fmt.Errorf("malformed pair %q (want k=v)", part)
		}
		out[k] = v
		if len(out) > max {
			return nil, fmt.Errorf("at most %d pairs", max)
		}
	}
	return out, nil
}

// addRecoveryPolicyFlags registers the budget overrides shared by
// resume and retry: 0 means the documented default.
func addRecoveryPolicyFlags(fs *flag.FlagSet) (*int, *int64) {
	maxRetries := fs.Int("max-retries", 0, "retry budget override (0 = default 3)")
	timeoutMS := fs.Int64("timeout-ms", 0, "per-attempt timeout in ms (0 = default 60000)")
	return maxRetries, timeoutMS
}

// recoveryPolicyFromFlags builds the effective policy: documented
// defaults with explicit overrides, validated before use.
func recoveryPolicyFromFlags(maxRetries *int, timeoutMS *int64, stderr interface{ Write([]byte) (int, error) }) (recovery.RetryPolicy, int, bool) {
	policy := recovery.DefaultRetryPolicy()
	if *maxRetries != 0 {
		policy.MaxRetries = *maxRetries
	}
	if *timeoutMS != 0 {
		policy.AttemptTimeoutMS = *timeoutMS
	}
	if err := policy.Validate(); err != nil {
		fmt.Fprintf(stderr, "recovery: %v\n", err)
		return policy, 2, false
	}
	return policy, 0, true
}

// loadOrCreateRecoveryRun opens the run store, creating the run on
// first checkpoint. created reports the fresh-run path.
func loadOrCreateRecoveryRun(root, runID string, now time.Time, stderr interface{ Write([]byte) (int, error) }) (*recovery.RunRecord, bool, int, bool) {
	if _, rec, err := recovery.LoadRun(root, runID); err == nil {
		return rec, false, 0, true
	} else if !os.IsNotExist(err) && !strings.Contains(err.Error(), "cannot read run") {
		fmt.Fprintf(stderr, "recovery: %v\n", err)
		return nil, false, 1, false
	}
	rec, err := recovery.NewRun(runID, now)
	if err != nil {
		fmt.Fprintf(stderr, "recovery: %v\n", err)
		return nil, false, 2, false
	}
	return rec, true, 0, true
}

// loadRecoveryRun opens an existing run; missing runs refuse (only
// checkpoint creates).
func loadRecoveryRun(root, runID string, stderr interface{ Write([]byte) (int, error) }) (*recovery.RunRecord, int, bool) {
	_, rec, err := recovery.LoadRun(root, runID)
	if err != nil {
		fmt.Fprintf(stderr, "recovery: %v\n", err)
		return nil, 1, false
	}
	return rec, 0, true
}

// RecoveryRetryCmd implements `regulus recovery retry`: classify
// the failure (or take --class), spend one budget unit, and report
// the deterministic backoff. Budget exhaustion lands the
// checkpoint on failed and persists it — the refusal is durable.
func (io IO) RecoveryRetryCmd(args []string) int {
	fs := flag.NewFlagSet("recovery retry", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "task id (required)")
	agent := fs.String("agent", "", "agent id (required)")
	errText := fs.String("error", "", "failure text to classify (default class when --class is empty)")
	class := fs.String("class", "", "explicit error class (crash|network|host_offline|timeout|partial|permanent|auth|unknown)")
	maxRetries, timeoutMS := addRecoveryPolicyFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" || *task == "" || *agent == "" {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	policy, code, ok := recoveryPolicyFromFlags(maxRetries, timeoutMS, io.Stderr)
	if !ok {
		return code
	}
	rec, code, ok := loadRecoveryRun(io.Dir, *run, io.Stderr)
	if !ok {
		return code
	}
	cls := *class
	if cls == "" {
		cls = recovery.ClassifyError(*errText)
	}
	now := time.Now().UTC()
	cp, lookErr := rec.Lookup(*task, *agent)
	if lookErr != nil {
		fmt.Fprintf(io.Stderr, "recovery retry: %v\n", lookErr)
		return 1
	}
	// Snapshot the evidence BEFORE the retry mutates the checkpoint in place:
	// Evidence is a flat value, so the before/after pair cannot drift through
	// the shared maps the record owns.
	beforeEv := recovery.EvidenceOf(*cp, rec.Circuit)
	started := time.Now()
	delay, err := rec.AttemptRetry(*task, *agent, cls, *errText, now, policy)
	// AttemptRetry may land the checkpoint on failed while
	// refusing: persist either way so the refusal is durable.
	if _, serr := recovery.SaveRun(io.Dir, rec, now); serr != nil {
		fmt.Fprintf(io.Stderr, "recovery retry: %v\n", serr)
		return 1
	}
	if err != nil {
		io.emitRecoveryEvent(*run, *task, "recovery retry", cp.State, map[string]any{"agent": *agent, "class": cls})
		fmt.Fprintf(io.Stderr, "recovery retry: %v\n", err)
		return 1
	}
	if !recovery.SameHostAllowed(cls) || cls == recovery.ClassCrash || cls == recovery.ClassTimeout {
		rec.RecordFailure(cp.Host, now, policy)
		if _, serr := recovery.SaveRun(io.Dir, rec, now); serr != nil {
			fmt.Fprintf(io.Stderr, "recovery retry: %v\n", serr)
			return 1
		}
	}
	io.emitRecoveryEvent(*run, *task, "recovery retry", recovery.StateRunning, map[string]any{"agent": *agent, "class": cls})
	fmt.Fprintf(io.Stdout, "recovery: retry %s/%s attempt=%d backoff=%dms class=%s key=%s\n",
		*task, *agent, cp.Attempt, delay, cls, cp.IdemKey)

	// Record the measured outcome. Bookkeeping failure must not fail a retry
	// that already happened, so it is reported on stderr and the command
	// still exits 0.
	if after, lerr := rec.Lookup(*task, *agent); lerr == nil {
		afterEv := recovery.EvidenceOf(*after, rec.Circuit)
		if outcome, oerr := recovery.OutcomeFrom(recovery.StrategyRetry, *task, *agent, beforeEv, afterEv, time.Since(started), now); oerr == nil {
			if _, serr := recovery.AppendOutcome(io.Dir, *run, outcome, now); serr != nil {
				fmt.Fprintf(io.Stderr, "recovery retry: outcome not recorded: %v\n", serr)
			} else {
				fmt.Fprintf(io.Stdout, "recovery: outcome %s\n", outcome.Delta())
			}
		}
	}
	return 0
}

// RecoveryStatusCmd implements `regulus recovery status`.
func (io IO) RecoveryStatusCmd(args []string) int {
	fs := flag.NewFlagSet("recovery status", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "only show this task (optional)")
	asJSON := fs.Bool("json", false, "print the run record as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	rec, code, ok := loadRecoveryRun(io.Dir, *run, io.Stderr)
	if !ok {
		return code
	}
	if *task != "" {
		filtered := &recovery.RunRecord{
			Version: rec.Version, RunID: rec.RunID, TraceRunID: rec.TraceRunID,
			CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
			Checks: map[string]*recovery.Checkpoint{}, Circuit: rec.Circuit,
		}
		for k, c := range rec.Checks {
			if c.TaskID == *task {
				filtered.Checks[k] = c
			}
		}
		if len(filtered.Checks) == 0 {
			fmt.Fprintf(io.Stderr, "recovery status: no checkpoints for task %q in run %q\n", *task, *run)
			return 1
		}
		rec = filtered
	}
	if *asJSON {
		raw, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			fmt.Fprintf(io.Stderr, "recovery status: %v\n", err)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, recovery.RenderStatus(rec))
	return 0
}

// rollbackRequest is the parsed `recovery rollback` input: the run,
// task and agent ids (all required, no positional args).
type rollbackRequest struct {
	run   string
	task  string
	agent string
}

// parseRollbackArgs parses the rollback flags without touching the
// store. Missing input prints the usage (byte identical to the inline
// command) and refuses with exit 2.
func parseRollbackArgs(args []string, stderr interface{ Write([]byte) (int, error) }) (rollbackRequest, int, bool) {
	fs := flag.NewFlagSet("recovery rollback", flag.ContinueOnError)
	fs.SetOutput(stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "task id (required)")
	agent := fs.String("agent", "", "agent id (required)")
	if err := fs.Parse(args); err != nil {
		return rollbackRequest{}, 2, false
	}
	if fs.NArg() != 0 || *run == "" || *task == "" || *agent == "" {
		fmt.Fprintln(stderr, recoveryUsage())
		return rollbackRequest{}, 2, false
	}
	return rollbackRequest{run: *run, task: *task, agent: *agent}, 0, true
}

// rollbackState selects the audited state: a clean rollback when the
// undo completes, rolled_back_partial when it stops early.
func rollbackState(err error) string {
	if err != nil {
		return recovery.StateRolledBackPartial
	}
	return recovery.StateRolledBack
}

// rollbackAllWithOutput performs every undo step in reverse order,
// narrating each compensation to stdout. The operator performs each
// undo; this layer records it durably in the checkpoint.
func rollbackAllWithOutput(rec *recovery.RunRecord, task, agent string, now time.Time, stdout interface{ Write([]byte) (int, error) }) ([]string, error) {
	return rec.RollbackAll(task, agent, now, func(s recovery.Compensation) error {
		fmt.Fprintf(stdout, "recovery: compensate %s: %s\n", s.Key, s.Desc)
		return nil
	})
}

// RecoveryRollbackCmd implements `regulus recovery rollback`: the
// operator performs each undo, the command records every step
// durably in reverse order. A failed undo stops the run on
// rolled_back_partial naming what remains.
func (io IO) RecoveryRollbackCmd(args []string) int {
	req, code, ok := parseRollbackArgs(args, io.Stderr)
	if !ok {
		return code
	}
	rec, code, ok := loadRecoveryRun(io.Dir, req.run, io.Stderr)
	if !ok {
		return code
	}
	now := time.Now().UTC()
	done, err := rollbackAllWithOutput(rec, req.task, req.agent, now, io.Stdout)
	if _, serr := recovery.SaveRun(io.Dir, rec, now); serr != nil {
		fmt.Fprintf(io.Stderr, "recovery rollback: %v\n", serr)
		return 1
	}
	state := rollbackState(err)
	io.emitRecoveryEvent(req.run, req.task, "recovery rollback", state, map[string]any{"agent": req.agent})
	if err != nil {
		fmt.Fprintf(io.Stderr, "recovery rollback: %v\n", err)
		return 1
	}
	fmt.Fprintf(io.Stdout, "recovery: rollback %s/%s %d compensation(s) applied\n", req.task, req.agent, len(done))
	return 0
}

// RecoveryCancelCmd implements `regulus recovery cancel`: terminal,
// audited, no resume afterwards.
func (io IO) RecoveryCancelCmd(args []string) int {
	fs := flag.NewFlagSet("recovery cancel", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "only cancel this task (default: whole run)")
	agent := fs.String("agent", "", "only cancel this agent (requires --task)")
	reason := fs.String("reason", "", "cancel reason (required, audit)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" || *reason == "" || (*agent != "" && *task == "") {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	if strings.HasPrefix(*reason, "-") {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	rec, code, ok := loadRecoveryRun(io.Dir, *run, io.Stderr)
	if !ok {
		return code
	}
	now := time.Now().UTC()
	closed, err := rec.Cancel(*task, *agent, *reason, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "recovery cancel: %v\n", err)
		return 1
	}
	if _, err := recovery.SaveRun(io.Dir, rec, now); err != nil {
		fmt.Fprintf(io.Stderr, "recovery cancel: %v\n", err)
		return 1
	}
	io.emitRecoveryEvent(*run, *task, "recovery cancel", recovery.StateCancelled, map[string]any{"agent": *agent})
	fmt.Fprintf(io.Stdout, "recovery: cancelled %d checkpoint(s) in %s\n", len(closed), *run)
	return 0
}

// emitRecoveryEvent links one recovery command to the F11.1 exec
// log under the shared run_id: task-scoped, redacted pre-disk by
// RecordExec, best-effort (a trace failure never fails recovery).
func (io IO) emitRecoveryEvent(runID, task, command, status string, extra map[string]any) {
	eventID, err := observability.MintEventID()
	if err != nil {
		return
	}
	payload := map[string]any{"command": command}
	for k, v := range extra {
		payload[k] = v
	}
	evType := observability.ExecTaskEnd
	switch command {
	case "recovery checkpoint":
		evType = observability.ExecArtifact
	case "recovery retry", "recovery resume":
		evType = observability.ExecRetry
	}
	if _, _, rerr := observability.RecordExec(io.Dir, observability.ExecEvent{
		SchemaVersion: observability.ExecSchemaVersion,
		EventID:       eventID, RunID: runID, TaskID: task,
		Type: evType, Status: recoveryEventStatus(status),
		Payload: payload,
	}, time.Now().UTC()); rerr != nil {
		fmt.Fprintf(io.Stderr, "recovery: evento de traza no registrado (%v)\n", rerr)
	}
}

// recoveryEventStatus maps recovery states onto the exec status
// vocabulary: terminal states close the span, live ones stay
// running.
func recoveryEventStatus(state string) string {
	switch state {
	case recovery.StateSucceeded:
		return observability.ExecSucceeded
	case recovery.StateFailed, recovery.StateTimedOut:
		return observability.ExecFailed
	case recovery.StateCancelled, recovery.StateRolledBack, recovery.StateRolledBackPartial:
		return observability.ExecCancelled
	default:
		return observability.ExecRunning
	}
}
