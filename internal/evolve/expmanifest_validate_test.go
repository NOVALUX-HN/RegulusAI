package evolve

import "testing"

func validManifestFixture() ExperimentManifest {
	return ExperimentManifest{
		Schema:       ExpManifestSchema,
		Protocol:     F7ProtocolVersion,
		ExperimentID: "exp-1",
		Candidate:    ManifestCandidateRef{ManifestPath: "m.json", RunID: "r1", SHA256: "abc"},
		Baseline:     "base",
		Suite:        "macizo-bench-v3",
		SuiteVersion: "macizo-bench-v3",
		Split:        ManifestSplitRef{File: "s.json", Name: "f7-split-v1", Version: 1, SHA256: "def"},
		Seed:         7,
		Evaluator:    RobustEvaluatorName,
		EvaluatorVer: RobustEvaluatorVersion,
		Results:      []ManifestResult{{TaskID: "B1", Status: string(ExplorePass)}},
		Decisions:    []ManifestDecision{{Verdict: "ADOPT", By: "human"}},
	}
}

func TestValidateExperimentManifestBranches(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ExperimentManifest)
	}{
		{"schema", func(m *ExperimentManifest) { m.Schema = 0 }},
		{"protocol", func(m *ExperimentManifest) { m.Protocol = "x" }},
		{"experiment", func(m *ExperimentManifest) { m.ExperimentID = "" }},
		{"candidate-path", func(m *ExperimentManifest) { m.Candidate.ManifestPath = "" }},
		{"candidate-sha", func(m *ExperimentManifest) { m.Candidate.SHA256 = "" }},
		{"baseline", func(m *ExperimentManifest) { m.Baseline = "" }},
		{"suite", func(m *ExperimentManifest) { m.Suite = "" }},
		{"suite-version", func(m *ExperimentManifest) { m.SuiteVersion = "" }},
		{"split-file", func(m *ExperimentManifest) { m.Split.File = "" }},
		{"split-name", func(m *ExperimentManifest) { m.Split.Name = "" }},
		{"split-version", func(m *ExperimentManifest) { m.Split.Version = 0 }},
		{"split-sha", func(m *ExperimentManifest) { m.Split.SHA256 = "" }},
		{"seed", func(m *ExperimentManifest) { m.Seed = 0 }},
		{"evaluator", func(m *ExperimentManifest) { m.Evaluator = "" }},
		{"evaluator-ver", func(m *ExperimentManifest) { m.EvaluatorVer = "" }},
		{"result-task", func(m *ExperimentManifest) { m.Results[0].TaskID = "" }},
		{"result-status", func(m *ExperimentManifest) { m.Results[0].Status = "bogus" }},
		{"decision-verdict", func(m *ExperimentManifest) { m.Decisions = []ManifestDecision{{Verdict: ""}} }},
	}
	for _, tc := range cases {
		m := validManifestFixture()
		tc.mutate(&m)
		if err := ValidateExperimentManifest(m); err == nil {
			t.Fatalf("%s: must fail validation", tc.name)
		}
	}
	if err := ValidateExperimentManifest(validManifestFixture()); err != nil {
		t.Fatalf("valid manifest must pass: %v", err)
	}
}
