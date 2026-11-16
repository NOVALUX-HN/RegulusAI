// Agent Lab tests (F11.2/NOV-88): registry shape, isolation,
// replay, cancellation and the F11.1 trace link. Shell checks only
// (no toolchain): every fixture check runs through bash like the
// corpus shell tasks do.
package bench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/observability"
	"regulus-ai/internal/trace"
)

// labFixAgent is a deterministic stand-in for a headless host: it
// writes fixed bodies into the fixture and optionally fails, sleeps
// or counts calls. No model, no network.
type labFixAgent struct {
	writes   map[string]string
	failErr  error
	failLeft int
	sleep    time.Duration
	calls    *int
	usage    trace.HostUsage
}

func (a *labFixAgent) Run(ctx context.Context, workdir, prompt, model string) (string, trace.HostUsage, error) {
	if a.calls != nil {
		*a.calls++
	}
	if a.sleep > 0 {
		select {
		case <-ctx.Done():
			return "", trace.HostUsage{}, ctx.Err()
		case <-time.After(a.sleep):
		}
	}
	if a.failLeft > 0 {
		a.failLeft--
		if a.failErr == nil {
			a.failErr = errors.New("host exploded")
		}
		return "", trace.HostUsage{}, a.failErr
	}
	for name, body := range a.writes {
		p := filepath.Join(workdir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", trace.HostUsage{}, err
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return "", trace.HostUsage{}, err
		}
	}
	u := a.usage
	if u.Source == "" && (u.InputTokens > 0 || u.OutputTokens > 0) {
		u.Source = trace.SourceMeasuredHost
	}
	return "lab-fixed", u, nil
}

// labShellTask is a minimal shell fixture: the agent must create
// out.txt with the word "fixed".
func labShellTask() AgentTask {
	return AgentTask{
		ID: "LAB1", Title: "write fixed", Lang: "shell",
		Prompt:  "Create out.txt containing the word fixed.",
		Files:   map[string]string{"in.txt": "x\n"},
		Check:   []string{"bash", "-c", `grep -q fixed out.txt`},
		BudgetS: 60,
	}
}

func labTestSplits() AgentSplits {
	return AgentSplits{Version: "lab-t", Seed: 11, Exploration: []string{"LAB1"}, Holdout: []string{}}
}

func mustLabAgent(t *testing.T, id string) LabAgent {
	t.Helper()
	a, ok := FindLabAgent(id)
	if !ok {
		t.Fatalf("lab agent %q not registered", id)
	}
	return a
}

func TestLabRegistryShape(t *testing.T) {
	reg := LabRegistry()
	if len(reg) < 2 {
		t.Fatalf("registry holds %d agents, want at least 2 comparable", len(reg))
	}
	seen := map[string]bool{}
	for _, a := range reg {
		if a.ID == "" || seen[a.ID] {
			t.Fatalf("duplicate or empty agent id: %q", a.ID)
		}
		seen[a.ID] = true
		if a.Responsibility == "" || a.Skill == "" || len(a.Tools) == 0 || a.Host == "" || len(a.Inputs) == 0 {
			t.Fatalf("agent %q misses inventory fields: %+v", a.ID, a)
		}
	}
	if _, ok := FindLabAgent("nope"); ok {
		t.Fatal("unknown agent must not resolve")
	}
}

func TestLabInstructionsControlIsVerbatim(t *testing.T) {
	base := mustLabAgent(t, "baseline")
	if got := LabInstructions(base, "/work", "Do X."); got != "Do X." {
		t.Fatalf("baseline must send the task prompt verbatim, got %q", got)
	}
	anch := mustLabAgent(t, "anchored")
	got := LabInstructions(anch, "/work", "Do X.")
	for _, want := range []string{"/work", "Do X.", anch.Prompt} {
		if !strings.Contains(got, want) {
			t.Fatalf("anchored instructions miss %q:\n%s", want, got)
		}
	}
}

