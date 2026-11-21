// Diagnose coverage tests (NOV-119 CRAP paydown): minimal fixtures
// exercising diagnoseRun through the public Diagnose entry point —
// wall from trace-span vs Lab result vs unknown, stage buckets, the
// inference residual (traced pair and Lab fallback) and tool folding.
// No shell, no filesystem: fully synthetic deterministic events.
package bench

import (
	"strings"
	"testing"

	"regulus-ai/internal/observability"
)

func diagCoverNotes(rep DiagReport) string {
	return strings.Join(rep.Notes, "\n")
}

func TestDiagnoseCoverWallSources(t *testing.T) {
	labRes := []LabResult{{RunID: "run_cov_wall", AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 1500, Verdict: LabPass}}
	cases := []struct {
		name       string
		run        DiagRunInput
		wantWall   int64
		wantSource string
		wantNote   string
	}{
		{
			name: "trace span",
			run: DiagRunInput{RunID: "run_cov_span", Events: []observability.ExecEvent{
				diagEv("run_cov_span", "cov_w01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
				diagEv("run_cov_span", "cov_w02", observability.ExecRunEnd, observability.ExecSucceeded, 2500, 0, "", "", nil),
			}},
			wantWall: 2500, wantSource: "trace-span", wantNote: "",
		},
		{
			name: "zero wall span",
			run: DiagRunInput{RunID: "run_cov_zero", Events: []observability.ExecEvent{
				diagEv("run_cov_zero", "cov_z01", observability.ExecRunStart, observability.ExecStarted, 100, 0, "", "", nil),
				diagEv("run_cov_zero", "cov_z02", observability.ExecTaskStart, observability.ExecStarted, 100, 0, "", "", nil),
			}},
			wantWall: 0, wantSource: "trace-span", wantNote: "zero wall span",
		},
		{
			name: "lab result",
			run: DiagRunInput{RunID: "run_cov_wall", Events: []observability.ExecEvent{
				diagEv("run_cov_wall", "cov_w03", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
			}, Results: labRes},
			wantWall: 1500, wantSource: "lab-result", wantNote: "wall from Lab result",
		},
		{
			name:       "unknown",
			run:        DiagRunInput{RunID: "run_cov_unknown"},
			wantWall:   0,
			wantSource: "unknown",
			wantNote:   "no trace events and no Lab results",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := Diagnose(DiagInput{Runs: []DiagRunInput{tc.run}}, DefaultDiagThresholds())
			if len(rep.Runs) != 1 {
				t.Fatalf("runs = %d, want 1", len(rep.Runs))
			}
			s := rep.Runs[0]
			if s.WallMS != tc.wantWall || s.WallSource != tc.wantSource {
				t.Fatalf("wall = %dms (%s), want %dms (%s)", s.WallMS, s.WallSource, tc.wantWall, tc.wantSource)
			}
			if tc.wantNote != "" && !strings.Contains(diagCoverNotes(rep), tc.wantNote) {
				t.Fatalf("notes lack %q: %q", tc.wantNote, diagCoverNotes(rep))
			}
		})
	}
}

func TestDiagnoseCoverStagesResidualTools(t *testing.T) {
	run := DiagRunInput{
		RunID: "run_cov_stage",
		Events: []observability.ExecEvent{
			diagEv("run_cov_stage", "cov_s01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
			diagEv("run_cov_stage", "cov_s02", observability.ExecAgentStart, observability.ExecStarted, 200, 0, "A1", "", nil),
			diagEv("run_cov_stage", "cov_s03", observability.ExecContext, observability.ExecRunning, 300, 120, "A1", "", nil),
			diagEv("run_cov_stage", "cov_s04", observability.ExecRetry, observability.ExecRunning, 400, 50, "A1", "", map[string]any{"attempt": 1}),
			diagEv("run_cov_stage", "cov_s05", observability.ExecDelegationRequest, observability.ExecRunning, 500, 40, "A1", "", map[string]any{"to": "anchored"}),
			diagEv("run_cov_stage", "cov_s06", observability.ExecToolCall, observability.ExecRunning, 600, 0, "A1", "", toolPayload("bash", nil)),
			diagEv("run_cov_stage", "cov_s07", observability.ExecToolResult, observability.ExecRunning, 700, 300, "A1", "", toolPayload("bash", nil)),
			diagEv("run_cov_stage", "cov_s08", observability.ExecGate, observability.ExecSucceeded, 800, 200, "A1", "", nil),
			diagEv("run_cov_stage", "cov_s09", observability.ExecAgentEnd, observability.ExecSucceeded, 5200, 0, "A1", "", nil),
			diagEv("run_cov_stage", "cov_s10", observability.ExecRunEnd, observability.ExecSucceeded, 5200, 0, "", "", nil),
			diagEv("run_cov_stage", "cov_s11", "custom.unknown", observability.ExecRunning, 900, 50, "", "", nil),
		},
	}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{run}}, DefaultDiagThresholds())
	if len(rep.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rep.Runs))
	}
	s := rep.Runs[0]
	if s.WallMS != 5200 || s.WallSource != "trace-span" {
		t.Fatalf("wall = %dms (%s), want 5200ms (trace-span)", s.WallMS, s.WallSource)
	}
	if s.AggregateMS != 5000 {
		t.Fatalf("aggregate = %dms, want 5000ms (traced agent span)", s.AggregateMS)
	}
	if s.UnattributedMS != 0 {
		t.Fatalf("unattributed = %dms, want 0 (stages exceed wall)", s.UnattributedMS)
	}
	wantStages := TierPhases{PrepMS: 120, QueueMS: 90, InferenceMS: 4700, ToolsMS: 300, EvalMS: 200}
	if s.Stages != wantStages {
		t.Fatalf("stages = %+v, want %+v", s.Stages, wantStages)
	}
	if len(s.Tools) != 1 {
		t.Fatalf("tools = %+v, want one bash entry", s.Tools)
	}
	wantTool := DiagToolStat{Tool: "bash", Calls: 1, Results: 1, TotalMS: 300, MaxMS: 300, MeanMS: 300}
	if s.Tools[0] != wantTool {
		t.Fatalf("tool = %+v, want %+v", s.Tools[0], wantTool)
	}
	if len(s.Agents) != 1 || s.Agents[0].AgentID != "A1" || s.Agents[0].DurationMS != 5000 || s.Agents[0].Source != "trace-span" {
		t.Fatalf("agents = %+v, want one traced A1 span of 5000ms", s.Agents)
	}
	if s.DominantStage.Name != TierPhaseInference || s.DominantStage.MS != 4700 {
		t.Fatalf("dominant stage = %+v, want inference 4700ms", s.DominantStage)
	}
	if s.DominantAgent.Name != "A1" || s.DominantAgent.MS != 5000 {
		t.Fatalf("dominant agent = %+v, want A1 5000ms", s.DominantAgent)
	}
	if s.Counts.Retries != 1 {
		t.Fatalf("retries = %d, want 1", s.Counts.Retries)
	}
	for _, want := range []string{"custom.unknown", "staged costs 5410ms exceed wall 5200ms", "insufficient sample"} {
		if !strings.Contains(diagCoverNotes(rep), want) {
			t.Fatalf("notes lack %q: %q", want, diagCoverNotes(rep))
		}
	}
	if strings.Contains(diagCoverNotes(rep), "no terminal run.end") {
		t.Fatalf("terminal run.end present, notes must not claim a crash: %q", diagCoverNotes(rep))
	}
}

