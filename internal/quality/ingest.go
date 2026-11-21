// Ingestion from existing stores (F12.4/NOV-99): fold F11.1
// execution events and F11.2 Lab results into observations with
// provenance. No store is duplicated: loaders read the live logs and
// evidence dirs and cite them; every observation keeps its source,
// run id and evidence link so alerts point at cases, not aggregates.
package quality

import (
	"sort"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
)

// IngestLabResults folds Lab executions into observations. Segment
// dimensions reuse the outcome identity (agent, model, host, task,
// host version); tokens keep their Lab source (measured or unknown,
// never estimated); coverage stays unknown — Lab outcomes carry no
// coverage figure, and unknown is reported, never zeroed.
func IngestLabResults(results []bench.LabResult, evidenceRef string) []Observation {
	out := make([]Observation, 0, len(results))
	for _, r := range results {
		o := Observation{
			Agent: r.AgentID, Model: r.Outcome.Model, Host: r.Outcome.Host,
			Task: r.Outcome.TaskID, Version: r.Outcome.HostVersion,
			Success:      r.Verdict == bench.LabPass,
			Retries:      r.Retries,
			LatencyMS:    r.DurationMS,
			LatencyKnown: r.DurationMS > 0,
			TokensIn:     r.TokensIn,
			TokensOut:    r.TokensOut,
			TokensKnown:  r.TokenSource == "measured-host" || r.TokenSource == "measured-local",
			Errors:       len(r.Errors),
			Source:       SourceLab,
			RunID:        r.RunID,
			Evidence:     r.TraceHint,
		}
		if o.Evidence == "" {
			o.Evidence = evidenceRef
		}
		if err := o.validate(); err != nil {
			continue
		}
		out = append(out, o)
	}
	return out
}

// IngestExecEvents folds F11.1 execution events per run: success from
// the terminal run.end status, retries from retry events, errors
// from error/timeout events, latency from the event wall span.
// Tokens and coverage stay unknown — exec events carry no token
// counts — and runs without a terminal run.end are skipped
// (incomplete is a reconstruction verdict, never a stored success).
func IngestExecEvents(events []observability.ExecEvent) []Observation {
	byRun := map[string][]observability.ExecEvent{}
	for _, e := range events {
		if e.RunID == "" {
			continue
		}
		byRun[e.RunID] = append(byRun[e.RunID], e)
	}
	ids := make([]string, 0, len(byRun))
	for id := range byRun {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Observation
	for _, id := range ids {
		if o, ok := foldExecRun(id, byRun[id]); ok {
			out = append(out, o)
		}
	}
	return out
}

// foldExecRun folds one run's events. ok=false skips the run: no
// terminal run.end (crashed/incomplete) or invalid observation.
func foldExecRun(runID string, evs []observability.ExecEvent) (Observation, bool) {
	var terminal string
	var retries, errors int
	var first, last time.Time
	var hasSpan bool
	o := Observation{Source: SourceExec, RunID: runID}
	for _, e := range evs {
		if o.Agent == "" {
			o.Agent = e.AgentExecutionID
		}
		if o.Model == "" {
			o.Model = e.Model
		}
		if o.Host == "" {
			o.Host = e.Host
		}
		if o.Task == "" {
			o.Task = e.TaskID
		}
		if o.Version == "" {
			o.Version = e.HostVersion
		}
		if !e.At.IsZero() {
			if !hasSpan || e.At.Before(first) {
				first = e.At
			}
			if !hasSpan || e.At.After(last) {
				last = e.At
			}
			hasSpan = true
		}
		switch e.Type {
		case observability.ExecRetry:
			retries++
		case observability.ExecError, observability.ExecTimeout:
			errors++
		case observability.ExecRunEnd:
			terminal = e.Status
		}
	}
	if !observability.IsTerminal(terminal) {
		return Observation{}, false
	}
	o.Success = terminal == observability.ExecSucceeded
	o.Retries = retries
	o.Errors = errors
	if hasSpan {
		if d := last.Sub(first).Milliseconds(); d >= 0 {
			o.LatencyMS, o.LatencyKnown = d, true
		}
	}
	o.Evidence = "regulus trace timeline --run " + runID + " --rebuild"
	if err := o.validate(); err != nil {
		return Observation{}, false
	}
	return o, true
}
