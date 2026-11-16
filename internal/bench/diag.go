// Performance diagnosis (F11.4/NOV-90): explainable bottleneck
// diagnosis over F11.1 execution traces and Agent Lab outcomes.
//
// The diagnosis consumes data, never duplicates stores: execution
// events come from the F11.1 global log (internal/observability
// exec.jsonl), durations/tokens/verdicts from Agent Lab results
// (F11.2 LabResult reusing the suite AgentOutcome shape), and stage
// buckets reuse the F11.3 TierPhases vocabulary. Thresholds are
// configurable and echoed back on every report; they are never
// auto-tuned: Diagnose takes them by value and reports them verbatim.
//
// Sample discipline (documented minima, not configurable): the median
// needs DiagMinMedianN samples, p90 DiagMinP90N, p95 DiagMinP95N.
// Below the minimum the report states "insufficient sample (n=X,
// need Y)" and claims no anomaly.
package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"regulus-ai/internal/observability"
)

// DiagVersion pins the diagnosis document shape.
const DiagVersion = "perf-diag-v1"

// Sample minima for quantile claims. Median needs 5 points, p90
// needs 10, p95 needs 20 (nearest-rank becomes a coin flip below
// that). These are methodology constants on purpose: they are not
// part of DiagThresholds, so no configuration can weaken the guard.
const (
	DiagMinMedianN = 5
	DiagMinP90N    = 10
	DiagMinP95N    = 20
)

// DiagThresholds carries the configurable diagnosis rules. Every
// report echoes the exact values it ran with (see DiagReport), so a
// reader can always tell which rule fired.
type DiagThresholds struct {
	SlowStageMS int64 `json:"slow_stage_ms"`
	SlowToolMS  int64 `json:"slow_tool_ms"`
	LoopRepeats int   `json:"loop_repeats"`
	DupRepeats  int   `json:"dup_repeats"`
	RetryBudget int   `json:"retry_budget"`
}

// DefaultDiagThresholds returns the documented rule set: a stage
// totaling 10s in one run is slow, a tool costing 2s aggregate is
// slow, 5 consecutive same-tool calls are a loop, 3 identical
// tool+args repeats are duplication, more than 3 retries bust the
// budget.
func DefaultDiagThresholds() DiagThresholds {
	return DiagThresholds{
		SlowStageMS: 10000,
		SlowToolMS:  2000,
		LoopRepeats: 5,
		DupRepeats:  3,
		RetryBudget: 3,
	}
}

// validate refuses non-positive costs, repeat rules below 2 (a
// single call is never a loop) and negative retry budgets.
func (t DiagThresholds) validate() error {
	if t.SlowStageMS <= 0 {
		return fmt.Errorf("perf diagnosis: slow_stage_ms must be positive, got %d", t.SlowStageMS)
	}
	if t.SlowToolMS <= 0 {
		return fmt.Errorf("perf diagnosis: slow_tool_ms must be positive, got %d", t.SlowToolMS)
	}
	if t.LoopRepeats < 2 {
		return fmt.Errorf("perf diagnosis: loop_repeats must be >= 2, got %d", t.LoopRepeats)
	}
	if t.DupRepeats < 2 {
		return fmt.Errorf("perf diagnosis: dup_repeats must be >= 2, got %d", t.DupRepeats)
	}
	if t.RetryBudget < 0 {
		return fmt.Errorf("perf diagnosis: retry_budget must not be negative, got %d", t.RetryBudget)
	}
	return nil
}

// LoadDiagThresholds reads one thresholds file: strict JSON (unknown
// fields fail), starting from the defaults so omitted keys keep
// documented values. The result is validated before use.
func LoadDiagThresholds(path string) (DiagThresholds, error) {
	th := DefaultDiagThresholds()
	raw, err := os.ReadFile(path)
	if err != nil {
		return DiagThresholds{}, err
	}
	if len(raw) > 64*1024 {
		return DiagThresholds{}, fmt.Errorf("perf diagnosis: thresholds file too large")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&th); err != nil {
		return DiagThresholds{}, fmt.Errorf("perf diagnosis: malformed thresholds: %v", err)
	}
	if err := th.validate(); err != nil {
		return DiagThresholds{}, err
	}
	return th, nil
}

