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
	evs := append([]observability.ExecEvent(nil), r.Events...)
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].At.Equal(evs[j].At) {
			return evs[i].EventID < evs[j].EventID
		}
		return evs[i].At.Before(evs[j].At)
	})
	s.WallSource = "unknown"
	switch {
	case len(evs) >= 2:
		d := evs[len(evs)-1].At.Sub(evs[0].At).Milliseconds()
		if d < 0 {
			d = 0
		}
		s.WallMS = d
		s.WallSource = "trace-span"
		if d == 0 {
			*notes = append(*notes, fmt.Sprintf("run %s: zero wall span (single instant); stage costs still aggregate", r.RunID))
		}
	case len(r.Results) > 0:
		s.WallMS = maxResultMS(r.Results)
		s.WallSource = "lab-result"
		*notes = append(*notes, fmt.Sprintf("run %s: wall from Lab result (fewer than 2 trace events)", r.RunID))
	default:
		*notes = append(*notes, fmt.Sprintf("run %s: no trace events and no Lab results; wall unknown", r.RunID))
	}
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
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		*notes = append(*notes, fmt.Sprintf("run %s: %d event(s) of unmapped type(s) %s carry no stage cost",
			r.RunID, len(unmapped), strings.Join(unmapped, ", ")))
	}
	// Inference residual per traced agent: the agent.start/agent.end
	// span minus the tool time already attributed to that agent.
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
	toolsByAgent := map[string]int64{}
	toolStats := map[string]*DiagToolStat{}
	var toolOrder []string
	var latencies []int64
	for _, e := range evs {
		switch e.Type {
		case observability.ExecToolCall:
			name := diagToolName(e)
			st, ok := toolStats[name]
			if !ok {
				st = &DiagToolStat{Tool: name}
				toolStats[name] = st
				toolOrder = append(toolOrder, name)
			}
			st.Calls++
		case observability.ExecToolResult:
			name := diagToolName(e)
			st, ok := toolStats[name]
			if !ok {
				st = &DiagToolStat{Tool: name}
				toolStats[name] = st
				toolOrder = append(toolOrder, name)
			}
			st.Results++
			st.TotalMS += e.DurationMS
			if e.DurationMS > st.MaxMS {
				st.MaxMS = e.DurationMS
			}
			latencies = append(latencies, e.DurationMS)
			if e.AgentExecutionID != "" {
				toolsByAgent[e.AgentExecutionID] += e.DurationMS
			}
		}
	}
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
			stages.InferenceMS += rest
		}
	}
	// Fallback: agents known only from Lab results (no traced pair)
	// attribute their duration minus traced tool time to inference.
	for _, res := range r.Results {
		if res.AgentExecutionID == "" || paired[res.AgentExecutionID] || res.DurationMS <= 0 {
			continue
		}
		if rest := res.DurationMS - toolsByAgent[res.AgentExecutionID]; rest > 0 {
			stages.InferenceMS += rest
		}
		paired[res.AgentExecutionID] = true
		*notes = append(*notes, fmt.Sprintf("run %s: inference for agent %s from Lab result (no agent.start/end pair)",
			r.RunID, res.AgentID))
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
	if len(r.Results) > 0 {
		var agg int64
		for _, res := range r.Results {
			agg += res.DurationMS
		}
		s.AggregateMS = agg
	} else {
		for id, sp := range spans {
			if sp.hasStart && sp.hasEnd {
				d := sp.end.Sub(sp.start).Milliseconds()
				if d > 0 {
					s.AggregateMS += d
				}
				_ = id
			}
		}
	}
	if s.WallMS > 0 && s.AggregateMS > s.WallMS {
		*notes = append(*notes, fmt.Sprintf("run %s: aggregate %dms exceeds wall %dms: overlapping or parallel work; wall is not a sum",
			r.RunID, s.AggregateMS, s.WallMS))
	}
	sort.Strings(toolOrder)
	for _, name := range toolOrder {
		st := toolStats[name]
		if st.Results > 0 {
			st.MeanMS = st.TotalMS / int64(st.Results)
		}
		s.Tools = append(s.Tools, *st)
	}
	s.Counts = foldCounts(r)
	s.Latencies = SummarizeLatencies(latencies)
	if s.Latencies.N > 0 && s.Latencies.N < DiagMinP90N {
		*notes = append(*notes, fmt.Sprintf("run %s: outlier scan needs n>=%d, have n=%d: insufficient sample, no outlier claimed",
			r.RunID, DiagMinP90N, s.Latencies.N))
	}
	s.DominantStage = dominantStage(r.RunID, stages, s.AggregateMS, evs)
	s.DominantAgent = dominantAgent(r.RunID, s.Agents, s.AggregateMS)
	var sawTerminal bool
	for _, e := range evs {
		if e.Type == observability.ExecRunEnd && observability.IsTerminal(e.Status) {
			sawTerminal = true
		}
	}
	if len(evs) > 0 && !sawTerminal {
		*notes = append(*notes, fmt.Sprintf("run %s: no terminal run.end (possible crash; see %s)", r.RunID, DiagTraceHint(r.RunID)))
	}
	if r.Malformed > 0 || r.Duplicates > 0 {
		*notes = append(*notes, fmt.Sprintf("run %s: tolerant reader skipped %d malformed line(s), %d duplicate event(s)",
			r.RunID, r.Malformed, r.Duplicates))
	}
	return s
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
