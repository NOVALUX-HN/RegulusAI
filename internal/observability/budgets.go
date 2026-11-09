// Budget tracking (Phase 8 T3, spec S5): measured-vs-estimated spend
// over EXISTING stores — stdlib plus regulus-ai/internal only, no cost
// invention. Execution time is measured from receipt duration_ms, tool
// usage from trace entry counts, experiment budgets from evolve Budget
// MaxRuns plus strategy probe-attempt history, resource consumption
// from receipt cpu_time_ms. F1 adds the trace proxy cost (wall time,
// output bytes) to the tool-usage line; token counts are measured from
// trace input/output token fields when any entry carries host-exposed
// counts, else from caller-supplied figures, else unmeasured. A second
// token line (token_usage_local) always measures the local proxy
// (chars/4 per entry) whenever traces exist in scope, so budgets show
// measured-local tokens while host counts stay honestly unknown. Model
// costs have no backing store: they stay unmeasured (Measured false
// with a reason) unless the caller supplies reliable figures, which
// are then recorded as measured caller-supplied values.
package observability

import (
	"fmt"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/trace"
)

// Budget item names (S5 categories).
const (
	BudgetExecutionTime = "execution_time"
	BudgetToolUsage     = "tool_usage"
	BudgetExperiment    = "experiment_budget"
	BudgetResource      = "resource_consumption"
	BudgetTokens        = "token_usage"
	BudgetTokensLocal   = "token_usage_local"
	BudgetCosts         = "model_costs"
)

// Figures carries caller-supplied reliable numbers for stores Regulus
// does not keep (token counts, model costs). A nil field means no
// reliable figure exists: the item stays unmeasured, never estimated
// from thin air.
type Figures struct {
	Tokens    *int64
	ModelCost *float64
	Currency  string
}

