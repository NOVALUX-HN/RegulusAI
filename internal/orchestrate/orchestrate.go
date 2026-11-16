// Adaptive Agent Orchestration (F12.1/NOV-96): explainable routing
// over the Lab agents, reusing every F11 surface instead of
// duplicating it.
//
// Reuse map (never parallel stores, never parallel shapes):
//   - agents: bench.LabRegistry / bench.FindLabAgent (F11.2 LabAgent:
//     responsibility, tools, host, inputs);
//   - capabilities: bench capability matrix (F11.7 hostcapabilities.go:
//     probada/no-soportada/no-ejecutada/desconocida) — only probada
//     (verified, with dated evidence) may ever be assigned;
//   - latency: bench.LabResult durations + bench.SummarizeLatencies
//     (F11.4 quantiles with sample minima);
//   - cost: bench.LabResult tokens + trace.RateCard/trace.CostOfUsage
//     (F11.5 money with explicit tariffs; unknown never reads as zero);
//   - delegation: mcp.Principal.Delegate (F11.8: delegating only
//     restricts, Host/User/Workspace pinned);
//   - trace: bench.TraceHintFor + observability exec store (F11.1).
//
// Nothing here executes an agent: Decide proposes (and the CLI
// records) a routing decision. Dry-run computes the same decision
// and records nothing.
package orchestrate

import (
	"fmt"
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/trace"
)

// RoutingVersion pins the routing document shape: decisions compare
// only within one version.
const RoutingVersion = "routing-v1"

// Risk and complexity closed vocabularies. Levels order low < medium
// < high < critical; an agent may take solo tasks up to its MaxRisk.
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

const (
	ComplexityLow    = "low"
	ComplexityMedium = "medium"
	ComplexityHigh   = "high"
)

// riskLevel orders the risk vocabulary for margin scoring.
func riskLevel(r string) int {
	switch r {
	case RiskLow:
		return 0
	case RiskMedium:
		return 1
	case RiskHigh:
		return 2
	case RiskCritical:
		return 3
	}
	return -1
}