func TestDiagnoseCoverLabResidual(t *testing.T) {
	run := DiagRunInput{
		RunID: "run_cov_lab",
		Events: []observability.ExecEvent{
			diagEv("run_cov_lab", "cov_l01", observability.ExecToolResult, observability.ExecRunning, 100, 100, "A9", "", toolPayload("bash", nil)),
		},
		Results: []LabResult{
			{RunID: "run_cov_lab", AgentID: "baseline", AgentExecutionID: "A9", DurationMS: 1000, Verdict: LabPass},
		},
	}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{run}}, DefaultDiagThresholds())
	if len(rep.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rep.Runs))
	}
	s := rep.Runs[0]
	if s.WallMS != 1000 || s.WallSource != "lab-result" {
		t.Fatalf("wall = %dms (%s), want 1000ms (lab-result)", s.WallMS, s.WallSource)
	}
	if s.Stages.ToolsMS != 100 || s.Stages.InferenceMS != 900 {
		t.Fatalf("stages = %+v, want tools 100ms + inference residual 900ms", s.Stages)
	}
	if s.AggregateMS != 1000 {
		t.Fatalf("aggregate = %dms, want 1000ms from Lab result", s.AggregateMS)
	}
	if len(s.Agents) != 1 || s.Agents[0].AgentID != "baseline" || s.Agents[0].Source != "lab-result" {
		t.Fatalf("agents = %+v, want one lab-result baseline entry", s.Agents)
	}
	if len(s.Tools) != 1 || s.Tools[0].Calls != 0 || s.Tools[0].Results != 1 || s.Tools[0].MeanMS != 100 {
		t.Fatalf("tools = %+v, want bash with 0 calls 1 result mean 100ms", s.Tools)
	}
	for _, want := range []string{"inference for agent baseline from Lab result", "no terminal run.end", "insufficient sample"} {
		if !strings.Contains(diagCoverNotes(rep), want) {
			t.Fatalf("notes lack %q: %q", want, diagCoverNotes(rep))
		}
	}
}

func TestDiagnoseCoverTolerantCounts(t *testing.T) {
	run := DiagRunInput{
		RunID:      "run_cov_tol",
		Malformed:  3,
		Duplicates: 1,
		Results: []LabResult{
			{RunID: "run_cov_tol", AgentID: "x", AgentExecutionID: "X1", DurationMS: 500, Verdict: LabTimeout, Retries: 2},
		},
	}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{run}}, DefaultDiagThresholds())
	if len(rep.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(rep.Runs))
	}
	s := rep.Runs[0]
	if s.WallMS != 500 || s.WallSource != "lab-result" {
		t.Fatalf("wall = %dms (%s), want 500ms (lab-result)", s.WallMS, s.WallSource)
	}
	if s.AggregateMS != 500 {
		t.Fatalf("aggregate = %dms, want 500ms", s.AggregateMS)
	}
	if s.Counts.Retries != 2 || s.Counts.Timeouts != 1 || s.Counts.Malformed != 3 || s.Counts.Duplicates != 1 {
		t.Fatalf("counts = %+v, want retries 2 timeouts 1 malformed 3 duplicates 1", s.Counts)
	}
	if !strings.Contains(diagCoverNotes(rep), "tolerant reader skipped 3 malformed line(s), 1 duplicate event(s)") {
		t.Fatalf("notes lack tolerant-reader evidence: %q", diagCoverNotes(rep))
	}
}
