package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"time"

	"regulus-ai/internal/observability"
)

// timelineCmd implements the F11.1 execution-traceability surface
// under `regulus trace`:
//
//	trace emit --run <r> --type <t> [--status <s>] ...   record one versioned execution event
//	trace timeline --run <r> [--task <t>] [--agent <a>] [--session <s>] [--type <t>] [--status <s>] [--rebuild] [--json]
//	                                                     query the timeline by run, task or agent; --rebuild reconstructs after a crash
//	trace prune-exec [--keep N]                         rotate the global execution log
//
// IDs are minted once and propagated: run_id scopes one integrated
// run, task_id the Sextant task, agent_execution_id one agent
// invocation, delegation_id one delegation edge, session_id the host
// session. Every stored event carries schema_version, host/model
// version when known, UTC + monotonic timestamps, a duration, and a
// terminal-unequivocal status (see docs/trace-schema.md).
func (c CLI) traceEmitCmd(args []string) int {
	fs := flag.NewFlagSet("trace emit", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	run := fs.String("run", "", "the run id (required)")
	typ := fs.String("type", "", "the event type (required: run.start|run.end|task.start|task.end|agent.start|agent.end|context|tool.call|tool.result|delegation.request|delegation.result|retry|timeout|gate|artifact|decision|error)")
	status := fs.String("status", observability.ExecStarted, "the event status (started|running|succeeded|failed|timed_out|cancelled)")
	task := fs.String("task", "", "the Sextant task served")
	agent := fs.String("agent", "", "the agent execution id (origin of this event)")
	delegation := fs.String("delegation", "", "the delegation id (shared by its request and result)")
	session := fs.String("session", "", "the host session id")
	parent := fs.String("parent", "", "the parent event id")
	host := fs.String("host", "", "the host that served the call (host-exposed only)")
	model := fs.String("model", "", "the model that served the call (host-exposed only)")
	hostVersion := fs.String("host-version", "", "the host version (host-exposed only)")
	durationMS := fs.Int64("duration-ms", 0, "the measured duration in ms")
	eventID := fs.String("event-id", "", "the event id (default: minted; resubmitting one is a no-op)")
	payload := fs.String("payload", "", "extra metadata as a JSON object (values truncated, secrets redacted, never full transcripts)")
	errDetails := fs.String("error", "", "error details (free text, redacted before disk)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *run == "" || *typ == "" || fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace emit --run <r> --type <t> [--status <s>] [--task <t>] [--agent <a>] [--delegation <d>] [--session <s>] [--parent <p>] [--host <h>] [--model <m>] [--host-version <v>] [--duration-ms N] [--event-id <e>] [--payload <json>] [--error <msg>]")
		return 2
	}
	var payloadMap map[string]any
	if strings.TrimSpace(*payload) != "" {
		if err := json.Unmarshal([]byte(*payload), &payloadMap); err != nil {
			fmt.Fprintf(c.Stderr, "trace emit: malformed --payload JSON: %v\n", err)
			return 1
		}
	}
	saved, dup, err := observability.RecordExec(c.dir(), observability.ExecEvent{
		SchemaVersion:    observability.ExecSchemaVersion,
		EventID:          *eventID,
		RunID:            *run,
		TaskID:           *task,
		AgentExecutionID: *agent,
		DelegationID:     *delegation,
		SessionID:        *session,
		ParentEventID:    *parent,
		Type:             *typ,
		Status:           *status,
		Host:             *host,
		Model:            *model,
		HostVersion:      *hostVersion,
		DurationMS:       *durationMS,
		Payload:          payloadMap,
		ErrorDetails:     *errDetails,
	}, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace emit: %v\n", err)
		return 1
	}
	if dup {
		fmt.Fprintf(c.Stdout, "exec event %s already recorded (duplicate ignored)\n", saved.EventID)
		return 0
	}
	fmt.Fprintf(c.Stdout, "exec recorded %s\n", saved.EventID)
	return 0
}

