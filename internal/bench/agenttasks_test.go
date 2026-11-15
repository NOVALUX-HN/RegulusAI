// NOV-68: agent-task evals. Splits validation, honest bucketing,
// degrade path, and controls the robust evaluator must reject/accept.
package bench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evolve"
	"regulus-ai/internal/trace"
)

func writeSplits(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "splits.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAgentSplitsValid(t *testing.T) {
	corpus := AgentCorpus()
	// Derive a valid split from the corpus itself so the corpus can
	// grow without rewriting this test: last 20% holdout.
	var ids []string
	for _, task := range corpus {
		ids = append(ids, task.ID)
	}
	hold := (len(ids) + 4) / 5
	cut := len(ids) - hold
	doc := `{"version":"v","seed":1,"exploration":["` + strings.Join(ids[:cut], `","`) + `"],"holdout":["` + strings.Join(ids[cut:], `","`) + `"]}`
	sp, err := LoadAgentSplits(writeSplits(t, doc), corpus)
	if err != nil {
		t.Fatalf("valid splits rejected: %v", err)
	}
	if len(sp.Holdout) != hold {
		t.Fatalf("splits wrong: %+v", sp)
	}
}

func TestLoadAgentSplitsRejects(t *testing.T) {
	corpus := AgentCorpus()
	var ids []string
	for _, task := range corpus {
		ids = append(ids, task.ID)
	}
	all := `"` + strings.Join(ids, `","`) + `"`
	cases := map[string]string{
		"overlap":   `{"version":"v","seed":1,"exploration":[` + all + `],"holdout":["` + ids[0] + `"]}`,
		"unknown":   `{"version":"v","seed":1,"exploration":[` + all + `],"holdout":["ATX"]}`,
		"thin":      `{"version":"v","seed":1,"exploration":[` + all + `],"holdout":[]}`,
		"partial":   `{"version":"v","seed":1,"exploration":["` + ids[0] + `"],"holdout":["` + ids[1] + `"]}`,
		"extra-key": `{"version":"v","seed":1,"exploration":[` + all + `],"holdout":[],"x":1}`,
	}
	for name, doc := range cases {
		if _, err := LoadAgentSplits(writeSplits(t, doc), corpus); err == nil {
			t.Fatalf("%s must fail", name)
		}
	}
}

// stubAgent is a test-only stand-in for a headless host.
type stubAgent struct {
	fix   map[string]string // taskID -> fixed file body (nil = leave broken)
	sleep time.Duration
	fail  error
}

var stubFixes = map[string]struct{ file, body string }{
	"AT1": {"sum.go", "package calc\n\n// Sum returns the sum of xs.\nfunc Sum(xs []int) int {\n\ttotal := 0\n\tfor i := 0; i < len(xs); i++ {\n\t\ttotal += xs[i]\n\t}\n\treturn total\n}\n"},
	"AT2": {"rev.go", "package text\n\n// Reverse returns s reversed.\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r)\n}\n"},
	"AT3": {"median.js", "export function median(xs) {\n  const s = [...xs].sort((a, b) => a - b);\n  const m = Math.floor(s.length / 2);\n  if (s.length % 2 === 0) return (s[m - 1] + s[m]) / 2;\n  return s[m];\n}\n"},
	"AT4": {"sortnums.sh", "#!/usr/bin/env bash\nsort nums.txt\n"},
	"AT5": {"clamp.go", "package math2\n\n// Clamp restricts v to [lo, hi].\nfunc Clamp(v, lo, hi int) int {\n\tif v < lo {\n\t\treturn lo\n\t}\n\tif v > hi {\n\t\treturn hi\n\t}\n\treturn v\n}\n"},
}

func (s stubAgent) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	if s.fail != nil {
		return "", trace.HostUsage{}, s.fail
	}
	if s.sleep > 0 {
		select {
		case <-ctx.Done():
			return "", trace.HostUsage{}, ctx.Err()
		case <-time.After(s.sleep):
		}
	}
	if s.fix != nil {
		// Apply every known fix whose file exists in this fixture.
		for _, fx := range stubFixes {
			p := filepath.Join(workdir, fx.file)
			if _, err := os.Stat(p); err == nil {
				if err := os.WriteFile(p, []byte(fx.body), 0o644); err != nil {
					return "", trace.HostUsage{}, err
				}
			}
		}
		return "fixed", trace.HostUsage{InputTokens: 10, OutputTokens: 5, Source: trace.SourceMeasuredHost}, nil
	}
	return "no-op", trace.HostUsage{}, nil
}

func testSplits() AgentSplits {
	return AgentSplits{Version: "t", Seed: 7, Exploration: []string{"AT1", "AT2", "AT3", "AT4"}, Holdout: []string{"AT5"}}
}