// DiagStageOf maps one F11.1 event type onto an F11.3 stage bucket.
// Terminal run/task/agent end events map to "": their DurationMS is
// the enclosing scope wall (the Lab emits the full run duration on
// run.end), never a stage cost. Timeout/error events are outcomes:
// counted separately, never staged. Unknown types also map to "",
// and the diagnosis notes them instead of misattributing them.
func DiagStageOf(eventType string) string {
	switch eventType {
	case observability.ExecRunStart, observability.ExecTaskStart,
		observability.ExecAgentStart, observability.ExecContext:
		return TierPhasePrep
	case observability.ExecDelegationRequest, observability.ExecDelegationResult,
		observability.ExecRetry:
		return TierPhaseQueue
	case observability.ExecToolCall, observability.ExecToolResult:
		return TierPhaseTools
	case observability.ExecGate, observability.ExecArtifact,
		observability.ExecDecision:
		return TierPhaseEval
	}
	return ""
}

// diagStageOrder fixes the render order of stage buckets.
var diagStageOrder = []string{
	TierPhasePrep, TierPhaseQueue, TierPhaseInference,
	TierPhaseTools, TierPhaseEval, TierPhaseTeardown,
}

// diagStageVal reads one bucket out of aggregated phases.
func diagStageVal(p TierPhases, stage string) int64 {
	switch stage {
	case TierPhasePrep:
		return p.PrepMS
	case TierPhaseQueue:
		return p.QueueMS
	case TierPhaseInference:
		return p.InferenceMS
	case TierPhaseTools:
		return p.ToolsMS
	case TierPhaseEval:
		return p.EvalMS
	case TierPhaseTeardown:
		return p.TeardownMS
	}
	return 0
}

// diagToolName names the tool of a tool.call/tool.result event: the
// payload "tool" key wins, "name" is the fallback, anything else
// reads as unnamed (counted, never dropped).
func diagToolName(e observability.ExecEvent) string {
	if s, _ := e.Payload["tool"].(string); strings.TrimSpace(s) != "" {
		return s
	}
	if s, _ := e.Payload["name"].(string); strings.TrimSpace(s) != "" {
		return s
	}
	return "(unnamed)"
}

// diagToolKey identifies one repeated unit of work: the tool name
// plus its canonical arguments (encoding/json sorts map keys, so the
// key is stable). Volatile "tool"/"name" keys stay out of the args.
func diagToolKey(e observability.ExecEvent) string {
	args := map[string]any{}
	for k, v := range e.Payload {
		if k != "tool" && k != "name" {
			args[k] = v
		}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return diagToolName(e) + "|(unhashable)"
	}
	return diagToolName(e) + "|" + string(raw)
}

