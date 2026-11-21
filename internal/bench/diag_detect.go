package bench

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/observability"
)

// dominantStage names the costliest stage bucket with its share of
// the aggregate and the run evidence link.
func dominantStage(runID string, p TierPhases, aggregate int64, evs []observability.ExecEvent) DiagDominant {
	best, bestMS := "(none)", int64(0)
	for _, stage := range diagStageOrder {
		if v := diagStageVal(p, stage); v > bestMS {
			best, bestMS = stage, v
		}
	}
	d := DiagDominant{Name: best, MS: bestMS, Evidence: runEvidence(runID, evs)}
	if aggregate > 0 && bestMS > 0 {
		d.SharePct = 100 * float64(bestMS) / float64(aggregate)
	}
	return d
}

// dominantAgent names the costliest agent with its share of the
// aggregate and the agent evidence link.
func dominantAgent(runID string, agents []DiagAgentStat, aggregate int64) DiagDominant {
	d := DiagDominant{Name: "(none)", Evidence: DiagTraceHint(runID)}
	for _, a := range agents {
		if a.DurationMS > d.MS {
			exec := a.ExecID
			if exec == "" {
				exec = a.AgentID
			}
			d = DiagDominant{Name: a.AgentID, MS: a.DurationMS,
				Evidence: fmt.Sprintf("agent=%s run=%s (%s)", exec, runID, DiagTraceHint(runID))}
		}
	}
	if aggregate > 0 && d.MS > 0 {
		d.SharePct = 100 * float64(d.MS) / float64(aggregate)
	}
	return d
}

// runEvidence links a stage finding to its run: id, event count and
// the exact rebuild command.
func runEvidence(runID string, evs []observability.ExecEvent) string {
	return fmt.Sprintf("run=%s events=%d (%s)", runID, len(evs), DiagTraceHint(runID))
}

// runAlerts raises every finding for one run in deterministic order:
// slow stages (bucket order), slow tools (name order), loops (first
// occurrence), duplications, retry budget, timeouts, cancellations,
// failures, dead delegations, outliers.
func runAlerts(s DiagRunSummary, r DiagRunInput, th DiagThresholds) []DiagAlert {
	var out []DiagAlert
	for _, stage := range diagStageOrder {
		if v := diagStageVal(s.Stages, stage); v >= th.SlowStageMS {
			share := ""
			if s.AggregateMS > 0 {
				share = fmt.Sprintf(" (%.0f%% of aggregate)", 100*float64(v)/float64(s.AggregateMS))
			}
			out = append(out, DiagAlert{
				RunID: s.RunID, Kind: "slow-stage", Subject: stage,
				Sample:    fmt.Sprintf("%s: %dms%s", stage, v, share),
				Threshold: fmt.Sprintf("slow_stage_ms=%d", th.SlowStageMS),
				Evidence:  runEvidence(s.RunID, r.Events),
			})
		}
	}
	for _, t := range s.Tools {
		if t.TotalMS >= th.SlowToolMS {
			out = append(out, DiagAlert{
				RunID: s.RunID, Kind: "slow-tool", Subject: t.Tool,
				Sample:    fmt.Sprintf("%s: total %dms over %d call(s), max %dms", t.Tool, t.TotalMS, t.Calls, t.MaxMS),
				Threshold: fmt.Sprintf("slow_tool_ms=%d", th.SlowToolMS),
				Evidence:  runEvidence(s.RunID, r.Events),
			})
		}
	}
	out = append(out, loopAndDupAlerts(s.RunID, r.Events, th)...)
	if s.Counts.Retries > th.RetryBudget {
		out = append(out, DiagAlert{
			RunID: s.RunID, Kind: "retry-budget", Subject: "retries",
			Sample:    fmt.Sprintf("%d retries in run %s", s.Counts.Retries, s.RunID),
			Threshold: fmt.Sprintf("retry_budget=%d", th.RetryBudget),
			Evidence:  runEvidence(s.RunID, r.Events),
		})
	}
	out = append(out, occurrenceAlerts(s.RunID, r, s.Counts)...)
	out = append(out, delegationAlerts(s.RunID, r)...)
	out = append(out, outlierAlerts(s, r)...)
	return out
}

