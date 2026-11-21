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
