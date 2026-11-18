package planning

import (
	"strings"
	"testing"
	"time"
)

// Covers RenderPlan branches: headers, lists, task details, quality.
func TestRenderPlanCoverageBranches(t *testing.T) {
	p := &Plan{
		PlanVersion:    3,
		Kind:           KindSimple,
		MissionID:      "M1",
		Status:         "ready",
		Objective:      "ship it",
		ChangeReason:   "user asked",
		Author:         "tester",
		CreatedAt:      time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		RepoHead:       "abcdef1234567890",
		WorktreeStatus: "clean",
		Hypotheses:     []string{"h1"},
		Assumptions:    []string{"a1"},
		Risks:          []string{"r1"},
		OpenQuestions:  []Question{{Question: "q?", Why: "why", Blocks: "b"}},
		Deliverables:   []string{"d1"},
		Criteria: []Criterion{
			{Text: "c-ok", Verifiable: true},
			{Text: "c-no", Verifiable: false, UnverifiableWs: "vague"},
		},
		Tasks: []PlanTask{
			{
				ID:                   "T1",
				Title:                "do thing",
				Autonomy:             "auto",
				Agent:                "",
				Reviewer:             "rev",
				Objective:            "obj",
				Deliverable:          "del",
				DependsOn:            []string{"T0"},
				Excludes:             []string{"X"},
				Scope:                Scope{Files: []string{"a.go"}, Components: []string{"comp"}},
				RequiredCapabilities: []string{"go"},
				CapabilityEvidence:   []string{"ev1"},
				MinScopes:            []string{"s1"},
				ContextRefs:          []string{"ctx"},
				Tests:                []string{"TestX"},
				Verification:         Verification{Strategy: "run tests", Criteria: []string{"green"}},
			},
		},
	}
	out := RenderPlan(p)
	for _, want := range []string{
		"plan v3", "M1", "ready",
		"objective: ship it", "change: user asked",
		"tester", "hypothesis: h1", "assumption: a1", "risk: r1",
		"question: q?", "deliverable: d1",
		"c-ok", "NO verificable",
		"T1 do thing", "reviewer: rev",
		"depends_on: T0", "excludes: X",
		"components: comp", "evidence: ev1",
		"verify: run tests",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderPlan must contain %q, got:\n%s", want, out)
		}
	}
	// Empty author/repo path stays covered without panicking.
	min := &Plan{PlanVersion: 1, Kind: KindSimple, MissionID: "m", Status: "draft", Objective: "o"}
	if got := RenderPlan(min); !strings.Contains(got, "plan v1") {
		t.Fatalf("minimal RenderPlan must render, got %q", got)
	}
}