// loopAndDupAlerts detects hot loops (LoopRepeats consecutive calls
// of one tool) and duplicated work (DupRepeats identical tool+args
// repeats). A key already reported as a loop never double-reports as
// duplication. Order follows first occurrence for determinism.
func loopAndDupAlerts(runID string, evs []observability.ExecEvent, th DiagThresholds) []DiagAlert {
	type call struct {
		name, key, id string
		at            int64
	}
	var calls []call
	for _, e := range evs {
		if e.Type != observability.ExecToolCall {
			continue
		}
		calls = append(calls, call{diagToolName(e), diagToolKey(e), e.EventID, e.At.UnixMilli()})
	}
	looped := map[string]bool{}
	var out []DiagAlert
	i := 0
	for i < len(calls) {
		j := i + 1
		for j < len(calls) && calls[j].name == calls[i].name {
			j++
		}
		if j-i >= th.LoopRepeats {
			looped[calls[i].key] = true
			var total int64
			for _, e := range evs {
				if e.Type == observability.ExecToolResult && diagToolName(e) == calls[i].name {
					total += e.DurationMS
				}
			}
			out = append(out, DiagAlert{
				RunID: runID, Kind: "loop", Subject: calls[i].name,
				Sample: fmt.Sprintf("%d consecutive %s calls (%s..%s, tool total %dms)",
					j-i, calls[i].name, calls[i].id, calls[j-1].id, total),
				Threshold: fmt.Sprintf("loop_repeats=%d", th.LoopRepeats),
				Evidence:  fmt.Sprintf("run=%s events=%s..%s (%s)", runID, calls[i].id, calls[j-1].id, DiagTraceHint(runID)),
			})
		}
		i = j
	}
	byKey := map[string][]call{}
	var keyOrder []string
	for _, c := range calls {
		if looped[c.key] {
			continue
		}
		if _, ok := byKey[c.key]; !ok {
			keyOrder = append(keyOrder, c.key)
		}
		byKey[c.key] = append(byKey[c.key], c)
	}
	sort.Strings(keyOrder)
	for _, k := range keyOrder {
		if len(byKey[k]) >= th.DupRepeats {
			ids := make([]string, 0, len(byKey[k]))
			for _, c := range byKey[k] {
				ids = append(ids, c.id)
			}
			out = append(out, DiagAlert{
				RunID: runID, Kind: "duplication", Subject: byKey[k][0].name,
				Sample:    fmt.Sprintf("%d identical %s calls: %s", len(ids), byKey[k][0].name, strings.Join(ids, ", ")),
				Threshold: fmt.Sprintf("dup_repeats=%d", th.DupRepeats),
				Evidence:  fmt.Sprintf("run=%s (%s)", runID, DiagTraceHint(runID)),
			})
		}
	}
	return out
}

// occurrenceAlerts reports timeout/cancel/fail facts: they have no
// numeric threshold, so the threshold field states the occurrence
// rule explicitly and the sample lists the evidence.
func occurrenceAlerts(runID string, r DiagRunInput, c DiagCounts) []DiagAlert {
	var out []DiagAlert
	join := func(ids []string) string {
		if len(ids) > 5 {
			return strings.Join(ids[:5], ", ") + fmt.Sprintf(" (+%d more)", len(ids)-5)
		}
		return strings.Join(ids, ", ")
	}
	var timeoutIDs, cancelIDs, failIDs []string
	for _, e := range r.Events {
		switch {
		case e.Type == observability.ExecTimeout:
			timeoutIDs = append(timeoutIDs, e.EventID)
		case e.Type == observability.ExecAgentEnd && e.Status == observability.ExecCancelled:
			cancelIDs = append(cancelIDs, e.EventID)
		case e.Type == observability.ExecError:
			failIDs = append(failIDs, e.EventID)
		case observability.IsTerminal(e.Status) && e.Status == observability.ExecFailed &&
			(e.Type == observability.ExecRunEnd || e.Type == observability.ExecTaskEnd):
			failIDs = append(failIDs, e.EventID)
		}
	}
	for _, res := range r.Results {
		switch res.Verdict {
		case LabTimeout:
			timeoutIDs = append(timeoutIDs, "lab:"+res.AgentID)
		case LabNotRun:
			cancelIDs = append(cancelIDs, "lab:"+res.AgentID)
		case LabFail, LabInfraError:
			failIDs = append(failIDs, "lab:"+res.AgentID)
		}
	}
	if c.Timeouts > 0 {
		out = append(out, DiagAlert{
			RunID: runID, Kind: "timeout", Subject: "timeout",
			Sample:    fmt.Sprintf("%d timeout(s): %s", c.Timeouts, join(timeoutIDs)),
			Threshold: "occurrence (any timeout is reported)",
			Evidence:  fmt.Sprintf("run=%s (%s)", runID, DiagTraceHint(runID)),
		})
	}
	if c.Cancels > 0 {
		out = append(out, DiagAlert{
			RunID: runID, Kind: "cancellation", Subject: "cancellation",
			Sample:    fmt.Sprintf("%d cancellation(s): %s", c.Cancels, join(cancelIDs)),
			Threshold: "occurrence (any cancellation is reported)",
			Evidence:  fmt.Sprintf("run=%s (%s)", runID, DiagTraceHint(runID)),
		})
	}
	if c.Fails > 0 {
		out = append(out, DiagAlert{
			RunID: runID, Kind: "failure", Subject: "failure",
			Sample:    fmt.Sprintf("%d failure(s): %s", c.Fails, join(failIDs)),
			Threshold: "occurrence (any failure is reported)",
			Evidence:  fmt.Sprintf("run=%s (%s)", runID, DiagTraceHint(runID)),
		})
	}
	return out
}

