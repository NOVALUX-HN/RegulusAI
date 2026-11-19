package bench

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/observability"
)

// diagnoseRun folds one run: wall from the event span (falling back
// to the linked Lab durations, then to unknown), aggregate from Lab
// durations (else traced agent spans), stages from attributed event
// durations plus the inference residual.
func diagnoseRun(r DiagRunInput, th DiagThresholds, notes *[]string) DiagRunSummary {
	_ = th
	s := DiagRunSummary{RunID: r.RunID, TraceHint: DiagTraceHint(r.RunID)}
	evs := sortedDiagEvents(r.Events)
	wall := diagWallFor(r.RunID, evs, r.Results)
	s.WallMS, s.WallSource = wall.ms, wall.source
	if wall.note != "" {
		*notes = append(*notes, wall.note)
	}
	fold := diagFoldStages(r.RunID, evs)
	stages := fold.stages
	if fold.note != "" {
		*notes = append(*notes, fold.note)
	}
	spans := diagCollectSpans(evs)
	tools := diagFoldTools(evs)
	tracedMS, paired := diagTracedResidual(spans, tools.byAgent)
	lab := diagLabResidual(r.RunID, r.Results, tools.byAgent, paired)
	stages.InferenceMS += tracedMS + lab.ms
	for _, n := range lab.notes {
		*notes = append(*notes, n)
	}
	s.Stages = stages
	stageTotal := stages.PrepMS + stages.QueueMS + stages.InferenceMS + stages.ToolsMS + stages.EvalMS + stages.TeardownMS
	if s.WallMS > stageTotal {
		s.UnattributedMS = s.WallMS - stageTotal
	} else if stageTotal > s.WallMS {
		*notes = append(*notes, fmt.Sprintf("run %s: staged costs %dms exceed wall %dms (parallel or overlapping work; wall is not a sum)",
			r.RunID, stageTotal, s.WallMS))
	}
	s.Agents = foldAgents(r, spans)
	s.AggregateMS = diagAggregateMS(r.Results, spans)
	if s.WallMS > 0 && s.AggregateMS > s.WallMS {
		*notes = append(*notes, fmt.Sprintf("run %s: aggregate %dms exceeds wall %dms: overlapping or parallel work; wall is not a sum",
			r.RunID, s.AggregateMS, s.WallMS))
	}
	s.Tools = diagFinalizeTools(tools.stats, tools.order)
	s.Counts = foldCounts(r)
	s.Latencies = SummarizeLatencies(tools.latencies)
	if s.Latencies.N > 0 && s.Latencies.N < DiagMinP90N {
		*notes = append(*notes, fmt.Sprintf("run %s: outlier scan needs n>=%d, have n=%d: insufficient sample, no outlier claimed",
			r.RunID, DiagMinP90N, s.Latencies.N))
	}
	s.DominantStage = dominantStage(r.RunID, stages, s.AggregateMS, evs)
	s.DominantAgent = dominantAgent(r.RunID, s.Agents, s.AggregateMS)
	if len(evs) > 0 && !diagHasTerminal(evs) {
		*notes = append(*notes, fmt.Sprintf("run %s: no terminal run.end (possible crash; see %s)", r.RunID, DiagTraceHint(r.RunID)))
	}
	if r.Malformed > 0 || r.Duplicates > 0 {
		*notes = append(*notes, fmt.Sprintf("run %s: tolerant reader skipped %d malformed line(s), %d duplicate event(s)",
			r.RunID, r.Malformed, r.Duplicates))
	}
	return s
}

// sortedDiagEvents returns a stable time-ordered copy of the run
// events (event id breaks wall-clock ties).
func sortedDiagEvents(evs []observability.ExecEvent) []observability.ExecEvent {
	out := append([]observability.ExecEvent(nil), evs...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].EventID < out[j].EventID
		}
		return out[i].At.Before(out[j].At)
	})
	return out
}

// diagWall is the folded wall clock: milliseconds, its source and
// the optional evidence note.
type diagWall struct {
	ms     int64
	source string
	note   string
}

// diagWallFor folds the wall clock: the event span when at least two
// events exist, else the largest linked Lab duration, else unknown.
func diagWallFor(runID string, evs []observability.ExecEvent, results []LabResult) diagWall {
	switch {
	case len(evs) >= 2:
		d := evs[len(evs)-1].At.Sub(evs[0].At).Milliseconds()
		if d < 0 {
			d = 0
		}
		w := diagWall{ms: d, source: "trace-span"}
		if d == 0 {
			w.note = fmt.Sprintf("run %s: zero wall span (single instant); stage costs still aggregate", runID)
		}
		return w
	case len(results) > 0:
		return diagWall{
			ms:     maxResultMS(results),
			source: "lab-result",
			note:   fmt.Sprintf("run %s: wall from Lab result (fewer than 2 trace events)", runID),
		}
	default:
		return diagWall{
			source: "unknown",
			note:   fmt.Sprintf("run %s: no trace events and no Lab results; wall unknown", runID),
		}
	}
}