func TestValidateLabConfig(t *testing.T) {
	corpus := []AgentTask{labShellTask()}
	okSingle := LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1"}
	if err := ValidateLabConfig(okSingle, corpus); err != nil {
		t.Fatalf("valid single rejected: %v", err)
	}
	okCompare := LabConfig{Mode: LabModeCompare, Agents: []string{"baseline", "anchored"}, Task: "LAB1"}
	if err := ValidateLabConfig(okCompare, corpus); err != nil {
		t.Fatalf("valid compare rejected: %v", err)
	}
	cases := map[string]LabConfig{
		"bad mode":          {Mode: "corpus", Agent: "baseline", Task: "LAB1"},
		"unknown agent":     {Mode: LabModeSingle, Agent: "ghost", Task: "LAB1"},
		"unknown task":      {Mode: LabModeSingle, Agent: "baseline", Task: "NOPE"},
		"empty task":        {Mode: LabModeSingle, Agent: "baseline"},
		"single compare":    {Mode: LabModeCompare, Agents: []string{"baseline"}, Task: "LAB1"},
		"dup agents":        {Mode: LabModeCompare, Agents: []string{"baseline", "baseline"}, Task: "LAB1"},
		"agent in cmp":      {Mode: LabModeCompare, Agent: "baseline", Agents: []string{"baseline", "anchored"}, Task: "LAB1"},
		"agents in sng":     {Mode: LabModeSingle, Agent: "baseline", Agents: []string{"anchored"}, Task: "LAB1"},
		"neg budget":        {Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", BudgetS: -1},
		"neg retries":       {Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", Retries: -1},
		"retries unbounded": {Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", Retries: MaxLabRetries + 1},
	}
	for name, cfg := range cases {
		if err := ValidateLabConfig(cfg, corpus); err == nil {
			t.Fatalf("%s must fail validation", name)
		}
	}
}

func TestLabConfigReplayRoundTrip(t *testing.T) {
	corpus := []AgentTask{labShellTask()}
	cfg := LabConfig{Mode: LabModeSingle, Agent: "anchored", Task: "LAB1", BudgetS: 30, Seed: 11, Retries: 1, Variant: "v"}
	path := filepath.Join(t.TempDir(), "lab.json")
	if err := ExportLabConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	back, err := LoadLabConfig(path, corpus)
	if err != nil {
		t.Fatal(err)
	}
	if back.Agent != "anchored" || back.Task != "LAB1" || back.BudgetS != 30 || back.Seed != 11 || back.Retries != 1 {
		t.Fatalf("replay changed the config: %+v", back)
	}
	if back.LabVersion != LabVersion || back.SuiteVersion != AgentSuiteVersion {
		t.Fatalf("replay must stamp versions: %+v", back)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(raw), `"single"`, `"corpus"`, 1)
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLabConfig(path, corpus); err == nil {
		t.Fatal("tampered mode must fail replay")
	}
	if _, err := LoadLabConfig(filepath.Join(t.TempDir(), "missing.json"), corpus); err == nil {
		t.Fatal("missing config must fail replay")
	}
}

func TestRunLabPassRecordsFullReport(t *testing.T) {
	root := t.TempDir()
	agent := mustLabAgent(t, "anchored")
	runner := &labFixAgent{
		writes: map[string]string{"out.txt": "fixed\n"},
		usage:  trace.HostUsage{InputTokens: 10, OutputTokens: 5, Source: trace.SourceMeasuredHost},
	}
	res := RunLab(context.Background(), runner, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "anchored", Task: "LAB1", Seed: 11}, root)
	if res.Verdict != LabPass {
		t.Fatalf("verdict = %q (%v), want pass", res.Verdict, res.Errors)
	}
	if res.Outcome.Status != AgentPass || res.Outcome.TaskID != "LAB1" {
		t.Fatalf("outcome must reuse the suite shape: %+v", res.Outcome)
	}
	if res.TokensIn != 10 || res.TokensOut != 5 || res.TokenSource != trace.SourceMeasuredHost {
		t.Fatalf("tokens must carry measured-host provenance: %+v", res)
	}
	if !strings.Contains(res.Instructions, "Create out.txt") || !strings.Contains(res.Instructions, agent.Prompt) {
		t.Fatalf("instructions must hold the exact prompt sent:\n%s", res.Instructions)
	}
	if len(res.ChangedFiles) != 1 || res.ChangedFiles[0] != "out.txt" {
		t.Fatalf("changes must name the agent file: %v", res.ChangedFiles)
	}
	if !res.WorkdirCleaned {
		t.Fatal("workspace must be removed after the run")
	}
	if md, ok := res.Metadata["workdir"].(string); !ok || md == "" {
		t.Fatal("metadata must record the isolated workdir")
	} else if _, err := os.Stat(md); !os.IsNotExist(err) {
		t.Fatalf("workdir %q must be gone, stat err = %v", md, err)
	}
	if res.RunID == "" || res.AgentExecutionID == "" || res.SessionID == "" || len(res.EventIDs) == 0 {
		t.Fatalf("result must link F11.1 ids: %+v", res)
	}
	if !strings.Contains(res.TraceHint, res.RunID) {
		t.Fatalf("trace hint must name the run: %q", res.TraceHint)
	}
	if len(res.Delegations) != 0 {
		t.Fatalf("the Lab never delegates, got %+v", res.Delegations)
	}
	if res.TimelineStatus != observability.ExecSucceeded {
		t.Fatalf("timeline status = %q, want succeeded", res.TimelineStatus)
	}
	tl, err := observability.ReconstructRun(root, res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Status != observability.ExecSucceeded || len(tl.Events) != len(res.EventIDs) {
		t.Fatalf("stored trace must rebuild the run: %+v", tl)
	}
}

func TestRunLabFailAndGaming(t *testing.T) {
	agent := mustLabAgent(t, "baseline")
	res := RunLab(context.Background(), &labFixAgent{}, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1"}, t.TempDir())
	if res.Verdict != LabFail {
		t.Fatalf("no-op verdict = %q, want fail", res.Verdict)
	}
	if len(res.Errors) == 0 {
		t.Fatal("fail must carry the reason")
	}
	gaming := AgentTask{
		ID: "LABT", Title: "t", Lang: "shell",
		Prompt:  "Keep data_test.txt intact and create out.txt with fixed.",
		Files:   map[string]string{"data_test.txt": "spec\n"},
		Check:   []string{"bash", "-c", `grep -q fixed out.txt`},
		BudgetS: 60,
	}
	tamper := &labFixAgent{writes: map[string]string{
		"out.txt":       "fixed\n",
		"data_test.txt": "neutered\n",
	}}
	res = RunLab(context.Background(), tamper, agent, gaming, "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LABT"}, t.TempDir())
	if res.Verdict != LabFail || !strings.Contains(strings.Join(res.Errors, " "), "gaming") {
		t.Fatalf("tampered tests must fail as gaming: %+v", res)
	}
}

func TestRunLabIsolatesWorkspaces(t *testing.T) {
	agent := mustLabAgent(t, "anchored")
	marker := &labFixAgent{writes: map[string]string{"out.txt": "fixed\n", "marker.txt": "mine\n"}}
	cfg := LabConfig{Mode: LabModeSingle, Agent: "anchored", Task: "LAB1", Seed: 11}
	a := RunLab(context.Background(), marker, agent, labShellTask(), "exploration", cfg, t.TempDir())
	b := RunLab(context.Background(), marker, agent, labShellTask(), "exploration", cfg, t.TempDir())
	if a.Verdict != LabPass || b.Verdict != LabPass {
		t.Fatalf("verdicts = %q, %q; want pass, pass", a.Verdict, b.Verdict)
	}
	wa, _ := a.Metadata["workdir"].(string)
	wb, _ := b.Metadata["workdir"].(string)
	if wa == "" || wa == wb {
		t.Fatalf("executions must use distinct workspaces: %q vs %q", wa, wb)
	}
	if a.RunID == b.RunID || a.SessionID == b.SessionID || a.AgentExecutionID == b.AgentExecutionID {
		t.Fatal("run, session and execution ids must be unique per execution")
	}
	// marker.txt is captured as a change, never leaks: the second run
	// diffs against the pristine fixture, so a leaked marker would
	// still show exactly one extra file either way — the workdir
	// identity above plus cleanup below is the isolation proof.
	if len(a.ChangedFiles) != 2 || len(b.ChangedFiles) != 2 {
		t.Fatalf("changes = %v, %v; want [marker.txt out.txt] each", a.ChangedFiles, b.ChangedFiles)
	}
}

func TestRunLabJailsCheckEnv(t *testing.T) {
	t.Setenv("LAB_TEST_SECRET_XYZ", "s3cr3t")
	task := AgentTask{
		ID: "LABE", Title: "env jail", Lang: "shell",
		Prompt:  "Nothing to do.",
		Files:   map[string]string{"f.txt": "x\n"},
		Check:   []string{"bash", "-c", `[ -n "$FIXTURE_DIR" ] && [ -f "$FIXTURE_DIR/f.txt" ] && [ -z "$LAB_TEST_SECRET_XYZ" ] && [ -n "$PATH" ]`},
		BudgetS: 60,
	}
	res := RunLab(context.Background(), &labFixAgent{}, mustLabAgent(t, "baseline"), task, "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LABE"}, t.TempDir())
	if res.Verdict != LabPass {
		t.Fatalf("jailed check must pass, verdict = %q errors = %v", res.Verdict, res.Errors)
	}
	env := labChildEnv([]string{"FIXTURE_DIR=/w"})
	joined := "\n" + strings.Join(env, "\n") + "\n"
	if !strings.Contains(joined, "\nPATH=") || !strings.Contains(joined, "\nFIXTURE_DIR=/w\n") {
		t.Fatalf("allowlist must keep PATH and extras: %v", env)
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "LAB_TEST_SECRET_XYZ=") {
			t.Fatalf("secrets must not reach the check: %v", env)
		}
	}
}

func TestRunLabCancellation(t *testing.T) {
	agent := mustLabAgent(t, "baseline")
	block := &labFixAgent{sleep: 30 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan LabResult, 1)
	go func() {
		done <- RunLab(ctx, block, agent, labShellTask(), "exploration",
			LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1"}, t.TempDir())
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case res := <-done:
		if res.Verdict != LabNotRun {
			t.Fatalf("canceled verdict = %q, want not-run", res.Verdict)
		}
		if !res.WorkdirCleaned {
			t.Fatal("canceled runs must still clean the workspace")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("canceled run did not stop")
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	res := RunLab(ctx2, block, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1"}, t.TempDir())
	if res.Verdict != LabNotRun {
		t.Fatalf("entry-cancel verdict = %q, want not-run", res.Verdict)
	}
}

func TestRunLabTimeoutAndRetry(t *testing.T) {
	agent := mustLabAgent(t, "baseline")
	slow := &labFixAgent{sleep: 30 * time.Second}
	res := RunLab(context.Background(), slow, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", BudgetS: 1}, t.TempDir())
	if res.Verdict != LabTimeout {
		t.Fatalf("slow verdict = %q, want timeout", res.Verdict)
	}
	calls := 0
	flaky := &labFixAgent{failErr: errors.New("boom"), usage: trace.HostUsage{InputTokens: 1, Source: trace.SourceMeasuredHost}}
	flaky.calls = &calls
	flaky.failLeft = 1
	res = RunLab(context.Background(), flaky, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", Retries: 1}, t.TempDir())
	if res.Verdict != LabFail {
		t.Fatalf("retried verdict = %q, want fail (host recovered, check fails)", res.Verdict)
	}
	if res.Retries != 1 || calls != 2 {
		t.Fatalf("retries = %d calls = %d, want 1 retry and 2 calls", res.Retries, calls)
	}
	calls = 0
	flaky2 := &labFixAgent{failErr: errors.New("boom")}
	flaky2.calls = &calls
	flaky2.failLeft = 5
	res = RunLab(context.Background(), flaky2, agent, labShellTask(), "exploration",
		LabConfig{Mode: LabModeSingle, Agent: "baseline", Task: "LAB1", Retries: 0}, t.TempDir())
	if res.Verdict != LabInfraError || calls != 1 {
		t.Fatalf("no-retry verdict = %q calls = %d, want infra-error after 1 call", res.Verdict, calls)
	}
}

func TestCompareLabSameTask(t *testing.T) {
	pass := &labFixAgent{writes: map[string]string{"out.txt": "fixed\n"}}
	fail := &labFixAgent{}
	cmp, err := CompareLab(context.Background(), []LabContestant{
		{Agent: mustLabAgent(t, "baseline"), Runner: fail},
		{Agent: mustLabAgent(t, "anchored"), Runner: pass},
	}, labShellTask(), "exploration",
		LabConfig{Mode: LabModeCompare, Agents: []string{"baseline", "anchored"}, Task: "LAB1"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cmp.Winner != "anchored" {
		t.Fatalf("winner = %q, want anchored: %+v", cmp.Winner, cmp)
	}
	if cmp.Evaluable != 2 {
		t.Fatalf("evaluable = %d, want 2", cmp.Evaluable)
	}
	for _, r := range cmp.Results {
		if r.TaskID != "LAB1" {
			t.Fatalf("compare must run the same task, got %q", r.TaskID)
		}
	}
	if _, err := CompareLab(context.Background(), []LabContestant{{Agent: mustLabAgent(t, "baseline")}},
		labShellTask(), "exploration", LabConfig{}, t.TempDir()); err == nil {
		t.Fatal("one contestant is not a comparison")
	}
}

func TestCompareLabVerdictTieAndNone(t *testing.T) {
	pass := func(id string) LabResult { return LabResult{AgentID: id, Verdict: LabPass} }
	winner, _, evaluable := CompareLabVerdict([]LabResult{pass("baseline"), pass("anchored")})
	if winner != "tie" || evaluable != 2 {
		t.Fatalf("tie = %q evaluable = %d, want tie/2", winner, evaluable)
	}
	winner, _, evaluable = CompareLabVerdict([]LabResult{
		{AgentID: "baseline", Verdict: LabInfraError},
		{AgentID: "anchored", Verdict: LabTimeout},
	})
	if winner != "none" || evaluable != 0 {
		t.Fatalf("none = %q evaluable = %d, want none/0", winner, evaluable)
	}
	winner, reason, _ := CompareLabVerdict([]LabResult{
		{AgentID: "baseline", Verdict: LabFail},
		{AgentID: "anchored", Verdict: LabTimeout},
	})
	if winner != "baseline" || !strings.Contains(reason, "fail") {
		t.Fatalf("fail beats timeout: %q (%s)", winner, reason)
	}
}

func TestLabEvidenceRoundTripAndTamper(t *testing.T) {
	dir := t.TempDir()
	cfg := LabConfig{Mode: LabModeCompare, Agents: []string{"baseline", "anchored"}, Task: "LAB1", Seed: 11}
	results := []LabResult{
		{AgentID: "baseline", Verdict: LabFail, RunID: "run_a", SessionID: "ses_a", TraceHint: "hint a",
			Outcome: AgentOutcome{TaskID: "LAB1", Split: "exploration", Status: AgentFail, Detail: "nope"}},
		{AgentID: "anchored", Verdict: LabPass, RunID: "run_b", SessionID: "ses_b", TraceHint: "hint b",
			Outcome: AgentOutcome{TaskID: "LAB1", Split: "exploration", Status: AgentPass}},
	}
	if _, err := WriteLabEvidence(dir, cfg, results, "abc123", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	rcfg, m, outcomes, rresults, err := ReadLabEvidence(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rcfg.Task != "LAB1" || len(rcfg.Agents) != 2 {
		t.Fatalf("config changed: %+v", rcfg)
	}
	if len(outcomes) != 2 || len(rresults) != 2 || rresults[1].Verdict != LabPass {
		t.Fatalf("reports changed: %+v", rresults)
	}
	if m.GitHead != "abc123" {
		t.Fatalf("manifest changed: %+v", m)
	}
	report := RenderLabReport(rcfg, m, rresults)
	for _, want := range []string{"agent-lab", "baseline: fail", "anchored: pass", "hint a", "winner anchored"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report misses %q:\n%s", want, report)
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, EvidenceOutcomesFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, EvidenceOutcomesFile), append(raw, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := ReadLabEvidence(dir); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("tampered outcomes must fail on sha256, got %v", err)
	}
}
