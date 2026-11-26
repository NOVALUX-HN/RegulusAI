// Recovery handlers (F12.5/NOV-100): `regulus recovery ...`
// Moved-out-of-cli by construction (the F12.4 pattern): the logic
// lives here, internal/cli keeps only a thin forwarder plus init.
//
// Reuse, never duplication:
//   - durability: internal/recovery (states, checkpoints, ledger,
//     budgets, circuit, store) — this layer only parses flags,
//     loads/saves runs and prints.
//   - permissions: mcp.PrincipalFor + subset checks (F11.8: the
//     recorded scopes must narrow the caller's, never widen).
//   - capabilities: orchestrate.RegistryFromManifests over the
//     F11.7 matrix (only verified caps gate a reassignment).
//   - trace: observability.RecordExec sharing run_id (F11.1); the
//     store here never duplicates the exec log, it cites it.
package cmdx

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mcp"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/recovery"
)

func recoveryUsage() string {
	return "usage: regulus recovery checkpoint --run <id> --task <t> --agent <a> --state <s> [--host <h>] [--version <v>] [--scope <a,b>] [--cap <a,b>] [--require-cap <a,b>] [--input <k=v,...>] [--output <k=v,...>] [--artifact <k=v,...>] [--compensate <k=desc,...>] | regulus recovery resume --run <id> --task <t> --agent <a> [--new-agent <b>] [--agent-caps <a,b>] [--to-host <h>] [--scope <a,b>] [--reason <r>] [--effect <e,...>] [--max-retries N] [--timeout-ms N] | regulus recovery retry --run <id> --task <t> --agent <a> [--error <text>] [--class <c>] [--max-retries N] [--timeout-ms N] | regulus recovery status --run <id> [--task <t>] [--json] | regulus recovery rollback --run <id> --task <t> --agent <a> | regulus recovery cancel --run <id> [--task <t>] [--agent <a>] --reason <r>"
}

// RecoveryCmd routes `regulus recovery ...`.
func (io IO) RecoveryCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, recoveryUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	switch args[0] {
	case "checkpoint":
		return io.RecoveryCheckpointCmd(args[1:])
	case "resume":
		return io.RecoveryResumeCmd(args[1:])
	case "retry":
		return io.RecoveryRetryCmd(args[1:])
	case "status":
		return io.RecoveryStatusCmd(args[1:])
	case "rollback":
		return io.RecoveryRollbackCmd(args[1:])
	case "cancel":
		return io.RecoveryCancelCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "recovery: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
}

// RecoveryCheckpointCmd implements `regulus recovery checkpoint`.
func (io IO) RecoveryCheckpointCmd(args []string) int {
	fs := flag.NewFlagSet("recovery checkpoint", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required, creates the run on first use)")
	task := fs.String("task", "", "task id (required)")
	agent := fs.String("agent", "", "agent id (required)")
	state := fs.String("state", "", "pending|running|checkpointed|succeeded|failed|timed_out (required)")
	host := fs.String("host", "", "executing host")
	version := fs.String("version", "", "code/agent version stamp")
	scopes := fs.String("scope", "", "comma-separated permission scopes (must narrow the caller)")
	caps := fs.String("cap", "", "comma-separated advertised capabilities")
	requireCaps := fs.String("require-cap", "", "comma-separated required capabilities")
	inputs := fs.String("input", "", "comma-separated k=v inputs")
	outputs := fs.String("output", "", "comma-separated k=v outputs")
	artifacts := fs.String("artifact", "", "comma-separated k=v artifact refs")
	compensate := fs.String("compensate", "", "comma-separated k=desc undo steps")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *run == "" || *task == "" || *agent == "" || *state == "" {
		fmt.Fprintln(io.Stderr, recoveryUsage())
		return 2
	}
	for _, f := range []string{*run, *task, *agent} {
		if strings.HasPrefix(f, "-") {
			fmt.Fprintln(io.Stderr, recoveryUsage())
			return 2
		}
	}
	in, code, ok := parseRecoveryCheckpointIO(*inputs, *outputs, *artifacts, *compensate, io.Stderr)
	if !ok {
		return code
	}
	caller := mcp.PrincipalFor(io.Dir)
	wantScopes := splitList(*scopes)
	if err := recovery.CheckScopesSubset(wantScopes, caller.GrantedScopes()); err != nil {
		fmt.Fprintf(io.Stderr, "recovery checkpoint: %v\n", err)
		return 1
	}
	rec, created, code, ok := loadOrCreateRecoveryRun(io.Dir, *run, time.Now().UTC(), io.Stderr)
	if !ok {
		return code
	}
	_ = created
	cp, err := rec.RecordCheckpoint(recovery.CheckpointInput{
		TaskID: *task, AgentID: *agent, Host: *host, Version: *version,
		State: *state, Inputs: in.inputs, Outputs: in.outputs,
		Artifacts: in.artifacts, Scopes: wantScopes,
		Capabilities: splitList(*caps), RequiredCaps: splitList(*requireCaps),
		Compensate: in.compensate,
	}, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(io.Stderr, "recovery checkpoint: %v\n", err)
		return 1
	}
	if cp.State == recovery.StateSucceeded {
		rec.RecordSuccess(cp.Host)
	}
	if _, err := recovery.SaveRun(io.Dir, rec, time.Now().UTC()); err != nil {
		fmt.Fprintf(io.Stderr, "recovery checkpoint: %v\n", err)
		return 1
	}
	io.emitRecoveryEvent(*run, *task, "recovery checkpoint", cp.State, map[string]any{"agent": *agent})
	fmt.Fprintf(io.Stdout, "recovery: checkpoint %s/%s %s key=%s\n", *task, *agent, cp.State, cp.IdemKey)
	return 0
}

