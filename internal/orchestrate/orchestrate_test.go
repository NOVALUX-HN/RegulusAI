// Routing tests (F12.1/NOV-96): determinism, the verified-evidence
// gate, permission-respecting fallback, budgets and the evaluation
// that may keep the fixed router.
package orchestrate

import (
	"reflect"
	"testing"

	"regulus-ai/internal/bench"
)

// testManifests builds a pinned host matrix: opencode fully probada,
// codex probada except subagents (no-ejecutada), devin strategy
// without probada. Tests pin their own matrix instead of the live
// default, so a matrix drift cannot silently flip routing.
func testManifests() map[string]bench.HostManifest {
	opencodeCaps := []bench.HostCapability{}
	for _, id := range bench.CapabilityIDs() {
		opencodeCaps = append(opencodeCaps, bench.HostCapability{
			ID: id, Status: bench.CapVerified,
			Detail: "test verified", Evidence: "test-evidence 2026-11-16",
		})
	}
	codexCaps := []bench.HostCapability{}
	for _, id := range bench.CapabilityIDs() {
		st := bench.CapVerified
		if id == bench.CapSubagents {
			st = bench.CapNotExecuted
		}
		codexCaps = append(codexCaps, bench.HostCapability{
			ID: id, Status: st,
			Detail: "test", Evidence: "test-evidence 2026-11-16",
		})
	}
	devinCaps := []bench.HostCapability{}
	for _, id := range bench.CapabilityIDs() {
		devinCaps = append(devinCaps, bench.HostCapability{
			ID: id, Status: bench.CapUnknown, Detail: "test", Evidence: "test-note",
		})
	}
	return map[string]bench.HostManifest{
		"opencode": {Host: "opencode", Version: "vX", Class: bench.ClassJourney, Capabilities: opencodeCaps},
		"codex":    {Host: "codex", Version: "vX", Class: bench.ClassJourney, Capabilities: codexCaps},
		"devin":    {Host: "devin", Version: "strategy", Class: bench.ClassStrategy, Capabilities: devinCaps},
	}
}

// testRegistry grades the Lab agents against the pinned matrix and
// forces deterministic estimates so tests never depend on defaults.
func testRegistry() []RoutedAgent {
	reg := RegistryFromManifests(testManifests())
	for i := range reg {
		reg[i].EstimateTimeMS = 1000 * int64(i+1)
		reg[i].EstimateTokens = 100 * int64(i+1)
		reg[i].EstimateCost = -1
	}
	return reg
}

func testDescriptor() TaskDescriptor {
	return TaskDescriptor{
		Domain: "harness", Language: "go",
		Complexity: ComplexityLow, Risk: RiskLow,
		Constraints: []string{"sin red"}, Outputs: []string{"parche"},
		Criteria:             []string{"checks en verde"},
		RequiredCapabilities: []string{bench.CapTools},
	}
}

func decideOrFail(t *testing.T, reg []RoutedAgent, m map[string]bench.HostManifest, hist []AgentHistory, d TaskDescriptor, b Budgets, maxDeleg int, fixed string) Decision {
	t.Helper()
	dec, err := Decide(reg, m, hist, d, b, maxDeleg, fixed, DefaultScoreWeights())
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	return dec
}

// The same inputs always produce the byte-identical decision.
func TestDecideDeterministic(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	hist := SummarizeHistory(SamplesFromLabResults([]bench.LabResult{
		{AgentID: "anchored", Verdict: bench.LabPass, DurationMS: 1200, TokensIn: 100, TokensOut: 20},
		{AgentID: "anchored", Verdict: bench.LabFail, DurationMS: 1500, TokensIn: 110, TokensOut: 25},
		{AgentID: "baseline", Verdict: bench.LabPass, DurationMS: 900, TokensIn: 80, TokensOut: 10},
	}, nil))
	d := testDescriptor()
	a := decideOrFail(t, reg, m, hist, d, Budgets{}, 1, "baseline")
	b := decideOrFail(t, reg, m, hist, d, Budgets{}, 1, "baseline")
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same inputs diverged:\n%v\n%v", RenderDecision(a), RenderDecision(b))
	}
	if a.Primary == "" {
		t.Fatalf("expected a primary, got refusal: %v", a.Excluded)
	}
}