// DiagQuantile returns the nearest-rank q-quantile over ascending
// values (rank = ceil(q*n), 1-indexed): deterministic, explainable,
// no interpolation surprises. ok is false on empty input or q
// outside (0,1].
func DiagQuantile(ascending []int64, q float64) (v int64, ok bool) {
	if len(ascending) == 0 || q <= 0 || q > 1 {
		return 0, false
	}
	rank := int(math.Ceil(q * float64(len(ascending))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(ascending) {
		rank = len(ascending)
	}
	return ascending[rank-1], true
}

// DiagQuantiles carries one latency multiset with its sufficiency
// flags: a quantile is reported only when n reaches its documented
// minimum, otherwise the *_ok flag stays false and the render states
// the minimum instead of a number.
type DiagQuantiles struct {
	N        int   `json:"n"`
	MedianMS int64 `json:"median_ms"`
	MedianOK bool  `json:"median_ok"`
	P90MS    int64 `json:"p90_ms"`
	P90OK    bool  `json:"p90_ok"`
	P95MS    int64 `json:"p95_ms"`
	P95OK    bool  `json:"p95_ok"`
}

// SummarizeLatencies sorts one latency multiset and claims only the
// quantiles its size supports.
func SummarizeLatencies(values []int64) DiagQuantiles {
	cp := append([]int64(nil), values...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	q := DiagQuantiles{N: len(cp)}
	if len(cp) >= DiagMinMedianN {
		q.MedianMS, _ = DiagQuantile(cp, 0.5)
		q.MedianOK = true
	}
	if len(cp) >= DiagMinP90N {
		q.P90MS, _ = DiagQuantile(cp, 0.9)
		q.P90OK = true
	}
	if len(cp) >= DiagMinP95N {
		q.P95MS, _ = DiagQuantile(cp, 0.95)
		q.P95OK = true
	}
	return q
}

// DiagRunInput is one diagnosed scope: the F11.1 events of one run
// plus the F11.2 Lab results linked to it. Malformed/Duplicates carry
// the tolerant-reader counts from the log load, so partial reads stay
// visible. Notes pass caller context through verbatim.
type DiagRunInput struct {
	RunID      string
	Events     []observability.ExecEvent
	Results    []LabResult
	Malformed  int
	Duplicates int
}

// DiagInput scopes one diagnosis: the runs plus the optional
// baseline run id for the controlled-scenario delta.
type DiagInput struct {
	Runs          []DiagRunInput
	BaselineRunID string
	Notes         []string
}

// DiagAgentStat is one agent's cost: from the Lab result when linked,
// else from the traced agent.start/agent.end span.
type DiagAgentStat struct {
	AgentID    string `json:"agent_id"`
	ExecID     string `json:"exec_id"`
	TaskID     string `json:"task_id"`
	DurationMS int64  `json:"duration_ms"`
	Verdict    string `json:"verdict"`
	Source     string `json:"source"`
}

// DiagToolStat folds one tool: calls count tool.call events,
// latencies sum tool.result durations (the result closes the call,
// so only results carry latency and nothing double-counts).
type DiagToolStat struct {
	Tool    string `json:"tool"`
	Calls   int    `json:"calls"`
	Results int    `json:"results"`
	TotalMS int64  `json:"total_ms"`
	MaxMS   int64  `json:"max_ms"`
	MeanMS  int64  `json:"mean_ms"`
}

// DiagCounts folds retries, timeouts, cancellations, failures and
// the tolerant-reader evidence. Lab verdicts join event counts:
// timeouts add LabTimeout results, cancels add LabNotRun, failures
// add LabFail/LabInfraError.
type DiagCounts struct {
	Retries    int `json:"retries"`
	Timeouts   int `json:"timeouts"`
	Cancels    int `json:"cancels"`
	Fails      int `json:"fails"`
	Errors     int `json:"errors"`
	Malformed  int `json:"malformed"`
	Duplicates int `json:"duplicates"`
}

// DiagDominant names the costliest stage or agent with its evidence.
type DiagDominant struct {
	Name     string  `json:"name"`
	MS       int64   `json:"ms"`
	SharePct float64 `json:"share_pct"`
	Evidence string  `json:"evidence"`
}

// DiagAlert is one finding: what fired, the observed sample, the
// rule threshold it crossed, and the evidence linking it. Occurrence
// facts (timeout/cancel/fail) carry threshold "occurrence (any ...)":
// every alert shows both sample and threshold.
type DiagAlert struct {
	RunID     string `json:"run_id"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject"`
	Sample    string `json:"sample"`
	Threshold string `json:"threshold"`
	Evidence  string `json:"evidence"`
}

// DiagRunSummary is the per-run diagnosis: wall vs aggregate, stage
// buckets, tools, counts, quantiles and dominants. Teardown stays 0:
// F11.1 emits no teardown event (workdir cleanup is untraced), so the
// wall remainder that no bucket explains reads as UnattributedMS,
// never as invented teardown.
type DiagRunSummary struct {
	RunID          string          `json:"run_id"`
	WallMS         int64           `json:"wall_ms"`
	WallSource     string          `json:"wall_source"`
	AggregateMS    int64           `json:"aggregate_ms"`
	Stages         TierPhases      `json:"stages"`
	UnattributedMS int64           `json:"unattributed_ms"`
	Agents         []DiagAgentStat `json:"agents"`
	Tools          []DiagToolStat  `json:"tools"`
	Counts         DiagCounts      `json:"counts"`
	Latencies      DiagQuantiles   `json:"latencies"`
	DominantStage  DiagDominant    `json:"dominant_stage"`
	DominantAgent  DiagDominant    `json:"dominant_agent"`
	TraceHint      string          `json:"trace_hint"`
}

// DiagDelta contrasts one run against the baseline: wall and
// aggregate deltas plus per-stage deltas. Verdict is slower/faster
// when the wall moved, else equal.
type DiagDelta struct {
	RunID        string           `json:"run_id"`
	BaselineID   string           `json:"baseline_id"`
	WallDeltaMS  int64            `json:"wall_delta_ms"`
	WallDeltaPct float64          `json:"wall_delta_pct"`
	AggDeltaMS   int64            `json:"agg_delta_ms"`
	StageDeltas  map[string]int64 `json:"stage_deltas"`
	Verdict      string           `json:"verdict"`
}

// DiagReport is the full diagnosis: version, the thresholds it ran
// with (echoed verbatim, never tuned), per-run summaries, cross-run
// agent-duration quantiles, alerts in deterministic order, baseline
// deltas and notes (insufficient samples, partial data, overlap).
type DiagReport struct {
	Version    string           `json:"version"`
	Thresholds DiagThresholds   `json:"thresholds"`
	Runs       []DiagRunSummary `json:"runs"`
	CrossRun   DiagQuantiles    `json:"cross_run_agent_ms"`
	Alerts     []DiagAlert      `json:"alerts"`
	Deltas     []DiagDelta      `json:"deltas"`
	Notes      []string         `json:"notes"`
}

// DiagTraceHint renders the evidence link for one run: the exact
// F11.1 rebuild command.
func DiagTraceHint(runID string) string {
	return "regulus trace timeline --run " + runID + " --rebuild"
}

// Diagnose explains where time went over the input runs. It is pure:
// thresholds arrive by value and leave untouched (no auto-tune), and
// every number on the report derives from the input events/results.
func Diagnose(in DiagInput, th DiagThresholds) DiagReport {
	rep := DiagReport{
		Version:    DiagVersion,
		Thresholds: th,
		Notes:      append([]string(nil), in.Notes...),
	}
	byID := map[string]*DiagRunSummary{}
	for _, r := range in.Runs {
		s := diagnoseRun(r, th, &rep.Notes)
		rep.Runs = append(rep.Runs, s)
		byID[r.RunID] = &rep.Runs[len(rep.Runs)-1]
		rep.Alerts = append(rep.Alerts, runAlerts(s, r, th)...)
	}
	var agentDurations []int64
	for _, s := range rep.Runs {
		for _, a := range s.Agents {
			agentDurations = append(agentDurations, a.DurationMS)
		}
	}
	rep.CrossRun = SummarizeLatencies(agentDurations)
	if len(agentDurations) > 0 && len(agentDurations) < DiagMinMedianN {
		rep.Notes = append(rep.Notes,
			fmt.Sprintf("cross-run agent durations: insufficient sample (n=%d, need %d); no cross-run claim",
				len(agentDurations), DiagMinMedianN))
	}
	if in.BaselineRunID != "" {
		base, ok := byID[in.BaselineRunID]
		if !ok {
			rep.Notes = append(rep.Notes,
				fmt.Sprintf("baseline %q is not in scope; no delta computed", in.BaselineRunID))
		} else {
			for _, s := range rep.Runs {
				if s.RunID == base.RunID {
					continue
				}
				rep.Deltas = append(rep.Deltas, diffRuns(*base, s))
			}
		}
	}
	if rep.Alerts == nil {
		rep.Alerts = []DiagAlert{}
	}
	if rep.Deltas == nil {
		rep.Deltas = []DiagDelta{}
	}
	if rep.Notes == nil {
		rep.Notes = []string{}
	}
	return rep
}

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

// RenderDiagReport builds the human diagnosis: scope and rules,
// wall-vs-aggregate, stages with shares and dominants, tools,
// counts, quantiles with sufficiency, alerts with sample+threshold,
// baseline deltas and notes. Deterministic: every list is sorted.
func RenderDiagReport(r DiagReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "perf-diagnosis %s: %d run(s), %d alert(s)\n", r.Version, len(r.Runs), len(r.Alerts))
	fmt.Fprintf(&b, "rules: slow_stage_ms=%d slow_tool_ms=%d loop_repeats=%d dup_repeats=%d retry_budget=%d (configured, never auto-tuned)\n",
		r.Thresholds.SlowStageMS, r.Thresholds.SlowToolMS, r.Thresholds.LoopRepeats, r.Thresholds.DupRepeats, r.Thresholds.RetryBudget)
	for _, s := range r.Runs {
		fmt.Fprintf(&b, "\n== run %s ==\n", s.RunID)
		fmt.Fprintf(&b, "wall %dms (%s) vs aggregate %dms\n", s.WallMS, s.WallSource, s.AggregateMS)
		for _, stage := range diagStageOrder {
			v := diagStageVal(s.Stages, stage)
			share := ""
			if s.AggregateMS > 0 && v > 0 {
				share = fmt.Sprintf(" (%.0f%% of aggregate)", 100*float64(v)/float64(s.AggregateMS))
			}
			fmt.Fprintf(&b, "  stage %-10s %6dms%s\n", stage, v, share)
		}
		if s.UnattributedMS > 0 {
			fmt.Fprintf(&b, "  unattributed (wall remainder) %dms\n", s.UnattributedMS)
		}
		fmt.Fprintf(&b, "dominant stage: %s (%dms) [%s]\n", s.DominantStage.Name, s.DominantStage.MS, s.DominantStage.Evidence)
		fmt.Fprintf(&b, "dominant agent: %s (%dms) [%s]\n", s.DominantAgent.Name, s.DominantAgent.MS, s.DominantAgent.Evidence)
		if len(s.Agents) > 0 {
			b.WriteString("agents:\n")
			for _, a := range s.Agents {
				fmt.Fprintf(&b, "  - %s task=%s %dms verdict=%s (%s)\n", a.AgentID, a.TaskID, a.DurationMS, a.Verdict, a.Source)
			}
		}
		if len(s.Tools) > 0 {
			b.WriteString("tools:\n")
			for _, t := range s.Tools {
				fmt.Fprintf(&b, "  - %s: %d call(s) total %dms max %dms mean %dms\n",
					t.Tool, t.Calls, t.TotalMS, t.MaxMS, t.MeanMS)
			}
		}
		fmt.Fprintf(&b, "counts: retries=%d timeouts=%d cancels=%d fails=%d errors=%d malformed=%d duplicates=%d\n",
			s.Counts.Retries, s.Counts.Timeouts, s.Counts.Cancels, s.Counts.Fails,
			s.Counts.Errors, s.Counts.Malformed, s.Counts.Duplicates)
		fmt.Fprintf(&b, "latencies (tool.result, n=%d): %s\n", s.Latencies.N, renderQuantiles(s.Latencies))
		fmt.Fprintf(&b, "trace: %s\n", s.TraceHint)
	}
	if len(r.Runs) > 1 || len(r.Deltas) > 0 {
		fmt.Fprintf(&b, "\ncross-run agent durations (n=%d): %s\n", r.CrossRun.N, renderQuantiles(r.CrossRun))
	}
	if len(r.Alerts) > 0 {
		b.WriteString("\nalerts:\n")
		for _, a := range r.Alerts {
			fmt.Fprintf(&b, "  [%s] %s run=%s\n    sample: %s\n    threshold: %s\n    evidence: %s\n",
				a.Kind, a.Subject, a.RunID, a.Sample, a.Threshold, a.Evidence)
		}
	} else {
		b.WriteString("\nalerts: none\n")
	}
	for _, d := range r.Deltas {
		fmt.Fprintf(&b, "\nbaseline %s vs %s: %s (wall %+dms %+.0f%%, aggregate %+dms)\n",
			d.BaselineID, d.RunID, d.Verdict, d.WallDeltaMS, d.WallDeltaPct, d.AggDeltaMS)
		for _, stage := range diagStageOrder {
			fmt.Fprintf(&b, "  stage %-10s %+.0fms\n", stage, float64(d.StageDeltas[stage]))
		}
	}
	if len(r.Notes) > 0 {
		b.WriteString("\nnotes:\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	return b.String()
}

// renderQuantiles prints one quantile set with sufficiency: claimed
// numbers carry n, unclaimed ones state the minimum instead.
func renderQuantiles(q DiagQuantiles) string {
	med, p90, p95 := "n/a", "n/a", "n/a"
	if q.MedianOK {
		med = fmt.Sprintf("%dms", q.MedianMS)
	} else {
		med = fmt.Sprintf("insufficient sample (n=%d, need %d)", q.N, DiagMinMedianN)
	}
	if q.P90OK {
		p90 = fmt.Sprintf("%dms", q.P90MS)
	} else {
		p90 = fmt.Sprintf("insufficient sample (n=%d, need %d)", q.N, DiagMinP90N)
	}
	if q.P95OK {
		p95 = fmt.Sprintf("%dms", q.P95MS)
	} else {
		p95 = fmt.Sprintf("insufficient sample (n=%d, need %d)", q.N, DiagMinP95N)
	}
	return fmt.Sprintf("median %s, p90 %s, p95 %s", med, p90, p95)
}
