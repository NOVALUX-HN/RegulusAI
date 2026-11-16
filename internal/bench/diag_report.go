package bench

import (
	"fmt"
	"strings"
)

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
