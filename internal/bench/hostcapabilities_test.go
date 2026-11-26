// F11.7: capability manifest rules. Probes never promote, failures
// never upgrade, every probada cites evidence, and the matrix only
// speaks the four state words.
package bench

import (
	"strings"
	"testing"
)

func testManifests() map[string]HostManifest {
	return DefaultHostManifests("2026-11-16T00:00:00Z")
}

func TestDefaultManifestsAttributable(t *testing.T) {
	if err := ValidateCapabilitiesAttributable(testManifests()); err != nil {
		t.Fatalf("declared manifests must be attributable: %v", err)
	}
}

func TestProbeOutcomeNeverPromotesOrDemotes(t *testing.T) {
	before := testManifests()
	probe := HostJourneyOutcome{
		Host: "opencode", Version: "v9.9.9",
		Level: LevelContract, Status: AgentNotRun,
		Reason: "probe only: binary present (journey not executed, no model cost)",
	}
	after := ApplyJourneyOutcome(before["opencode"], probe, "2026-11-17T00:00:00Z")
	if after.Class != ClassJourney {
		t.Fatalf("probe must preserve the dated journey class, got %q", after.Class)
	}
	if after.Version != "v9.9.9" {
		t.Fatalf("probe must refresh the version, got %q", after.Version)
	}
	for i, c := range after.Capabilities {
		if c.Status != before["opencode"].Capabilities[i].Status {
			t.Fatalf("probe must not touch capability %s: %q -> %q", c.ID, before["opencode"].Capabilities[i].Status, c.Status)
		}
	}
	if !strings.Contains(after.ProbeNote, "probe") {
		t.Fatalf("probe outcome must leave a probe note, got %q", after.ProbeNote)
	}
	if err := ValidateNoProbeAsJourney(map[string]HostManifest{"opencode": after}); err != nil {
		t.Fatalf("preserved journey must not trip the probe rule: %v", err)
	}
}

func TestNotRunUndatedManifestBecomesProbe(t *testing.T) {
	m := HostManifest{
		Host:  "fresh",
		Class: ClassJourney,
		Capabilities: []HostCapability{
			{ID: CapTools, Status: CapNotExecuted, Detail: "d", Evidence: "e"},
		},
	}
	missing := HostJourneyOutcome{
		Host: "fresh", Level: LevelNotRun, Status: AgentNotRun, Reason: "no binary",
	}
	after := ApplyJourneyOutcome(m, missing, "2026-11-17T00:00:00Z")
	if after.Class != ClassProbe {
		t.Fatalf("undated missing host must become probe, got %q", after.Class)
	}
}

func TestNotRunStrategyHostStaysStrategy(t *testing.T) {
	before := testManifests()
	missing := HostJourneyOutcome{
		Host: "devin", Level: LevelNotRun, Status: AgentNotRun,
		Reason: "devin ships desktop-only, no non-interactive CLI",
	}
	after := ApplyJourneyOutcome(before["devin"], missing, "2026-11-17T00:00:00Z")
	if after.Class != ClassStrategy {
		t.Fatalf("devin must stay strategy, got %q", after.Class)
	}
	for i, c := range after.Capabilities {
		if c.Status != before["devin"].Capabilities[i].Status {
			t.Fatalf("not-run must not touch devin capability %s", c.ID)
		}
	}
}

func TestFailOutcomeRecordsWithoutUpgrading(t *testing.T) {
	m := testManifests()["opencode"]
	// Downgrade one cap to prove failure does not upgrade it back.
	for i, c := range m.Capabilities {
		if c.ID == CapGate {
			m.Capabilities[i].Status = CapNotExecuted
			m.Capabilities[i].Evidence = "stale"
		}
	}
	failed := HostJourneyOutcome{
		Host: "opencode", Version: "v2.2.5",
		Level: LevelHost, Status: AgentFail, Reason: "gate: boom",
	}
	after := ApplyJourneyOutcome(m, failed, "2026-11-17T00:00:00Z")
	if after.Class != ClassJourney {
		t.Fatalf("failed journey is still a journey execution, got %q", after.Class)
	}
	got, _ := after.Capability(CapGate)
	if got.Status != CapNotExecuted {
		t.Fatalf("failure must not upgrade %s to %q", CapGate, got.Status)
	}
	if !strings.Contains(got.Evidence, "journey fallido") {
		t.Fatalf("failure must be recorded in evidence, got %q", got.Evidence)
	}
}