func TestAgentSuitePositiveFixPasses(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{fix: map[string]string{}}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "candidate", Split: "exploration"})
	sum := SummarizeAgent("candidate", out)
	if sum.Evaluable != 4 || sum.Pass != 4 {
		t.Fatalf("fixing stub must pass 4/4 exploration, got %+v", sum)
	}
	for _, o := range out {
		if o.TokenSource != trace.SourceMeasuredHost {
			t.Fatalf("stub usage must label measured-host, got %q", o.TokenSource)
		}
	}
}

func TestAgentExplorationNeverTouchesHoldout(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{fix: map[string]string{}}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "exploration"})
	if len(out) != 4 {
		t.Fatalf("exploration must run 4 tasks, ran %d", len(out))
	}
	for _, o := range out {
		if o.TaskID == "AT5" {
			t.Fatal("holdout task AT5 must never run in exploration mode (anti-leakage)")
		}
		if o.Split != "exploration" {
			t.Fatalf("split label wrong: %+v", o)
		}
	}
	hold := RunAgentSuite(context.Background(), stubAgent{}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout"})
	if len(hold) != 1 || hold[0].TaskID != "AT5" {
		t.Fatalf("holdout must run only AT5, got %+v", hold)
	}
}

func TestAgentSuiteNoOpFixFails(t *testing.T) {
	out := RunAgentSuite(context.Background(), stubAgent{}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "baseline", Split: "exploration"})
	sum := SummarizeAgent("baseline", out)
	if sum.Evaluable != 4 || sum.Fail != 4 || sum.Pass != 0 {
		t.Fatalf("no-op stub must fail 4/4, got %+v", sum)
	}
}