// diagStageFold is the folded stage buckets plus the unmapped-type
// evidence note (empty when every event mapped).
type diagStageFold struct {
	stages TierPhases
	note   string
}

// diagFoldStages attributes event durations to stage buckets and
// reports the unmapped types (sorted, deduplicated).
func diagFoldStages(runID string, evs []observability.ExecEvent) diagStageFold {
	var stages TierPhases
	var unmapped []string
	seenUnmapped := map[string]bool{}
	for _, e := range evs {
		stage := DiagStageOf(e.Type)
		if stage == "" || e.DurationMS <= 0 {
			if stage == "" && !isScopeWallEvent(e.Type) && !isOutcomeEvent(e.Type) && !seenUnmapped[e.Type] {
				seenUnmapped[e.Type] = true
				unmapped = append(unmapped, e.Type)
			}
			continue
		}
		_ = stages.Add(stage, e.DurationMS)
	}
	var fold diagStageFold
	fold.stages = stages
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		fold.note = fmt.Sprintf("run %s: %d event(s) of unmapped type(s) %s carry no stage cost",
			runID, len(unmapped), strings.Join(unmapped, ", "))
	}
	return fold
}

// diagCollectSpans indexes the traced agent.start/agent.end pairs by
// agent_execution_id.
func diagCollectSpans(evs []observability.ExecEvent) map[string]*diagSpan {
	spans := map[string]*diagSpan{}
	for _, e := range evs {
		if e.AgentExecutionID == "" {
			continue
		}
		sp, ok := spans[e.AgentExecutionID]
		if !ok {
			sp = &diagSpan{}
			spans[e.AgentExecutionID] = sp
		}
		switch e.Type {
		case observability.ExecAgentStart:
			if !sp.hasStart {
				sp.start, sp.hasStart = e.At, true
			}
		case observability.ExecAgentEnd:
			sp.end, sp.hasEnd = e.At, true
		}
	}
	return spans
}

// diagToolFold is the folded tool evidence: per-agent tool time,
// per-tool stats in first-seen order and the result latency sample.
type diagToolFold struct {
	byAgent   map[string]int64
	stats     map[string]*DiagToolStat
	order     []string
	latencies []int64
}

// diagFoldTools folds tool.call/tool.result events: calls count,
// results carry the latency (the result closes the call, so only
// results sum time and nothing double-counts).
func diagFoldTools(evs []observability.ExecEvent) diagToolFold {
	fold := diagToolFold{
		byAgent: map[string]int64{},
		stats:   map[string]*DiagToolStat{},
	}
	for _, e := range evs {
		switch e.Type {
		case observability.ExecToolCall:
			name := diagToolName(e)
			st, ok := fold.stats[name]
			if !ok {
				st = &DiagToolStat{Tool: name}
				fold.stats[name] = st
				fold.order = append(fold.order, name)
			}
			st.Calls++
		case observability.ExecToolResult:
			name := diagToolName(e)
			st, ok := fold.stats[name]
			if !ok {
				st = &DiagToolStat{Tool: name}
				fold.stats[name] = st
				fold.order = append(fold.order, name)
			}
			st.Results++
			st.TotalMS += e.DurationMS
			if e.DurationMS > st.MaxMS {
				st.MaxMS = e.DurationMS
			}
			fold.latencies = append(fold.latencies, e.DurationMS)
			if e.AgentExecutionID != "" {
				fold.byAgent[e.AgentExecutionID] += e.DurationMS
			}
		}
	}
	return fold
}

// diagTracedResidual folds the inference residual over the traced
// agent.start/agent.end spans: the span minus the tool time already
// attributed to that agent. It returns the residual milliseconds and
// the paired agent ids.
func diagTracedResidual(spans map[string]*diagSpan, toolsByAgent map[string]int64) (int64, map[string]bool) {
	var inference int64
	paired := map[string]bool{}
	for id, sp := range spans {
		if !sp.hasStart || !sp.hasEnd {
			continue
		}
		d := sp.end.Sub(sp.start).Milliseconds()
		if d < 0 {
			d = 0
		}
		paired[id] = true
		if rest := d - toolsByAgent[id]; rest > 0 {
			inference += rest
		}
	}
	return inference, paired
}

// diagLabFallback is the fallback inference for agents known only
// from linked Lab results (no traced pair): the residual
// milliseconds plus the evidence notes.
type diagLabFallback struct {
	ms    int64
	notes []string
}

// diagLabResidual folds Lab-only agents into the inference residual,
// marking them paired so the aggregate never double-counts.
func diagLabResidual(runID string, results []LabResult, toolsByAgent map[string]int64, paired map[string]bool) diagLabFallback {
	var out diagLabFallback
	for _, res := range results {
		if res.AgentExecutionID == "" || paired[res.AgentExecutionID] || res.DurationMS <= 0 {
			continue
		}
		if rest := res.DurationMS - toolsByAgent[res.AgentExecutionID]; rest > 0 {
			out.ms += rest
		}
		paired[res.AgentExecutionID] = true
		out.notes = append(out.notes, fmt.Sprintf("run %s: inference for agent %s from Lab result (no agent.start/end pair)",
			runID, res.AgentID))
	}
	return out
}

