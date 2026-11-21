// Performance diagnosis tests (F11.4/NOV-90): dominant stage/agent
// attribution, wall-vs-aggregate honesty, N-minimum quantile gating,
// loops, duplication, slow tools, dead delegations, timeouts,
// outliers and partial data. Fixtures are synthetic and fully
// deterministic (fixed wall timestamps, fixed durations): every
// asserted number is arithmetic over the fixture, never wall-clock.
package bench

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/observability"
)

var diagBase = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func diagAt(offsetMS int64) time.Time {
	return diagBase.Add(time.Duration(offsetMS) * time.Millisecond)
}

func diagEv(run, id, typ, status string, atMS, durMS int64, agent, dlg string, payload map[string]any) observability.ExecEvent {
	return observability.ExecEvent{
		SchemaVersion:    observability.ExecSchemaVersion,
		EventID:          id,
		RunID:            run,
		AgentExecutionID: agent,
		DelegationID:     dlg,
		Type:             typ,
		Status:           status,
		At:               diagAt(atMS),
		DurationMS:       durMS,
		Payload:          payload,
	}
}

func toolPayload(tool string, args map[string]any) map[string]any {
	p := map[string]any{"tool": tool}
	for k, v := range args {
		p[k] = v
	}
	return p
}

// diagSlowFixture builds the controlled slow run: a 6-call read loop
// (2000ms each), one slow exec call (3000ms), a pending delegation,
// one retry, and a second short agent. Baseline is the same task
// clean. Durations are exact:
//
//	slow: prep=150 queue=500 tools=15100 eval=200 inference=12200
//	      wall=30000 aggregate=24000 unattributed=1850
//	fast: wall=5000 aggregate=4500
func diagSlowFixture() (slow, fast DiagRunInput) {
	slowEvents := []observability.ExecEvent{
		diagEv("run_slow", "evt_s01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
		diagEv("run_slow", "evt_s02", observability.ExecTaskStart, observability.ExecStarted, 100, 50, "", "", nil),
		diagEv("run_slow", "evt_s03", observability.ExecAgentStart, observability.ExecStarted, 200, 0, "A1", "", nil),
		diagEv("run_slow", "evt_s04", observability.ExecContext, observability.ExecRunning, 300, 100, "A1", "", nil),
		diagEv("run_slow", "evt_s05", observability.ExecAgentStart, observability.ExecStarted, 500, 0, "A2", "", nil),
		diagEv("run_slow", "evt_s06", observability.ExecToolCall, observability.ExecRunning, 600, 0, "A2", "", toolPayload("write", map[string]any{"path": "o.txt"})),
		diagEv("run_slow", "evt_s07", observability.ExecToolResult, observability.ExecRunning, 700, 100, "A2", "", toolPayload("write", map[string]any{"path": "o.txt"})),
		diagEv("run_slow", "evt_s08", observability.ExecAgentEnd, observability.ExecSucceeded, 2000, 1400, "A2", "", nil),
	}
	at := int64(1000)
	for i := 1; i <= 6; i++ {
		slowEvents = append(slowEvents,
			diagEv("run_slow", "evt_loop_c", observability.ExecToolCall, observability.ExecRunning, at, 0, "A1", "", toolPayload("read", map[string]any{"path": "f.txt"})),
			diagEv("run_slow", "evt_loop_r", observability.ExecToolResult, observability.ExecRunning, at+100, 2000, "A1", "", toolPayload("read", map[string]any{"path": "f.txt"})),
		)
		// Unique event ids per iteration (the loop helper above reuses
		// the literal; rewrite with the index for determinism).
		slowEvents[len(slowEvents)-2].EventID = "evt_loop_c" + string(rune('0'+i))
		slowEvents[len(slowEvents)-1].EventID = "evt_loop_r" + string(rune('0'+i))
		at += 2100
	}
	slowEvents = append(slowEvents,
		diagEv("run_slow", "evt_s20", observability.ExecToolCall, observability.ExecRunning, 14000, 0, "A1", "", toolPayload("exec", map[string]any{"cmd": "make"})),
		diagEv("run_slow", "evt_s21", observability.ExecToolResult, observability.ExecRunning, 14100, 3000, "A1", "", toolPayload("exec", map[string]any{"cmd": "make"})),
		diagEv("run_slow", "evt_s22", observability.ExecRetry, observability.ExecRunning, 17500, 100, "A1", "", map[string]any{"attempt": 1}),
		diagEv("run_slow", "evt_s23", observability.ExecDelegationRequest, observability.ExecRunning, 18000, 400, "A1", "dlg_1", map[string]any{"to": "anchored"}),
		diagEv("run_slow", "evt_s24", observability.ExecGate, observability.ExecSucceeded, 19000, 200, "A1", "", nil),
		diagEv("run_slow", "evt_s25", observability.ExecAgentEnd, observability.ExecSucceeded, 26000, 25800, "A1", "", nil),
		diagEv("run_slow", "evt_s26", observability.ExecRunEnd, observability.ExecSucceeded, 30000, 25000, "", "", nil),
	)
	slow = DiagRunInput{
		RunID:  "run_slow",
		Events: slowEvents,
		Results: []LabResult{
			{RunID: "run_slow", AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 20000, Verdict: LabPass, Retries: 1, TraceHint: "trace run_slow"},
			{RunID: "run_slow", AgentID: "anchored", AgentExecutionID: "A2", DurationMS: 4000, Verdict: LabPass, TraceHint: "trace run_slow"},
		},
	}
	fast = DiagRunInput{
		RunID: "run_fast",
		Events: []observability.ExecEvent{
			diagEv("run_fast", "evt_f01", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
			diagEv("run_fast", "evt_f02", observability.ExecAgentStart, observability.ExecStarted, 100, 0, "A9", "", nil),
			diagEv("run_fast", "evt_f03", observability.ExecToolCall, observability.ExecRunning, 200, 0, "A9", "", toolPayload("read", map[string]any{"path": "f.txt"})),
			diagEv("run_fast", "evt_f04", observability.ExecToolResult, observability.ExecRunning, 300, 100, "A9", "", toolPayload("read", map[string]any{"path": "f.txt"})),
			diagEv("run_fast", "evt_f05", observability.ExecAgentEnd, observability.ExecSucceeded, 4600, 4500, "A9", "", nil),
			diagEv("run_fast", "evt_f06", observability.ExecRunEnd, observability.ExecSucceeded, 5000, 4000, "", "", nil),
		},
		Results: []LabResult{
			{RunID: "run_fast", AgentID: "baseline", AgentExecutionID: "A9", DurationMS: 4500, Verdict: LabPass, TraceHint: "trace run_fast"},
		},
	}
	return slow, fast
}

func TestDiagSlowRunFindsDominantStageAndAgent(t *testing.T) {
	slow, fast := diagSlowFixture()
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{slow, fast}, BaselineRunID: "run_fast"}, DefaultDiagThresholds())
	if rep.Version != DiagVersion {
		t.Fatalf("version = %q, want %q", rep.Version, DiagVersion)
	}
	if len(rep.Runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(rep.Runs))
	}
	s := rep.Runs[0]
	if s.WallMS != 30000 || s.WallSource != "trace-span" {
		t.Fatalf("slow wall = %d (%s), want 30000 (trace-span)", s.WallMS, s.WallSource)
	}
	if s.AggregateMS != 24000 {
		t.Fatalf("slow aggregate = %d, want 24000", s.AggregateMS)
	}
	got := map[string]int64{
		TierPhasePrep: s.Stages.PrepMS, TierPhaseQueue: s.Stages.QueueMS,
		TierPhaseInference: s.Stages.InferenceMS, TierPhaseTools: s.Stages.ToolsMS,
		TierPhaseEval: s.Stages.EvalMS, TierPhaseTeardown: s.Stages.TeardownMS,
	}
	want := map[string]int64{
		TierPhasePrep: 150, TierPhaseQueue: 500, TierPhaseInference: 12200,
		TierPhaseTools: 15100, TierPhaseEval: 200, TierPhaseTeardown: 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slow stages = %v, want %v", got, want)
	}
	if s.UnattributedMS != 1850 {
		t.Fatalf("slow unattributed = %d, want 1850", s.UnattributedMS)
	}
	if s.DominantStage.Name != TierPhaseTools || s.DominantStage.MS != 15100 {
		t.Fatalf("dominant stage = %+v, want tools/15100", s.DominantStage)
	}
	if !strings.Contains(s.DominantStage.Evidence, "run_slow") {
		t.Fatalf("dominant stage must link evidence, got %q", s.DominantStage.Evidence)
	}
	if s.DominantAgent.Name != "baseline" || s.DominantAgent.MS != 20000 {
		t.Fatalf("dominant agent = %+v, want baseline/20000", s.DominantAgent)
	}
	if s.Counts.Retries != 2 {
		t.Fatalf("retries = %d (1 event + 1 lab), want 2", s.Counts.Retries)
	}
	byKind := map[string]DiagAlert{}
	for _, a := range rep.Alerts {
		if a.RunID == "run_slow" {
			byKind[a.Kind+"/"+a.Subject] = a
		}
	}
	loop, ok := byKind["loop/read"]
	if !ok {
		t.Fatalf("slow run must raise a loop alert, got %+v", rep.Alerts)
	}
	if !strings.Contains(loop.Sample, "6 consecutive") || !strings.Contains(loop.Threshold, "loop_repeats=5") {
		t.Fatalf("loop alert must show sample and threshold, got %+v", loop)
	}
	if !strings.Contains(loop.Evidence, "run_slow") {
		t.Fatalf("loop alert must link evidence, got %+v", loop)
	}
	tool, ok := byKind["slow-tool/read"]
	if !ok {
		t.Fatalf("read must raise a slow-tool alert, got %+v", rep.Alerts)
	}
	if !strings.Contains(tool.Sample, "12000ms") || !strings.Contains(tool.Threshold, "slow_tool_ms=2000") {
		t.Fatalf("slow-tool alert must show sample and threshold, got %+v", tool)
	}
	dlg, ok := byKind["delegation/dlg_1"]
	if !ok {
		t.Fatalf("pending delegation must raise an alert, got %+v", rep.Alerts)
	}
	if !strings.Contains(dlg.Sample, "pending") || !strings.Contains(dlg.Threshold, "contribution required") {
		t.Fatalf("delegation alert must show sample and threshold, got %+v", dlg)
	}
	if len(rep.Deltas) != 1 {
		t.Fatalf("deltas = %+v, want one slow-vs-baseline delta", rep.Deltas)
	}
	d := rep.Deltas[0]
	if d.WallDeltaMS != 25000 || d.Verdict != "slower" {
		t.Fatalf("delta = %+v, want +25000 slower", d)
	}
	if d.StageDeltas[TierPhaseTools] != 15000 {
		t.Fatalf("tools delta = %d, want 15000", d.StageDeltas[TierPhaseTools])
	}
	if !reflect.DeepEqual(rep.Thresholds, DefaultDiagThresholds()) {
		t.Fatalf("report must echo the thresholds verbatim: %+v", rep.Thresholds)
	}
}