func TestPassOutcomeUpgradesCoveredOnly(t *testing.T) {
	m := testManifests()["devin"]
	// Devin has no probada caps; a green journey must upgrade exactly
	// the covered set and nothing else (hooks/filesystem stay).
	pass := HostJourneyOutcome{
		Host: "devin", Version: "desktop-1",
		Level: LevelHost, Status: AgentPass, Reason: "all green",
	}
	after := ApplyJourneyOutcome(m, pass, "2026-11-17T00:00:00Z")
	covered := map[string]bool{}
	for _, id := range JourneyCoveredCaps() {
		covered[id] = true
	}
	for _, c := range after.Capabilities {
		want := CapVerified
		if !covered[c.ID] {
			// Non-covered caps keep their declared state.
			before, _ := m.Capability(c.ID)
			want = before.Status
		}
		if c.Status != want {
			t.Fatalf("cap %s = %q, want %q", c.ID, c.Status, want)
		}
		if c.Status == CapVerified && !strings.Contains(c.Evidence, "journey green") {
			t.Fatalf("cap %s upgraded without journey evidence: %q", c.ID, c.Evidence)
		}
	}
}

func TestValidateNoProbeAsJourneyCatches(t *testing.T) {
	m := testManifests()["opencode"]
	m.Class = ClassProbe
	if err := ValidateNoProbeAsJourney(map[string]HostManifest{"opencode": m}); err == nil {
		t.Fatal("probe class with probada covered caps must fail validation")
	} else if !strings.Contains(err.Error(), "nunca es un journey") {
		t.Fatalf("violation must say a probe is never a journey, got %v", err)
	}
	strategy := testManifests()["devin"]
	if err := ValidateNoProbeAsJourney(map[string]HostManifest{"devin": strategy}); err != nil {
		t.Fatalf("strategy manifest must pass the probe rule: %v", err)
	}
}

func TestValidateAttributableCatches(t *testing.T) {
	m := testManifests()["opencode"]
	m.Capabilities[0].Evidence = ""
	if err := ValidateCapabilitiesAttributable(map[string]HostManifest{"opencode": m}); err == nil {
		t.Fatal("probada without evidence must fail")
	}
	m.Capabilities[0].Status = "a-medias"
	m.Capabilities[0].Evidence = "x"
	if err := ValidateCapabilitiesAttributable(map[string]HostManifest{"opencode": m}); err == nil {
		t.Fatal("fifth state word must fail")
	}
}

func TestRenderCapabilityMatrixClosedVocabulary(t *testing.T) {
	hosts := []string{"opencode", "claude", "codex", "devin", "ghost"}
	got := RenderCapabilityMatrix(hosts, testManifests())
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != len(CapabilityIDs())+2 {
		t.Fatalf("matrix has %d lines, want header + rule + %d caps", len(lines), len(CapabilityIDs()))
	}
	allowed := map[string]bool{
		CapVerified: true, CapSupported: true, CapInferred: true,
		CapPartial: true, CapDegraded: true,
		CapUnsupported: true, CapNotExecuted: true, CapUnknown: true,
	}
	for _, ln := range lines[2:] {
		cells := strings.Split(ln, "|")
		if len(cells) != len(hosts)+3 {
			t.Fatalf("row has %d cells, want %d: %q", len(cells), len(hosts)+3, ln)
		}
		for _, cell := range cells[2 : 2+len(hosts)] {
			if !allowed[strings.TrimSpace(cell)] {
				t.Fatalf("cell outside the closed vocabulary: %q in %q", cell, ln)
			}
		}
	}
	if !strings.Contains(got, "| devin |") && !strings.Contains(got, "devin") {
		t.Fatalf("devin column missing:\n%s", got)
	}
	// Ghost host renders desconocida, never pass.
	for _, ln := range lines[2:] {
		cells := strings.Split(ln, "|")
		if strings.TrimSpace(cells[2+len(hosts)-1]) != CapUnknown {
			t.Fatalf("unknown host must render desconocida: %q", ln)
		}
		if strings.Contains(ln, "pass") {
			t.Fatalf("matrix must never claim a pass: %q", ln)
		}
	}
	// Every state the manifests use appears somewhere in the full matrix.
	for _, want := range []string{CapVerified, CapSupported, CapInferred, CapUnsupported, CapNotExecuted, CapUnknown} {
		if !strings.Contains(got, want) {
			t.Fatalf("state %q missing from matrix:\n%s", want, got)
		}
	}
}

