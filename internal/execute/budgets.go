// Wave budget gates (F13.3/NOV-106): labelled estimates against
// global and per-task ceilings before anything dispatches.
package execute

import (
	"fmt"

	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

// CheckWaveBudgets refuses a wave before anything dispatches when
// the labelled estimates already break a ceiling: the aggregate of
// the wave against the global budget, and each task against its
// per-task ceiling. Zero means unset (that dimension never gates).
// Unknown money (CostKnown=false) never gates either: an unknown
// cost is reported, never zeroed, never refused.
func CheckWaveBudgets(tasks []orchestrate.ParallelTask, estimates map[string]planning.Estimate, global mission.Budget, perTask mission.Budget) error {
	var aggTime, aggTokens int64
	for _, t := range tasks {
		est := estimates[t.ID]
		aggTime += est.TimeMS
		aggTokens += est.Tokens
		if perTask.TimeMS > 0 && est.TimeMS > perTask.TimeMS {
			return fmt.Errorf("execute: task %s estimate %dms over per-task ceiling %dms (backpressure: raise the ceiling or shrink the task)", t.ID, est.TimeMS, perTask.TimeMS)
		}
		if perTask.Tokens > 0 && est.Tokens > perTask.Tokens {
			return fmt.Errorf("execute: task %s estimate %d tokens over per-task ceiling %d (backpressure: raise the ceiling or shrink the task)", t.ID, est.Tokens, perTask.Tokens)
		}
		if perTask.Cost > 0 && est.CostKnown && est.Cost > perTask.Cost {
			return fmt.Errorf("execute: task %s estimate cost %.4f over per-task ceiling %.4f", t.ID, est.Cost, perTask.Cost)
		}
	}
	if global.TimeMS > 0 && aggTime > global.TimeMS {
		return fmt.Errorf("execute: wave aggregate %dms over global budget %dms (backpressure: %d task(s) do not fit; approve a bigger budget or run fewer tasks)", aggTime, global.TimeMS, len(tasks))
	}
	if global.Tokens > 0 && aggTokens > global.Tokens {
		return fmt.Errorf("execute: wave aggregate %d tokens over global budget %d (backpressure: approve a bigger budget or run fewer tasks)", aggTokens, global.Tokens)
	}
	return nil
}

// estimateIndex indexes plan estimates by task id for budget checks.
func estimateIndex(p *planning.Plan) map[string]planning.Estimate {
	m := make(map[string]planning.Estimate, len(p.Tasks))
	for _, t := range p.Tasks {
		m[t.ID] = t.Estimate
	}
	return m
}