// delegationAlerts reports delegations without contribution: a
// request with no result reads as pending, a result that never
// reached succeeded wasted the wait. Evidence is the delegation id
// plus the run rebuild command.
func delegationAlerts(runID string, r DiagRunInput) []DiagAlert {
	type edge struct {
		from, to  string
		req       bool
		res       bool
		resStatus string
	}
	edges := map[string]*edge{}
	var order []string
	for _, e := range r.Events {
		if e.DelegationID == "" {
			continue
		}
		ed, ok := edges[e.DelegationID]
		if !ok {
			ed = &edge{}
			edges[e.DelegationID] = ed
			order = append(order, e.DelegationID)
		}
		switch e.Type {
		case observability.ExecDelegationRequest:
			ed.req = true
			ed.from = e.AgentExecutionID
			if to, _ := e.Payload["to"].(string); to != "" {
				ed.to = to
			}
		case observability.ExecDelegationResult:
			ed.res = true
			ed.resStatus = e.Status
		}
	}
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, res := range r.Results {
		for _, d := range res.Delegations {
			if d.DelegationID == "" || seen[d.DelegationID] {
				continue
			}
			seen[d.DelegationID] = true
			order = append(order, d.DelegationID)
			edges[d.DelegationID] = &edge{from: d.FromAgent, to: d.ToAgent, req: true, res: d.Status != "", resStatus: d.Status}
		}
	}
	sort.Strings(order)
	var out []DiagAlert
	for _, id := range order {
		ed := edges[id]
		from, to := ed.from, ed.to
		if from == "" {
			from = "(unknown)"
		}
		if to == "" {
			to = "(unknown)"
		}
		switch {
		case ed.req && !ed.res:
			out = append(out, DiagAlert{
				RunID: runID, Kind: "delegation", Subject: id,
				Sample:    fmt.Sprintf("%s: %s -> %s (pending: request without result)", id, from, to),
				Threshold: "contribution required (terminal succeeded)",
				Evidence:  fmt.Sprintf("delegation=%s run=%s (%s)", id, runID, DiagTraceHint(runID)),
			})
		case ed.res && ed.resStatus != observability.ExecSucceeded:
			out = append(out, DiagAlert{
				RunID: runID, Kind: "delegation", Subject: id,
				Sample:    fmt.Sprintf("%s: %s -> %s (result %s, no contribution)", id, from, to, ed.resStatus),
				Threshold: "contribution required (terminal succeeded)",
				Evidence:  fmt.Sprintf("delegation=%s run=%s (%s)", id, runID, DiagTraceHint(runID)),
			})
		}
	}
	return out
}

// outlierAlerts flags tool latencies above the run p90, only when n
// reaches DiagMinP90N. Below that the caller notes the insufficient
// sample instead (see diagnoseRun); no outlier is ever claimed there.
func outlierAlerts(s DiagRunSummary, r DiagRunInput) []DiagAlert {
	if !s.Latencies.P90OK {
		return nil
	}
	var lats []int64
	for _, e := range r.Events {
		if e.Type == observability.ExecToolResult {
			lats = append(lats, e.DurationMS)
		}
	}
	var over []int64
	for _, l := range lats {
		if l > s.Latencies.P90MS {
			over = append(over, l)
		}
	}
	if len(over) == 0 {
		return nil
	}
	sort.Slice(over, func(i, j int) bool { return over[i] > over[j] })
	strs := make([]string, 0, len(over))
	for _, v := range over {
		strs = append(strs, fmt.Sprintf("%dms", v))
	}
	sample := strings.Join(strs, ", ")
	if len(strs) > 5 {
		sample = strings.Join(strs[:5], ", ") + fmt.Sprintf(" (+%d more)", len(strs)-5)
	}
	return []DiagAlert{{
		RunID: s.RunID, Kind: "outlier", Subject: "tool latency",
		Sample:    fmt.Sprintf("latencies [%s] above p90=%dms (n=%d)", sample, s.Latencies.P90MS, s.Latencies.N),
		Threshold: fmt.Sprintf("p90=%dms (n=%d, need %d)", s.Latencies.P90MS, s.Latencies.N, DiagMinP90N),
		Evidence:  runEvidence(s.RunID, r.Events),
	}}
}

// diffRuns contrasts one run against the baseline: wall and aggregate
// deltas plus per-stage deltas in bucket order.
func diffRuns(base, cur DiagRunSummary) DiagDelta {
	d := DiagDelta{
		RunID: cur.RunID, BaselineID: base.RunID,
		WallDeltaMS: cur.WallMS - base.WallMS,
		AggDeltaMS:  cur.AggregateMS - base.AggregateMS,
		StageDeltas: map[string]int64{},
		Verdict:     "equal",
	}
	if base.WallMS != 0 {
		d.WallDeltaPct = 100 * float64(d.WallDeltaMS) / float64(base.WallMS)
	}
	for _, stage := range diagStageOrder {
		d.StageDeltas[stage] = diagStageVal(cur.Stages, stage) - diagStageVal(base.Stages, stage)
	}
	switch {
	case d.WallDeltaMS > 0:
		d.Verdict = "slower"
	case d.WallDeltaMS < 0:
		d.Verdict = "faster"
	}
	return d
}
