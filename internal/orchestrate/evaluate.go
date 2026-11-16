// Routing evaluation (F12.1/NOV-96): empirical contrast between
// dynamic routing and fixed routing over observed executions.
//
// Inputs reuse the F11.2 Lab result shape (bench.LabResult: agent,
// verdict, durations, tokens with provenance): quality is the pass
// rate over evaluable results, latency the median via
// bench.SummarizeLatencies (F11.4 minima apply: below n=5 the report
// states the minimum instead of claiming), cost the token means plus
// money via trace.CostOfUsage against explicit tariffs (F11.5).
//
// The verdict may conclude that dynamic routing adds nothing:
// KEEP_FIXED is the conservative default on ties and on insufficient
// evidence — switching routers needs a measured win, not a story.
package orchestrate

import (
	"fmt"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/trace"
)

// EvalVersion pins the evaluation document shape.
const EvalVersion = "routing-eval-v1"

// EvalThresholds carries the comparison rules, echoed verbatim on
// every report (never auto-tuned): the minimum quality delta that
// counts as a win, and the maximum latency/cost regressions tolerated
// alongside a quality win.
type EvalThresholds struct {
	MinQualityDelta float64 `json:"min_quality_delta"`
	MaxLatencyRegr  float64 `json:"max_latency_regression"`
	MaxCostRegr     float64 `json:"max_cost_regression"`
}

// DefaultEvalThresholds is the documented rule set: dynamic wins
// only with +5pp quality and no worse than +10% latency / +10% cost.
// Anything else keeps the fixed router.
func DefaultEvalThresholds() EvalThresholds {
	return EvalThresholds{
		MinQualityDelta: 0.05,
		MaxLatencyRegr:  0.10,
		MaxCostRegr:     0.10,
	}
}

// EvalArm is one observed arm: quality over evaluable results,
// median latency (F11.4), mean tokens and mean known money.
type EvalArm struct {
	Name         string  `json:"name"`
	Results      int     `json:"results"`
	Evaluable    int     `json:"evaluable"`
	Pass         int     `json:"pass"`
	Quality      float64 `json:"quality"`
	QualityOK    bool    `json:"quality_ok"`
	MedianMS     int64   `json:"median_ms"`
	LatencyOK    bool    `json:"latency_ok"`
	LatencyNote  string  `json:"latency_note,omitempty"`
	MeanTokens   int64   `json:"mean_tokens"`
	MeanCost     float64 `json:"mean_cost"`
	CostKnown    bool    `json:"cost_known"`
	CostCurrency string  `json:"cost_currency,omitempty"`
	CostNote     string  `json:"cost_note,omitempty"`
}

// EvalReport contrasts the dynamic arm against the fixed arm.
type EvalReport struct {
	Version    string         `json:"version"`
	Thresholds EvalThresholds `json:"thresholds"`
	Dynamic    EvalArm        `json:"dynamic"`
	Fixed      EvalArm        `json:"fixed"`
	QualityD   float64        `json:"quality_delta"`
	LatencyD   float64        `json:"latency_delta"`
	CostD      float64        `json:"cost_delta"`
	CostCmpOK  bool           `json:"cost_comparable"`
	Verdict    string         `json:"verdict"`
	Reason     string         `json:"reason"`
}

// Eval verdicts: the closed vocabulary.
const (
	VerdictDynamic = "DYNAMIC_ADDS_VALUE"
	VerdictFixed   = "KEEP_FIXED"
)

// summarizeArm folds one arm of Lab results. Quality counts pass
// over evaluable (pass/fail via bench.LabRank parity: pass and fail
// are the evaluable verdicts); latency reuses the F11.4 summarizer;
// money prices measured-host usage against the tariffs (F11.5) and
// stays unknown when nothing prices.
func summarizeArm(name string, results []bench.LabResult, rates []trace.RateCard) EvalArm {
	a := EvalArm{Name: name, Results: len(results)}
	var lats []int64
	var tokSum int64
	var costSum float64
	var costN int
	for _, r := range results {
		switch r.Verdict {
		case bench.LabPass:
			a.Evaluable++
			a.Pass++
		case bench.LabFail:
			a.Evaluable++
		}
		if r.DurationMS >= 0 {
			lats = append(lats, r.DurationMS)
		}
		tokSum += r.TokensIn + r.TokensOut
		if m := priceResult(r, rates); m.Known {
			costSum += m.Total
			costN++
			a.CostCurrency = m.Currency
		}
	}
	if a.Evaluable > 0 {
		a.Quality = float64(a.Pass) / float64(a.Evaluable)
		a.QualityOK = true
	}
	if len(results) > 0 {
		a.MeanTokens = tokSum / int64(len(results))
	}
	q := bench.SummarizeLatencies(lats)
	a.LatencyOK = q.MedianOK
	if q.MedianOK {
		a.MedianMS = q.MedianMS
	} else if len(lats) > 0 {
		var sum int64
		for _, v := range lats {
			sum += v
		}
		a.MedianMS = sum / int64(len(lats))
		a.LatencyNote = fmt.Sprintf("mean over n=%d (median needs %d); no latency claim", len(lats), bench.DiagMinMedianN)
	} else {
		a.LatencyNote = "no latency samples"
	}
	if costN > 0 {
		a.MeanCost = costSum / float64(costN)
		a.CostKnown = true
	} else {
		a.CostNote = "no priced samples (no tariff match or unmeasured usage); cost not comparable"
	}
	return a
}