func TestNewHostEvidenceRedactsSecrets(t *testing.T) {
	m := testManifests()["opencode"]
	m.TraceHint = TraceHintFor("run_abc")
	config := map[string]any{
		"model":   "opus",
		"api_key": "sk-live_super-secret-value",
		"nested":  map[string]any{"token": "ghp_abcdef123456"},
	}
	artifacts := []string{
		"opencode --version -> v2.2.5",
		"login with api_key=sk-live_super-secret-value failed",
		"Authorization: bearer tokengoeshere123",
	}
	ev := NewHostEvidence(m, config, artifacts)
	if ev.Class != ClassJourney || ev.TraceHint == "" || ev.RecordedAt == "" {
		t.Fatalf("evidence must carry class, trace and date: %+v", ev)
	}
	raw := ev.Config["api_key"]
	if raw != "[redacted]" {
		t.Fatalf("config api_key = %v, want [redacted]", raw)
	}
	if nested, ok := ev.Config["nested"].(map[string]any); !ok || nested["token"] != "[redacted]" {
		t.Fatalf("nested token must be redacted: %v", ev.Config["nested"])
	}
	if ev.Config["model"] != "opus" {
		t.Fatalf("benign config must survive: %v", ev.Config["model"])
	}
	for _, a := range ev.Artifacts {
		if strings.Contains(a, "sk-live_super-secret-value") || strings.Contains(a, "ghp_abcdef123456") || strings.Contains(a, "tokengoeshere123") {
			t.Fatalf("artifact leaks a secret: %q", a)
		}
	}
	if !strings.Contains(ev.Artifacts[0], "v2.2.5") {
		t.Fatalf("redaction must preserve benign text: %q", ev.Artifacts[0])
	}
}

func TestTraceHintFormat(t *testing.T) {
	if got := TraceHintFor("run_abc123"); got != "regulus trace timeline --run run_abc123 --rebuild" {
		t.Fatalf("trace hint = %q", got)
	}
}

func TestDegradationCoversEveryHostCap(t *testing.T) {
	for host := range testManifests() {
		for _, id := range CapabilityIDs() {
			if strings.TrimSpace(DegradationFor(host, id)) == "" {
				t.Fatalf("degradation missing for %s/%s", host, id)
			}
		}
	}
	if !strings.Contains(DegradationFor("codex", CapHooks), "inline") {
		t.Fatalf("codex hooks must degrade inline: %q", DegradationFor("codex", CapHooks))
	}
	if !strings.Contains(DegradationFor("devin", CapFilesystem), "export/import") {
		t.Fatalf("devin filesystem must degrade to export/import: %q", DegradationFor("devin", CapFilesystem))
	}
}

func TestJourneySpecsRepeatable(t *testing.T) {
	manifests := testManifests()
	for _, host := range []string{"opencode", "codex"} {
		m := manifests[host]
		if !m.RepeatableInCI {
			t.Fatalf("%s journey must be repeatable in CI", host)
		}
		if len(m.JourneyCommands) == 0 || len(m.ProbeCommands) == 0 || len(m.AgentCommands) == 0 {
			t.Fatalf("%s must record probe, journey and agent commands", host)
		}
	}
	devin := manifests["devin"]
	if devin.RepeatableInCI || devin.Class != ClassStrategy {
		t.Fatalf("devin must be strategy-only, not CI-repeatable: %+v", devin)
	}
	if manifests["claude"].RepeatableInCI {
		t.Fatal("claude agent leg needs login: must not claim CI repeatability")
	}
}

