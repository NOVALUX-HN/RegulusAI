// Adversarial fixtures for the capability contract (F16.4 / NOV-137).
//
// The happy path is covered elsewhere; these tests pin what happens at the
// boundaries the core actually depends on: a malformed manifest, a claim whose
// evidence cannot support it, and a journey outcome applied to dimensions it
// never exercised.
package bench

import "testing"

// TestMalformedManifestsAreRefused: an unknown or blank state word, and a
// verified claim with no (or blank) evidence, must fail validation instead of
// rendering as if they were trustworthy.
func TestMalformedManifestsAreRefused(t *testing.T) {
	cases := []struct {
		name string
		cap  HostCapability
	}{
		{"unknown state word", HostCapability{ID: CapTools, Status: "verified", Evidence: "ev"}},
		{"blank state word", HostCapability{ID: CapTools, Status: "", Evidence: "ev"}},
		{"verified without evidence", HostCapability{ID: CapTools, Status: CapVerified}},
		{"verified with blank evidence", HostCapability{ID: CapTools, Status: CapVerified, Evidence: "   "}},
	}
	for _, tc := range cases {
		m := map[string]HostManifest{"h": {Host: "h", Capabilities: []HostCapability{tc.cap}}}
		if err := ValidateCapabilitiesAttributable(m); err == nil {
			t.Fatalf("%s must be refused", tc.name)
		}
	}
	if err := ValidateCapabilitiesAttributable(nil); err != nil {
		t.Fatalf("nil manifests must not fail: %v", err)
	}
}

// TestEvidenceCoverageRejectsEveryUncoveredDimension is the trust boundary
// between a claim and its citation: for every dimension JourneyCoveredCaps
// does not exercise, a probada backed by the journey must be refused. The
// audit found four such dimensions silently accepted.
func TestEvidenceCoverageRejectsEveryUncoveredDimension(t *testing.T) {
	covered := map[string]bool{}
	for _, id := range JourneyCoveredCaps() {
		covered[id] = true
	}
	uncovered := 0
	for _, id := range CapabilityIDs() {
		if covered[id] {
			continue
		}
		uncovered++
		m := map[string]HostManifest{"h": {Host: "h", Capabilities: []HostCapability{
			{ID: id, Status: CapVerified, Evidence: "contract tests + " + journeyGreen},
		}}}
		if err := ValidateEvidenceCoverage(m); err == nil {
			t.Fatalf("dimension %q is not journey-covered and must refuse a probada journey claim", id)
		}
	}
	if uncovered == 0 {
		t.Fatal("the journey now covers every dimension; this test no longer exercises the boundary")
	}
}

// TestJourneyOutcomePromotesOnlyCoveredDimensions pins the lifecycle rule at
// the boundary: a green journey may promote the dimensions it covers and
// nothing else, and an outcome with an unknown level or status promotes
// nothing at all.
func TestJourneyOutcomePromotesOnlyCoveredDimensions(t *testing.T) {
	covered := map[string]bool{}
	for _, id := range JourneyCoveredCaps() {
		covered[id] = true
	}
	base := DefaultHostManifests("2026-11-16")["devin"]

	green := HostJourneyOutcome{Host: "devin", Level: LevelHost, Status: AgentPass, Reason: "green fixture", Version: "x"}
	after := ApplyJourneyOutcome(base, green, "2026-11-20")
	for _, c := range after.Capabilities {
		if c.Status == CapVerified && !covered[c.ID] {
			t.Fatalf("a green journey promoted uncovered dimension %q", c.ID)
		}
	}

	unknown := ApplyJourneyOutcome(base, HostJourneyOutcome{Host: "devin", Level: "???", Status: "???"}, "2026-11-20")
	for _, c := range unknown.Capabilities {
		if c.Status == CapVerified {
			t.Fatalf("an unknown outcome promoted %q; nothing is known to promote", c.ID)
		}
	}
}
