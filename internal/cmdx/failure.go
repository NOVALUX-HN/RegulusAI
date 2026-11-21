// Failure handlers (F13.5/NOV-108): `regulus failure ...`
// Moved-out-of-cli by construction (the F12.4 pattern): the logic
// lives here, internal/cli keeps only a thin forwarder plus init.
//
// The coordinator proposes, never executes: classify names the
// class, decide seals one DecisionRecord (cause, evidence,
// decision, discarded alternatives, proposed result) into the
// append-only log, and the operator runs the proposed recovery
// through the existing commands (`recovery retry`/`resume`,
// `mission replan`/`advance`/`pause`, `recovery rollback`,
// `learn verify`). Reuse, never duplication:
//   - mission truth: internal/mission (LoadMission snapshot the
//     decision cites; the handler never mutates a mission).
//   - task recovery: internal/recovery (the live run record the
//     reassignment bid is verified against; enforcement stays in
//     `recovery resume --new-agent`).
//   - learning: internal/almanac (hypotheses only, via
//     failure.BuildLesson; promotion stays with `learn verify`).
//   - trace: observability.RecordExec sharing run/mission ids
//     (F11.1); no second trace store.
package cmdx

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/almanac"
	"regulus-ai/internal/failure"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/observability"
)

func failureUsage() string {
	return "usage: regulus failure classify --error <text> [--gate <v>] [--class <c>] [--budget-exhausted] [--json] | regulus failure decide --mission <id> [--error <text>] [--class <c>] [--gate <v>] [--run <r> --task <t> --agent <a>] [--budget-exhausted] [--retries-used N] [--replans-used N] [--effects-applied N] [--compensations N] [--worktree-state <clean|dirty|unknown>] [--confirmed <a,b>] [--plan-tasks <a,b>] [--modified <a,b>] [--compensate-keys <a,b>] [--owned <a,b>] [--to-agent <b> --to-caps <a,b> --verified-caps <a,b> --scope <a,b> --to-host <h> --reason <r>] [--max-retries N] [--timeout-ms N] [--max-replans N] [--feed-learn] [--actor <a>] [--idem-key <k>] [--json] | regulus failure status --mission <id> [--json] | regulus failure explain --mission <id> --decision <n|id> [--json]"
}

// FailureCmd routes `regulus failure ...`.
func (io IO) FailureCmd(args []string) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(io.Stdout, failureUsage())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	switch args[0] {
	case "classify":
		return io.FailureClassifyCmd(args[1:])
	case "decide":
		return io.FailureDecideCmd(args[1:])
	case "status":
		return io.FailureStatusCmd(args[1:])
	case "explain":
		return io.FailureExplainCmd(args[1:])
	default:
		fmt.Fprintf(io.Stderr, "failure: unknown subcommand %q\n", args[0])
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
}

// FailureClassifyCmd implements `regulus failure classify`: name
// the class for one failure signal without touching any store.
func (io IO) FailureClassifyCmd(args []string) int {
	fs := flag.NewFlagSet("failure classify", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	errText := fs.String("error", "", "failure text to classify")
	gate := fs.String("gate", "", "gate verdict (red marks validation_failed)")
	class := fs.String("class", "", "explicit class override ("+strings.Join(failure.AllClasses, "|")+")")
	budgeted := fs.Bool("budget-exhausted", false, "an external budget ceiling is confirmed hit")
	asJSON := fs.Bool("json", false, "print the classification as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	c, detail, err := failure.Classify(failure.Signal{
		ErrorText: *errText, GateVerdict: *gate,
		ClassOverride: *class, BudgetExhausted: *budgeted,
	})
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure classify: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(map[string]any{
			"class": c, "detail": detail,
			"retryable":  failure.RetryableClass(c),
			"confidence": failure.ConfidenceFor(c),
		}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "failure classify: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprintf(io.Stdout, "failure: class=%s retryable=%v confidence=%s (%s)\n",
		c, failure.RetryableClass(c), failure.ConfidenceFor(c), detail)
	return 0
}

