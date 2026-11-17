// Mission evidence report tests (F13.7/NOV-109... NOV-110).
// Round-trip, integrity/tampering, redaction, partial/blocked
// missions, unknown data, multi-agent, recovery and report↔trace
// consistency live here; the stores they cite stay authoritative.
package mission

import (
	"strings"
	"testing"
	"time"
)

func terminalFixture(t *testing.T, state string) *Mission {
	t.Helper()
	m, err := NewMission(NewInput{
		Objective: "ship the mission engine",
		Criteria:  []string{"engine persists state", "resume duplicates nothing"},
		Risk:      "medium",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.State = state
	return m
}

func sampleInput(m *Mission) ReportInput {
	return ReportInput{
		Mission: m,
		Plans:   []PlanRef{{Version: 2, Digest: "plan-digest-abc", Revisions: 1, ChangeNote: "replanned after gate red"}},
		Tasks: []TaskReport{
			{ID: "t1", State: "succeeded", Agent: "agent-a", Host: "host-1", Commit: "deadbeef", Evidence: []string{"trace:tr-1", "receipt:rc-1"}},
			{ID: "t2", State: "failed", Agent: "agent-b", Host: "host-2", Evidence: []string{"trace:tr-2"}},
		},
		DAG:            [][2]string{{"t1", "t2"}},
		Agents:         []string{"agent-a", "agent-b"},
		Hosts:          []string{"host-1", "host-2"},
		Routing:        []string{"t1→agent-a: capability match", "t2→agent-b: isolated worktree"},
		ContextSources: []SourceRef{{Kind: "impact-graph", ID: "impact-9", Hash: "hash-1"}},
		Approvals:      []ApprovalRef{{ID: "apr_1", State: "APPROVED", Artifact: "deadbeef", Policy: "project@standard"}},
		Commits:        []string{"deadbeef"},
		Tests:          []TestRef{{Gate: "unit", Verdict: "PASS", RunID: "run-1"}, {Gate: "e2e", Verdict: "BLOCKED", Cause: "no windows host", RunID: "run-1"}},
		Failures:       []FailureEntry{{Class: "agent_unavailable", Detail: "agent-b went offline", Recovered: true}},
		Recovery:       []RecoveryEntry{{Action: "reassign", Reason: "agent offline", Outcome: "t2 retried on agent-c"}},
		Costs:          []Metric{{Label: "tokens", Quality: QualityUnknown, Value: "unavailable: no metered host"}},
		Latency:        Metric{Label: "wall-clock-ms", Quality: QualityEstimated, Value: "42000"},
		Limitations:    []string{"e2e ran on one host only"},
	}
}

func TestOutcomeFromStateTerminal(t *testing.T) {
	for state, want := range map[string]string{
		StateCompleted: "completed", StatePartiallyCompleted: "partially_completed",
		"blocked": "blocked", "failed": "failed", "cancelled": "cancelled",
	} {
		got, err := OutcomeFromState(state)
		if err != nil || got != want {
			t.Fatalf("OutcomeFromState(%q) = %q, %v (want %q)", state, got, err, want)
		}
	}
}

func TestBuildReportRefusesNonTerminal(t *testing.T) {
	m := terminalFixture(t, "executing")
	if _, err := BuildReport(sampleInput(m)); err == nil {
		t.Fatal("non-terminal mission must not produce an evidence report")
	}
}

func TestBuildReportRoundTrip(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StateCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back, err := VerifyReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if back.MissionID != r.MissionID || back.Outcome != "completed" || back.Hash != r.Hash {
		t.Fatalf("round-trip drift: %+v", back)
	}
	if len(back.Criteria) != 2 || back.Criteria[0].Satisfied != true {
		t.Fatalf("criteria not carried: %+v", back.Criteria)
	}
}

func TestVerifyReportDetectsTampering(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, StatePartiallyCompleted)))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-20] ^= 0xFF
	if _, err := VerifyReport(raw); err == nil {
		t.Fatal("tampered report must fail verification")
	}
}

func TestBuildReportRedactsSecrets(t *testing.T) {
	m := terminalFixture(t, "failed")
	m.Objective = "rotate api_key=supersecret before shipping"
	in := sampleInput(m)
	in.Failures = []FailureEntry{{Class: "validation_failed", Detail: "token=abc123 rejected", Recovered: false}}
	r, err := BuildReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.Objective, "supersecret") || strings.Contains(r.Failures[0].Detail, "abc123") {
		t.Fatalf("secrets leaked into report: %q / %q", r.Objective, r.Failures[0].Detail)
	}
}

func TestBuildReportKeepsUnknownAndFailures(t *testing.T) {
	r, err := BuildReport(sampleInput(terminalFixture(t, "blocked")))
	if err != nil {
		t.Fatal(err)
	}
	if r.Costs[0].Quality != QualityUnknown {
		t.Fatalf("unknown quality not preserved: %+v", r.Costs[0])
	}
	if len(r.Failures) != 1 || len(r.Recovery) != 1 || len(r.Limitations) != 1 {
		t.Fatalf("report hides failures/limitations: %+v", r)
	}
	if r.Outcome != "blocked" {
		t.Fatalf("outcome = %q, want blocked", r.Outcome)
	}
	if !strings.Contains(RenderReport(r), "blocked") {
		t.Fatal("readable summary must state the outcome")
	}
}