// TestValidateEvidenceCoverageCatchesOverClaim pins the gap F16.1 audited: a
// dimension cannot be probada on the journey citation when JourneyCoveredCaps
// says the journey never exercises it. Before the wider vocabulary this passed
// silently, which is how twelve claims (three hosts x four dimensions) ended up
// citing evidence for work the fixture did not do.
func TestValidateEvidenceCoverageCatchesOverClaim(t *testing.T) {
	bad := HostManifest{Host: "codex", Capabilities: []HostCapability{
		verified(CapHooks, "declarada, nunca journey-met", "contract tests + "+journeyGreen),
	}}
	if err := ValidateEvidenceCoverage(map[string]HostManifest{"codex": bad}); err == nil {
		t.Fatal("probada on an uncovered dimension citing the journey must fail")
	}

	good := HostManifest{Host: "codex", Capabilities: []HostCapability{
		declared(CapHooks, "declarada, nunca journey-met", "contract tests"),
		inferred(CapLimits, "derivada de la versión", "contract tests"),
		verified(CapTools, "cubierta por el journey", journeyGreen),
	}}
	if err := ValidateEvidenceCoverage(map[string]HostManifest{"codex": good}); err != nil {
		t.Fatalf("soportada, inferida and a covered probada must pass: %v", err)
	}

	if err := ValidateEvidenceCoverage(DefaultHostManifests("2026-11-16")); err != nil {
		t.Fatalf("default manifests must satisfy the coverage rule: %v", err)
	}
	if err := ValidateCapabilitiesAttributable(DefaultHostManifests("2026-11-16")); err != nil {
		t.Fatalf("default manifests must satisfy the vocabulary rule: %v", err)
	}
}

// TestDeclaredIsNotVerified keeps the new states honest: they must never be
// aliases of probada, or the gap would reopen under a different word.
func TestDeclaredIsNotVerified(t *testing.T) {
	if got := declared(CapHooks, "d", "e").Status; got != CapSupported {
		t.Fatalf("declared status = %q, want %q", got, CapSupported)
	}
	if got := inferred(CapHooks, "d", "e").Status; got != CapInferred {
		t.Fatalf("inferred status = %q, want %q", got, CapInferred)
	}
	for _, s := range []string{CapSupported, CapInferred, CapPartial, CapDegraded} {
		if s == CapVerified {
			t.Fatalf("%q must not equal probada", s)
		}
	}
}

// TestRouteForMapsEveryState pins the F16.3 routing contract: only the states
// that assert absence or ignorance are unroutable, and every routable
// restricted state carries an explicit restriction instead of being dropped.
func TestRouteForMapsEveryState(t *testing.T) {
	cases := []struct {
		status     string
		routable   bool
		restricted bool
	}{
		{CapVerified, true, false},
		{CapDegraded, true, true},
		{CapPartial, true, true},
		{CapSupported, true, true},
		{CapInferred, true, true},
		{CapUnsupported, false, false},
		{CapNotExecuted, false, false},
		{CapUnknown, false, false},
	}
	for _, tc := range cases {
		d := RouteFor(HostCapability{ID: CapTools, Status: tc.status, Detail: "detalle", Evidence: "ev"})
		if d.Routable != tc.routable {
			t.Fatalf("%s routable = %v, want %v", tc.status, d.Routable, tc.routable)
		}
		if tc.restricted && d.Restriction == "" {
			t.Fatalf("%s must carry a restriction", tc.status)
		}
		if !tc.restricted && d.Restriction != "" {
			t.Fatalf("%s must not carry a restriction, got %q", tc.status, d.Restriction)
		}
	}
	if d := RouteFor(HostCapability{ID: CapTools, Status: CapPartial}); d.Restriction == "" {
		t.Fatal("a partial capability without detail must still describe its restriction")
	}
}