// A capability without verified evidence is never assigned: every
// agent requiring it is excluded with a naming reason, and an empty
// field refuses instead of inventing a pick.
func TestCapabilityWithoutEvidenceNeverAssigned(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	d := testDescriptor()
	d.RequiredCapabilities = []string{bench.CapSubagents} // codex: no-ejecutada; opencode agents declare only probada... but opencode IS probada
	// Restrict the registry to the codex-hosted agents to prove the
	// gate: no test Lab agent rides codex, so build one explicitly.
	codexOnly := []RoutedAgent{{
		ID: "codex-worker", Host: "codex", MaxRisk: RiskHigh, Available: true,
		Capabilities:   []RoutedCapability{{ID: bench.CapSubagents, Evidence: "", Limit: "n/a"}},
		Scopes:         []string{"memory:read"},
		EstimateTimeMS: 1000, EstimateTokens: 100, EstimateCost: -1,
	}}
	dec := decideOrFail(t, codexOnly, m, nil, d, Budgets{}, 1, "baseline")
	if dec.Primary != "" {
		t.Fatalf("capability no-ejecutada assigned to %q", dec.Primary)
	}
	if len(dec.Excluded) == 0 {
		t.Fatal("expected exclusions naming the evidence gate")
	}
	found := false
	for _, e := range dec.Excluded {
		if e.AgentID == "codex-worker" {
			found = true
		}
	}
	if !found {
		t.Fatalf("codex-worker must be excluded, got %v", dec.Excluded)
	}

	// And the full registry still routes: opencode verifies subagents.
	dec2 := decideOrFail(t, reg, m, nil, d, Budgets{}, 1, "baseline")
	if dec2.Primary == "" {
		t.Fatalf("opencode agents verify subagents; refusal wrong: %v", dec2.Excluded)
	}
}

// Unknown capabilities fail validation instead of routing.
func TestUnknownCapabilityRefused(t *testing.T) {
	d := testDescriptor()
	d.RequiredCapabilities = []string{"teleport"}
	if _, err := Decide(testRegistry(), testManifests(), nil, d, Budgets{}, 1, "baseline", DefaultScoreWeights()); err == nil {
		t.Fatal("unknown capability must fail validation")
	}
}

// Fallback respects permissions (F11.8): the fallback keeps only the
// intersection with the primary scopes — delegation only restricts.
func TestFallbackRestrictsScopes(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	dec := decideOrFail(t, reg, m, nil, testDescriptor(), Budgets{}, 2, "baseline")
	if dec.Primary == "" {
		t.Fatalf("expected a primary: %v", dec.Excluded)
	}
	if dec.Fallback == "" {
		t.Fatalf("expected a fallback with 3 eligible agents: %+v", dec)
	}
	primary := map[string]bool{}
	for _, a := range reg {
		if a.ID == dec.Primary {
			for _, s := range a.Scopes {
				primary[s] = true
			}
		}
	}
	for _, s := range dec.FallbackScopes {
		if !primary[s] {
			t.Fatalf("fallback scope %q widens the primary (F11.8: delegate only restricts)", s)
		}
	}
}

// A delegation bound of 0 names no fallback and justifies the
// no-delegation instead.
func TestMaxDelegationsZeroJustifiesNoDelegation(t *testing.T) {
	dec := decideOrFail(t, testRegistry(), testManifests(), nil, testDescriptor(), Budgets{}, 0, "baseline")
	if dec.Primary == "" {
		t.Fatalf("expected a primary: %v", dec.Excluded)
	}
	if dec.Fallback != "" {
		t.Fatalf("max-delegations 0 must name no fallback, got %q", dec.Fallback)
	}
	if dec.NoDelegationReason == "" {
		t.Fatal("max-delegations 0 must justify the no-delegation")
	}
}

// Budgets gate: an agent over budget is excluded with a naming
// reason; when nobody fits, the decision refuses.
func TestBudgetsExcludeAndRefuse(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	d := testDescriptor()
	dec := decideOrFail(t, reg, m, nil, d, Budgets{TimeMS: 1}, 1, "baseline")
	if dec.Primary != "" {
		t.Fatalf("1ms budget must refuse everything, routed %q", dec.Primary)
	}
	if len(dec.Excluded) != len(reg) {
		t.Fatalf("every agent must be excluded, got %v", dec.Excluded)
	}

	// A fitting budget routes the fastest eligible agent first.
	dec2 := decideOrFail(t, reg, m, nil, d, Budgets{TimeMS: 2500, Tokens: 250}, 1, "baseline")
	if dec2.Primary == "" {
		t.Fatalf("fitting budget must route: %v", dec2.Excluded)
	}
	if dec2.Primary != "anchored" && dec2.Primary != "baseline" {
		t.Fatalf("unexpected primary %q", dec2.Primary)
	}
}

// Critical risk routes a pair and justifies the review; a lone
// eligible agent refuses instead of running solo.
func TestCriticalRiskNeedsPair(t *testing.T) {
	m := testManifests()
	d := testDescriptor()
	d.Risk = RiskCritical
	dec := decideOrFail(t, testRegistry(), m, nil, d, Budgets{}, 1, "baseline")
	if dec.Mode != ModePair || dec.Primary == "" || dec.Reviewer == "" {
		t.Fatalf("critical must route a pair, got %+v", dec)
	}

	// Lone eligible: only baseline is high enough? baseline caps at
	// medium, so mark every agent but one unavailable.
	reg := testRegistry()
	for i := range reg {
		if reg[i].ID != "verify-first" {
			reg[i].Available = false
		}
	}
	lone := decideOrFail(t, reg, m, nil, d, Budgets{}, 1, "baseline")
	if lone.Primary != "" {
		t.Fatalf("critical with a lone eligible must refuse, routed %q", lone.Primary)
	}
}

