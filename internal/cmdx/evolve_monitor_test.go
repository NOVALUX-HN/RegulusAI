// Governed cycle-monitor cover (CRAP paydown sin cambiar
// comportamiento): tabla parse/healthy true-false-invalido/record/next
// con IO{Dir:tmp}. Los helpers puros (flag, campos, next) se pinean
// sin disco; el comando se ejerce contra un ciclo real en CANARY.
package cmdx

import (
	"strings"
	"testing"

	"regulus-ai/internal/evolve"
)

func monitorTestSpec() evolve.GovernedSpec {
	return evolve.GovernedSpec{
		Signals: []evolve.GovernedSignal{
			{Source: "quality", Ref: "quality/baseline/v3", Summary: "p90 drift on retry storms"},
		},
		Hypothesis: evolve.Hypothesis{
			Problem: "latency spikes on retry storms", RootCause: "unbounded retries",
			Change: "cap retries at 2 with backoff", Benefit: "lower p90 without pass-rate loss",
			PotentialRegressions: "flaky tasks may fail", EvaluationCriteria: "pass-rate delta >= 0 and p90 down",
		},
		Surface:       evolve.ValidationSurface{Kind: "workflow", Description: "retry policy only", Reversible: true, RevertCommand: "regulus evolve rollback --exp <id> --reason <t>"},
		Roles:         evolve.GovernedRoles{Proposer: "sextus", Evaluator: "andragon", Approver: "ada", Deployer: "ops"},
		MaxCandidates: 2,
	}
}

func monitorTestPlan() evolve.ValidationPlanSpec {
	return evolve.ValidationPlanSpec{
		Hypothesis: evolve.Hypothesis{
			Problem: "latency spikes", RootCause: "unbounded retries",
			Change: "cap retries", Benefit: "lower p90",
			PotentialRegressions: "flaky fails", EvaluationCriteria: "pass-rate delta >= 0",
		},
		Surface:          evolve.ValidationSurface{Kind: "workflow", Description: "retry policy only", Reversible: true, RevertCommand: "regulus evolve rollback --exp <id> --reason <t>"},
		Baseline:         "retry-baseline-v1",
		Candidate:        "retry-cap-2",
		Dataset:          "agent-tasks-v3",
		Tier:             "L2",
		Models:           []string{"model-a"},
		Seeds:            []int64{20261116},
		Repetitions:      1,
		Primary:          "pass-rate",
		Budget:           evolve.ValidationBudget{MaxRuns: 8, MaxWallTimeMS: 60000, MaxTokens: 100000},
		Implementer:      "sextus",
		Evaluator:        "andragon",
		EvaluatorVersion: "robust-compare-v1",
		Commands:         []string{"bin/regulus bench agent-tasks --reps 1 --split exploration"},
	}
}

func monitorTestTrial(arm, task, status string) evolve.ValidationTrial {
	return evolve.ValidationTrial{Arm: arm, TaskID: task, Seed: 20261116, Repetition: 0, Model: "model-a",
		Split: evolve.ValidationSplitExploration, Status: status, LatencyMS: 120, Tokens: 40, TokenSource: "measured-host"}
}