// TaskDescriptor is the routing input (F12.1 spec): domain, language,
// complexity, risk, restrictions, outputs and criteria, plus the
// required capabilities the decision must cover with verified
// evidence.
type TaskDescriptor struct {
	Domain               string   `json:"domain"`
	Language             string   `json:"language"`
	Complexity           string   `json:"complexity"`
	Risk                 string   `json:"risk"`
	Constraints          []string `json:"constraints"`
	Outputs              []string `json:"outputs"`
	Criteria             []string `json:"criteria"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

// ValidateTaskDescriptor refuses empty domain/language, unknown
// complexity/risk words, unknown capability ids (outside the F11.7
// matrix vocabulary) and hostile free text (overlong, multiline or
// traversal-shaped), so a decision always cites a clean descriptor.
func ValidateTaskDescriptor(d TaskDescriptor) error {
	if strings.TrimSpace(d.Domain) == "" {
		return fmt.Errorf("orchestrate: descriptor needs a domain")
	}
	if strings.TrimSpace(d.Language) == "" {
		return fmt.Errorf("orchestrate: descriptor needs a language")
	}
	switch d.Complexity {
	case ComplexityLow, ComplexityMedium, ComplexityHigh:
	default:
		return fmt.Errorf("orchestrate: unknown complexity %q (low|medium|high)", d.Complexity)
	}
	switch d.Risk {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		return fmt.Errorf("orchestrate: unknown risk %q (low|medium|high|critical)", d.Risk)
	}
	for _, f := range []struct {
		name string
		v    string
	}{{"domain", d.Domain}, {"language", d.Language}} {
		if len(f.v) > 64 || strings.ContainsAny(f.v, "\n\r") || strings.Contains(f.v, "..") {
			return fmt.Errorf("orchestrate: %s must be a single short line, got %q", f.name, f.v)
		}
	}
	for _, lst := range []struct {
		name string
		vals []string
	}{
		{"constraints", d.Constraints}, {"outputs", d.Outputs},
		{"criteria", d.Criteria},
	} {
		if len(lst.vals) > 32 {
			return fmt.Errorf("orchestrate: %s lists at most 32 entries, got %d", lst.name, len(lst.vals))
		}
		for _, v := range lst.vals {
			if len(v) > 256 || strings.ContainsAny(v, "\n\r") {
				return fmt.Errorf("orchestrate: %s entry must be a single line under 256 chars, got %q", lst.name, v)
			}
		}
	}
	known := map[string]bool{}
	for _, id := range capabilityVocabulary() {
		known[id] = true
	}
	seen := map[string]bool{}
	for _, c := range d.RequiredCapabilities {
		if !known[c] {
			return fmt.Errorf("orchestrate: unknown capability %q (outside the F11.7 matrix vocabulary)", c)
		}
		if seen[c] {
			return fmt.Errorf("orchestrate: duplicate required capability %q", c)
		}
		seen[c] = true
	}
	return nil
}

// Budgets bound one routing decision: wall-clock time, proxy tokens
// and money. Zero means unset (that dimension is scored relatively
// and never gates); negative fails closed.
type Budgets struct {
	TimeMS int64   `json:"time_ms"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

// ValidateBudgets refuses negative ceilings.
func ValidateBudgets(b Budgets) error {
	if b.TimeMS < 0 {
		return fmt.Errorf("orchestrate: --budget-time-ms must not be negative, got %d", b.TimeMS)
	}
	if b.Tokens < 0 {
		return fmt.Errorf("orchestrate: --budget-tokens must not be negative, got %d", b.Tokens)
	}
	if b.Cost < 0 {
		return fmt.Errorf("orchestrate: --budget-cost must not be negative, got %v", b.Cost)
	}
	return nil
}

// MaxDelegations bounds the delegation chain: 0 forbids delegation
// (single agent or pair, no fallback delegation), N allows at most N
// delegation edges. Negative fails closed.
func ValidateMaxDelegations(n int) error {
	if n < 0 {
		return fmt.Errorf("orchestrate: --max-delegations must not be negative, got %d", n)
	}
	if n > 4 {
		return fmt.Errorf("orchestrate: --max-delegations at most 4, got %d", n)
	}
	return nil
}

// ScoreWeights carries the deterministic routing policy: every
// factor weight is fixed, echoed on every decision, and sums to 1.
// There is no auto-tuning: Decide takes them by value and reports
// them verbatim (same discipline as bench.DiagThresholds).
type ScoreWeights struct {
	Capability float64 `json:"capability"`
	Risk       float64 `json:"risk"`
	Quality    float64 `json:"quality"`
	Latency    float64 `json:"latency"`
	Cost       float64 `json:"cost"`
}

// DefaultScoreWeights is the documented policy: verified capability
// breadth first, risk fit second, then proven quality, latency and
// cost equally.
func DefaultScoreWeights() ScoreWeights {
	return ScoreWeights{
		Capability: 0.30,
		Risk:       0.25,
		Quality:    0.15,
		Latency:    0.15,
		Cost:       0.15,
	}
}

// validate checks the weights sum to 1 (within epsilon) and hold no
// negative share.
func (w ScoreWeights) validate() error {
	sum := w.Capability + w.Risk + w.Quality + w.Latency + w.Cost
	if sum < 0.999 || sum > 1.001 {
		return fmt.Errorf("orchestrate: weights must sum to 1, got %.4f", sum)
	}
	for name, v := range map[string]float64{
		"capability": w.Capability, "risk": w.Risk, "quality": w.Quality,
		"latency": w.Latency, "cost": w.Cost,
	} {
		if v < 0 {
			return fmt.Errorf("orchestrate: weight %s must not be negative, got %v", name, v)
		}
	}
	return nil
}

// HistorySample is one observed routed execution: pass/fail over an
// evaluable attempt, wall latency, proxy tokens and money cost.
// CostKnown distinguishes a priced zero (explicit free tariff) from
// an unpriced unknown (F11.5 honesty: unknown never reads as zero and
// never gates the cost budget).
type HistorySample struct {
	AgentID     string  `json:"agent_id"`
	Pass        bool    `json:"pass"`
	Evaluable   bool    `json:"evaluable"`
	LatencyMS   int64   `json:"latency_ms"`
	Tokens      int64   `json:"tokens"`
	Cost        float64 `json:"cost"`
	CostKnown   bool    `json:"cost_known"`
	Currency    string  `json:"currency,omitempty"`
	LatencyNote string  `json:"latency_note,omitempty"`
}

// AgentHistory folds one agent's samples: success rate over
// evaluable attempts only, median latency (claimed only at the F11.4
// minimum of 5, else the mean with a note), mean tokens and mean
// money over cost-known samples only.
type AgentHistory struct {
	AgentID         string  `json:"agent_id"`
	Samples         int     `json:"samples"`
	Evaluable       int     `json:"evaluable"`
	Pass            int     `json:"pass"`
	SuccessRate     float64 `json:"success_rate"`
	SuccessRateOK   bool    `json:"success_rate_ok"`
	MedianLatencyMS int64   `json:"median_latency_ms"`
	LatencyOK       bool    `json:"latency_ok"`
	LatencyNote     string  `json:"latency_note"`
	MeanTokens      int64   `json:"mean_tokens"`
	MeanCost        float64 `json:"mean_cost"`
	CostKnown       bool    `json:"cost_known"`
	Currency        string  `json:"currency,omitempty"`
}

// SummarizeHistory folds samples per agent in deterministic (sorted
// agent) order. Success rate needs at least one evaluable attempt;
// the median needs 5 latency points (the F11.4 DiagMinMedianN floor);
// money averages only cost-known samples and stays unknown when
// there are none.
func SummarizeHistory(samples []HistorySample) []AgentHistory {
	byAgent := map[string][]HistorySample{}
	for _, s := range samples {
		byAgent[s.AgentID] = append(byAgent[s.AgentID], s)
	}
	ids := make([]string, 0, len(byAgent))
	for id := range byAgent {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]AgentHistory, 0, len(ids))
	for _, id := range ids {
		seq := byAgent[id]
		h := AgentHistory{AgentID: id, Samples: len(seq)}
		var lats []int64
		var tokSum int64
		var costSum float64
		var costN int
		for _, s := range seq {
			if s.Evaluable {
				h.Evaluable++
				if s.Pass {
					h.Pass++
				}
			}
			if s.LatencyMS >= 0 {
				lats = append(lats, s.LatencyMS)
			}
			if s.Tokens >= 0 {
				tokSum += s.Tokens
			}
			if s.CostKnown {
				costSum += s.Cost
				costN++
				h.Currency = s.Currency
			}
		}
		if h.Evaluable > 0 {
			h.SuccessRate = float64(h.Pass) / float64(h.Evaluable)
			h.SuccessRateOK = true
		}
		if len(seq) > 0 {
			h.MeanTokens = tokSum / int64(len(seq))
		}
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		if len(lats) >= 5 {
			h.MedianLatencyMS = lats[len(lats)/2]
			h.LatencyOK = true
		} else if len(lats) > 0 {
			var sum int64
			for _, v := range lats {
				sum += v
			}
			h.MedianLatencyMS = sum / int64(len(lats))
			h.LatencyNote = fmt.Sprintf("mean over n=%d (median needs 5); no latency claim", len(lats))
		} else {
			h.LatencyNote = "no latency samples"
		}
		if costN > 0 {
			h.MeanCost = costSum / float64(costN)
			h.CostKnown = true
		}
		out = append(out, h)
	}
	return out
}

// SamplesFromLabResults converts observed Lab executions into routing
// history samples: pass/fail verdicts, wall durations (F11.4), proxy
// tokens (F11.5) and money priced against explicit tariffs (unknown
// when nothing prices, never zero).
func SamplesFromLabResults(results []bench.LabResult, rates []trace.RateCard) []HistorySample {
	out := make([]HistorySample, 0, len(results))
	for _, r := range results {
		s := HistorySample{AgentID: r.AgentID, LatencyMS: r.DurationMS, Tokens: r.TokensIn + r.TokensOut}
		switch r.Verdict {
		case bench.LabPass:
			s.Evaluable, s.Pass = true, true
		case bench.LabFail:
			s.Evaluable = true
		}
		if m := priceResult(r, rates); m.Known {
			s.Cost, s.CostKnown, s.Currency = m.Total, true, m.Currency
		}
		out = append(out, s)
	}
	return out
}

// historyByID indexes summarized history per agent.
func historyByID(h []AgentHistory) map[string]AgentHistory {
	m := make(map[string]AgentHistory, len(h))
	for _, e := range h {
		m[e.AgentID] = e
	}
	return m
}