// priceResult prices one Lab result against the tariffs: measured
// usage with an explicit model prices (F11.5); anything else stays
// unknown, never zero.
func priceResult(r bench.LabResult, rates []trace.RateCard) trace.MoneyCost {
	if len(rates) == 0 {
		return trace.MoneyCost{Known: false, Reason: "no tariffs"}
	}
	u := trace.HostUsage{
		InputTokens:  r.TokensIn,
		OutputTokens: r.TokensOut,
		Model:        r.Outcome.Model,
		Host:         r.Outcome.Host,
		HostVersion:  r.Outcome.HostVersion,
		Source:       r.TokenSource,
	}
	for _, rate := range rates {
		m, err := trace.CostOfUsage(u, rate)
		if err != nil {
			continue
		}
		if m.Known {
			return m
		}
	}
	return trace.MoneyCost{Known: false, Reason: "no tariff match or unmeasured usage"}
}

// Evaluate contrasts dynamic routing against fixed routing. Dynamic
// wins only when quality clears the minimum delta without busting
// the latency/cost regression ceilings; every other outcome —
// regression, tie, incomparable arms, insufficient evidence — keeps
// the fixed router with a naming reason.
func Evaluate(dynamic, fixed []bench.LabResult, rates []trace.RateCard, th EvalThresholds) EvalReport {
	rep := EvalReport{
		Version: EvalVersion, Thresholds: th,
		Dynamic: summarizeArm("dynamic", dynamic, rates),
		Fixed:   summarizeArm("fixed", fixed, rates),
	}
	rep.QualityD = rep.Dynamic.Quality - rep.Fixed.Quality
	if rep.Dynamic.Evaluable == 0 || rep.Fixed.Evaluable == 0 {
		rep.Verdict = VerdictFixed
		rep.Reason = fmt.Sprintf("evidencia insuficiente (evaluable dynamic=%d fixed=%d): sin tasa comparable no se cambia de router",
			rep.Dynamic.Evaluable, rep.Fixed.Evaluable)
		return rep
	}
	if rep.Dynamic.LatencyOK && rep.Fixed.LatencyOK && rep.Fixed.MedianMS > 0 {
		rep.LatencyD = float64(rep.Dynamic.MedianMS-rep.Fixed.MedianMS) / float64(rep.Fixed.MedianMS)
	}
	rep.CostCmpOK = rep.Dynamic.CostKnown && rep.Fixed.CostKnown
	if rep.CostCmpOK && rep.Fixed.MeanCost > 0 {
		rep.CostD = (rep.Dynamic.MeanCost - rep.Fixed.MeanCost) / rep.Fixed.MeanCost
	}
	switch {
	case rep.QualityD < th.MinQualityDelta:
		rep.Verdict = VerdictFixed
		rep.Reason = fmt.Sprintf("calidad %+0.3f bajo el mínimo +%.2f: el routing dinámico no aporta", rep.QualityD, th.MinQualityDelta)
	case rep.Dynamic.LatencyOK && rep.Fixed.LatencyOK && rep.LatencyD > th.MaxLatencyRegr:
		rep.Verdict = VerdictFixed
		rep.Reason = fmt.Sprintf("calidad %+0.3f pero latencia %+.1f%% sobre el techo +%.0f%%: no compensa", rep.QualityD, rep.LatencyD*100, th.MaxLatencyRegr*100)
	case rep.CostCmpOK && rep.CostD > th.MaxCostRegr:
		rep.Verdict = VerdictFixed
		rep.Reason = fmt.Sprintf("calidad %+0.3f pero coste %+.1f%% sobre el techo +%.0f%%: no compensa", rep.QualityD, rep.CostD*100, th.MaxCostRegr*100)
	default:
		rep.Verdict = VerdictDynamic
		rep.Reason = fmt.Sprintf("calidad %+0.3f con latencia %+.1f%% y coste %s: la dinámica aporta",
			rep.QualityD, rep.LatencyD*100, costWord(rep))
	}
	return rep
}

// costWord renders the cost delta honestly: comparable figures cite
// the delta, unknown money says so instead of faking a zero.
func costWord(rep EvalReport) string {
	if !rep.CostCmpOK {
		return "coste no comparable (sin tarifas o sin medición)"
	}
	return fmt.Sprintf("%+.1f%%", rep.CostD*100)
}

