package bench

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/observability"
)

func splitBase(t time.Time) observability.ExecEvent {
	return observability.ExecEvent{
		EventID: "evt_base",
		RunID:   "run_split",
		Type:    observability.ExecRunStart,
		Status:  observability.ExecStarted,
		At:      t,
	}
}

func TestSortedDiagEventsOrdersByTimeAndID(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	later := splitBase(base.Add(2 * time.Second))
	later.EventID = "evt_c"
	later.Type = observability.ExecRunEnd
	mid := splitBase(base.Add(time.Second))
	mid.EventID = "evt_b"
	mid.Type = observability.ExecToolResult
	early := splitBase(base)
	early.EventID = "evt_a"
	got := sortedDiagEvents([]observability.ExecEvent{later, mid, early})
	if len(got) != 3 || got[0].EventID != "evt_a" || got[1].EventID != "evt_b" || got[2].EventID != "evt_c" {
		t.Fatalf("events out of order: %v %v %v", got[0].EventID, got[1].EventID, got[2].EventID)
	}
	tie := splitBase(base)
	tie.EventID = "evt_0"
	got = sortedDiagEvents([]observability.ExecEvent{early, tie})
	if got[0].EventID != "evt_0" || got[1].EventID != "evt_a" {
		t.Fatalf("tie must break by event id, got %v %v", got[0].EventID, got[1].EventID)
	}
}

func TestDiagWallForTraceSpan(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	first := splitBase(base)
	last := splitBase(base.Add(1500 * time.Millisecond))
	w := diagWallFor("run_w", []observability.ExecEvent{first, last}, nil)
	if w.ms != 1500 || w.source != "trace-span" || w.note != "" {
		t.Fatalf("trace-span wall = %+v, want 1500ms without note", w)
	}
}

func TestDiagWallForZeroSpanNotes(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	w := diagWallFor("run_z", []observability.ExecEvent{splitBase(base), splitBase(base)}, nil)
	if w.ms != 0 || w.source != "trace-span" {
		t.Fatalf("zero wall = %+v, want 0ms trace-span", w)
	}
	if !strings.Contains(w.note, "zero wall span") {
		t.Fatalf("zero wall must note the single instant, got %q", w.note)
	}
}

func TestDiagWallForLabResultAndUnknown(t *testing.T) {
	w := diagWallFor("run_l", nil, []LabResult{{DurationMS: 900}, {DurationMS: 1200}})
	if w.ms != 1200 || w.source != "lab-result" {
		t.Fatalf("lab wall = %+v, want 1200ms lab-result", w)
	}
	if !strings.Contains(w.note, "fewer than 2 trace events") {
		t.Fatalf("lab wall must note the fallback, got %q", w.note)
	}
	w = diagWallFor("run_u", nil, nil)
	if w.ms != 0 || w.source != "unknown" {
		t.Fatalf("empty wall = %+v, want unknown", w)
	}
	if !strings.Contains(w.note, "wall unknown") {
		t.Fatalf("empty wall must note the unknown, got %q", w.note)
	}
}

func TestDiagFoldStagesAttributesAndUnmapped(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	tool := splitBase(base)
	tool.Type = observability.ExecToolResult
	tool.DurationMS = 100
	queue := splitBase(base)
	queue.Type = observability.ExecRetry
	queue.DurationMS = 50
	wall := splitBase(base)
	wall.Type = observability.ExecRunEnd
	wall.DurationMS = 9000
	outcome := splitBase(base)
	outcome.Type = observability.ExecTimeout
	outcome.DurationMS = 10
	custom := splitBase(base)
	custom.Type = "custom.widget"
	custom.DurationMS = 10
	dup := custom
	fold := diagFoldStages("run_s", []observability.ExecEvent{tool, queue, wall, outcome, custom, dup})
	if fold.stages.ToolsMS != 100 || fold.stages.QueueMS != 50 {
		t.Fatalf("stages = %+v, want tools=100 queue=50", fold.stages)
	}
	if !strings.Contains(fold.note, "custom.widget") {
		t.Fatalf("unmapped types must be reported, got %q", fold.note)
	}
	if strings.Contains(fold.note, observability.ExecRunEnd) || strings.Contains(fold.note, observability.ExecTimeout) {
		t.Fatalf("scope walls and outcomes are never unmapped, got %q", fold.note)
	}
	if got := strings.Count(fold.note, "custom.widget"); got != 1 {
		t.Fatalf("unmapped types must deduplicate, got %q", fold.note)
	}
}

