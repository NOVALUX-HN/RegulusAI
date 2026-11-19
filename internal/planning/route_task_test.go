package planning

import (
	"errors"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
	"regulus-ai/internal/impact"
	"regulus-ai/internal/mission"
	"regulus-ai/internal/orchestrate"
)

func routeTestMission(risk string) *mission.Mission {
	return &mission.Mission{
		Objective:   "Probar el ruteo sin tocar el store",
		Risk:        risk,
		Constraints: []string{"no network"},
		Budget:      mission.Budget{TimeMS: 1000, Tokens: 200, Cost: 3.5},
	}
}

func routeTestTask() PlanTask {
	return PlanTask{
		ID:          "t2",
		Title:       "Implementar cambio en alcance",
		Deliverable: "cambio aplicado y compilable",
		Scope:       Scope{Files: []string{"a.go"}},
		MinScopes:   []string{"spec:read", "memory:read"},
		Verification: Verification{
			Strategy: "tests",
			Criteria: []string{"Given x When y Then z"},
		},
		RequiredCapabilities: capsFor(taskImpl),
		ImpactKnown:          true,
	}
}

func routeTestReg() []orchestrate.RoutedAgent {
	return []orchestrate.RoutedAgent{
		{ID: "a1", Host: "h", Available: true, Scopes: []string{"spec:read"}},
		{ID: "a2", Host: "h", Available: true, Scopes: []string{"spec:read", "memory:read"}},
	}
}

func withFakeDecide(t *testing.T, fn func(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, d orchestrate.TaskDescriptor, b orchestrate.Budgets, maxDeleg int, fixedAgent string, w orchestrate.ScoreWeights) (orchestrate.Decision, error)) {
	t.Helper()
	prev := routeDecide
	routeDecide = fn
	t.Cleanup(func() { routeDecide = prev })
}

func TestRouteRiskDefaultsAndEscalates(t *testing.T) {
	if got := routeRisk("", nil); got != orchestrate.RiskMedium {
		t.Fatalf("empty risk = %q, want medium", got)
	}
	if got := routeRisk("low", nil); got != orchestrate.RiskLow {
		t.Fatalf("low risk without critical = %q, want low", got)
	}
	if got := routeRisk("low", []string{"pkg:crit"}); got != orchestrate.RiskHigh {
		t.Fatalf("low risk with critical = %q, want high", got)
	}
	if got := routeRisk("high", nil); got != orchestrate.RiskHigh {
		t.Fatalf("high risk = %q, want high", got)
	}
	if got := routeRisk("critical", nil); got != orchestrate.RiskCritical {
		t.Fatalf("critical risk = %q, want critical", got)
	}
}

func TestRouteComplexityBoundaries(t *testing.T) {
	if got := routeComplexity(taskImpl, 0); got != orchestrate.ComplexityMedium {
		t.Fatalf("implement work = %q, want medium", got)
	}
	if got := routeComplexity(taskDocs, 1); got != orchestrate.ComplexityLow {
		t.Fatalf("single file docs = %q, want low", got)
	}
	if got := routeComplexity(taskDocs, 3); got != orchestrate.ComplexityMedium {
		t.Fatalf("three files = %q, want medium", got)
	}
	if got := routeComplexity(taskDocs, 6); got != orchestrate.ComplexityHigh {
		t.Fatalf("six files = %q, want high", got)
	}
}

func TestRouteDescriptorCarriesBudgets(t *testing.T) {
	task := routeTestTask()
	m := routeTestMission("low")
	desc, budgets := routeDescriptor(task, m, taskImpl, "low", orchestrate.ComplexityMedium)
	if desc.Domain != "engineering" || desc.Language != "go" || desc.Risk != "low" || desc.Complexity != orchestrate.ComplexityMedium {
		t.Fatalf("descriptor = %+v, want engineering/go low/medium", desc)
	}
	if budgets.TimeMS != 1000 || budgets.Tokens != 200 || budgets.Cost != 3.5 {
		t.Fatalf("budgets = %+v, want the mission budgets", budgets)
	}
	if len(desc.Outputs) != 1 || desc.Outputs[0] != task.Deliverable {
		t.Fatalf("outputs = %v, want the deliverable", desc.Outputs)
	}
}

func TestChooseRouteAgentPrefersScoped(t *testing.T) {
	reg := routeTestReg()
	dec := orchestrate.Decision{
		Primary: "a1",
		Ranking: []orchestrate.RankedAgent{{AgentID: "a1"}, {AgentID: "a2"}},
	}
	chosen, note := chooseRouteAgent(dec, reg, []string{"spec:read", "memory:read"})
	if chosen != "a2" || note != "" {
		t.Fatalf("scoped pick = %q/%q, want a2 without note", chosen, note)
	}
	dec = orchestrate.Decision{
		Primary: "a1",
		Ranking: []orchestrate.RankedAgent{{AgentID: "a1"}},
	}
	chosen, note = chooseRouteAgent(dec, reg, []string{"spec:read", "memory:read"})
	if chosen != "a1" || !strings.HasPrefix(note, "aprobación:") {
		t.Fatalf("over-grant pick = %q/%q, want a1 behind approval", chosen, note)
	}
	dec = orchestrate.Decision{Excluded: []orchestrate.Exclusion{{AgentID: "a1", Reason: "busy"}}}
	chosen, note = chooseRouteAgent(dec, reg, []string{"spec:read"})
	if chosen != "" || note != "" {
		t.Fatalf("refusal = %q/%q, want empty", chosen, note)
	}
}