// traceTimelineCmd implements `regulus trace timeline --run <r>
// [--task <t>] [--agent <a>] ...`: prints the reconstructed timeline
// of one run, task or agent from persisted execution events — ordered
// by wall timestamp so out-of-order writes still read coherently.
// --rebuild adds the post-crash reconstruction verdict (status,
// duration, delegation attribution, malformed/duplicate evidence).
// --json prints the machine-readable timeline for Core, plugin and
// adapter consumers. An empty store is an empty report, never an
// error.
func (c CLI) traceTimelineCmd(args []string) int {
	fs := flag.NewFlagSet("trace timeline", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	run := fs.String("run", "", "show this run")
	task := fs.String("task", "", "show this task")
	agent := fs.String("agent", "", "show this agent execution")
	delegation := fs.String("delegation", "", "show this delegation")
	session := fs.String("session", "", "restrict to this host session")
	typ := fs.String("type", "", "restrict to this event type")
	status := fs.String("status", "", "restrict to this status")
	rebuild := fs.Bool("rebuild", false, "reconstruct the run after a crash (status, delegations, evidence)")
	asJSON := fs.Bool("json", false, "print the timeline as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (*run == "" && *task == "" && *agent == "" && *delegation == "") {
		fmt.Fprintln(c.Stderr, "usage: regulus trace timeline --run <r> [--task <t>] [--agent <a>] [--delegation <d>] [--session <s>] [--type <t>] [--status <s>] [--rebuild] [--json]")
		return 2
	}
	filter := observability.ExecFilter{
		RunID: *run, TaskID: *task, AgentExecutionID: *agent,
		DelegationID: *delegation, SessionID: *session,
		Type: *typ, Status: *status,
	}
	events, malformed, dups, err := observability.QueryExec(c.dir(), filter)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace timeline: %v\n", err)
		return 1
	}
	if *asJSON {
		raw, err := json.MarshalIndent(struct {
			Events    []observability.ExecEvent `json:"events"`
			Malformed int                       `json:"malformed"`
			Dups      int                       `json:"duplicates"`
		}{Events: orEmptyExec(events), Malformed: malformed, Dups: dups}, "", "  ")
		if err != nil {
			fmt.Fprintf(c.Stderr, "trace timeline: %v\n", err)
			return 1
		}
		fmt.Fprintln(c.Stdout, string(raw))
		return 0
	}
	if len(events) == 0 {
		fmt.Fprintln(c.Stdout, "(no execution events match)")
		return 0
	}
	for _, e := range events {
		fmt.Fprintf(c.Stdout, "%s %s %s run=%s%s%s\n",
			e.At.UTC().Format(time.RFC3339), e.Type, e.Status,
			e.RunID, execIDScope(" task=", e.TaskID), execIDScope(" agent=", e.AgentExecutionID))
	}
	if *rebuild {
		if *run == "" {
			fmt.Fprintln(c.Stderr, "trace timeline: --rebuild needs --run <r>")
			return 2
		}
		tl, err := observability.ReconstructRun(c.dir(), *run)
		if err != nil {
			fmt.Fprintf(c.Stderr, "trace timeline: %v\n", err)
			return 1
		}
		fmt.Fprintf(c.Stdout, "run %s: status %s, %d event(s), %d delegation(s), %d malformed, %d duplicate(s)\n",
			tl.RunID, tl.Status, len(tl.Events), len(tl.Delegations), tl.Malformed, tl.Duplicates)
		if tl.Crashed {
			fmt.Fprintf(c.Stdout, "run %s crashed: started without a terminal run.end; %d event(s) recovered\n", tl.RunID, len(tl.Events))
		}
		for _, d := range tl.Delegations {
			fmt.Fprintf(c.Stdout, "delegation %s: %s -> %s (%s)\n", d.DelegationID, orUnknown(d.FromAgent), orUnknown(d.ToAgent), d.Status)
		}
	}
	return 0
}

// tracePruneExecCmd implements `regulus trace prune-exec [--keep N]`:
// rotation for the global execution log — rewrites it keeping only
// the last N events (default observability.DefaultExecKeep). Files
// are never removed (prune rewrites via temp+rename only).
func (c CLI) tracePruneExecCmd(args []string) int {
	fs := flag.NewFlagSet("trace prune-exec", flag.ContinueOnError)
	fs.SetOutput(c.Stderr)
	keep := fs.Int("keep", observability.DefaultExecKeep, "keep the last N execution events")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(c.Stderr, "usage: regulus trace prune-exec [--keep N]")
		return 2
	}
	n, err := observability.PruneExec(c.dir(), *keep)
	if err != nil {
		fmt.Fprintf(c.Stderr, "trace prune-exec: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.Stdout, "exec log: %d event(s) kept\n", n)
	return 0
}

func execIDScope(prefix, v string) string {
	if v == "" {
		return ""
	}
	return prefix + v
}

func orUnknown(v string) string {
	if v == "" {
		return "(unknown)"
	}
	return v
}

// orEmptyExec keeps the JSON timeline shape stable (events: [] instead
// of events: null) on an empty store.
func orEmptyExec(e []observability.ExecEvent) []observability.ExecEvent {
	if e == nil {
		return []observability.ExecEvent{}
	}
	return e
}