// The evaluation may conclude the dynamic router adds nothing:
// ties and regressions keep the fixed router.
func TestEvaluateMayKeepFixed(t *testing.T) {
	mk := func(agent, verdict string, ms int64, tok int64, n int) []bench.LabResult {
		var out []bench.LabResult
		for i := 0; i < n; i++ {
			out = append(out, bench.LabResult{
				AgentID: agent, Verdict: verdict, DurationMS: ms,
				TokensIn: tok / 2, TokensOut: tok / 2,
			})
		}
		return out
	}
	th := DefaultEvalThresholds()

	// Identical arms: tie keeps fixed.
	tied := Evaluate(mk("a", bench.LabPass, 100, 50, 6), mk("b", bench.LabPass, 100, 50, 6), nil, th)
	if tied.Verdict != VerdictFixed {
		t.Fatalf("tie must keep fixed, got %s (%s)", tied.Verdict, tied.Reason)
	}

	// Dynamic worse on quality: keeps fixed.
	worse := Evaluate(mk("a", bench.LabFail, 100, 50, 6), mk("b", bench.LabPass, 100, 50, 6), nil, th)
	if worse.Verdict != VerdictFixed {
		t.Fatalf("regression must keep fixed, got %s", worse.Verdict)
	}

	// Dynamic clearly better: quality +1.0, same latency/cost.
	better := Evaluate(mk("a", bench.LabPass, 100, 50, 6), mk("b", bench.LabFail, 100, 50, 6), nil, th)
	if better.Verdict != VerdictDynamic {
		t.Fatalf("clear win must adopt dynamic, got %s (%s)", better.Verdict, better.Reason)
	}

	// Quality win with a latency blowout: keeps fixed.
	slow := Evaluate(mk("a", bench.LabPass, 1000, 50, 6), mk("b", bench.LabFail, 100, 50, 6), nil, th)
	if slow.Verdict != VerdictFixed {
		t.Fatalf("latency blowout must keep fixed, got %s", slow.Verdict)
	}
}

// Nothing evaluable keeps the fixed router instead of claiming.
func TestEvaluateInsufficientEvidence(t *testing.T) {
	rep := Evaluate(
		[]bench.LabResult{{AgentID: "a", Verdict: bench.LabNotRun}},
		[]bench.LabResult{{AgentID: "b", Verdict: bench.LabNotRun}},
		nil, DefaultEvalThresholds())
	if rep.Verdict != VerdictFixed {
		t.Fatalf("no evaluable results must keep fixed, got %s", rep.Verdict)
	}
}

// History honesty: unknown money never gates the cost budget and
// scores neutral with a note.
func TestUnknownCostNeverGates(t *testing.T) {
	m := testManifests()
	reg := testRegistry()
	dec := decideOrFail(t, reg, m, nil, testDescriptor(), Budgets{Cost: 0.0001}, 1, "baseline")
	if dec.Primary == "" {
		t.Fatalf("unknown cost must not gate: %v", dec.Excluded)
	}
}

// TestRegistryRoutesRestrictedCapabilities pins F16.3 at the router: a
// capability the host can do with a restriction must reach the router instead
// of being silently dropped, and only the states that assert absence are
// excluded. Before this, the filter accepted probada alone, so the core lost
// a dimension the moment it became partial.
func TestRegistryRoutesRestrictedCapabilities(t *testing.T) {
	m := bench.HostManifest{
		Host: "opencode",
		Capabilities: []bench.HostCapability{
			{ID: bench.CapTools, Status: bench.CapVerified, Detail: "ok", Evidence: "ev"},
			{ID: bench.CapHooks, Status: bench.CapDegraded, Detail: "hooks version-gated", Evidence: "contract tests"},
			{ID: bench.CapLimits, Status: bench.CapPartial, Detail: "cotas parciales", Evidence: "contract tests"},
			{ID: bench.CapSubagents, Status: bench.CapUnsupported, Detail: "sin superficie", Evidence: "probe"},
		},
	}
	reg := RegistryFromManifests(map[string]bench.HostManifest{"opencode": m})
	var got *RoutedAgent
	for i := range reg {
		if reg[i].Host == "opencode" {
			got = &reg[i]
			break
		}
	}
	if got == nil {
		t.Fatal("opencode must still be routed when only some capabilities are restricted")
	}
	byID := map[string]RoutedCapability{}
	for _, c := range got.Capabilities {
		byID[c.ID] = c
	}
	if _, ok := byID[bench.CapTools]; !ok {
		t.Fatal("verified capability must be routed")
	}
	hooks, ok := byID[bench.CapHooks]
	if !ok {
		t.Fatal("degraded capability must be routed, not dropped")
	}
	if hooks.Limit != "hooks version-gated" {
		t.Fatalf("degraded capability must carry its own restriction, got %q", hooks.Limit)
	}
	limits, ok := byID[bench.CapLimits]
	if !ok {
		t.Fatal("partial capability must be routed, not dropped")
	}
	if limits.Limit != "cotas parciales" {
		t.Fatalf("partial capability must carry its own restriction, got %q", limits.Limit)
	}
	if _, ok := byID[bench.CapSubagents]; ok {
		t.Fatal("unsupported capability must not be routed")
	}
}
