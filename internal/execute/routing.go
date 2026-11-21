// Wave routing re-validation (F13.3/NOV-106): F12.1 Decide runs
// again per wave, so an agent that went offline (or lost its
// verified capability) since planning blocks with the naming
// reason instead of dispatching to nobody.
package execute

import (
	"sort"
	"strings"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
	"regulus-ai/internal/planning"
)

// RevalidateRouting re-runs F12.1 Decide per wave task with the
// mission budgets: an agent that went offline (or lost its verified
// capability) since planning is excluded here, and the task blocks
// with the naming reason instead of dispatching to nobody. It
// returns the tasks admitted and the per-task block reasons.
func RevalidateRouting(tasks []orchestrate.ParallelTask, p *planning.Plan, m *mission.Mission) (admitted []orchestrate.ParallelTask, blocked map[string]string) {
	blocked = map[string]string{}
	manifests := bench.DefaultHostManifests(bench.EvidenceStamp())
	reg := orchestrate.RegistryFromManifests(manifests)
	byPlan := map[string]planning.PlanTask{}
	for _, t := range p.Tasks {
		byPlan[t.ID] = t
	}
	for _, t := range tasks {
		pt, ok := byPlan[t.ID]
		if !ok {
			blocked[t.ID] = "task vanished from the plan (nothing to route)"
			continue
		}
		desc := orchestrate.TaskDescriptor{
			Domain: "engineering", Language: "go",
			Complexity: taskComplexity(len(pt.Scope.Files)), Risk: missionRisk(m, pt),
			Constraints: m.Constraints, Outputs: []string{pt.Deliverable},
			Criteria: pt.Verification.Criteria, RequiredCapabilities: pt.RequiredCapabilities,
		}
		budgets := orchestrate.Budgets{TimeMS: m.Budget.TimeMS, Tokens: m.Budget.Tokens, Cost: m.Budget.Cost}
		dec, err := orchestrate.Decide(reg, manifests, nil, desc, budgets, 1, "", orchestrate.DefaultScoreWeights())
		if err != nil {
			blocked[t.ID] = "routing imposible: " + err.Error()
			continue
		}
		if dec.Primary == "" {
			reasons := []string{}
			for _, e := range dec.Excluded {
				reasons = append(reasons, e.AgentID+": "+e.Reason)
			}
			if len(reasons) == 0 {
				reasons = append(reasons, "sin agente elegible (sin evidencia verificada)")
			}
			blocked[t.ID] = "worker offline o sin capability verificada: " + strings.Join(reasons, "; ")
			continue
		}
		admitted = append(admitted, t)
	}
	return admitted, blocked
}

// taskComplexity mirrors the planner's file-count rule so
// re-validation routes on the same words planning assigned with.
func taskComplexity(nfiles int) string {
	if nfiles > 5 {
		return orchestrate.ComplexityHigh
	}
	if nfiles > 2 {
		return orchestrate.ComplexityMedium
	}
	return orchestrate.ComplexityLow
}

// missionRisk mirrors the planner's risk rule: the mission tier,
// raised to high when the task touches a critical package the plan
// already flagged in its rationale.
func missionRisk(m *mission.Mission, pt planning.PlanTask) string {
	risk := strings.TrimSpace(m.Risk)
	if risk == "" {
		risk = orchestrate.RiskMedium
	}
	if strings.Contains(pt.Rationale, "crítico") {
		if risk == orchestrate.RiskLow || risk == orchestrate.RiskMedium {
			return orchestrate.RiskHigh
		}
	}
	return risk
}

// agentIndex indexes planned agents by task id for ledger seeding.
func agentIndex(p *planning.Plan) map[string]string {
	m := make(map[string]string, len(p.Tasks))
	for _, t := range p.Tasks {
		m[t.ID] = t.Agent
	}
	return m
}

// planTaskIDs returns the deterministic plan task order.
func planTaskIDs(p *planning.Plan) []string {
	ids := make([]string, 0, len(p.Tasks))
	for _, t := range p.Tasks {
		ids = append(ids, t.ID)
	}
	sort.Strings(ids)
	return ids
}
