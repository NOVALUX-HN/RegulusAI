// Extended evidence-report tests (coverage-80/T3): ParseQuality,
// OutcomeFromState refusals, criterion defaults/overrides, the
// redact* helpers, seal/Bytes/VerifyReport tamper paths and
// RenderReport over empty and full expedientes.
package mission

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseQualityAcceptsClosedVocabulary(t *testing.T) {
	for in, want := range map[string]string{
		"measured":   QualityMeasured,
		"Measured":   QualityMeasured,
		" MEASURED ": QualityMeasured,
		"estimated":  QualityEstimated,
		"Estimated":  QualityEstimated,
		"unknown":    QualityUnknown,
		"UNKNOWN":    QualityUnknown,
		"na":         QualityNA,
		"NA":         QualityNA,
		" Na ":       QualityNA,
	} {
		got, err := ParseQuality(in)
		if err != nil || got != want {
			t.Fatalf("ParseQuality(%q) = %q, %v (want %q)", in, got, err, want)
		}
	}
}

func TestParseQualityRefusesOpenVocabulary(t *testing.T) {
	for _, in := range []string{"", "  ", "good", "medido", "measured-ish", "n/a", "none", "measured estimated"} {
		if got, err := ParseQuality(in); err == nil {
			t.Fatalf("ParseQuality(%q) = %q, want refusal", in, got)
		} else if !strings.Contains(err.Error(), "unknown data quality") {
			t.Fatalf("ParseQuality(%q) must name the closed vocabulary, got %v", in, err)
		}
	}
}

func TestOutcomeFromStateRefusesNonTerminal(t *testing.T) {
	for _, state := range []string{
		StateCreated, StateAnalyzing, StatePlanning, StateAwaitingApproval,
		StateExecuting, StateIntegrating, StateValidating, StateRecovering,
		StateBlocked + "-x", "teleport", "",
	} {
		if got, err := OutcomeFromState(state); err == nil {
			t.Fatalf("OutcomeFromState(%q) = %q, want refusal (no report before the finish)", state, got)
		} else if !strings.Contains(err.Error(), "not terminal") {
			t.Fatalf("OutcomeFromState(%q) must say not-terminal, got %v", state, err)
		}
	}
}

func TestBuildReportNeedsMission(t *testing.T) {
	if _, err := BuildReport(ReportInput{}); err == nil {
		t.Fatal("a report without a mission must refuse")
	} else if !strings.Contains(err.Error(), "needs a mission") {
		t.Fatalf("must name the missing mission, got %v", err)
	}
}

func TestBuildReportValidatesMetricQuality(t *testing.T) {
	m := terminalFixture(t, StateCompleted)
	bad := sampleInput(m)
	bad.Costs = []Metric{{Label: "tokens", Quality: "precise-ish", Value: "42"}}
	if _, err := BuildReport(bad); err == nil {
		t.Fatal("an open cost quality must refuse")
	} else if !strings.Contains(err.Error(), "unknown data quality") {
		t.Fatalf("must name the bad quality, got %v", err)
	}
	bad = sampleInput(m)
	bad.Latency = Metric{Label: "wall-clock", Quality: "fast", Value: "9"}
	if _, err := BuildReport(bad); err == nil {
		t.Fatal("an open latency quality must refuse")
	} else if !strings.Contains(err.Error(), "unknown data quality") {
		t.Fatalf("must name the bad latency quality, got %v", err)
	}
	// Empty qualities stay unsettable: no metric, no validation.
	empty := sampleInput(m)
	empty.Costs = nil
	empty.Latency = Metric{}
	if _, err := BuildReport(empty); err != nil {
		t.Fatalf("empty metrics must not trip validation: %v", err)
	}
}

func TestDefaultCriteriaWithoutEvidenceStaysUnproven(t *testing.T) {
	m := terminalFixture(t, StateCompleted)
	r, err := BuildReport(ReportInput{Mission: m})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Criteria) != len(m.Criteria) {
		t.Fatalf("every mission criterion needs a verdict, got %d for %d", len(r.Criteria), len(m.Criteria))
	}
	for _, c := range r.Criteria {
		if c.Satisfied {
			t.Fatalf("plan-only runs must never claim satisfied criteria: %+v", c)
		}
		if !strings.Contains(c.Reason, "unproven") {
			t.Fatalf("open criteria must explain they are unproven, got %q", c.Reason)
		}
		if len(c.Evidence) != 0 {
			t.Fatalf("unproven criteria cite no evidence, got %v", c.Evidence)
		}
	}
}

func TestBuildReportWithCriteriaLengthMismatch(t *testing.T) {
	m := terminalFixture(t, StateCompleted)
	in := sampleInput(m)
	for _, criteria := range [][]CriterionReport{
		{{Name: "only one", Satisfied: true, Reason: "x"}},
		{{Name: "a", Satisfied: true, Reason: "x"}, {Name: "b", Satisfied: false, Reason: "y"}, {Name: "c", Satisfied: true, Reason: "z"}},
	} {
		if _, err := BuildReportWithCriteria(in, criteria); err == nil {
			t.Fatalf("verdicts (%d) for %d criteria must refuse", len(criteria), len(m.Criteria))
		} else if !strings.Contains(err.Error(), "every criterion needs a verdict") {
			t.Fatalf("must demand one verdict per criterion, got %v", err)
		}
	}
}