func TestDiagQuantileGating(t *testing.T) {
	vals := func(n int, v int64) []int64 {
		out := make([]int64, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	q := SummarizeLatencies(vals(4, 100))
	if q.MedianOK || q.P90OK || q.P95OK {
		t.Fatalf("n=4 must claim nothing, got %+v", q)
	}
	q = SummarizeLatencies(vals(5, 100))
	if !q.MedianOK || q.MedianMS != 100 || q.P90OK || q.P95OK {
		t.Fatalf("n=5 claims median only, got %+v", q)
	}
	q = SummarizeLatencies([]int64{10, 20, 30, 40, 50, 60, 70, 80, 90})
	if !q.MedianOK || q.P90OK {
		t.Fatalf("n=9 claims median but not p90, got %+v", q)
	}
	// Nearest-rank: sorted 1..10, p90 rank=ceil(9)=9 -> 9.
	ten := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	q = SummarizeLatencies(ten)
	if !q.P90OK || q.P90MS != 9 {
		t.Fatalf("n=10 p90 = %d (ok=%v), want 9", q.P90MS, q.P90OK)
	}
	if q.P95OK {
		t.Fatalf("n=10 must not claim p95, got %+v", q)
	}
	if q.MedianMS != 5 {
		t.Fatalf("n=10 median (nearest-rank) = %d, want 5", q.MedianMS)
	}
	// n=20: p95 rank=ceil(19)=19 -> 19.
	twenty := make([]int64, 20)
	for i := range twenty {
		twenty[i] = int64(i + 1)
	}
	q = SummarizeLatencies(twenty)
	if !q.P95OK || q.P95MS != 19 {
		t.Fatalf("n=20 p95 = %d (ok=%v), want 19", q.P95MS, q.P95OK)
	}
}

func TestDiagOutliersNeedSufficientN(t *testing.T) {
	mkRun := func(id string, lats ...int64) DiagRunInput {
		evs := []observability.ExecEvent{
			diagEv(id, "e_start", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
		}
		for i, l := range lats {
			evs = append(evs,
				diagEv(id, "e_c", observability.ExecToolCall, observability.ExecRunning, int64(100+i*200), 0, "A1", "", toolPayload("q", nil)),
				diagEv(id, "e_r", observability.ExecToolResult, observability.ExecRunning, int64(150+i*200), l, "A1", "", toolPayload("q", nil)),
			)
			evs[len(evs)-2].EventID = "e_c" + string(rune('a'+i))
			evs[len(evs)-1].EventID = "e_r" + string(rune('a'+i))
		}
		evs = append(evs, diagEv(id, "e_end", observability.ExecRunEnd, observability.ExecSucceeded, int64(100+len(lats)*200+100), 0, "", "", nil))
		return DiagRunInput{RunID: id, Events: evs}
	}
	small := []int64{10, 20, 30, 40, 50, 60, 70, 80, 900}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{mkRun("r_small", small...)}}, DefaultDiagThresholds())
	for _, a := range rep.Alerts {
		if a.Kind == "outlier" {
			t.Fatalf("n=9 must not claim outliers, got %+v", a)
		}
	}
	found := false
	for _, n := range rep.Notes {
		if strings.Contains(n, "r_small") && strings.Contains(n, "insufficient sample") {
			found = true
		}
	}
	if !found {
		t.Fatalf("n=9 must note the insufficient sample, notes = %v", rep.Notes)
	}
	big := []int64{500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 8000}
	rep = Diagnose(DiagInput{Runs: []DiagRunInput{mkRun("r_big", big...)}}, DefaultDiagThresholds())
	var outlier *DiagAlert
	for i := range rep.Alerts {
		if rep.Alerts[i].Kind == "outlier" {
			outlier = &rep.Alerts[i]
		}
	}
	if outlier == nil {
		t.Fatalf("n=12 with a spike must raise an outlier, alerts = %+v", rep.Alerts)
	}
	if !strings.Contains(outlier.Sample, "8000ms") || !strings.Contains(outlier.Threshold, "p90=") {
		t.Fatalf("outlier must show sample and p90 threshold, got %+v", outlier)
	}
}

func TestDiagTimeoutsAndPartialData(t *testing.T) {
	run := DiagRunInput{
		RunID:      "run_partial",
		Malformed:  2,
		Duplicates: 1,
		Events: []observability.ExecEvent{
			diagEv("run_partial", "e1", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
			diagEv("run_partial", "e2", observability.ExecAgentStart, observability.ExecStarted, 100, 0, "A1", "", nil),
			diagEv("run_partial", "e3", observability.ExecTimeout, observability.ExecTimedOut, 5000, 5000, "A1", "", nil),
			diagEv("run_partial", "e4", observability.ExecError, observability.ExecFailed, 5100, 0, "A1", "", nil),
			diagEv("run_partial", "e5", observability.ExecAgentEnd, observability.ExecCancelled, 5200, 5100, "A1", "", nil),
			// No run.end: the run crashed; wall still spans the events.
		},
		Results: []LabResult{
			{RunID: "run_partial", AgentID: "baseline", AgentExecutionID: "A1", DurationMS: 5100, Verdict: LabTimeout},
		},
	}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{run}}, DefaultDiagThresholds())
	s := rep.Runs[0]
	if s.WallMS != 5200 {
		t.Fatalf("partial wall = %d, want event span 5200", s.WallMS)
	}
	if s.Counts.Timeouts != 2 || s.Counts.Cancels != 1 || s.Counts.Errors != 1 {
		t.Fatalf("counts = %+v, want timeouts=2 cancels=1 errors=1", s.Counts)
	}
	if s.Counts.Malformed != 2 || s.Counts.Duplicates != 1 {
		t.Fatalf("tolerant-reader counts must be echoed, got %+v", s.Counts)
	}
	seen := map[string]bool{}
	for _, a := range rep.Alerts {
		seen[a.Kind] = true
		if (a.Kind == "timeout" || a.Kind == "cancellation" || a.Kind == "failure") &&
			!strings.Contains(a.Threshold, "occurrence") {
			t.Fatalf("fact alert must carry an occurrence threshold, got %+v", a)
		}
		if !strings.Contains(a.Evidence, "run_partial") {
			t.Fatalf("alert must link evidence, got %+v", a)
		}
	}
	for _, k := range []string{"timeout", "cancellation"} {
		if !seen[k] {
			t.Fatalf("partial run must alert %s, alerts = %+v", k, rep.Alerts)
		}
	}
	joined := strings.Join(rep.Notes, "\n")
	if !strings.Contains(joined, "no terminal run.end") {
		t.Fatalf("missing run.end must be noted as possible crash, notes = %v", rep.Notes)
	}
	empty := Diagnose(DiagInput{Runs: []DiagRunInput{{RunID: "run_empty"}}}, DefaultDiagThresholds())
	if len(empty.Runs) != 1 || empty.Runs[0].WallSource != "unknown" {
		t.Fatalf("empty run must still report with unknown wall, got %+v", empty.Runs)
	}
	if len(empty.Alerts) != 0 {
		t.Fatalf("empty run must raise no alerts, got %+v", empty.Alerts)
	}
}