// RenderEvalReport renders the evaluation as human text: both arms,
// the deltas against the echoed thresholds, and the verdict.
func RenderEvalReport(rep EvalReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "routing-eval %s: dynamic vs fixed\n", rep.Version)
	fmt.Fprintf(&b, "reglas: calidad mínima %+.2f, latencia máx %+.0f%%, coste máx %+.0f%%\n",
		rep.Thresholds.MinQualityDelta, rep.Thresholds.MaxLatencyRegr*100, rep.Thresholds.MaxCostRegr*100)
	for _, a := range []EvalArm{rep.Dynamic, rep.Fixed} {
		lat := "sin mediana"
		if a.LatencyOK {
			lat = fmt.Sprintf("mediana %dms", a.MedianMS)
		} else if a.LatencyNote != "" {
			lat = a.LatencyNote
		}
		cost := "coste desconocido"
		if a.CostKnown {
			cost = fmt.Sprintf("coste medio %.4f %s", a.MeanCost, a.CostCurrency)
		}
		fmt.Fprintf(&b, "- %s: n=%d evaluable=%d pass=%d calidad=%.3f %s tokens-medios=%d %s\n",
			a.Name, a.Results, a.Evaluable, a.Pass, a.Quality, lat, a.MeanTokens, cost)
	}
	fmt.Fprintf(&b, "deltas: calidad %+.3f latencia %+.1f%% coste %s\n", rep.QualityD, rep.LatencyD*100, costWord(rep))
	fmt.Fprintf(&b, "veredicto: %s — %s\n", rep.Verdict, rep.Reason)
	return b.String()
}

// RenderDecision renders one routing decision as human text: the
// weights it ran with, the ranking with per-factor values, the
// exclusions, the mode, fallback/no-delegation and the fixed
// contrast. Deterministic: same decision, same bytes (the trace hint
// is recording metadata, printed last).
func RenderDecision(dec Decision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "routing %s: %s/%s complejidad %s riesgo %s\n",
		dec.Version, dec.Task.Domain, dec.Task.Language, dec.Task.Complexity, dec.Task.Risk)
	if len(dec.Task.RequiredCapabilities) > 0 {
		fmt.Fprintf(&b, "capabilities exigidas: %s\n", strings.Join(dec.Task.RequiredCapabilities, ", "))
	}
	fmt.Fprintf(&b, "pesos: capability=%.2f risk=%.2f quality=%.2f latency=%.2f cost=%.2f\n",
		dec.Weights.Capability, dec.Weights.Risk, dec.Weights.Quality, dec.Weights.Latency, dec.Weights.Cost)
	fmt.Fprintf(&b, "budgets: tiempo=%dms tokens=%d coste=%v delegación máx=%d\n",
		dec.Budgets.TimeMS, dec.Budgets.Tokens, dec.Budgets.Cost, dec.MaxDelegations)
	for _, r := range dec.Ranking {
		fmt.Fprintf(&b, "- %s (host %s): %.3f", r.AgentID, r.Host, r.Total)
		parts := make([]string, 0, len(r.Factors))
		for _, f := range r.Factors {
			parts = append(parts, fmt.Sprintf("%s=%.2f", f.Name, f.Value))
		}
		fmt.Fprintf(&b, " [%s]\n", strings.Join(parts, " "))
		for _, f := range r.Factors {
			fmt.Fprintf(&b, "    %s: %s\n", f.Name, f.Why)
		}
	}
	for _, e := range dec.Excluded {
		fmt.Fprintf(&b, "  x %s: %s\n", e.AgentID, e.Reason)
	}
	if dec.Primary == "" {
		fmt.Fprintf(&b, "decisión: SIN AGENTE (rehúsa; %d exclusiones)\n", len(dec.Excluded))
	} else {
		fmt.Fprintf(&b, "decisión: modo %s primario %s", dec.Mode, dec.Primary)
		if dec.Reviewer != "" {
			fmt.Fprintf(&b, " revisor %s", dec.Reviewer)
		}
		fmt.Fprintf(&b, "\n")
	}
	for _, j := range dec.Justification {
		fmt.Fprintf(&b, "porque: %s\n", j)
	}
	if dec.Fallback != "" {
		fmt.Fprintf(&b, "fallback: %s scopes=%v\n", dec.Fallback, dec.FallbackScopes)
	}
	fmt.Fprintf(&b, "fallback: %s\n", dec.FallbackReason)
	if dec.NoDelegationReason != "" {
		fmt.Fprintf(&b, "no-delegación: %s\n", dec.NoDelegationReason)
	}
	fmt.Fprintf(&b, "fijo: %s\n", dec.FixedContrast)
	if dec.DryRun {
		fmt.Fprintf(&b, "dry-run: decisión no registrada\n")
	} else if dec.TraceHint != "" {
		fmt.Fprintf(&b, "traza: %s\n", dec.TraceHint)
	}
	return b.String()
}
