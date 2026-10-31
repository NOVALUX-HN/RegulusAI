// Budget tracking (Phase 8 T3, spec S5): measured-vs-estimated spend
// over EXISTING stores — stdlib plus regulus-ai/internal only, no cost
// invention. Execution time is measured from receipt duration_ms, tool
// usage from trace entry counts, experiment budgets from evolve Budget
// MaxRuns plus strategy probe-attempt history, resource consumption
// from receipt cpu_time_ms. Tokens and model costs have no backing
// store: they stay unmeasured (Measured false with a reason) unless the
// caller supplies reliable figures, which are then recorded as measured
// caller-supplied values.
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
// figures they are unmeasured zeros with a reason. Never invents a cost.
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
		Detail:   fmt.Sprintf("%d call(s), %d error(s) across %s", calls, failed, scope),
		Reason:   "trace entry count; entries carry names and order only, never values",
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
	if figs != nil && figs.Tokens != nil {
		tokens.Measured = true
		tokens.Value = float64(*figs.Tokens)
		tokens.Reason = "caller-supplied reliable figure"
	} else {
		tokens.Reason = "no token-count store; caller supplied no reliable figures"
	}
	u.Items = append(u.Items, tokens)

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
