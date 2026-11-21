// F11.3/NOV-89: tier CLI paths. Levels list, selective planning with
// inclusion/omission reasons, the L3 policy escalation, and verified
// tier reports.
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"regulus-ai/internal/bench"
)

// tierTestEnv mirrors labTestEnv but keeps the L0 smoke tasks in
// exploration: the shared writeAgentSplits helper fills holdout from
// the corpus head (AT1 included), which would seal the smoke set and
// fail every L0 plan for fixture reasons instead of product reasons.
func tierTestEnv(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	corpus := bench.AgentCorpus()
	smoke := map[string]bool{}
	for _, id := range bench.TierSmokeIDs {
		smoke[id] = true
	}
	var rest []string
	for _, task := range corpus {
		if !smoke[task.ID] {
			rest = append(rest, task.ID)
		}
	}
	holdN := (len(corpus) + 4) / 5
	cut := len(rest) - holdN
	exploration := append(append([]string{}, bench.TierSmokeIDs...), rest[:cut]...)
	holdout := append([]string{}, rest[cut:]...)
	dir := t.TempDir()
	sub := filepath.Join(dir, "bench")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(sub, "agent-splits-v1.json")
	body := `{"version":"agent-splits-v1","seed":7,"exploration":["` + strings.Join(exploration, `","`) + `"],"holdout":["` + strings.Join(holdout, `","`) + `"]}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REGULUS_AGENT_SPLITS", p)
}

func TestBenchTiersListsLevels(t *testing.T) {
	tierTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "tiers"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	got := out.String()
	for _, want := range []string{"L0", "L1", "L2", "L3", "holdout sealed", "holdout included", "44 tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tiers must print %q:\n%s", want, got)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "tiers", "--json"}); code != 0 {
		t.Fatalf("json exit = %d, want 0", code)
	}
	var specs []bench.TierSpec
	if err := json.Unmarshal([]byte(out2.String()), &specs); err != nil {
		t.Fatalf("tiers --json must parse: %v\n%s", err, out2.String())
	}
	if len(specs) != 4 {
		t.Fatalf("tiers hold %d levels, want 4", len(specs))
	}
}

func TestBenchTierPlanAutoDocsSmoke(t *testing.T) {
	tierTestEnv(t)
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"bench", "tier-plan", "--tier", "auto", "--changed", "docs/eval-tiers.md"})
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"tier L0", "executed 4:", "AT1", "AT14", "AT19", "AT23", "omitted", "holdout sealed", "run: regulus bench agent-tasks --task", "--split exploration"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan must print %q:\n%s", want, got)
		}
	}
	for _, omission := range []string{"AT5", "AT15"} {
		if !strings.Contains(got, omission) {
			t.Fatalf("plan must omit holdout %s with reason:\n%s", omission, got)
		}
	}
}

func TestBenchTierPlanL3Full(t *testing.T) {
	tierTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "tier-plan", "--tier", "L3"}); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"tier L3", "executed 44:", "holdout: included", "omitted 0:", "--split all"} {
		if !strings.Contains(got, want) {
			t.Fatalf("L3 plan must print %q:\n%s", want, got)
		}
	}
}

func TestBenchTierPlanReleaseEscalates(t *testing.T) {
	tierTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "tier-plan", "--tier", "L0", "--release"}); code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	got := out.String()
	for _, want := range []string{"tier L3", "pre-release L3 required"} {
		if !strings.Contains(got, want) {
			t.Fatalf("release must escalate to L3 with the reason:\n%s", got)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "tier-plan", "--tier", "L1", "--changed", "a.go,b.go", "--last-l3", "2020-01-01T00:00:00Z"}); code != 0 {
		t.Fatalf("stale exit = %d, want 0\n%s", code, out2.String())
	}
	if !strings.Contains(out2.String(), "tier L3") {
		t.Fatalf("stale L3 must escalate to L3:\n%s", out2.String())
	}
}

func TestBenchTierPlanRejectsUsage(t *testing.T) {
	tierTestEnv(t)
	cases := [][]string{
		{"bench", "tier-plan", "--tier", "L9"},
		{"bench", "tier-plan", "--tier", "L1", "--capability", "cobol"},
		{"bench", "tier-plan", "--tier", "L0", "--capability", "go"},
		{"bench", "tier-plan", "--tier", "L2", "--jobs", "99"},
		{"bench", "tier-plan", "--tier", "L2", "extra"},
		{"bench", "tier-report"},
		{"bench", "tier-report", "--evidence-dir", "x", "extra"},
		{"bench", "tiers", "extra"},
	}
	for _, args := range cases {
		c, _, _ := newCLI(nil)
		if code := c.Run(args); code != 2 {
			t.Fatalf("%v exit = %d, want 2", args, code)
		}
	}
}

func TestBenchTierPlanJSONAndEvidence(t *testing.T) {
	tierTestEnv(t)
	dir := t.TempDir()
	ev := filepath.Join(dir, "ev")
	plan := filepath.Join(dir, "plan.json")
	c, out, _ := newCLI(nil)
	code := c.Run([]string{"bench", "tier-plan", "--tier", "L2", "--json", "--save", plan, "--evidence-dir", ev})
	if code != 0 {
		t.Fatalf("exit = %d, want 0\n%s", code, out.String())
	}
	var sel bench.TierSelection
	if err := json.Unmarshal([]byte(out.String()), &sel); err != nil {
		// The human lines (saved/recorded) precede the JSON: extract it.
		raw, rerr := os.ReadFile(plan)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if err := json.Unmarshal(raw, &sel); err != nil {
			t.Fatalf("saved plan must parse: %v", err)
		}
	}
	if sel.Tier != "L2" || len(sel.Included) == 0 || len(sel.Omitted) == 0 {
		t.Fatalf("L2 selection wrong: %+v", sel)
	}
	for _, f := range []string{bench.TierPlanFile, bench.TierPlanManifest} {
		if _, err := os.Stat(filepath.Join(ev, f)); err != nil {
			t.Fatalf("evidence must hold %s: %v", f, err)
		}
	}
	c2, out2, _ := newCLI(nil)
	if code := c2.Run([]string{"bench", "tier-report", "--evidence-dir", ev}); code != 0 {
		t.Fatalf("report exit = %d, want 0", code)
	}
	for _, want := range []string{"tier L2", "executed", "omitted", "holdout: sealed"} {
		if !strings.Contains(out2.String(), want) {
			t.Fatalf("report must print %q:\n%s", want, out2.String())
		}
	}
	raw, err := os.ReadFile(filepath.Join(ev, bench.TierPlanFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ev, bench.TierPlanFile), append(raw, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	c3, _, errb := newCLI(nil)
	if code := c3.Run([]string{"bench", "tier-report", "--evidence-dir", ev}); code != 1 {
		t.Fatalf("tampered exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "sha256") {
		t.Fatalf("tamper must name the sha256 mismatch, got %q", errb.String())
	}
}

func TestBenchTierPlanRunHintExecutes(t *testing.T) {
	tierTestEnv(t)
	c, out, _ := newCLI(nil)
	if code := c.Run([]string{"bench", "tier-plan", "--tier", "L0"}); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	hint := ""
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "run: ") {
			hint = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "run: "))
		}
	}
	if !strings.HasPrefix(hint, "regulus bench agent-tasks --task AT1,AT14,AT19,AT23 --split exploration") {
		t.Fatalf("run hint must target the L0 smoke set through agent-tasks, got %q", hint)
	}
}
