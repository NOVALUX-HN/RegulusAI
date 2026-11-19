package cmdx

import (
	"bytes"
	"strings"
	"testing"

	"regulus-ai/internal/evolve"
)

func fixtureRunSpec() evolve.ValidationPlanSpec {
	return evolve.ValidationPlanSpec{
		Hypothesis: evolve.Hypothesis{
			Problem: "latency spikes on retry storms", RootCause: "unbounded retries",
			Change: "cap retries at 2 with backoff", Benefit: "lower p90 without pass-rate loss",
			PotentialRegressions: "flaky tasks may fail", EvaluationCriteria: "pass-rate delta >= 0",
		},
		Surface:          evolve.ValidationSurface{Kind: "workflow", Description: "retry policy only", Reversible: true, RevertCommand: "regulus evolve rollback --exp <id> --reason <t>"},
		Baseline:         "retry-baseline-v1",
		Candidate:        "retry-cap-2",
		Dataset:          "agent-tasks-v3",
		Tier:             "L2",
		Models:           []string{"model-a"},
		Seeds:            []int64{7},
		Repetitions:      1,
		Primary:          "pass-rate",
		Budget:           evolve.ValidationBudget{MaxRuns: 4, MaxWallTimeMS: 60000, MaxTokens: 1000},
		Implementer:      "sextus",
		Evaluator:        "andragon",
		EvaluatorVersion: "robust-compare-v1",
		Commands:         []string{"bin/regulus bench agent-tasks --reps 1 --split exploration"},
	}
}

func declareRunPlan(t *testing.T, dir string) evolve.ValidationPlan {
	t.Helper()
	p, err := evolve.DeclareValidationPlan(dir, fixtureRunSpec())
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	return p
}

func validRunArgs(plan string) []string {
	return []string{
		"--plan", plan, "--arm", "baseline", "--task", "t1",
		"--seed", "7", "--rep", "0", "--model", "model-a",
		"--status", "pass", "--latency-ms", "100", "--tokens", "10",
		"--source", "measured-host",
	}
}

func TestParseValidationRunMissingPlanRefuses(t *testing.T) {
	var errBuf bytes.Buffer
	_, code, ok := parseValidationRun(validRunArgs(""), false, &errBuf)
	if ok || code != 2 {
		t.Fatalf("missing plan must refuse with exit 2, got code=%d ok=%v", code, ok)
	}
	if got := errBuf.String(); got != evolveValidationUsage+"\n" {
		t.Fatalf("missing plan must print the usage byte for byte, got %q", got)
	}
}

func TestParseValidationRunMissingAuditRefusesHoldout(t *testing.T) {
	var errBuf bytes.Buffer
	_, code, ok := parseValidationRun(validRunArgs("val_1"), true, &errBuf)
	if ok || code != 2 {
		t.Fatalf("holdout without audit must refuse with exit 2, got code=%d ok=%v", code, ok)
	}
	if got := errBuf.String(); got != evolveValidationUsage+"\n" {
		t.Fatalf("holdout without audit must print the usage byte for byte, got %q", got)
	}
}

func TestParseValidationRunTrimsAndSplits(t *testing.T) {
	var errBuf bytes.Buffer
	args := []string{
		"--plan", " val_1 ", "--arm", " candidate ", "--task", " t1 ",
		"--seed", "7", "--rep", "0", "--model", " model-a ",
		"--status", " pass ", "--latency-ms", "100", "--tokens", "10",
		"--source", " measured-host ", "--receipt", " rc ", "--trace", " tr ",
	}
	req, code, ok := parseValidationRun(args, false, &errBuf)
	if !ok || code != 0 {
		t.Fatalf("valid args must parse, got code=%d ok=%v err=%q", code, ok, errBuf.String())
	}
	if req.plan != "val_1" || req.trial.Arm != "candidate" || req.trial.TaskID != "t1" ||
		req.trial.Model != "model-a" || req.trial.Status != "pass" || req.trial.TokenSource != "measured-host" ||
		req.trial.ReceiptID != "rc" || req.trial.TraceID != "tr" {
		t.Fatalf("parse must trim every field, got %+v", req)
	}
	if req.trial.Split != evolve.ValidationSplitExploration {
		t.Fatalf("exploration split = %q, want exploration", req.trial.Split)
	}
	req, _, ok = parseValidationRun(append(validRunArgs("val_1"), "--audit", "audit-9"), true, &errBuf)
	if !ok || req.trial.Split != evolve.ValidationSplitHoldout || req.audit != "audit-9" {
		t.Fatalf("holdout must split holdout with the audit ref, got %+v ok=%v", req, ok)
	}
}

func TestEvolveValidationRunRecordsTrial(t *testing.T) {
	dir := t.TempDir()
	p := declareRunPlan(t, dir)
	var outBuf, errBuf bytes.Buffer
	io := IO{Stdout: &outBuf, Stderr: &errBuf, Dir: dir}
	if code := io.EvolveValidationRunCmd(validRunArgs(p.ID), false); code != 0 {
		t.Fatalf("run = %d, want 0 (stderr %q)", code, errBuf.String())
	}
	want := "verdict: TRIAL_RECORDED " + p.ID + " baseline/t1 seed=7 rep=0\n" +
		"next: regulus evolve validation compare --plan " + p.ID + " (after the exploration matrix is complete)\n"
	if outBuf.String() != want {
		t.Fatalf("stdout = %q, want %q", outBuf.String(), want)
	}
	loaded, err := evolve.LoadValidationPlan(dir, p.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.State != evolve.ValidationRunning || len(loaded.Trials) != 1 {
		t.Fatalf("plan must flip to RUNNING with one trial, got %+v", loaded)
	}
}

func TestEvolveValidationRunUnknownPlanFails(t *testing.T) {
	dir := t.TempDir()
	var outBuf, errBuf bytes.Buffer
	io := IO{Stdout: &outBuf, Stderr: &errBuf, Dir: dir}
	if code := io.EvolveValidationRunCmd(validRunArgs("val_missing"), false); code != 1 {
		t.Fatalf("unknown plan must exit 1, got %d", code)
	}
	if !strings.HasPrefix(errBuf.String(), "evolve validation run: ") {
		t.Fatalf("stderr must carry the run prefix, got %q", errBuf.String())
	}
}

func TestEvolveValidationHoldoutSealedBelowL3(t *testing.T) {
	dir := t.TempDir()
	p := declareRunPlan(t, dir)
	var outBuf, errBuf bytes.Buffer
	io := IO{Stdout: &outBuf, Stderr: &errBuf, Dir: dir}
	args := append(validRunArgs(p.ID), "--audit", "audit-9")
	if code := io.EvolveValidationRunCmd(args, true); code != 1 {
		t.Fatalf("sealed holdout must exit 1, got %d", code)
	}
	if !strings.HasPrefix(errBuf.String(), "evolve validation holdout-run: ") {
		t.Fatalf("stderr must carry the holdout prefix, got %q", errBuf.String())
	}
}

func TestEvolveValidationRunFlagErrorExits2(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	io := IO{Stdout: &outBuf, Stderr: &errBuf, Dir: t.TempDir()}
	if code := io.EvolveValidationRunCmd([]string{"--bogus"}, false); code != 2 {
		t.Fatalf("flag error must exit 2, got %d", code)
	}
}
