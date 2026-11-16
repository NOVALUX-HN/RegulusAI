package cli

// F11.6 (NOV-92) CLI: pre-registration before trials, governed
// comparison, sealed decision and reproducible report. Handlers are
// exercised directly (same package); the parent wires the
// `validation` verb into evolveCmd plus the usage line.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/evolve"
)

func writeValSpec(t *testing.T, path string) {
	t.Helper()
	spec := map[string]any{
		"Hypothesis": map[string]string{
			"Problem": "latency spikes", "RootCause": "unbounded retries", "Change": "cap at 2",
			"Benefit": "lower p90", "PotentialRegressions": "flaky fails", "EvaluationCriteria": "delta >= 0",
		},
		"Surface": map[string]any{
			"kind": "workflow", "description": "retry policy", "reversible": true,
			"revert_command": "regulus evolve rollback --exp <id> --reason <t>",
		},
		"baseline": "retry-baseline-v1", "candidate": "retry-cap-2",
		"dataset": "agent-tasks-v3", "tier": "L2",
		"models": []string{"model-a"}, "seeds": []int64{20261116}, "repetitions": 1,
		"primary": "pass-rate", "min_delta": 0.0,
		"budget":      map[string]int64{"max_runs": 8, "max_wall_time_ms": 60000, "max_tokens": 100000},
		"implementer": "sextus", "evaluator": "andragon", "evaluator_version": "robust-compare-v1",
		"commands": []string{"bin/regulus bench agent-tasks --reps 1 --split exploration"},
	}
	raw, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func valCLI(dir string) (CLI, *strings.Builder, *strings.Builder) {
	var out, errb strings.Builder
	return CLI{Version: "test", Stdout: &out, Stderr: &errb, Dir: dir}, &out, &errb
}

func declareValCLI(t *testing.T, dir string) string {
	t.Helper()
	specFile := filepath.Join(dir, "plan.json")
	writeValSpec(t, specFile)
	c, out, _ := valCLI(dir)
	if code := c.evolveValidationCmd([]string{"declare", "--plan", specFile}); code != 0 {
		t.Fatalf("declare exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "VALIDATION_DECLARED val_") || !strings.Contains(got, "next:") {
		t.Fatalf("declare must print verdict + next, got %q", got)
	}
	plans, err := filepath.Glob(filepath.Join(evolve.ValidationDir(dir), "val_*.json"))
	if err != nil || len(plans) != 1 {
		t.Fatalf("one frozen plan must exist: %v %v", plans, err)
	}
	raw, _ := os.ReadFile(plans[0])
	var p evolve.ValidationPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func runValTrial(t *testing.T, dir, id, arm, task, status string) {
	t.Helper()
	c, _, _ := valCLI(dir)
	args := []string{"run", "--plan", id, "--arm", arm, "--task", task,
		"--seed", "20261116", "--rep", "0", "--model", "model-a",
		"--status", status, "--latency-ms", "120", "--tokens", "40",
		"--source", "measured-host", "--receipt", "rc_" + task, "--trace", "run_" + task}
	if code := c.evolveValidationCmd(args); code != 0 {
		t.Fatalf("run %s/%s exit = %d, want 0", arm, task, code)
	}
}

func TestEvolveValidationUsage(t *testing.T) {
	c, _, errb := valCLI(t.TempDir())
	if code := c.evolveValidationCmd(nil); code != 2 {
		t.Fatalf("no args: exit = %d, want 2", code)
	}
	if code := c.evolveValidationCmd([]string{"bogus"}); code != 2 {
		t.Fatalf("unknown verb: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve validation") {
		t.Fatalf("misuse must print usage, got %q", errb.String())
	}
}

func TestEvolveValidationDeclareRefusesSelfEval(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "plan.json")
	writeValSpec(t, specFile)
	raw, _ := os.ReadFile(specFile)
	bad := strings.Replace(string(raw), `"andragon"`, `"sextus"`, 1)
	if err := os.WriteFile(specFile, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := valCLI(dir)
	if code := c.evolveValidationCmd([]string{"declare", "--plan", specFile}); code != 1 {
		t.Fatalf("self-evaluation: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "independent") {
		t.Fatalf("error must name independence, got %q", errb.String())
	}
}

func TestEvolveValidationFullJourney(t *testing.T) {
	dir := t.TempDir()
	id := declareValCLI(t, dir)
	runValTrial(t, dir, id, "baseline", "AT1", "pass")
	runValTrial(t, dir, id, "candidate", "AT1", "pass")

	c, out, _ := valCLI(dir)
	if code := c.evolveValidationCmd([]string{"control", "--plan", id,
		"--kind", "negative", "--name", "sabotage", "--expectation", "KEEP_BASELINE",
		"--outcome", "KEEP_BASELINE", "--held", "true", "--detail", "broken candidate rejected"}); code != 0 {
		t.Fatalf("control exit = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "CONTROL_RECORDED") {
		t.Fatalf("control must print its verdict, got %q", out.String())
	}
	c2, out2, _ := valCLI(dir)
	if code := c2.evolveValidationCmd([]string{"discard", "--plan", id,
		"--candidate", "retry-cap-3", "--reason", "infra noise, not signal", "--by", "andragon"}); code != 0 {
		t.Fatalf("discard exit = %d, want 0", code)
	}
	if !strings.Contains(out2.String(), "DISCARDED") {
		t.Fatalf("discard must print its verdict, got %q", out2.String())
	}
	c3, out3, _ := valCLI(dir)
	if code := c3.evolveValidationCmd([]string{"compare", "--plan", id}); code != 0 {
		t.Fatalf("compare exit = %d, want 0", code)
	}
	got3 := out3.String()
	for _, want := range []string{"COMPARED", "pass-rate", "latency", "tokens measured", "no significance", "finalize"} {
		if !strings.Contains(got3, want) {
			t.Fatalf("compare must contain %q, got:\n%s", want, got3)
		}
	}
	c4, _, errb4 := valCLI(dir)
	if code := c4.evolveValidationCmd([]string{"finalize", "--plan", id,
		"--verdict", "TIE_COST_TIEBREAK", "--approved-by", "bot",
		"--reason", "ok", "--inherited", "keep", "--inherited-reason", "untouched",
		"--commits", "abc1234", "--manifests", "m.json"}); code != 1 {
		t.Fatalf("bot approval: exit = %d, want 1", code)
	}
	if !strings.Contains(errb4.String(), "human") {
		t.Fatalf("bot refusal must name the human rule, got %q", errb4.String())
	}
	c5, out5, _ := valCLI(dir)
	if code := c5.evolveValidationCmd([]string{"finalize", "--plan", id,
		"--verdict", "TIE_COST_TIEBREAK", "--approved-by", "andragon",
		"--reason", "tie keeps baseline; no regression adopted", "--inherited", "keep",
		"--inherited-reason", "NOV-71 trims stay; no inherited change touched",
		"--commits", "abc1234", "--manifests", "bench/evidence/v/manifest.json"}); code != 0 {
		t.Fatalf("finalize exit = %d, want 0", code)
	}
	if !strings.Contains(out5.String(), "FINALIZED") {
		t.Fatalf("finalize must print its verdict, got %q", out5.String())
	}
	c6, out6, _ := valCLI(dir)
	if code := c6.evolveValidationCmd([]string{"report", "--plan", id}); code != 0 {
		t.Fatalf("report exit = %d, want 0", code)
	}
	got6 := out6.String()
	for _, want := range []string{
		"retry-cap-2", "pass-rate", "evaluable", "latency", "tokens",
		"retry-cap-3", "andragon", "keep", "abc1234", "manifest.json",
		"sabotage", "TIE_COST_TIEBREAK", "no significance",
	} {
		if !strings.Contains(got6, want) {
			t.Fatalf("report must contain %q\n---\n%s", want, got6)
		}
	}
}

func TestEvolveValidationHoldoutSealed(t *testing.T) {
	dir := t.TempDir()
	id := declareValCLI(t, dir)
	base := []string{"holdout-run", "--plan", id, "--arm", "candidate", "--task", "AT5",
		"--seed", "20261116", "--rep", "0", "--model", "model-a",
		"--status", "pass", "--latency-ms", "120", "--tokens", "40", "--source", "measured-host"}
	c, _, errb := valCLI(dir)
	if code := c.evolveValidationCmd(base); code != 2 {
		t.Fatalf("holdout-run without --audit: exit = %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "usage: regulus evolve validation") {
		t.Fatalf("holdout misuse must print usage, got %q", errb.String())
	}
	// Sealed before the exploration verdict even with an audit ref:
	// the holdout confirms, it never opens the decision.
	c2, _, errb2 := valCLI(dir)
	if code := c2.evolveValidationCmd(append(base, "--audit", "audit: pre-release")); code != 1 {
		t.Fatalf("holdout before COMPARED: exit = %d, want 1", code)
	}
	if !strings.Contains(errb2.String(), "sealed") {
		t.Fatalf("seal error must name the gate, got %q", errb2.String())
	}
}