// BudgetItem is one tracked spend line. Measured distinguishes store
// facts (true) from estimates or absent data (false); Reason always
// explains an unmeasured item and sources a measured one.
type BudgetItem struct {
	Name     string  `json:"name"`
	Measured bool    `json:"measured"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
	Detail   string  `json:"detail,omitempty"`
	Reason   string  `json:"reason"`
}

// BudgetUsage is the S5 set for one task scope ("" aggregates every
// task; experiments are global and always aggregate every experiment).
type BudgetUsage struct {
	Task    string       `json:"task"`
	Session string       `json:"session,omitempty"`
	Items   []BudgetItem `json:"items"`
}

// Item returns the named budget line, if present.
func (u BudgetUsage) Item(name string) (BudgetItem, bool) {
	for _, it := range u.Items {
		if it.Name == name {
			return it, true
		}
	}
	return BudgetItem{}, false
}

// Usage aggregates the S5 budget set read-only: receipts for time and
// resource, trace logs for tool usage, evolution experiments for run
// budgets. Tokens and costs are measured only from figs; without
// figures they are unmeasured zeros with a reason. A second token line
// (token_usage_local) measures the local proxy (chars/4 per entry)
// whenever traces exist in scope; host counts stay honestly unknown.
// Never invents a cost.
func Usage(root, task, session string, figs *Figures) (BudgetUsage, error) {
	u := BudgetUsage{Task: task, Session: session}

	entries, err := evidence.List(root, task)
	if err != nil {
		return BudgetUsage{}, err
	}
	var execMS, cpuMS int64
	for _, e := range entries {
		execMS += e.Receipt.DurationMS
		cpuMS += e.Receipt.CPUTimeMS
	}
	u.Items = append(u.Items, BudgetItem{
		Name:     BudgetExecutionTime,
		Measured: true,
		Value:    float64(execMS),
		Unit:     "ms",
		Detail:   fmt.Sprintf("%d receipt(s)", len(entries)),
		Reason:   "sum of receipt duration_ms",
	})
	u.Items = append(u.Items, BudgetItem{
		Name:     BudgetResource,
		Measured: true,
		Value:    float64(cpuMS),
		Unit:     "ms",
		Detail:   fmt.Sprintf("%d receipt(s)", len(entries)),
		Reason:   "sum of receipt cpu_time_ms; gates that omit the field contribute 0 (lower bound)",
	})

	sessions := []string{session}
	if session == "" {
		sessions, err = trace.Sessions(root)
		if err != nil {
			return BudgetUsage{}, err
		}
	}
	calls, failed := 0, 0
	var proxyMS, proxyBytes, traceIn, traceOut, traceLocal int64
	for _, s := range sessions {
		got, err := trace.List(root, s)
		if err != nil {
			return BudgetUsage{}, err
		}
		for _, e := range got {
			calls++
			if e.Status == trace.StatusError {
				failed++
			}
			proxyMS += e.DurationMS
			proxyBytes += e.OutputBytes
			traceIn += e.InputTokens
			traceOut += e.OutputTokens
			traceLocal += trace.EstimateLocalTokens(e)
		}
	}
	scope := fmt.Sprintf("session %s", session)
	if session == "" {
		scope = fmt.Sprintf("all %d session(s)", len(sessions))
	}
	u.Items = append(u.Items, BudgetItem{
		Name:     BudgetToolUsage,
		Measured: true,
		Value:    float64(calls),
		Unit:     "calls",
		Detail:   fmt.Sprintf("%d call(s), %d error(s) across %s; proxy %dms wall time, %dB output (zero = unknown, never estimated)", calls, failed, scope, proxyMS, proxyBytes),
		Reason:   "trace entry count; entries carry names and order plus F1 proxy cost (duration_ms, output_bytes), never values",
	})

	exps, err := listExperiments(root)
	if err != nil {
		return BudgetUsage{}, err
	}
	maxRuns, attempts := 0, 0
	for _, e := range exps {
		maxRuns += e.Budget.MaxRuns
		attempts += probeAttempts(e)
	}
	u.Items = append(u.Items, BudgetItem{
		Name:     BudgetExperiment,
		Measured: true,
		Value:    float64(attempts),
		Unit:     "runs",
		Detail:   fmt.Sprintf("%d experiment(s), max %d runs, %d probe attempt(s) used", len(exps), maxRuns, attempts),
		Reason:   "evolve Budget.MaxRuns plus strategy probe history; experiments are global (no task field)",
	})

	tokens := BudgetItem{Name: BudgetTokens, Unit: "tokens"}
	switch {
	case figs != nil && figs.Tokens != nil:
		tokens.Measured = true
		tokens.Value = float64(*figs.Tokens)
		tokens.Reason = "caller-supplied reliable figure"
	case traceIn+traceOut > 0:
		tokens.Measured = true
		tokens.Value = float64(traceIn + traceOut)
		tokens.Detail = fmt.Sprintf("%d in + %d out across %s", traceIn, traceOut, scope)
		tokens.Reason = "sum of trace input_tokens+output_tokens; host-exposed counts only, never estimated"
	default:
		tokens.Reason = "no token-count store and no trace token figures (host exposes none); caller supplied no reliable figures"
	}
	u.Items = append(u.Items, tokens)

	local := BudgetItem{Name: BudgetTokensLocal, Unit: "tokens"}
	if calls > 0 {
		local.Measured = true
		local.Value = float64(traceLocal)
		local.Detail = fmt.Sprintf("%d local token(s) chars/4 across %s", traceLocal, scope)
		local.Reason = fmt.Sprintf("measured-local proxy chars/4 over trace Tool/Message/Call/Task/Files/OutputBytes; host-exposed counts stay unknown (%d in + %d out across %s)", traceIn, traceOut, scope)
	} else {
		local.Reason = "no traces in scope; nothing measured locally"
	}
	u.Items = append(u.Items, local)

	costs := BudgetItem{Name: BudgetCosts, Unit: "cost"}
	if figs != nil && figs.ModelCost != nil {
		costs.Measured = true
		costs.Value = *figs.ModelCost
		if figs.Currency != "" {
			costs.Unit = figs.Currency
		}
		costs.Reason = "caller-supplied reliable figure"
	} else {
		costs.Reason = "no model-cost store; caller supplied no reliable figures — never estimated"
	}
	u.Items = append(u.Items, costs)

	return u, nil
}