func TestDiagCollectSpansKeepsFirstStart(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	start := splitBase(base)
	start.Type = observability.ExecAgentStart
	start.AgentExecutionID = "A1"
	second := splitBase(base.Add(time.Second))
	second.Type = observability.ExecAgentStart
	second.AgentExecutionID = "A1"
	end := splitBase(base.Add(2 * time.Second))
	end.Type = observability.ExecAgentEnd
	end.AgentExecutionID = "A1"
	stray := splitBase(base)
	stray.Type = observability.ExecToolCall
	spans := diagCollectSpans([]observability.ExecEvent{start, second, end, stray})
	sp, ok := spans["A1"]
	if !ok || !sp.hasStart || !sp.hasEnd {
		t.Fatalf("span A1 must pair, got %+v", spans)
	}
	if !sp.start.Equal(base) {
		t.Fatalf("first start wins, got %v", sp.start)
	}
	if len(spans) != 1 {
		t.Fatalf("events without agent id carry no span, got %d", len(spans))
	}
}

func TestDiagFoldToolsAggregates(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	call := splitBase(base)
	call.Type = observability.ExecToolCall
	call.Payload = map[string]any{"tool": "ls"}
	res := splitBase(base)
	res.Type = observability.ExecToolResult
	res.DurationMS = 10
	res.AgentExecutionID = "A1"
	res.Payload = map[string]any{"tool": "ls"}
	res2 := res
	res2.DurationMS = 30
	fold := diagFoldTools([]observability.ExecEvent{call, call, res, res2})
	st := fold.stats["ls"]
	if st == nil || st.Calls != 2 || st.Results != 2 || st.TotalMS != 40 || st.MaxMS != 30 {
		t.Fatalf("tool fold = %+v, want 2 calls 2 results total=40 max=30", st)
	}
	if fold.byAgent["A1"] != 40 {
		t.Fatalf("per-agent tool time = %v, want 40", fold.byAgent)
	}
	if len(fold.latencies) != 2 {
		t.Fatalf("latencies = %v, want 2 samples", fold.latencies)
	}
}

func TestDiagTracedResidualSubtractsTools(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	start := splitBase(base)
	start.Type = observability.ExecAgentStart
	start.AgentExecutionID = "A1"
	end := splitBase(base.Add(100 * time.Millisecond))
	end.Type = observability.ExecAgentEnd
	end.AgentExecutionID = "A1"
	lone := splitBase(base)
	lone.Type = observability.ExecAgentStart
	lone.AgentExecutionID = "A2"
	spans := diagCollectSpans([]observability.ExecEvent{start, end, lone})
	ms, paired := diagTracedResidual(spans, map[string]int64{"A1": 40})
	if ms != 60 {
		t.Fatalf("traced residual = %d, want 60", ms)
	}
	if !paired["A1"] || paired["A2"] {
		t.Fatalf("only paired spans mark, got %v", paired)
	}
}

func TestDiagLabResidualCoversUntraced(t *testing.T) {
	paired := map[string]bool{"A1": true}
	out := diagLabResidual("run_r", []LabResult{
		{AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 500},
		{AgentID: "anchored", AgentExecutionID: "A9", DurationMS: 1000},
	}, map[string]int64{}, paired)
	if out.ms != 1000 {
		t.Fatalf("lab residual = %d, want 1000", out.ms)
	}
	if len(out.notes) != 1 || !strings.Contains(out.notes[0], "anchored") || !strings.Contains(out.notes[0], "no agent.start/end pair") {
		t.Fatalf("lab residual must note the fallback, got %v", out.notes)
	}
	if !paired["A9"] {
		t.Fatalf("lab residual must mark the agent paired")
	}
}

