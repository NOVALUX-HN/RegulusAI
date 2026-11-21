package cli

// F12.7 (NOV-102) CLI: the governed cycle through `evolve validation
// cycle-*`. Handlers are exercised through evolveValidationCmd (the
// same entry point production uses); the dispatch pin and usage line
// in library.go/cli.go stay with the parent.
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGovSpec(t *testing.T, path string, sensitive bool) {
	t.Helper()
	sens := map[string]bool{}
	if sensitive {
		sens = map[string]bool{"touches_policy": true}
	}
	spec := map[string]any{
		"signals": []any{
			map[string]string{"source": "quality", "ref": "quality/baseline/v3", "summary": "p90 drift on retry storms"},
			map[string]string{"source": "memory", "ref": "learn:retry-cap-pattern", "summary": "verified cap-at-2 pattern"},
		},
		"Hypothesis": map[string]string{
			"Problem": "latency spikes", "RootCause": "unbounded retries", "Change": "cap at 2",
			"Benefit": "lower p90", "PotentialRegressions": "flaky fails", "EvaluationCriteria": "delta >= 0",
		},
		"surface": map[string]any{
			"kind": "workflow", "description": "retry policy", "reversible": true,
			"revert_command": "regulus evolve rollback --exp <id> --reason <t>",
		},
		"roles": map[string]string{
			"proposer": "sextus", "evaluator": "andragon", "approver": "ada", "deployer": "ops",
		},
		"sensitivity":    sens,
		"max_candidates": 2,
	}
	raw, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func declareGovCLI(t *testing.T, dir string, sensitive bool) string {
	t.Helper()
	specFile := filepath.Join(dir, "cycle.json")
	writeGovSpec(t, specFile, sensitive)
	c, out, _ := valCLI(dir)
	if code := c.evolveValidationCmd([]string{"cycle-declare", "--spec", specFile}); code != 0 {
		t.Fatalf("cycle-declare exit = %d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "GOVERNED_DECLARED gov_") || !strings.Contains(got, "next:") {
		t.Fatalf("declare must print verdict + next, got %q", got)
	}
	plans, err := filepath.Glob(filepath.Join(dir, ".regulus", "evolution", "governed", "gov_*.json"))
	if err != nil || len(plans) != 1 {
		t.Fatalf("one frozen cycle must exist: %v %v", plans, err)
	}
	raw, _ := os.ReadFile(plans[0])
	var cyc map[string]any
	if err := json.Unmarshal(raw, &cyc); err != nil {
		t.Fatal(err)
	}
	id, _ := cyc["id"].(string)
	return id
}

func govRun(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	c, out, errb := valCLI(dir)
	code := c.evolveValidationCmd(args)
	return code, out.String(), errb.String()
}

func TestEvolveGovernedFullJourney(t *testing.T) {
	dir := t.TempDir()
	id := declareGovCLI(t, dir, false)

	if code, out, _ := govRun(t, dir, "cycle-propose", "--cycle", id, "--candidate", "retry-cap-2", "--content", "cap retries at 2 with backoff", "--by", "sextus"); code != 0 {
		t.Fatalf("propose exit = %d, want 0", code)
	} else if !strings.Contains(out, "CANDIDATE_RECORDED") {
		t.Fatalf("propose must print its verdict, got %q", out)
	}
	if code, _, errb := govRun(t, dir, "cycle-propose", "--cycle", id, "--candidate", "evil", "--content", "x", "--by", "andragon"); code != 1 {
		t.Fatalf("evaluator propose: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "proposer") {
		t.Fatalf("refusal must name the pinned proposer, got %q", errb)
	}

	plan := declareValCLI(t, dir)
	runValTrial(t, dir, plan, "baseline", "AT1", "fail")
	runValTrial(t, dir, plan, "candidate", "AT1", "pass")

	if code, out, _ := govRun(t, dir, "cycle-link", "--cycle", id, "--plan", plan); code != 0 {
		t.Fatalf("link exit = %d, want 0", code)
	} else if !strings.Contains(out, "CYCLE_LINKED") {
		t.Fatalf("link must print its verdict, got %q", out)
	}
	if code, out, _ := govRun(t, dir, "cycle-compare", "--cycle", id, "--by", "andragon"); code != 0 {
		t.Fatalf("compare exit = %d, want 0", code)
	} else {
		for _, want := range []string{"CYCLE_COMPARED", "quality:", "latency:", "cost:", "security:", "holdout:"} {
			if !strings.Contains(out, want) {
				t.Fatalf("compare must contain %q, got:\n%s", want, out)
			}
		}
	}
	if code, _, errb := govRun(t, dir, "cycle-review", "--cycle", id, "--reviewer", "sextus", "--verdict", "pass", "--reason", "x"); code != 1 {
		t.Fatalf("role-bound reviewer: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "outside") {
		t.Fatalf("refusal must name independence, got %q", errb)
	}
	if code, _, _ := govRun(t, dir, "cycle-review", "--cycle", id, "--reviewer", "mira", "--verdict", "pass", "--reason", "probed sabotage and markers: held"); code != 0 {
		t.Fatalf("review exit = %d, want 0", code)
	}
	if code, _, errb := govRun(t, dir, "cycle-approve", "--cycle", id, "--by", "bot", "--reason", "ok"); code != 1 {
		t.Fatalf("bot approval: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "pinned approver") && !strings.Contains(errb, "human") {
		t.Fatalf("refusal must name the approval rule, got %q", errb)
	}
	if code, _, _ := govRun(t, dir, "cycle-approve", "--cycle", id, "--by", "ada", "--reason", "risk accepted: reversible"); code != 0 {
		t.Fatalf("approve exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-canary", "--cycle", id, "--by", "ops", "--kind", "workflow", "--conditions", "p90 up 20%,pass-rate down", "--reason", "reversible surface"); code != 0 {
		t.Fatalf("canary exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-monitor", "--cycle", id, "--by", "ops", "--healthy", "true", "--detail", "p90 flat over 50 runs"); code != 0 {
		t.Fatalf("monitor exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-promote", "--cycle", id, "--by", "ops", "--winner", "retry-cap-2"); code != 0 {
		t.Fatalf("promote exit = %d, want 0", code)
	}
	if code, out, _ := govRun(t, dir, "cycle-report", "--cycle", id); code != 0 {
		t.Fatalf("report exit = %d, want 0", code)
	} else {
		for _, want := range []string{"retry-cap-2", "sextus", "andragon", "ada", "ops", "mira", "PROMOTED", "quality:", "cost:"} {
			if !strings.Contains(out, want) {
				t.Fatalf("report must contain %q\n---\n%s", want, out)
			}
		}
	}
}

func TestEvolveGovernedBlockedJourney(t *testing.T) {
	dir := t.TempDir()
	id := declareGovCLI(t, dir, false)
	if code, _, _ := govRun(t, dir, "cycle-propose", "--cycle", id, "--candidate", "retry-cap-2", "--content", "cap retries", "--by", "sextus"); code != 0 {
		t.Fatalf("propose exit = %d, want 0", code)
	}
	plan := declareValCLI(t, dir)
	runValTrial(t, dir, plan, "baseline", "AT1", "pass")
	runValTrial(t, dir, plan, "candidate", "AT1", "fail")
	if code, _, _ := govRun(t, dir, "cycle-link", "--cycle", id, "--plan", plan); code != 0 {
		t.Fatalf("link exit = %d, want 0", code)
	}
	if code, _, errb := govRun(t, dir, "cycle-compare", "--cycle", id, "--by", "andragon"); code != 1 {
		t.Fatalf("regression compare: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "BLOCKED") {
		t.Fatalf("error must say BLOCKED, got %q", errb)
	}
	if code, out, _ := govRun(t, dir, "cycle-report", "--cycle", id); code != 0 {
		t.Fatalf("blocked report exit = %d, want 0", code)
	} else if !strings.Contains(out, "BLOCKED") || !strings.Contains(out, "never promotes") {
		t.Fatalf("blocked report must close the loop:\n%s", out)
	}
}

func TestEvolveGovernedSensitiveAndRollback(t *testing.T) {
	dir := t.TempDir()
	id := declareGovCLI(t, dir, true)
	if code, _, _ := govRun(t, dir, "cycle-propose", "--cycle", id, "--candidate", "pol", "--content", "tighten policy", "--by", "sextus"); code != 0 {
		t.Fatalf("propose exit = %d, want 0", code)
	}
	plan := declareValCLI(t, dir)
	runValTrial(t, dir, plan, "baseline", "AT1", "fail")
	runValTrial(t, dir, plan, "candidate", "AT1", "pass")
	if code, _, _ := govRun(t, dir, "cycle-link", "--cycle", id, "--plan", plan); code != 0 {
		t.Fatalf("link exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-compare", "--cycle", id, "--by", "andragon"); code != 0 {
		t.Fatalf("compare exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-review", "--cycle", id, "--reviewer", "mira", "--verdict", "pass", "--reason", "probed: held"); code != 0 {
		t.Fatalf("review exit = %d, want 0", code)
	}
	if code, _, errb := govRun(t, dir, "cycle-approve", "--cycle", id, "--by", "ada", "--reason", "ok"); code != 1 {
		t.Fatalf("unpinned sensitive approval: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "commit") {
		t.Fatalf("refusal must name the hash binding, got %q", errb)
	}
	if code, _, _ := govRun(t, dir, "cycle-approve", "--cycle", id, "--by", "ada", "--reason", "policy diff reviewed", "--commit", "abc1234", "--policy", "mcp-policy/v7"); code != 0 {
		t.Fatalf("approve exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-canary", "--cycle", id, "--by", "ops", "--kind", "workflow", "--conditions", "p90 up 20%", "--reason", "safe"); code != 0 {
		t.Fatalf("canary exit = %d, want 0", code)
	}
	if code, _, errb := govRun(t, dir, "cycle-rollback", "--cycle", id, "--by", "ops", "--reason", "x", "--condition", "sunspots"); code != 1 {
		t.Fatalf("ad-hoc rollback: exit = %d, want 1", code)
	} else if !strings.Contains(errb, "preapproved") {
		t.Fatalf("refusal must name preapproval, got %q", errb)
	}
	if code, _, _ := govRun(t, dir, "cycle-monitor", "--cycle", id, "--by", "ops", "--healthy", "false", "--detail", "p90 up 35%"); code != 0 {
		t.Fatalf("monitor exit = %d, want 0", code)
	}
	if code, _, _ := govRun(t, dir, "cycle-rollback", "--cycle", id, "--by", "ops", "--reason", "p90 up 35%", "--condition", "p90 up 20%"); code != 0 {
		t.Fatalf("rollback exit = %d, want 0", code)
	}
	if code, out, _ := govRun(t, dir, "cycle-report", "--cycle", id); code != 0 {
		t.Fatalf("report exit = %d, want 0", code)
	} else if !strings.Contains(out, "ROLLED_BACK") || !strings.Contains(out, "abc1234") {
		t.Fatalf("rollback report must cite the rollback and the approval pins:\n%s", out)
	}
}

func TestEvolveGovernedDeclareRefusals(t *testing.T) {
	dir := t.TempDir()
	specFile := filepath.Join(dir, "cycle.json")
	writeGovSpec(t, specFile, false)
	raw, _ := os.ReadFile(specFile)
	bad := strings.Replace(string(raw), `"approver": "ada"`, `"approver": "sextus"`, 1)
	if err := os.WriteFile(specFile, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, errb := valCLI(dir)
	if code := c.evolveValidationCmd([]string{"cycle-declare", "--spec", specFile}); code != 1 {
		t.Fatalf("self-approval declare: exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "distinct") {
		t.Fatalf("refusal must name role separation, got %q", errb.String())
	}
}