func TestDiagWallVsAggregateHonesty(t *testing.T) {
	run := DiagRunInput{
		RunID: "run_parallel",
		Events: []observability.ExecEvent{
			diagEv("run_parallel", "p1", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
			diagEv("run_parallel", "p2", observability.ExecRunEnd, observability.ExecSucceeded, 9000, 9000, "", "", nil),
		},
		Results: []LabResult{
			{RunID: "run_parallel", AgentID: "a1", DurationMS: 8000, Verdict: LabPass},
			{RunID: "run_parallel", AgentID: "a2", DurationMS: 8000, Verdict: LabPass},
		},
	}
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{run}}, DefaultDiagThresholds())
	s := rep.Runs[0]
	if s.WallMS != 9000 || s.AggregateMS != 16000 {
		t.Fatalf("wall=%d aggregate=%d, want 9000 vs 16000", s.WallMS, s.AggregateMS)
	}
	found := false
	for _, n := range rep.Notes {
		if strings.Contains(n, "aggregate 16000ms exceeds wall 9000ms") {
			found = true
		}
	}
	if !found {
		t.Fatalf("parallel overlap must be noted, notes = %v", rep.Notes)
	}
}

func TestDiagDuplicationWithoutLoop(t *testing.T) {
	seq := []string{"stat", "read", "stat", "write", "stat", "fetch", "fetch", "fetch", "fetch", "fetch"}
	evs := []observability.ExecEvent{
		diagEv("run_dup", "d0", observability.ExecRunStart, observability.ExecStarted, 0, 0, "", "", nil),
	}
	for i, tool := range seq {
		args := map[string]any{"path": "f.txt"}
		if tool == "fetch" {
			args = map[string]any{"url": "x"}
		}
		evs = append(evs, diagEv("run_dup", "dc", observability.ExecToolCall, observability.ExecRunning, int64(100+i*100), 0, "A1", "", toolPayload(tool, args)))
		evs[len(evs)-1].EventID = "dc" + string(rune('a'+i))
	}
	evs = append(evs, diagEv("run_dup", "dz", observability.ExecRunEnd, observability.ExecSucceeded, 2000, 0, "", "", nil))
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{{RunID: "run_dup", Events: evs}}}, DefaultDiagThresholds())
	seenLoop, seenDup := false, false
	for _, a := range rep.Alerts {
		switch {
		case a.Kind == "loop" && strings.Contains(a.Subject, "fetch"):
			seenLoop = true
		case a.Kind == "duplication" && strings.Contains(a.Subject, "stat"):
			seenDup = true
			if !strings.Contains(a.Sample, "3 identical") || !strings.Contains(a.Threshold, "dup_repeats=3") {
				t.Fatalf("duplication must show sample and threshold, got %+v", a)
			}
		case a.Kind == "duplication" && strings.Contains(a.Subject, "fetch"):
			t.Fatalf("a reported loop must not double-report as duplication: %+v", a)
		}
	}
	if !seenLoop || !seenDup {
		t.Fatalf("want fetch loop + stat duplication, alerts = %+v", rep.Alerts)
	}
}