func TestFinalizeRouteAutonomyHighRiskFloor(t *testing.T) {
	task := routeTestTask()
	task.Autonomy = ""
	finalizeRouteAutonomy(&task, taskSurvey, orchestrate.RiskHigh, nil)
	if task.Autonomy != AutonomyNeedsApproval {
		t.Fatalf("high risk autonomy = %q, want needs-approval", task.Autonomy)
	}
	if !hasApprovalBlocker(task.Blockers) {
		t.Fatalf("needs-approval must name a blocker, got %v", task.Blockers)
	}
	clean := routeTestTask()
	clean.Autonomy = ""
	finalizeRouteAutonomy(&clean, taskSurvey, orchestrate.RiskLow, nil)
	if clean.Autonomy != AutonomyAutonomous {
		t.Fatalf("low risk survey autonomy = %q, want autonomous", clean.Autonomy)
	}
	if len(clean.Blockers) != 0 {
		t.Fatalf("autonomous tasks carry no blockers, got %v", clean.Blockers)
	}
}

func TestRouteTaskUsesFakeDecide(t *testing.T) {
	withFakeDecide(t, func(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, d orchestrate.TaskDescriptor, b orchestrate.Budgets, maxDeleg int, fixedAgent string, w orchestrate.ScoreWeights) (orchestrate.Decision, error) {
		return orchestrate.Decision{
			Mode:    orchestrate.ModeSingle,
			Primary: "a2",
			Ranking: []orchestrate.RankedAgent{{AgentID: "a2"}},
		}, nil
	})
	task := routeTestTask()
	routeTask(&task, routeTestMission("low"), taskImpl, impact.Graph{}, routeTestReg(), map[string]bench.HostManifest{})
	if task.Agent != "a2" {
		t.Fatalf("agent = %q, want a2", task.Agent)
	}
	if task.Estimate.Risk != orchestrate.RiskLow || task.Estimate.Complexity != orchestrate.ComplexityMedium {
		t.Fatalf("estimate = %+v, want low/medium", task.Estimate)
	}
	if task.Autonomy != AutonomyAutonomous {
		t.Fatalf("autonomy = %q, want autonomous", task.Autonomy)
	}
	if len(task.CapabilityEvidence) != len(task.RequiredCapabilities) {
		t.Fatalf("evidence = %v, want one entry per capability", task.CapabilityEvidence)
	}
}

func TestRouteTaskDecideErrorEscalates(t *testing.T) {
	withFakeDecide(t, func(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, d orchestrate.TaskDescriptor, b orchestrate.Budgets, maxDeleg int, fixedAgent string, w orchestrate.ScoreWeights) (orchestrate.Decision, error) {
		return orchestrate.Decision{}, errors.New("boom")
	})
	task := routeTestTask()
	routeTask(&task, routeTestMission("low"), taskImpl, impact.Graph{}, routeTestReg(), map[string]bench.HostManifest{})
	if task.Autonomy != AutonomyNeedsApproval {
		t.Fatalf("autonomy = %q, want needs-approval", task.Autonomy)
	}
	if len(task.Blockers) != 1 || !strings.HasPrefix(task.Blockers[0], "routing imposible: ") {
		t.Fatalf("blockers = %v, want the routing failure", task.Blockers)
	}
	if task.Estimate.Risk == "" {
		t.Fatal("estimate must stay priced even when routing fails")
	}
}

func TestRouteTaskNoEligibleNamesReasons(t *testing.T) {
	withFakeDecide(t, func(reg []orchestrate.RoutedAgent, manifests map[string]bench.HostManifest, hist []orchestrate.AgentHistory, d orchestrate.TaskDescriptor, b orchestrate.Budgets, maxDeleg int, fixedAgent string, w orchestrate.ScoreWeights) (orchestrate.Decision, error) {
		return orchestrate.Decision{
			Excluded: []orchestrate.Exclusion{{AgentID: "a1", Reason: "busy"}},
		}, nil
	})
	task := routeTestTask()
	routeTask(&task, routeTestMission("low"), taskImpl, impact.Graph{}, routeTestReg(), map[string]bench.HostManifest{})
	if task.Agent != "" {
		t.Fatalf("agent = %q, want empty on refusal", task.Agent)
	}
	if task.Autonomy != AutonomyNeedsApproval {
		t.Fatalf("autonomy = %q, want needs-approval", task.Autonomy)
	}
	if len(task.Blockers) != 1 || !strings.Contains(task.Blockers[0], "a1: busy") {
		t.Fatalf("blockers = %v, want the exclusion reason", task.Blockers)
	}
}
