// Ingestion tests: observations fold live stores without
// duplicating them, keeping source, run and evidence links.
package quality

import (
	"testing"
	"time"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/observability"
	"regulus-ai/internal/trace"
)

// Lab results fold with their verdict, retries, tokens and
// provenance: pass reads as success, measured-host counts stay
// measured, and every observation links its run.
func TestIngestLabResultsKeepsProvenance(t *testing.T) {
	results := []bench.LabResult{
		{
			AgentID: "atlas", TaskID: "t1", RunID: "run-1",
			DurationMS: 120, TokensIn: 100, TokensOut: 200,
			TokenSource: trace.SourceMeasuredHost,
			Retries:     2, Errors: []string{"boom"},
			Verdict:   "pass",
			Outcome:   bench.AgentOutcome{TaskID: "t1", Model: "m1", Host: "opencode", HostVersion: "v3"},
			TraceHint: "regulus trace timeline --run run-1 --rebuild",
		},
		{
			AgentID: "atlas", TaskID: "t1", RunID: "run-2",
			Verdict: "fail", TokenSource: trace.SourceUnknown,
			Outcome: bench.AgentOutcome{TaskID: "t1", Model: "m1", Host: "opencode", HostVersion: "v3"},
		},
	}
	obs := IngestLabResults(results, "lab-dir")
	if len(obs) != 2 {
		t.Fatalf("ingested %d observations, want 2", len(obs))
	}
	if !obs[0].Success || obs[0].Retries != 2 || len(obs) != 2 {
		t.Fatalf("first observation misfolded: %+v", obs[0])
	}
	if !obs[0].TokensKnown || obs[1].TokensKnown {
		t.Fatalf("token provenance lost: %+v / %+v", obs[0], obs[1])
	}
	if obs[1].Success {
		t.Fatal("fail verdict must not read as success")
	}
	for _, o := range obs {
		if o.Source != SourceLab || o.RunID == "" || o.Evidence == "" {
			t.Fatalf("observation lost provenance: %+v", o)
		}
		if o.Agent != "atlas" || o.Host != "opencode" || o.Version != "v3" {
			t.Fatalf("segment misfolded: %+v", o)
		}
	}
}

// Exec events fold per run: terminal run.end decides success,
// retries and errors count, the wall span measures latency, and
// runs without a terminal close are skipped, never guessed.
func TestIngestExecEventsFoldsRuns(t *testing.T) {
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	mk := func(run, typ, status string, shift time.Duration, agent string) observability.ExecEvent {
		return observability.ExecEvent{
			SchemaVersion: observability.ExecSchemaVersion,
			EventID:       "evt-" + run + "-" + typ, RunID: run,
			TaskID: "t1", AgentExecutionID: agent,
			Type: typ, Status: status, Host: "opencode",
			Model: "m1", HostVersion: "v3", At: at.Add(shift),
		}
	}
	events := []observability.ExecEvent{
		mk("run-ok", observability.ExecRunStart, observability.ExecStarted, 0, "a1"),
		mk("run-ok", observability.ExecRetry, observability.ExecRunning, time.Second, "a1"),
		mk("run-ok", observability.ExecRunEnd, observability.ExecSucceeded, 3*time.Second, "a1"),
		mk("run-bad", observability.ExecRunStart, observability.ExecStarted, 0, "a2"),
		mk("run-bad", observability.ExecError, observability.ExecRunning, time.Second, "a2"),
		mk("run-bad", observability.ExecRunEnd, observability.ExecFailed, 2*time.Second, "a2"),
		mk("run-open", observability.ExecRunStart, observability.ExecStarted, 0, "a3"),
	}
	obs := IngestExecEvents(events)
	if len(obs) != 2 {
		t.Fatalf("ingested %d observations, want 2 (open run skipped)", len(obs))
	}
	byID := map[string]Observation{}
	for _, o := range obs {
		byID[o.RunID] = o
	}
	okRun, badRun := byID["run-ok"], byID["run-bad"]
	if !okRun.Success || okRun.Retries != 1 || !okRun.LatencyKnown || okRun.LatencyMS != 3000 {
		t.Fatalf("run-ok misfolded: %+v", okRun)
	}
	if badRun.Success || badRun.Errors != 1 {
		t.Fatalf("run-bad misfolded: %+v", badRun)
	}
	for _, o := range obs {
		if o.Source != SourceExec || !observability.IsTerminal(observability.ExecSucceeded) {
			t.Fatalf("observation lost provenance: %+v", o)
		}
		if o.TokensKnown || o.CoverageKnown {
			t.Fatalf("exec carries no tokens/coverage: unknown, never zero: %+v", o)
		}
	}
}
