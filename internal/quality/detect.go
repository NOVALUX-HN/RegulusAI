// Change detection over one segment (F12.4/NOV-99).
//
// Check compares a versioned baseline against a matched window using
// N-adequate methods: rates and means need MinN, median latency
// needs MinNMedian, p90 MinNP90, p95 MinNP95. Below the floor the
// diagnosis describes and claims nothing. Every alert carries the
// baseline, the window, N, the magnitude and its limitations; worst
// cases name run ids with evidence links. The diagnosis closes with
// investigation proposals — never production actions.
package quality

import (
	"fmt"
	"sort"
	"time"
)

// Finding is one metric comparison: baseline vs observed with the
// method that produced it and the threshold it crossed (or not).
type Finding struct {
	Metric    string  `json:"metric"`
	Baseline  float64 `json:"baseline"`
	Observed  float64 `json:"observed"`
	DeltaAbs  float64 `json:"delta_abs"`
	DeltaPct  float64 `json:"delta_pct"`
	Method    string  `json:"method"`
	Threshold string  `json:"threshold"`
	Alerted   bool    `json:"alerted"`
}

// Alert is one fired rule: what moved, how much, in which cases,
// with the limits that bound the claim.
type Alert struct {
	Metric     string   `json:"metric"`
	Magnitude  string   `json:"magnitude"`
	Cases      []string `json:"cases"`
	Evidence   []string `json:"evidence"`
	Limitation string   `json:"limitation"`
	NextStep   string   `json:"next_step"`
}

// Diagnosis is the full check report: version, the segment and
// evidence it ran on, matched/excluded N, findings, alerts,
// limitations, investigation proposals and the reproduction command.
type Diagnosis struct {
	Version     string     `json:"version"`
	Segment     Segment    `json:"segment"`
	SegmentKey  string     `json:"segment_key"`
	BaselineN   int        `json:"baseline_n"`
	WindowN     int        `json:"window_n"`
	ExcludedN   int        `json:"excluded_n"`
	From        time.Time  `json:"from,omitempty"`
	To          time.Time  `json:"to,omitempty"`
	Stats       Stats      `json:"stats"`
	Thresholds  Thresholds `json:"thresholds"`
	Findings    []Finding  `json:"findings"`
	Alerts      []Alert    `json:"alerts"`
	Limitations []string   `json:"limitations"`
	NextSteps   []string   `json:"next_steps"`
	Reproduce   string     `json:"reproduce"`
	Breakdown   []string   `json:"breakdown,omitempty"`
}

// Check compares baseline against obs. Observations outside the
// baseline segment are excluded (counted, never folded); a mixed
// window adds a confound limitation and a host/model breakdown. With
// fewer than MinN matched observations the diagnosis describes only.
func Check(b Baseline, obs []Observation, baselineRef, windowRef string) Diagnosis {
	matched, excluded := splitWindow(b.Segment, obs)
	from, to, known := windowSpan(matched)
	d := Diagnosis{
		Version: b.Version, Segment: b.Segment,
		SegmentKey: b.Segment.Key(), BaselineN: b.N,
		WindowN: len(matched), ExcludedN: len(excluded),
		Stats: summarize(matched), Thresholds: b.Thresholds,
		Reproduce: reproduceHint(baselineRef, windowRef),
	}
	if known {
		d.From, d.To = from.UTC(), to.UTC()
	}
	if len(excluded) > 0 {
		d.Limitations = append(d.Limitations,
			fmt.Sprintf("mixed window: %d of %d observations fall outside %s and were excluded; compared within segment only",
				len(excluded), len(obs), b.Segment.Key()))
		d.Breakdown = append(d.Breakdown,
			"window host mix: "+joinOrNone(breakdown(obs, func(o Observation) string { return o.Host })),
			"window model mix: "+joinOrNone(breakdown(obs, func(o Observation) string { return o.Model })),
		)
		noteNaiveConfound(&d, b, obs, matched)
	}
	if len(matched) == 0 {
		d.Limitations = append(d.Limitations,
			fmt.Sprintf("no observations match %s: nothing to compare", b.Segment.Key()))
		d.NextSteps = investigationSteps(nil)
		return d
	}
	if len(matched) < MinN {
		d.Limitations = append(d.Limitations,
			fmt.Sprintf("insufficient sample (n=%d, need >=%d): describes the window only, claims no regression",
				len(matched), MinN))
		d.NextSteps = investigationSteps(nil)
		return d
	}
	d.Findings, d.Alerts = compareAll(b, matched)
	d.Limitations = append(d.Limitations, coverageAndTokensLimits(b, d.Stats)...)
	d.NextSteps = investigationSteps(d.Alerts)
	return d
}