// monitorCanaryCycle declares a full governed cycle up to CANARY so
// cycle-monitor has a live canary to observe. Portable: tmp dir, sin
// red, sin shell.
func monitorCanaryCycle(t *testing.T, dir string) string {
	t.Helper()
	c, err := evolve.DeclareGovernedCycle(dir, monitorTestSpec())
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	if _, err := evolve.ProposeGovernedCandidate(dir, c.ID, "retry-cap-2", "cap retries at 2", "sextus"); err != nil {
		t.Fatalf("propose: %v", err)
	}
	p, err := evolve.DeclareValidationPlan(dir, monitorTestPlan())
	if err != nil {
		t.Fatalf("declare plan: %v", err)
	}
	if err := evolve.LinkGovernedPlan(dir, c.ID, p.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	if _, err := evolve.RecordValidationTrial(dir, p.ID, monitorTestTrial("baseline", "AT1", evolve.ManifestFail)); err != nil {
		t.Fatalf("baseline trial: %v", err)
	}
	if _, err := evolve.RecordValidationTrial(dir, p.ID, monitorTestTrial("candidate", "AT1", evolve.ManifestPass)); err != nil {
		t.Fatalf("candidate trial: %v", err)
	}
	if _, err := evolve.CompareGovernedCycle(dir, c.ID, "andragon"); err != nil {
		t.Fatalf("compare: %v", err)
	}
	if err := evolve.ReviewGovernedCycle(dir, c.ID, "mira", "pass", "probed sabotage controls: held"); err != nil {
		t.Fatalf("review: %v", err)
	}
	if err := evolve.ApproveGovernedCycle(dir, c.ID, evolve.GovernedApproval{By: "ada", Reason: "risk accepted: reversible workflow"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := evolve.EnterGovernedCanary(dir, c.ID, "ops",
		evolve.CanaryScope{Kind: "workflow", Reversible: true, Reason: "reversible prompt surface"},
		[]string{"p90 up 20%"}); err != nil {
		t.Fatalf("canary: %v", err)
	}
	return c.ID
}

func TestParseHealthyFlagTable(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		want  bool
		valid bool
	}{
		{name: "true", raw: "true", want: true, valid: true},
		{name: "TRUE spaces", raw: "  TRUE  ", want: true, valid: true},
		{name: "false", raw: "false", want: false, valid: true},
		{name: "False mixed", raw: "False", want: false, valid: true},
		{name: "invalid", raw: "maybe", want: false, valid: false},
		{name: "empty", raw: "", want: false, valid: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, valid := parseHealthyFlag(c.raw)
			if valid != c.valid || got != c.want {
				t.Fatalf("parseHealthyFlag(%q) = (%v,%v), want (%v,%v)", c.raw, got, valid, c.want, c.valid)
			}
		})
	}
}

func TestMonitorFieldsComplete(t *testing.T) {
	if !monitorFieldsComplete("c", "ops", "true", "detail", 0) {
		t.Fatal("complete fields must pass")
	}
	for name, args := range map[string]struct {
		cycle, by, healthy, detail string
		nArg                       int
	}{
		"missing cycle":   {cycle: "", by: "ops", healthy: "true", detail: "d"},
		"missing by":      {cycle: "c", by: "", healthy: "true", detail: "d"},
		"missing healthy": {cycle: "c", by: "ops", healthy: "", detail: "d"},
		"missing detail":  {cycle: "c", by: "ops", healthy: "true", detail: ""},
		"positional":      {cycle: "c", by: "ops", healthy: "true", detail: "d", nArg: 1},
	} {
		if monitorFieldsComplete(args.cycle, args.by, args.healthy, args.detail, args.nArg) {
			t.Fatalf("%s must be incomplete", name)
		}
	}
}

func TestMonitorNextHint(t *testing.T) {
	if got := monitorNextHint("gov_1", true); !strings.Contains(got, "cycle-promote --cycle gov_1") {
		t.Fatalf("healthy next must promote, got %q", got)
	}
	if got := monitorNextHint("gov_1", false); !strings.Contains(got, "cycle-rollback --cycle gov_1") {
		t.Fatalf("unhealthy next must roll back, got %q", got)
	}
}

func TestEvolveCycleMonitorCmdTable(t *testing.T) {
	t.Run("parse missing flags", func(t *testing.T) {
		io, _, errb := missionIO(t.TempDir())
		if code := io.EvolveCycleMonitorCmd(nil); code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(errb.String(), "regulus evolve validation") {
			t.Fatalf("must print usage, got %q", errb.String())
		}
	})
	t.Run("parse positional refused", func(t *testing.T) {
		io, _, _ := missionIO(t.TempDir())
		args := []string{"--cycle", "c", "--by", "ops", "--healthy", "true", "--detail", "d", "extra"}
		if code := io.EvolveCycleMonitorCmd(args); code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
	})
	t.Run("healthy invalid", func(t *testing.T) {
		io, _, errb := missionIO(t.TempDir())
		args := []string{"--cycle", "c", "--by", "ops", "--healthy", "maybe", "--detail", "d"}
		if code := io.EvolveCycleMonitorCmd(args); code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(errb.String(), "regulus evolve validation") {
			t.Fatalf("invalid healthy must print usage, got %q", errb.String())
		}
	})
	t.Run("record failure", func(t *testing.T) {
		dir := t.TempDir()
		io, _, errb := missionIO(dir)
		args := []string{"--cycle", "gov_missing", "--by", "ops", "--healthy", "true", "--detail", "flat"}
		if code := io.EvolveCycleMonitorCmd(args); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		if !strings.Contains(errb.String(), "cycle-monitor:") {
			t.Fatalf("record failure must name the command, got %q", errb.String())
		}
	})
	t.Run("healthy true records and promotes", func(t *testing.T) {
		dir := t.TempDir()
		id := monitorCanaryCycle(t, dir)
		io, out, _ := missionIO(dir)
		args := []string{"--cycle", id, "--by", "ops", "--healthy", "true", "--detail", "p90 flat over 50 runs"}
		if code := io.EvolveCycleMonitorCmd(args); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		got := out.String()
		if !strings.Contains(got, "MONITOR_RECORDED "+id+" healthy=true") {
			t.Fatalf("must record healthy=true, got %q", got)
		}
		if !strings.Contains(got, "cycle-promote --cycle "+id) {
			t.Fatalf("healthy next must promote, got %q", got)
		}
	})
	t.Run("healthy false records and rolls back", func(t *testing.T) {
		dir := t.TempDir()
		id := monitorCanaryCycle(t, dir)
		io, out, _ := missionIO(dir)
		args := []string{"--cycle", id, "--by", "ops", "--healthy", "false", "--detail", "p90 up 35% on canary"}
		if code := io.EvolveCycleMonitorCmd(args); code != 0 {
			t.Fatalf("exit = %d", code)
		}
		got := out.String()
		if !strings.Contains(got, "MONITOR_RECORDED "+id+" healthy=false") {
			t.Fatalf("must record healthy=false, got %q", got)
		}
		if !strings.Contains(got, "cycle-rollback --cycle "+id) {
			t.Fatalf("unhealthy next must roll back, got %q", got)
		}
	})
}