func TestDiagThresholdsLoadStrict(t *testing.T) {
	th := DefaultDiagThresholds()
	before := th
	_ = Diagnose(DiagInput{Runs: []DiagRunInput{{RunID: "r"}}}, th)
	if !reflect.DeepEqual(th, before) {
		t.Fatal("Diagnose must never mutate thresholds (no auto-tune)")
	}
	if err := th.validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	for _, bad := range []DiagThresholds{
		{SlowStageMS: 0, SlowToolMS: 1, LoopRepeats: 5, DupRepeats: 3, RetryBudget: 3},
		{SlowStageMS: 1, SlowToolMS: -1, LoopRepeats: 5, DupRepeats: 3, RetryBudget: 3},
		{SlowStageMS: 1, SlowToolMS: 1, LoopRepeats: 1, DupRepeats: 3, RetryBudget: 3},
		{SlowStageMS: 1, SlowToolMS: 1, LoopRepeats: 5, DupRepeats: 1, RetryBudget: 3},
		{SlowStageMS: 1, SlowToolMS: 1, LoopRepeats: 5, DupRepeats: 3, RetryBudget: -1},
	} {
		if err := bad.validate(); err == nil {
			t.Fatalf("thresholds %+v must fail validation", bad)
		}
	}
}

func TestDiagEndEventsCarryScopeWalls(t *testing.T) {
	// run.end DurationMS is the enclosing scope wall (the Lab emits
	// the full run duration there): it must never land in a stage.
	slow, _ := diagSlowFixture()
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{slow}}, DefaultDiagThresholds())
	s := rep.Runs[0]
	total := s.Stages.PrepMS + s.Stages.QueueMS + s.Stages.InferenceMS +
		s.Stages.ToolsMS + s.Stages.EvalMS + s.Stages.TeardownMS
	if total != 28150 {
		t.Fatalf("stage total = %d, want 28150 (run.end 25000 excluded)", total)
	}
}

func TestDiagMissingBaselineIsNoted(t *testing.T) {
	slow, _ := diagSlowFixture()
	rep := Diagnose(DiagInput{Runs: []DiagRunInput{slow}, BaselineRunID: "run_ghost"}, DefaultDiagThresholds())
	if len(rep.Deltas) != 0 {
		t.Fatalf("missing baseline must produce no delta, got %+v", rep.Deltas)
	}
	found := false
	for _, n := range rep.Notes {
		if strings.Contains(n, "run_ghost") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing baseline must be noted, notes = %v", rep.Notes)
	}
}