func TestDiagAggregateMSPrefersLab(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	start := splitBase(base)
	start.Type = observability.ExecAgentStart
	start.AgentExecutionID = "A1"
	end := splitBase(base.Add(100 * time.Millisecond))
	end.Type = observability.ExecAgentEnd
	end.AgentExecutionID = "A1"
	spans := diagCollectSpans([]observability.ExecEvent{start, end})
	if got := diagAggregateMS([]LabResult{{DurationMS: 900}}, spans); got != 900 {
		t.Fatalf("lab aggregate = %d, want 900", got)
	}
	if got := diagAggregateMS(nil, spans); got != 100 {
		t.Fatalf("span aggregate = %d, want 100", got)
	}
}

func TestDiagFinalizeToolsSortsAndMeans(t *testing.T) {
	stats := map[string]*DiagToolStat{
		"b": {Tool: "b", Calls: 1, Results: 3, TotalMS: 10},
		"a": {Tool: "a", Calls: 1, Results: 0, TotalMS: 0},
	}
	got := diagFinalizeTools(stats, []string{"b", "a"})
	if len(got) != 2 || got[0].Tool != "a" || got[1].Tool != "b" {
		t.Fatalf("tools must render in name order, got %+v", got)
	}
	if got[1].MeanMS != 3 {
		t.Fatalf("mean = %d, want 3", got[1].MeanMS)
	}
}

func TestDiagHasTerminal(t *testing.T) {
	done := splitBase(time.Now().UTC())
	done.Type = observability.ExecRunEnd
	done.Status = observability.ExecSucceeded
	if !diagHasTerminal([]observability.ExecEvent{done}) {
		t.Fatal("succeeded run.end is terminal")
	}
	running := done
	running.Status = observability.ExecRunning
	if diagHasTerminal([]observability.ExecEvent{running}) {
		t.Fatal("running run.end is not terminal")
	}
	if diagHasTerminal(nil) {
		t.Fatal("empty events have no terminal")
	}
}

func TestDiagnoseRunFoldsWallStagesAndTools(t *testing.T) {
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	start := splitBase(base)
	start.Type = observability.ExecAgentStart
	start.AgentExecutionID = "A1"
	tool := splitBase(base.Add(100 * time.Millisecond))
	tool.Type = observability.ExecToolResult
	tool.DurationMS = 200
	tool.AgentExecutionID = "A1"
	tool.Payload = map[string]any{"tool": "ls"}
	finishAgent := splitBase(base.Add(time.Second))
	finishAgent.Type = observability.ExecAgentEnd
	finishAgent.AgentExecutionID = "A1"
	finish := splitBase(base.Add(2 * time.Second))
	finish.Type = observability.ExecRunEnd
	finish.Status = observability.ExecSucceeded
	finish.DurationMS = 2000
	in := DiagRunInput{
		RunID:   "run_e2e",
		Events:  []observability.ExecEvent{start, tool, finishAgent, finish},
		Results: []LabResult{{RunID: "run_e2e", AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 1000, Verdict: LabPass}},
	}
	var notes []string
	s := diagnoseRun(in, DefaultDiagThresholds(), &notes)
	if s.WallMS != 2000 || s.WallSource != "trace-span" {
		t.Fatalf("wall = %d/%s, want 2000/trace-span", s.WallMS, s.WallSource)
	}
	if s.Stages.ToolsMS != 200 || s.Stages.InferenceMS != 800 {
		t.Fatalf("stages = %+v, want tools=200 inference=800", s.Stages)
	}
	if s.AggregateMS != 1000 {
		t.Fatalf("aggregate = %d, want 1000", s.AggregateMS)
	}
	if len(s.Tools) != 1 || s.Tools[0].Tool != "ls" || s.Tools[0].MeanMS != 200 {
		t.Fatalf("tools = %+v, want one ls with mean 200", s.Tools)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "insufficient sample") {
		t.Fatalf("single latency must note the sample floor, got %v", notes)
	}
}
