package bench

import (
	"strings"
	"testing"
)

// Covers RenderDiagReport branches: empty, single run with/without
// shares, agents/tools present/absent, alerts none/some, deltas, notes
// and quantile sufficiency. Pure formatting, no FS/shell.
func TestRenderDiagReportCoverage(t *testing.T) {
	// Empty report: no runs, no alerts.
	empty := DiagReport{Version: "v1", Thresholds: DefaultDiagThresholds()}
	out := RenderDiagReport(empty)
	for _, want := range []string{"perf-diagnosis v1", "alerts: none"} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty report must contain %q, got:\n%s", want, out)
		}
	}
	// Single run: AggregateMS 0 skips shares, Unattributed 0 skips line.
	single := DiagReport{
		Version:    "v1",
		Thresholds: DefaultDiagThresholds(),
		Runs: []DiagRunSummary{
			{
				RunID:         "r1",
				WallMS:        100,
				WallSource:    "trace",
				AggregateMS:   0,
				Stages:        TierPhases{PrepMS: 10},
				DominantStage: DiagDominant{Name: "prep", MS: 10, Evidence: "ev"},
				DominantAgent: DiagDominant{Name: "a1", MS: 5, Evidence: "ev2"},
				Counts:        DiagCounts{},
				Latencies:     DiagQuantiles{N: 2},
				TraceHint:     "hint",
			},
		},
	}
	out = RenderDiagReport(single)
	for _, want := range []string{"== run r1 ==", "dominant stage: prep", "insufficient sample"} {
		if !strings.Contains(out, want) {
			t.Fatalf("single run must contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "of aggregate") {
		t.Fatalf("AggregateMS 0 must skip share, got:\n%s", out)
	}
	// Full run: shares, unattributed, agents, tools, alerts, deltas, notes.
	full := DiagReport{
		Version:    "v2",
		Thresholds: DefaultDiagThresholds(),
		Runs: []DiagRunSummary{
			{
				RunID:          "r1",
				WallMS:         200,
				WallSource:     "trace",
				AggregateMS:    100,
				Stages:         TierPhases{PrepMS: 50, ToolsMS: 50},
				UnattributedMS: 20,
				DominantStage:  DiagDominant{Name: "prep", MS: 50, Evidence: "ev"},
				DominantAgent:  DiagDominant{Name: "a1", MS: 60, Evidence: "ev2"},
				Agents: []DiagAgentStat{
					{AgentID: "a1", TaskID: "T1", DurationMS: 60, Verdict: "pass", Source: "lab"},
				},
				Tools: []DiagToolStat{
					{Tool: "grep", Calls: 2, TotalMS: 30, MaxMS: 20, MeanMS: 15},
				},
				Counts:    DiagCounts{Retries: 1},
				Latencies: DiagQuantiles{N: 10, MedianMS: 5, MedianOK: true, P90MS: 9, P90OK: true, P95MS: 10, P95OK: true},
				TraceHint: "hint",
			},
			{
				RunID:         "r2",
				WallMS:        210,
				WallSource:    "trace",
				AggregateMS:   110,
				Stages:        TierPhases{PrepMS: 60},
				DominantStage: DiagDominant{Name: "prep", MS: 60, Evidence: "ev"},
				DominantAgent: DiagDominant{Name: "a2", MS: 61, Evidence: "ev2"},
				Latencies:     DiagQuantiles{N: 10, MedianMS: 6, MedianOK: true, P90MS: 9, P90OK: true, P95MS: 10, P95OK: true},
				TraceHint:     "hint2",
			},
		},
		CrossRun: DiagQuantiles{N: 10, MedianMS: 6, MedianOK: true, P90MS: 9, P90OK: true, P95MS: 10, P95OK: true},
		Alerts: []DiagAlert{
			{RunID: "r1", Kind: "slow", Subject: "prep", Sample: "50ms", Threshold: "10ms", Evidence: "ev"},
		},
		Deltas: []DiagDelta{
			{RunID: "r2", BaselineID: "r1", Verdict: "slower", WallDeltaMS: 10, WallDeltaPct: 5, AggDeltaMS: 10, StageDeltas: map[string]int64{"prep": 10}},
		},
		Notes: []string{"note1"},
	}
	out = RenderDiagReport(full)
	for _, want := range []string{
		"of aggregate", "unattributed (wall remainder)",
		"agents:", "tools:", "counts: retries=1",
		"median 5ms", "cross-run agent durations",
		"[slow] prep", "baseline r1 vs r2", "notes:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("full report must contain %q, got:\n%s", want, out)
		}
	}
}