// RecoveryResumeCmd implements `regulus recovery resume`: optional
// reassignment first (capability/state-gated), stale-running
// timeout rotation, then the idempotent plan.
func (io IO) RecoveryResumeCmd(args []string) int {
	fs := flag.NewFlagSet("recovery resume", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	run := fs.String("run", "", "run id (required)")
	task := fs.String("task", "", "task id (required)")
	agent := fs.String("agent", "", "agent id (required)")
	newAgent := fs.String("new-agent", "", "reassign to this agent before resuming")
	agentCaps := fs.String("agent-caps", "", "advertised caps of an off-registry target agent")
	toHost := fs.String("to-host", "", "target host for the reassignment")
	scopes := fs.String("scope", "", "target scopes (must narrow the stored ones)")
	reason := fs.String("reason", "", "reassignment reason (audit)")
	effects := fs.String("effect", "", "comma-separated known effect keys to plan")
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
	now := time.Now().UTC()
	cur := *agent
	if *newAgent != "" {
		src, err := rec.Lookup(*task, *agent)
		if err != nil {
			fmt.Fprintf(io.Stderr, "recovery resume: %v\n", err)
			return 1
		}
		toScopes := splitList(*scopes)
		if len(toScopes) == 0 {
			toScopes = append([]string(nil), src.Scopes...)
		}
		targetCaps, verified := recoveryTargetCaps(*newAgent, splitList(*agentCaps))
		if _, err := rec.Reassign(recovery.ReassignInput{
			TaskID: *task, FromAgent: *agent, ToAgent: *newAgent,
			ToCaps: targetCaps, ToScopes: toScopes, ToHost: *toHost,
			VerifiedCaps: verified, Reason: *reason,
		}, now, policy); err != nil {
			fmt.Fprintf(io.Stderr, "recovery resume: reassign refused: %v\n", err)
			return 1
		}
		cur = *newAgent
	}
	if err := rotateStaleRunning(rec, *task, cur, now, policy); err != nil {
		fmt.Fprintf(io.Stderr, "recovery resume: %v\n", err)
		return 1
	}
	want := splitList(*effects)
	if len(want) == 0 {
		if cp, err := rec.Lookup(*task, cur); err == nil {
			want = cp.PendingEffects()
		}
	}
	plan, err := rec.Resume(*task, cur, want, now, policy)
	if err != nil {
		// Persist terminal side effects the failed resume may
		// have recorded (none today) and report the refusal.
		fmt.Fprintf(io.Stderr, "recovery resume: %v\n", err)
		return 1
	}
	if _, err := recovery.SaveRun(io.Dir, rec, now); err != nil {
		fmt.Fprintf(io.Stderr, "recovery resume: %v\n", err)
		return 1
	}
	io.emitRecoveryEvent(*run, *task, "recovery resume", plan.State, map[string]any{"agent": cur})
	fmt.Fprint(io.Stdout, recovery.RenderPlan(plan))
	return 0
}

// recoveryTargetCaps resolves a reassignment target. A routed capability and a
// verified one are different things since F16.2: the registry also carries
// capabilities that are declared, inferred or degraded, and only a probada is
// something the host has been observed to do. Passing the whole routed set as
// `verified` would make a reassignment gate claim more than it knows, so
// `verified` is filtered back to probada while `caps` keeps the full routed
// surface. Off-registry agents contribute only the advertised --agent-caps
// with an empty verified set, so required-cap work refuses them.
func recoveryTargetCaps(agentID string, advertised []string) (caps, verified []string) {
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	for _, a := range reg {
		if a.ID != agentID {
			continue
		}
		m := manifests[a.Host]
		for _, c := range a.Capabilities {
			caps = append(caps, c.ID)
			if proven, found := m.Capability(c.ID); found && proven.Status == bench.CapVerified {
				verified = append(verified, c.ID)
			}
		}
		return caps, verified
	}
	return advertised, nil
}

// rotateStaleRunning marks a running checkpoint whose last
// confirmation exceeds the attempt timeout as timed_out before the
// resume proceeds, so timeouts surface as classified history
// instead of silent restarts.
func rotateStaleRunning(rec *recovery.RunRecord, task, agent string, now time.Time, p recovery.RetryPolicy) error {
	return recovery.RotateStaleRunning(rec, task, agent, now, p)
}