func TestBuildReportWithCriteriaSealsAndRedacts(t *testing.T) {
	m := terminalFixture(t, StateCompleted)
	in := sampleInput(m)
	verdicts := []CriterionReport{
		{Name: "engine persists state api_key=supersecret", Satisfied: true, Reason: "run-1 green", Evidence: []string{"run:run-1"}},
		{Name: "resume duplicates nothing", Satisfied: false, Reason: "token=abc123 blocks rerun", Evidence: nil},
	}
	r, err := BuildReportWithCriteria(in, verdicts)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Criteria) != 2 || !r.Criteria[0].Satisfied || r.Criteria[1].Satisfied {
		t.Fatalf("explicit verdicts must survive sealing: %+v", r.Criteria)
	}
	if strings.Contains(r.Criteria[0].Name, "supersecret") || strings.Contains(r.Criteria[1].Reason, "abc123") {
		t.Fatalf("criterion free text must redact secrets: %+v", r.Criteria)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReport(raw); err != nil {
		t.Fatalf("explicit-criteria report must verify: %v", err)
	}
}

func TestRedactHelpersCoverEveryFreeTextSection(t *testing.T) {
	m := terminalFixture(t, "failed")
	m.Objective = "rotate api_key=supersecret before shipping"
	m.Description = "notes token=abc123 for later"
	m.Constraints = []string{"keep api_key=supersecret out of logs"}
	in := sampleInput(m)
	in.Routing = []string{"t1→agent-a api_key=supersecret"}
	in.Limitations = []string{"e2e skipped: token=abc123 missing"}
	in.Recovery = []RecoveryEntry{{Action: "reassign", Reason: "token=abc123 leaked", Outcome: "rotated api_key=supersecret"}}
	r, err := BuildReport(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range append(append(append([]string{r.Objective, r.Description}, r.Constraints...), r.Routing...), r.Limitations...) {
		if strings.Contains(s, "supersecret") || strings.Contains(s, "abc123") {
			t.Fatalf("free-text section leaked a secret: %q", s)
		}
	}
	if strings.Contains(r.Recovery[0].Reason, "abc123") || strings.Contains(r.Recovery[0].Outcome, "supersecret") {
		t.Fatalf("recovery free text leaked a secret: %+v", r.Recovery[0])
	}
	// Nil free-text slices stay nil (no invented sections).
	empty, err := BuildReport(ReportInput{Mission: terminalFixture(t, StateCompleted)})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Routing != nil || empty.Limitations != nil {
		t.Fatalf("nil sections must stay nil, no invented sections: %+v", empty)
	}
}

func TestBytesRefusesUnsealed(t *testing.T) {
	var r Report
	if _, err := r.Bytes(); err == nil {
		t.Fatal("an unsealed report must not export bytes")
	} else if !strings.Contains(err.Error(), "unsealed") {
		t.Fatalf("must name the unsealed report, got %v", err)
	}
	r.Hash = "  "
	if _, err := r.Bytes(); err == nil {
		t.Fatal("a blank hash is not a seal")
	}
}

func TestVerifyReportRefusesMalformed(t *testing.T) {
	if _, err := VerifyReport([]byte("{not json")); err == nil {
		t.Fatal("malformed bytes must refuse")
	} else if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("must name the malformed report, got %v", err)
	}
	if _, err := VerifyReport(nil); err == nil {
		t.Fatal("empty bytes must refuse")
	}
}

func TestVerifyReportRefusesUnknownVersion(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StateCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["version"] = "regulus.mission-report/v99"
	mut, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReport(mut); err == nil {
		t.Fatal("an unknown schema version must refuse, never guess")
	} else if !strings.Contains(err.Error(), "refusing to guess") {
		t.Fatalf("must refuse to guess versions, got %v", err)
	}
}

func TestVerifyReportRefusesUnknownOutcome(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StateCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["outcome"] = "done-ish"
	mut, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReport(mut); err == nil {
		t.Fatal("an open outcome vocabulary must refuse")
	} else if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("must name the unknown outcome, got %v", err)
	}
}

func TestVerifyReportRefusesMissingHash(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StateCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "hash")
	mut, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReport(mut); err == nil {
		t.Fatal("a hashless report must refuse (nothing proves the bytes)")
	} else if !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("must name the broken integrity, got %v", err)
	}
}

func TestVerifyReportDetectsMidBodyTamper(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StatePartiallyCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Flip a body byte (the objective), far from the trailing hash.
	idx := strings.Index(string(raw), "ship the mission engine")
	if idx < 0 {
		t.Fatal("fixture objective must be present in the sealed bytes")
	}
	raw[idx] ^= 0xFF
	if _, err := VerifyReport(raw); err == nil {
		t.Fatal("an edited body byte must fail verification")
	} else if !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("must name the broken integrity, got %v", err)
	}
}

func TestRenderReportEmpty(t *testing.T) {
	out := RenderReport(Report{})
	for _, want := range []string{"mission report", "outcome=", "plans: 0", "hash:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty render must still quote %q, got:\n%s", want, out)
		}
	}
}

func TestRenderReportFull(t *testing.T) {
	r, err := BuildReportWithCriteria(sampleInput(terminalFixture(t, StateCompleted)), []CriterionReport{
		{Name: "engine persists state", Satisfied: true, Reason: "run-1 green", Evidence: []string{"run:run-1"}},
		{Name: "resume duplicates nothing", Satisfied: false, Reason: "no rerun evidence", Evidence: nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := RenderReport(r)
	for _, want := range []string{
		"mission report", "outcome=completed", "ship the mission engine",
		"[SATISFIED]: engine persists state", "[UNMET]: resume duplicates nothing",
		"gate unit: PASS", "failure agent_unavailable recovered=true",
		"limitation: e2e ran on one host only",
		"[unknown tokens=unavailable: no metered host]",
		"hash: " + r.Hash,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("full render must quote %q, got:\n%s", want, out)
		}
	}
}