// noteNaiveConfound flags when the unsegmented read would claim the
// opposite of the segmented one: segmentation controlled a confound.
func noteNaiveConfound(d *Diagnosis, b Baseline, all, matched []Observation) {
	naive := summarize(all).SuccessRate
	seg := d.Stats.SuccessRate
	naiveDrop := b.Stats.SuccessRate - naive
	segDrop := b.Stats.SuccessRate - seg
	naiveFires := naiveDrop >= b.Thresholds.SuccessDropAbs
	segFires := segDrop >= b.Thresholds.SuccessDropAbs
	if naiveFires != segFires {
		d.Limitations = append(d.Limitations,
			fmt.Sprintf("segmentation reversed the naive read (unsegmented success Δ=%+.2f vs segmented Δ=%+.2f): the excluded segment confounded the aggregate",
				-naiveDrop, -segDrop))
	}
}

// coverageAndTokensLimits reports honest unknowns: unmeasured
// dimensions skip their rules instead of reading as zero.
func coverageAndTokensLimits(b Baseline, s Stats) []string {
	var out []string
	if !s.TokensKnown || !b.Stats.TokensKnown {
		out = append(out, "tokens unmeasured on one side (unknown, never zero): the cost rule did not run")
	}
	if !s.CoverAvgOK || !b.Stats.CoverAvgOK {
		out = append(out, "coverage unknown on one side: the compatibility rule did not run")
	}
	return out
}

// compareAll runs every metric rule adequate to the window N.
func compareAll(b Baseline, matched []Observation) ([]Finding, []Alert) {
	s := summarize(matched)
	th := b.Thresholds
	var findings []Finding
	var alerts []Alert
	add := func(f Finding, cases, evidence []string, next string) {
		findings = append(findings, f)
		if f.Alerted {
			mag := fmt.Sprintf("%s: %.3f -> %.3f (Δ=%+.3f, %+.1f%%)", f.Metric, f.Baseline, f.Observed, f.DeltaAbs, f.DeltaPct*100)
			alerts = append(alerts, Alert{
				Metric: f.Metric, Magnitude: mag,
				Cases: cases, Evidence: evidence,
				Limitation: "within-segment comparison, n=" + itoa(len(matched)) + "; see diagnosis limitations",
				NextStep:   next,
			})
		}
	}
	add(successFinding(b.Stats, s, th), failedRuns(matched, 5), runEvidence(matched, false, 5),
		"inspect the failed runs above, then bisect the change between the baseline head and the window runs")
	add(errorFinding(b.Stats, s, th), erroredRuns(matched, 5), runEvidence(matched, false, 5),
		"group the error cases by message/host before suspecting the code")
	add(retryFinding(b.Stats, s, th), retriedRuns(matched, 5), runEvidence(matched, false, 5),
		"check whether retries cluster on one host or task before tuning any budget")
	for _, f := range latencyFindings(b.Stats, s, th, len(matched)) {
		add(f, slowestRuns(matched, 5), runEvidence(matched, true, 5),
			"profile the slowest cases via their trace timelines; no budget changes until the cause is named")
	}
	if f, ok := tokensFinding(b.Stats, s, th, len(matched)); ok {
		add(f, costliestRuns(matched, 5), runEvidence(matched, true, 5),
			"confirm the token rise on measured-host counts; proxy figures never justify a cost verdict")
	}
	if f, ok := coverageFinding(b.Stats, s, th, len(matched)); ok {
		add(f, uncoveredRuns(matched, 5), runEvidence(matched, false, 5),
			"re-run the uncovered cases against the baseline evidence before calling it a compatibility loss")
	}
	return findings, alerts
}

// successFinding fires when the success rate falls by the absolute drop.
func successFinding(base, cur Stats, th Thresholds) Finding {
	f := Finding{Metric: "success", Baseline: base.SuccessRate, Observed: cur.SuccessRate,
		Method:    fmt.Sprintf("success rate (n=%d, min %d)", cur.N, MinN),
		Threshold: fmt.Sprintf("drop >= %.2f absolute", th.SuccessDropAbs)}
	f.DeltaAbs = cur.SuccessRate - base.SuccessRate
	if base.SuccessRate != 0 {
		f.DeltaPct = f.DeltaAbs / base.SuccessRate
	}
	f.Alerted = -f.DeltaAbs >= th.SuccessDropAbs
	return f
}