func TestAgentSuiteBucketsTimeoutInfra(t *testing.T) {
	slow := RunAgentSuite(context.Background(), stubAgent{sleep: 5 * time.Second}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout", BudgetS: 1})
	if len(slow) != 1 || slow[0].Status != AgentTimeout {
		t.Fatalf("slow stub must time out, got %+v", slow)
	}
	broken := RunAgentSuite(context.Background(), stubAgent{fail: errors.New("boom")}, AgentCorpus(), testSplits(),
		AgentOptions{Reps: 1, Variant: "v", Split: "holdout"})
	if len(broken) != 1 || broken[0].Status != AgentInfra {
		t.Fatalf("failing stub must be infra, got %+v", broken)
	}
	if broken[0].Detail == "" {
		t.Fatal("infra must carry the reason")
	}
}

func TestAgentControlsRobustRejectAccept(t *testing.T) {
	neg := []evolve.ExplorationOutcome{
		{ID: "neg/AT1", Status: evolve.ExploreFail},
		{ID: "neg/AT2", Status: evolve.ExploreFail},
	}
	pos := []evolve.ExplorationOutcome{
		{ID: "pos/AT1", Status: evolve.ExplorePass},
		{ID: "pos/AT2", Status: evolve.ExplorePass},
	}
	base := []evolve.ExplorationOutcome{
		{ID: "base/AT1", Status: evolve.ExplorePass},
		{ID: "base/AT2", Status: evolve.ExploreFail},
	}
	g := evolve.RobustGuardrails{Functional: true, Compliance: true, NoRegression: true, Adversarial: true, Maintainable: true}
	negD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("neg", neg), g, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if negD.Verdict == evolve.RobustAdoptCandidate {
		t.Fatalf("degraded control must not adopt, got %v", negD.Verdict)
	}
	posD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("pos", pos), g, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if posD.Verdict != evolve.RobustAdoptCandidate {
		t.Fatalf("known improvement must adopt, got %v (%s)", posD.Verdict, posD.Reason)
	}
	badG := g
	badG.Adversarial = false
	guardD, err := evolve.CompareRobust(evolve.SummarizeRobust("base", base), evolve.SummarizeRobust("pos", pos), badG, evolve.RobustCost{}, evolve.RobustCost{}, evolve.DefaultRobustOptions())
	if err != nil {
		t.Fatal(err)
	}
	if guardD.Verdict != evolve.RobustRejectGuardrail {
		t.Fatalf("guardrail failure must reject, got %v", guardD.Verdict)
	}
}

func TestAppControlBlockIsInfraNotFail(t *testing.T) {
	blocked := AgentTask{ID: "ATX", Title: "x", Lang: "shell",
		Prompt: "x", Files: map[string]string{"f.txt": "x\n"},
		Check:   []string{"bash", "-c", "echo 'fork/exec x: Una directiva de Control de aplicaciones bloqueó este archivo.' >&2; exit 1"},
		BudgetS: 60}
	out := RunAgentSuite(context.Background(), stubAgent{}, []AgentTask{blocked},
		AgentSplits{Version: "t", Seed: 1, Exploration: []string{"ATX"}},
		AgentOptions{Reps: 1, Variant: "v", Split: "exploration"})
	if len(out) != 1 || out[0].Status != AgentInfra {
		t.Fatalf("blocked check must be infra, got %+v", out)
	}
}

func TestAgentSummarySeparatesBuckets(t *testing.T) {
	out := []AgentOutcome{
		{TaskID: "AT1", Rep: 0, Status: AgentPass},
		{TaskID: "AT1", Rep: 1, Status: AgentFail},
		{TaskID: "AT2", Rep: 0, Status: AgentTimeout},
		{TaskID: "AT2", Rep: 1, Status: AgentInfra},
		{TaskID: "AT3", Rep: 0, Status: AgentNotRun},
	}
	sum := SummarizeAgent("v", out)
	if sum.Evaluable != 2 || sum.Pass != 1 || sum.Fail != 1 || sum.Timeout != 1 || sum.Infra != 1 || sum.NotRun != 1 {
		t.Fatalf("buckets wrong: %+v", sum)
	}
	if sum.PassRate != 0.5 {
		t.Fatalf("pass-rate must be pass/evaluable = 0.5, got %v", sum.PassRate)
	}
	if sum.RepMin != 0.5 || sum.RepMax != 0.5 {
		t.Fatalf("rep spread wrong: %+v", sum)
	}
}

func TestToExplorationOutcomesMapsStatuses(t *testing.T) {
	out := ToExplorationOutcomes("v", []AgentOutcome{
		{TaskID: "A", Rep: 0, Status: AgentPass},
		{TaskID: "B", Rep: 0, Status: AgentFail},
		{TaskID: "C", Rep: 0, Status: AgentTimeout},
		{TaskID: "D", Rep: 0, Status: AgentInfra},
		{TaskID: "E", Rep: 0, Status: AgentNotRun},
	})
	want := map[string]string{"A": "pass", "B": "fail", "C": "timeout", "D": "infra", "E": "non-evaluable"}
	if len(out) != len(want) {
		t.Fatalf("got %d outcomes, want %d", len(out), len(want))
	}
	for _, o := range out {
		id := strings.TrimPrefix(o.ID, "v/")
		if want[id] == "" || string(o.Status) != want[id] {
			t.Fatalf("task %s mapped to %q, want %q", id, o.Status, want[id])
		}
	}
}

// NOV-77 v2 corpus: 30 tasks, 6 holdout (20%). Structural pins so the
// corpus can grow without silent rot: unique ids/titles, holdout share
// and language spread, budgets on every task, and the committed splits
// file must validate against the code corpus.
func TestAgentCorpusV2Shape(t *testing.T) {
	corpus := AgentCorpus()
	if len(corpus) < 30 {
		t.Fatalf("v2 corpus has %d tasks, want >= 30", len(corpus))
	}
	seenID, seenTitle := map[string]bool{}, map[string]bool{}
	langs := map[string]int{}
	for _, task := range corpus {
		if task.ID == "" || seenID[task.ID] {
			t.Fatalf("duplicate or empty task id: %q", task.ID)
		}
		seenID[task.ID] = true
		if task.Title == "" || seenTitle[task.Title] {
			t.Fatalf("duplicate or empty task title: %q", task.Title)
		}
		seenTitle[task.Title] = true
		if task.BudgetS <= 0 {
			t.Fatalf("task %s needs a positive budget", task.ID)
		}
		if task.Prompt == "" || len(task.Files) == 0 || len(task.Check) == 0 {
			t.Fatalf("task %s needs prompt, files and check", task.ID)
		}
		langs[task.Lang]++
	}
	_, thisFile, _, _ := runtime.Caller(0)
	sp, err := LoadAgentSplits(filepath.Join(filepath.Dir(thisFile), "..", "..", "bench", "agent-splits-v2.json"), corpus)
	if err != nil {
		t.Fatalf("committed v2 splits must validate: %v", err)
	}
	if got := float64(len(sp.Holdout)) / float64(len(corpus)); got < 0.2 {
		t.Fatalf("holdout share = %.3f, want >= 0.20", got)
	}
	holdLangs := map[string]bool{}
	for _, id := range sp.Holdout {
		for _, task := range corpus {
			if task.ID == id {
				holdLangs[task.Lang] = true
			}
		}
	}
	if len(holdLangs) < 3 {
		t.Fatalf("holdout must span >= 3 languages, got %v", holdLangs)
	}
}

// Every shipped fixture must fail its own check: a task that passes
// pre-fix is vacuous and can never discriminate. Runs every check for
// real (same binaries the agent gets); App Control blocks also fail
// the check, which still proves non-passing.
func TestAgentCorpusV2AllFailPreFix(t *testing.T) {
	for _, task := range AgentCorpus() {
		dir := t.TempDir()
		for name, body := range task.Files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if task.Lang == "go" {
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		cmd := exec.CommandContext(ctx, task.Check[0], task.Check[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		cancel()
		if err == nil {
			t.Fatalf("task %s passes pre-fix, it can never discriminate:\n%s", task.ID, out)
		}
	}
}

// Gaming traps must catch hardcoded answers and pass real fixes.
func TestAgentTrapsCatchHardcodedFix(t *testing.T) {
	byID := map[string]AgentTask{}
	for _, task := range AgentCorpus() {
		byID[task.ID] = task
	}
	cheats := map[string]map[string]string{
		"AT28": {"sum.go": "package calc\n\nfunc Sum(xs []int) int {\n\tif len(xs) == 3 {\n\t\treturn 6\n\t}\n\treturn -999\n}\n"},
		"AT29": {"median.js": "export function median(xs) {\n  if (xs.length === 4) return 2.5;\n  return 0;\n}\n"},
	}
	fixes := map[string]map[string]string{
		"AT28": {"sum.go": "package calc\n\nfunc Sum(xs []int) int {\n\ttotal := 0\n\tfor _, x := range xs {\n\t\ttotal += x\n\t}\n\treturn total\n}\n"},
		"AT29": {"median.js": "export function median(xs) {\n  const s = [...xs].sort((a, b) => a - b);\n  const m = Math.floor(s.length / 2);\n  if (s.length % 2 === 1) return s[m];\n  return (s[m - 1] + s[m]) / 2;\n}\n"},
	}
	for _, id := range []string{"AT28", "AT29"} {
		task := byID[id]
		runCheck := func(files map[string]string) error {
			dir := t.TempDir()
			merged := map[string]string{}
			for name, body := range task.Files {
				merged[name] = body
			}
			for name, body := range files {
				merged[name] = body
			}
			for name, body := range merged {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if task.Lang == "go" {
				if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.27\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, task.Check[0], task.Check[1:]...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("task %s check output:\n%s", id, out)
			}
			return err
		}
		if err := runCheck(cheats[id]); err == nil {
			t.Fatalf("trap %s must catch the hardcoded answer", id)
		}
		if err := runCheck(fixes[id]); err != nil {
			t.Fatalf("trap %s must pass the real fix", id)
		}
	}
}

// Trap hidden files must never be visible to the agent: the hidden
// check is created by the runner at check time, never shipped in Files.
func TestAgentTrapsHideTheirChecks(t *testing.T) {
	hidden := []string{"zz_hidden_test.go", "zz_hidden.test.mjs"}
	for _, task := range AgentCorpus() {
		if !strings.Contains(task.Title, "gaming trap") {
			continue
		}
		for f := range task.Files {
			for _, h := range hidden {
				if f == h {
					t.Fatalf("trap %s exposes its hidden check %q in Files", task.ID, h)
				}
			}
		}
	}
}

type tamperAgent struct{ stubAgent }

func (t tamperAgent) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	p := filepath.Join(workdir, "sum_test.go")
	_ = os.WriteFile(p, []byte("package calc\n\nimport \"testing\"\n\nfunc TestSum(t *testing.T) {}\n"), 0o644)
	return "neutered", trace.HostUsage{}, nil
}

func TestAgentTamperingWithTestsFails(t *testing.T) {
	corpus := AgentCorpus()
	var task AgentTask
	for _, cand := range corpus {
		if cand.ID == "AT1" {
			task = cand
		}
	}
	if task.ID == "" {
		t.Fatal("AT1 missing")
	}
	sp := AgentSplits{Version: "t", Seed: 1, Exploration: []string{"AT1"}, Holdout: []string{}}
	if _, err := LoadAgentSplits(writeSplits(t, `{"version":"v","seed":1,"exploration":["AT1"],"holdout":[]}`), []AgentTask{task}); err == nil {
		t.Fatal("holdout below 20% must fail even for single-task splits")
	}
	out := RunAgentSuite(context.Background(), tamperAgent{}, []AgentTask{task}, sp, AgentOptions{Reps: 1, Variant: "v", Split: "exploration"})
	if len(out) != 1 || out[0].Status != AgentFail || !strings.Contains(out[0].Detail, "gaming") {
		t.Fatalf("tampered tests must fail as gaming, got %+v", out)
	}
}