// diagAggregateMS folds the aggregate cost: the Lab duration sum
// when linked results exist, else the traced agent span sum.
func diagAggregateMS(results []LabResult, spans map[string]*diagSpan) int64 {
	if len(results) > 0 {
		var agg int64
		for _, res := range results {
			agg += res.DurationMS
		}
		return agg
	}
	var agg int64
	for _, sp := range spans {
		if sp.hasStart && sp.hasEnd {
			if d := sp.end.Sub(sp.start).Milliseconds(); d > 0 {
				agg += d
			}
		}
	}
	return agg
}

// diagFinalizeTools renders the per-tool stats in name order with
// the mean latency filled in.
func diagFinalizeTools(stats map[string]*DiagToolStat, order []string) []DiagToolStat {
	sorted := append([]string(nil), order...)
	sort.Strings(sorted)
	var out []DiagToolStat
	for _, name := range sorted {
		st := stats[name]
		if st.Results > 0 {
			st.MeanMS = st.TotalMS / int64(st.Results)
		}
		out = append(out, *st)
	}
	return out
}

// diagHasTerminal reports whether any event closes a run with a
// terminal status.
func diagHasTerminal(evs []observability.ExecEvent) bool {
	for _, e := range evs {
		if e.Type == observability.ExecRunEnd && observability.IsTerminal(e.Status) {
			return true
		}
	}
	return false
}

// isScopeWallEvent reports the terminal end events whose DurationMS
// is the enclosing scope wall, never a stage cost.
func isScopeWallEvent(typ string) bool {
	return typ == observability.ExecRunEnd || typ == observability.ExecTaskEnd ||
		typ == observability.ExecAgentEnd
}

// isOutcomeEvent reports events that are counted as outcomes
// (timeout/error), never staged.
func isOutcomeEvent(typ string) bool {
	return typ == observability.ExecTimeout || typ == observability.ExecError
}

// maxResultMS returns the largest linked Lab duration.
func maxResultMS(results []LabResult) int64 {
	var m int64
	for _, res := range results {
		if res.DurationMS > m {
			m = res.DurationMS
		}
	}
	return m
}

// foldAgents builds the per-agent table: Lab results first (exact
// durations with verdicts), traced spans for agents the Lab never
// linked. Sorted by agent id for determinism.
func foldAgents(r DiagRunInput, spans map[string]*diagSpan) []DiagAgentStat {
	var out []DiagAgentStat
	seen := map[string]bool{}
	for _, res := range r.Results {
		id := res.AgentID
		if id == "" {
			id = res.AgentExecutionID
		}
		out = append(out, DiagAgentStat{
			AgentID: id, ExecID: res.AgentExecutionID, TaskID: res.TaskID,
			DurationMS: res.DurationMS, Verdict: res.Verdict, Source: "lab-result",
		})
		if res.AgentExecutionID != "" {
			seen[res.AgentExecutionID] = true
		}
	}
	var ids []string
	for id := range spans {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		sp := spans[id]
		if !sp.hasStart || !sp.hasEnd {
			continue
		}
		d := sp.end.Sub(sp.start).Milliseconds()
		if d < 0 {
			d = 0
		}
		out = append(out, DiagAgentStat{
			AgentID: id, ExecID: id, DurationMS: d, Verdict: "traced", Source: "trace-span",
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AgentID == out[j].AgentID {
			return out[i].ExecID < out[j].ExecID
		}
		return out[i].AgentID < out[j].AgentID
	})
	return out
}

// diagSpan is the traced agent.start/agent.end pair for one
// agent_execution_id.
type diagSpan struct {
	start, end       time.Time
	hasStart, hasEnd bool
}

// foldCounts folds retries, timeouts, cancellations, failures and
// the tolerant-reader evidence.
func foldCounts(r DiagRunInput) DiagCounts {
	var c DiagCounts
	c.Malformed, c.Duplicates = r.Malformed, r.Duplicates
	for _, e := range r.Events {
		switch e.Type {
		case observability.ExecRetry:
			c.Retries++
		case observability.ExecTimeout:
			c.Timeouts++
		case observability.ExecError:
			c.Errors++
		}
		if observability.IsTerminal(e.Status) &&
			(e.Type == observability.ExecRunEnd || e.Type == observability.ExecTaskEnd || e.Type == observability.ExecAgentEnd) {
			switch e.Status {
			case observability.ExecTimedOut:
				c.Timeouts++
			case observability.ExecCancelled:
				// A cancelled agent.end counts even when no terminal
				// run.end closes the run: the attempt was cancelled.
				c.Cancels++
			case observability.ExecFailed:
				c.Fails++
			}
		}
	}
	for _, res := range r.Results {
		c.Retries += res.Retries
		switch res.Verdict {
		case LabTimeout:
			c.Timeouts++
		case LabNotRun:
			c.Cancels++
		case LabFail, LabInfraError:
			c.Fails++
		}
	}
	return c
}