// errorFinding fires when the error-bearing share rises.
func errorFinding(base, cur Stats, th Thresholds) Finding {
	f := Finding{Metric: "errors", Baseline: base.ErrorRate, Observed: cur.ErrorRate,
		Method:    fmt.Sprintf("error rate (n=%d, min %d)", cur.N, MinN),
		Threshold: fmt.Sprintf("rise >= %.2f absolute", th.ErrorRiseAbs)}
	f.DeltaAbs = cur.ErrorRate - base.ErrorRate
	if base.ErrorRate != 0 {
		f.DeltaPct = f.DeltaAbs / base.ErrorRate
	}
	f.Alerted = f.DeltaAbs >= th.ErrorRiseAbs
	return f
}

// retryFinding fires when mean retries rise by the absolute budget.
func retryFinding(base, cur Stats, th Thresholds) Finding {
	f := Finding{Metric: "retries", Baseline: base.MeanRetries, Observed: cur.MeanRetries,
		Method:    fmt.Sprintf("mean retries (n=%d, min %d)", cur.N, MinN),
		Threshold: fmt.Sprintf("rise >= %.2f mean retries", th.RetryRiseAbs)}
	f.DeltaAbs = cur.MeanRetries - base.MeanRetries
	if base.MeanRetries != 0 {
		f.DeltaPct = f.DeltaAbs / base.MeanRetries
	}
	f.Alerted = f.DeltaAbs >= th.RetryRiseAbs
	return f
}

// latencyFinding builds one latency comparison; ok=false skips the
// rule (unknown side or zero baseline with no relative meaning).
func latencyFinding(metric string, baseMS int64, baseOK bool, curMS int64, curOK bool, th Thresholds, method string) (Finding, bool) {
	f := Finding{Metric: metric, Baseline: float64(baseMS), Observed: float64(curMS),
		Method: method, Threshold: fmt.Sprintf("rise >= %.0f%%", th.LatencyRisePct*100)}
	if !baseOK || !curOK || baseMS <= 0 {
		return f, false
	}
	f.DeltaAbs = float64(curMS - baseMS)
	f.DeltaPct = float64(curMS-baseMS) / float64(baseMS)
	f.Alerted = f.DeltaPct >= th.LatencyRisePct
	return f, true
}

// latencyFindings runs p50/p90/p95 each gated by its N minimum.
func latencyFindings(base, cur Stats, th Thresholds, n int) []Finding {
	var out []Finding
	if f, ok := latencyFinding("latency-p50", base.MedianMS, base.MedianValid, cur.MedianMS, cur.MedianValid, th,
		fmt.Sprintf("median (n=%d, min %d)", n, MinNMedian)); ok {
		out = append(out, f)
	}
	if n >= MinNP90 {
		if f, ok := latencyFinding("latency-p90", base.P90MS, base.P90Valid, cur.P90MS, cur.P90Valid, th,
			fmt.Sprintf("p90 (n=%d, min %d)", n, MinNP90)); ok {
			out = append(out, f)
		}
	}
	if n >= MinNP95 {
		if f, ok := latencyFinding("latency-p95", base.P95MS, base.P95Valid, cur.P95MS, cur.P95Valid, th,
			fmt.Sprintf("p95 (n=%d, min %d)", n, MinNP95)); ok {
			out = append(out, f)
		}
	}
	return out
}

// tokensFinding fires on measured-token rise; unknown sides skip.
func tokensFinding(base, cur Stats, th Thresholds, n int) (Finding, bool) {
	f := Finding{Metric: "tokens", Baseline: base.MeanTokens, Observed: cur.MeanTokens,
		Method:    fmt.Sprintf("mean measured tokens (n=%d, min %d)", n, MinN),
		Threshold: fmt.Sprintf("rise >= %.0f%%", th.TokensRisePct*100)}
	if !base.TokensKnown || !cur.TokensKnown || base.MeanTokens <= 0 {
		return f, false
	}
	f.DeltaAbs = cur.MeanTokens - base.MeanTokens
	f.DeltaPct = f.DeltaAbs / base.MeanTokens
	f.Alerted = f.DeltaPct >= th.TokensRisePct
	return f, true
}