// FailureDecideCmd implements `regulus failure decide`: seal one
// coordinator decision for a mission. The mission must load (a
// corrupt or missing mission fails closed — tampered state is
// never decided upon); the reassignment bid is verified against
// the live F12.5 run when --to-agent names a target; the record
// appends idempotently under --idem-key; --feed-learn saves one
// hypothesis to F12.6 (never a verified-solution).
func (io IO) FailureDecideCmd(args []string) int {
	fs := flag.NewFlagSet("failure decide", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	missionID := fs.String("mission", "", "mission id (required)")
	errText := fs.String("error", "", "failure text to classify")
	class := fs.String("class", "", "explicit class override")
	gate := fs.String("gate", "", "gate verdict (red marks validation_failed)")
	run := fs.String("run", "", "linked F12.5 run id (evidence + reassignment source)")
	task := fs.String("task", "", "failed task id (requires --run)")
	agent := fs.String("agent", "", "failed agent id (requires --run)")
	budgeted := fs.Bool("budget-exhausted", false, "an external budget ceiling is confirmed hit")
	retriesUsed := fs.Int("retries-used", 0, "prior mission-level retries spent")
	replansUsed := fs.Int("replans-used", 0, "prior mission-level replans spent")
	effectsApplied := fs.Int("effects-applied", 0, "mission effects already confirmed (never re-executed)")
	compensations := fs.Int("compensations", 0, "explicit undo steps pending")
	worktree := fs.String("worktree-state", "unknown", "clean|dirty|unknown (reassignment requires clean)")
	confirmed := fs.String("confirmed", "", "task ids with confirmed results the replan must preserve")
	planTasks := fs.String("plan-tasks", "", "proposed replan task ids")
	modified := fs.String("modified", "", "confirmed tasks the replan would change")
	compKeys := fs.String("compensate-keys", "", "compensation keys a rollback would run")
	owned := fs.String("owned", "", "mission-owned key prefixes (default: mission id, plus run id when linked)")
	toAgent := fs.String("to-agent", "", "reassignment target agent (requires --run --task --agent)")
	toCaps := fs.String("to-caps", "", "target advertised capabilities")
	verifiedCaps := fs.String("verified-caps", "", "verified (F12.1 registry) capabilities covering the target")
	toScopes := fs.String("scope", "", "target scopes (must narrow the stored ones)")
	toHost := fs.String("to-host", "", "target host for the reassignment")
	reason := fs.String("reason", "", "audit reason for the reassignment bid")
	maxReplans := fs.Int("max-replans", 2, "mission replan budget (0..5)")
	maxRetries, timeoutMS := addRecoveryPolicyFlags(fs)
	feedLearn := fs.Bool("feed-learn", false, "save the decision as an F12.6 hypothesis (never verified)")
	actor := fs.String("actor", "", "operator ordering this decision (audit)")
	idem := fs.String("idem-key", "", "idempotency key: a replay returns the stored record without appending")
	asJSON := fs.Bool("json", false, "print the sealed record as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *missionID == "" || strings.HasPrefix(*missionID, "-") {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	if *toAgent != "" && (*run == "" || *task == "" || *agent == "") {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	policy, code, ok := recoveryPolicyFromFlags(maxRetries, timeoutMS, io.Stderr)
	if !ok {
		return code
	}
	fpol := failure.Policy{Retry: policy, MaxReplans: *maxReplans}
	if err := fpol.Validate(); err != nil {
		fmt.Fprintf(io.Stderr, "failure decide: %v\n", err)
		return 2
	}
	_, m, err := mission.LoadMission(io.Dir, *missionID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure decide: mission unreadable, failing closed: %v\n", err)
		return 1
	}
	ownedPrefixes := splitList(*owned)
	if len(ownedPrefixes) == 0 {
		ownedPrefixes = []string{*missionID}
		if *run != "" {
			ownedPrefixes = append(ownedPrefixes, *run)
		}
	}
	now := time.Now().UTC()
	bid := failure.ReassignBid{}
	if *toAgent != "" {
		bid = io.failureVerifyBid(*run, *task, *agent, *toAgent, splitList(*toCaps), splitList(*verifiedCaps), splitList(*toScopes), *toHost, *reason, *worktree, now, fpol)
	}
	sig := failure.Signal{
		MissionID: *missionID, MissionState: m.State,
		ErrorText: *errText, ClassOverride: *class, GateVerdict: *gate,
		BudgetExhausted: *budgeted, RetriesUsed: *retriesUsed, ReplansUsed: *replansUsed,
		EffectsApplied: *effectsApplied, CompensationsPending: *compensations,
		WorktreeState: *worktree,
		Confirmed:     splitList(*confirmed), PlanTasks: splitList(*planTasks), ModifiedPlan: splitList(*modified),
		CompensationKeys: splitList(*compKeys), OwnedPrefixes: ownedPrefixes,
		EvidenceRefs: failureEvidenceRefs(*run, *task, *agent),
		Actor:        *actor, IdemKey: *idem,
	}
	rec, err := failure.Decide(sig, bid, fpol, now)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure decide: %v\n", err)
		return 1
	}
	stored, appended, err := failure.AppendDecision(io.Dir, rec)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure decide: %v\n", err)
		return 1
	}
	io.emitFailureEvent(*missionID, *run, "failure decide", stored.Decision, map[string]any{"actor": *actor, "class": stored.Class})
	if *feedLearn {
		saved, lerr := almanac.LearnSave(io.Dir, learnObservation(failure.BuildLesson(stored, nil, now)), now)
		if lerr != nil {
			fmt.Fprintf(io.Stderr, "failure decide: lesson refused: %v\n", lerr)
			return 1
		}
		io.learnRefreshIndex("failure decide")
		io.recordLearnTrace("failure decide", stored.MissionID)
		fmt.Fprintf(io.Stdout, "learn: hypothesis saved as %s (scope task, confidence %s — verify before trusting)\n", saved.ID, saved.Confidence)
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(map[string]any{"record": stored, "replayed": !appended}, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "failure decide: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, failure.RenderDecision(stored, !appended))
	return 0
}

// failureVerifyBid attests one reassignment candidate against the
// live F12.5 run. Verification failures never fail the command:
// they fold into an unverified bid so Decide degrades to isolate
// with the reason recorded.
func (io IO) failureVerifyBid(runID, taskID, agentID, toAgent string, toCaps, verifiedCaps, toScopes []string, toHost, reason, worktree string, now time.Time, p failure.Policy) failure.ReassignBid {
	bid := failure.ReassignBid{Present: true, ToAgent: toAgent, ToHost: toHost}
	rec, _, ok := loadRecoveryRun(io.Dir, runID, io.Stderr)
	if !ok {
		bid.Detail = "run " + runID + " unreadable (failing closed on the bid)"
		return bid
	}
	detail, err := failure.VerifyReassign(failure.VerifyReassignInput{
		Rec: rec, TaskID: taskID, FromAgent: agentID,
		ToAgent: toAgent, ToCaps: toCaps, VerifiedCaps: verifiedCaps,
		ToScopes: toScopes, ToHost: toHost,
		WorktreeState: worktree, Reason: reason,
	}, now, p)
	if err != nil {
		bid.Detail = err.Error()
		return bid
	}
	bid.Verified = true
	bid.Detail = detail
	return bid
}

// failureEvidenceRefs cites the linked recovery ids for the
// decision record without owning any of those stores.
func failureEvidenceRefs(run, task, agent string) []string {
	var out []string
	if run != "" {
		out = append(out, "run:"+run)
	}
	if task != "" {
		out = append(out, "task:"+task)
	}
	if agent != "" {
		out = append(out, "agent:"+agent)
	}
	return out
}

// learnObservation adapts the built hypothesis to the LearnSave
// pointer contract.
func learnObservation(o almanac.Observation) *almanac.Observation {
	return &o
}

// FailureStatusCmd implements `regulus failure status`: list every
// sealed decision for one mission in append order.
func (io IO) FailureStatusCmd(args []string) int {
	fs := flag.NewFlagSet("failure status", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	missionID := fs.String("mission", "", "mission id (required)")
	asJSON := fs.Bool("json", false, "print the decision log as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *missionID == "" || strings.HasPrefix(*missionID, "-") {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	recs, err := failure.LoadDecisions(io.Dir, *missionID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure status: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(recs, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "failure status: %v\n", merr)
			return 1
		}
		if recs == nil {
			fmt.Fprintln(io.Stdout, "[]")
			return 0
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, failure.RenderList(*missionID, recs))
	return 0
}

// FailureExplainCmd implements `regulus failure explain`: quote
// why one sealed decision chose what it chose — class, reason,
// suggested edge and every discarded alternative.
func (io IO) FailureExplainCmd(args []string) int {
	fs := flag.NewFlagSet("failure explain", flag.ContinueOnError)
	fs.SetOutput(io.Stderr)
	missionID := fs.String("mission", "", "mission id (required)")
	sel := fs.String("decision", "", "1-based index, id or unique id prefix (required)")
	asJSON := fs.Bool("json", false, "print the decision as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *missionID == "" || *sel == "" || strings.HasPrefix(*missionID, "-") {
		fmt.Fprintln(io.Stderr, failureUsage())
		return 2
	}
	recs, err := failure.LoadDecisions(io.Dir, *missionID)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure explain: %v\n", err)
		return 1
	}
	rec, err := failure.FindDecision(recs, *sel)
	if err != nil {
		fmt.Fprintf(io.Stderr, "failure explain: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, merr := json.MarshalIndent(rec, "", "  ")
		if merr != nil {
			fmt.Fprintf(io.Stderr, "failure explain: %v\n", merr)
			return 1
		}
		fmt.Fprintln(io.Stdout, string(raw))
		return 0
	}
	fmt.Fprint(io.Stdout, failure.Explain(rec))
	return 0
}

// emitFailureEvent links one failure command to the F11.1 exec
// log under the linked run when present, else mission/<id>.
// TaskID always carries the mission id, so every decision is
// queryable by it. Best-effort (a trace failure never fails the
// sealed decision).
func (io IO) emitFailureEvent(missionID, runID, command, status string, extra map[string]any) {
	if runID == "" {
		runID = "mission/" + missionID
	}
	eventID, err := observability.MintEventID()
	if err != nil {
		return
	}
	payload := map[string]any{"command": command, "mission_id": missionID}
	for k, v := range extra {
		payload[k] = v
	}
	if _, _, rerr := observability.RecordExec(io.Dir, observability.ExecEvent{
		SchemaVersion: observability.ExecSchemaVersion,
		EventID:       eventID, RunID: runID, TaskID: missionID,
		Type: observability.ExecArtifact, Status: failureEventStatus(status),
		Payload: payload,
	}, time.Now().UTC()); rerr != nil {
		fmt.Fprintf(io.Stderr, "failure: evento de traza no registrado (%v)\n", rerr)
	}
}

// failureEventStatus maps decisions onto the exec status
// vocabulary: stops close the span, live recoveries stay running.
func failureEventStatus(decision string) string {
	switch decision {
	case failure.DecisionFailClosed:
		return observability.ExecFailed
	case failure.DecisionPauseForApproval, failure.DecisionIsolate:
		return observability.ExecCancelled
	default:
		return observability.ExecRunning
	}
}