// coverageFinding fires on coverage fall; unknown sides skip.
func coverageFinding(base, cur Stats, th Thresholds, n int) (Finding, bool) {
	f := Finding{Metric: "coverage", Baseline: base.CoverageAvg, Observed: cur.CoverageAvg,
		Method:    fmt.Sprintf("mean coverage (n=%d, min %d)", n, MinN),
		Threshold: fmt.Sprintf("drop >= %.2f points", th.CoverageDrop)}
	if !base.CoverAvgOK || !cur.CoverAvgOK {
		return f, false
	}
	f.DeltaAbs = cur.CoverageAvg - base.CoverageAvg
	if base.CoverageAvg != 0 {
		f.DeltaPct = f.DeltaAbs / base.CoverageAvg
	}
	f.Alerted = -f.DeltaAbs >= th.CoverageDrop
	return f, true
}

// reproduceHint renders the exact re-check command from evidence refs.
func reproduceHint(baselineRef, windowRef string) string {
	cmd := "regulus quality check --baseline <baseline-dir>"
	if baselineRef != "" {
		cmd = "regulus quality check --baseline " + baselineRef
	}
	if windowRef != "" {
		cmd += " --window " + windowRef
	}
	return cmd
}

// investigationSteps proposes follow-ups only: reproduce, isolate,
// confirm — never a production change. The module has no action
// path, and these steps say so.
func investigationSteps(alerts []Alert) []string {
	steps := []string{
		"reproduce from versioned evidence with the reproduce command above (same baseline sha, same window sha)",
		"isolate: confirm the finding holds within the segment before suspecting shared code",
		"confirm on a larger window (n>=10 for p90, n>=20 for p95) before concluding",
		"this module proposes investigation only: it never modifies production, rolls back, or retunes anything",
	}
	for _, a := range alerts {
		steps = append(steps, a.NextStep)
	}
	return steps
}

// Case selectors: worst runs first, deterministic order, capped.

func failedRuns(obs []Observation, k int) []string {
	var out []string
	for _, o := range obs {
		if !o.Success {
			out = append(out, o.RunID)
		}
	}
	return cappedSorted(out, k)
}

func erroredRuns(obs []Observation, k int) []string {
	type pair struct {
		id string
		n  int
	}
	var ps []pair
	for _, o := range obs {
		if o.Errors > 0 {
			ps = append(ps, pair{o.RunID, o.Errors})
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].n != ps[j].n {
			return ps[i].n > ps[j].n
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.id)
	}
	return capped(out, k)
}

func retriedRuns(obs []Observation, k int) []string {
	type pair struct {
		id string
		n  int
	}
	var ps []pair
	for _, o := range obs {
		if o.Retries > 0 {
			ps = append(ps, pair{o.RunID, o.Retries})
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].n != ps[j].n {
			return ps[i].n > ps[j].n
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.id)
	}
	return capped(out, k)
}

func slowestRuns(obs []Observation, k int) []string {
	type pair struct {
		id string
		ms int64
	}
	var ps []pair
	for _, o := range obs {
		if o.LatencyKnown {
			ps = append(ps, pair{o.RunID, o.LatencyMS})
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].ms != ps[j].ms {
			return ps[i].ms > ps[j].ms
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.id)
	}
	return capped(out, k)
}

func costliestRuns(obs []Observation, k int) []string {
	type pair struct {
		id string
		t  int64
	}
	var ps []pair
	for _, o := range obs {
		if t, ok := o.tokensTotal(); ok {
			ps = append(ps, pair{o.RunID, t})
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].t != ps[j].t {
			return ps[i].t > ps[j].t
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.id)
	}
	return capped(out, k)
}

func uncoveredRuns(obs []Observation, k int) []string {
	type pair struct {
		id string
		c  float64
	}
	var ps []pair
	for _, o := range obs {
		if o.CoverageKnown {
			ps = append(ps, pair{o.RunID, o.CoveragePct})
		}
	}
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].c != ps[j].c {
			return ps[i].c < ps[j].c
		}
		return ps[i].id < ps[j].id
	})
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.id)
	}
	return capped(out, k)
}

// runEvidence links cases back to evidence: matching run ids in
// deterministic order, capped like the cases.
func runEvidence(obs []Observation, _ bool, k int) []string {
	byID := map[string]string{}
	for _, o := range obs {
		if _, ok := byID[o.RunID]; !ok {
			byID[o.RunID] = o.Evidence
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if ev := byID[id]; ev != "" {
			out = append(out, id+": "+ev)
		} else {
			out = append(out, id)
		}
	}
	return capped(out, k)
}

func capped(in []string, k int) []string {
	if len(in) > k {
		return in[:k]
	}
	return in
}

func cappedSorted(in []string, k int) []string {
	sort.Strings(in)
	return capped(in, k)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func joinOrNone(in []string) string {
	if len(in) == 0 {
		return "(none)"
	}
	out := ""
	for i, s := range in {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
