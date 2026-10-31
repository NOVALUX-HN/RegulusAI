package observability

// Phase 8 T3 (specs S3+S5): read-only metrics aggregation over existing
// stores, measured-vs-estimated budgets, redacting exporters. RED test:
// pins rate math on fixture receipts, empty-input zeros, measured flags
// and exporter failure surfacing before the implementation exists.

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func floatPtr(f float64) *float64 { return &f }

func int64Ptr(i int64) *int64 { return &i }

func seedMetricsReceipt(t *testing.T, root, task, gate, verdict string, durMS, cpuMS int64, metric *float64, at time.Time) {
	t.Helper()
	stdout, stdoutSHA := receipts.Capture("stdout of " + gate + " for " + task)
	stderr, stderrSHA := receipts.Capture("")
	exit := 0
	if verdict == receipts.VerdictFail {
		exit = 1
	}
	r := receipts.Receipt{
		Task: task, Gate: gate, Layer: 1,
		Command: "go test ./...", ExitCode: exit, Verdict: verdict,
		StartedAt: at, DurationMS: durMS, CPUTimeMS: cpuMS,
		Stdout: stdout, StdoutSHA: stdoutSHA, Stderr: stderr, StderrSHA: stderrSHA,
	}
	if metric != nil {
		r.MetricValue = metric
		r.MetricThreshold = floatPtr(0.9)
		r.MetricOperator = "gte"
	}
	if _, err := receipts.Write(evidence.TaskDir(root, task), r); err != nil {
		t.Fatalf("receipts.Write: %v", err)
	}
}

func seedMetricsWorld(t *testing.T, root string) {
	t.Helper()
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	// task-a: fully passing (complete). task-b: one metric-gated
	// failure (incomplete, regressed).
	seedMetricsReceipt(t, root, "task-a", "build", receipts.VerdictPass, 1000, 100, nil, base)
	seedMetricsReceipt(t, root, "task-a", "test", receipts.VerdictPass, 2000, 200, nil, base.Add(time.Second))
	seedMetricsReceipt(t, root, "task-b", "build", receipts.VerdictPass, 1500, 150, nil, base.Add(2*time.Second))
	seedMetricsReceipt(t, root, "task-b", "test", receipts.VerdictFail, 500, 50, floatPtr(0.5), base.Add(3*time.Second))

	for i, st := range []string{trace.StatusCompleted, trace.StatusError} {
		if _, err := trace.Record(root, trace.Entry{
			Session: "sess-1", Message: "m", Call: "c",
			Tool: "bash", Status: st,
		}, base.Add(time.Duration(10+i)*time.Second)); err != nil {
			t.Fatalf("trace.Record: %v", err)
		}
	}

	seedMetricsExperiment(t, root, "exp-keep", 3, true, evolve.StateAdopted)
	seedMetricsExperiment(t, root, "exp-drop", 2, false, evolve.StateRejected)
}

func seedMetricsExperiment(t *testing.T, root, id string, maxRuns int, probePass bool, terminal string) {
	t.Helper()
	h := evolve.Hypothesis{
		Problem: "p", RootCause: "r", Change: "c",
		Benefit: "b", PotentialRegressions: "pr", EvaluationCriteria: "ec",
	}
	exp, err := evolve.ProposeExperiment(root, evolve.ExperimentSpec{
		ID: id, Hypothesis: h, Baseline: "b", Candidate: "c",
		Dataset: "d", Domain: evolve.DomainDOM, Budget: evolve.Budget{MaxRuns: maxRuns},
	})
	if err != nil {
		t.Fatalf("ProposeExperiment: %v", err)
	}
	if err := exp.Advance(root, evolve.StateRunning); err != nil {
		t.Fatalf("Advance RUNNING: %v", err)
	}
	probe := func(e evolve.Experiment, attempt int) (evolve.ProbeOutcome, error) {
		return evolve.ProbeOutcome{Pass: probePass, Detail: "fixture"}, nil
	}
	if _, err := evolve.RunExperiments(root, evolve.StrategySequential, []evolve.Experiment{exp}, evolve.RunCap{}, probe); err != nil {
		t.Fatalf("RunExperiments: %v", err)
	}
	exp, err = evolve.LoadExperiment(root, id)
	if err != nil {
		t.Fatalf("LoadExperiment: %v", err)
	}
	if err := exp.SetOutcome("results", "no regression", "fixture decision"); err != nil {
		t.Fatalf("SetOutcome: %v", err)
	}
	if err := exp.Advance(root, evolve.StateEvaluated); err != nil {
		t.Fatalf("Advance EVALUATED: %v", err)
	}
	if err := exp.Advance(root, terminal); err != nil {
		t.Fatalf("Advance %s: %v", terminal, err)
	}
}

func assertNoNaN(t *testing.T, m Metrics) {
	t.Helper()
	for name, f := range map[string]float64{
		"task_completion_rate": m.TaskCompletionRate, "verification_pass_rate": m.VerificationPassRate,
		"verification_failure_rate": m.VerificationFailureRate, "guardian_disagreement_rate": m.GuardianDisagreementRate,
		"evidence_completeness": m.EvidenceCompleteness, "evolution_acceptance_rate": m.EvolutionAcceptanceRate,
		"regression_rate": m.RegressionRate, "mean_duration_ms": m.MeanDurationMS,
		"cost_per_successful_task": m.CostPerSuccessfulTask,
	} {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Errorf("metric %s is NaN/Inf: %v", name, f)
		}
	}
}

func TestMetricsRateMathOnFixtures(t *testing.T) {
	root := t.TempDir()
	seedMetricsWorld(t, root)

	m, err := Summarize(root, "")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	assertNoNaN(t, m)

	if m.ReceiptsTotal != 4 || m.ReceiptsPass != 3 || m.ReceiptsFail != 1 {
		t.Errorf("receipt counts = %d/%d/%d, want 4/3/1",
			m.ReceiptsTotal, m.ReceiptsPass, m.ReceiptsFail)
	}
	if m.Tasks != 2 {
		t.Errorf("tasks = %d, want 2", m.Tasks)
	}
	if m.TaskCompletionRate != 0.5 {
		t.Errorf("task completion rate = %v, want 0.5 (task-a only)", m.TaskCompletionRate)
	}
	if m.VerificationPassRate != 0.75 {
		t.Errorf("verification pass rate = %v, want 0.75", m.VerificationPassRate)
	}
	if m.VerificationFailureRate != 0.25 {
		t.Errorf("verification failure rate = %v, want 0.25", m.VerificationFailureRate)
	}
	if m.RegressionRate != 0.25 {
		t.Errorf("regression rate = %v, want 0.25 (one metric-gated failure of 4)", m.RegressionRate)
	}
	if m.EvidenceCompleteness != 1.0 {
		t.Errorf("evidence completeness = %v, want 1.0 (all pins verify)", m.EvidenceCompleteness)
	}
	if m.ExperimentsTotal != 2 || m.ExperimentsAdopted != 1 {
		t.Errorf("experiments = %d/%d adopted, want 2/1", m.ExperimentsTotal, m.ExperimentsAdopted)
	}
	if m.EvolutionAcceptanceRate != 0.5 {
		t.Errorf("evolution acceptance rate = %v, want 0.5", m.EvolutionAcceptanceRate)
	}
	if m.TotalDurationMS != 5000 {
		t.Errorf("total duration = %dms, want 5000ms", m.TotalDurationMS)
	}
	if m.MeanDurationMS != 1250 {
		t.Errorf("mean duration = %vms, want 1250ms", m.MeanDurationMS)
	}
	if m.TotalCPUTimeMS != 500 {
		t.Errorf("total cpu = %dms, want 500ms", m.TotalCPUTimeMS)
	}
	if m.ToolCalls != 2 {
		t.Errorf("tool calls = %d, want 2", m.ToolCalls)
	}

	// Sources with no backing store aggregate honestly: zero with an
	// explicit coverage note, never an invented number.
	if m.GuardianDisagreementRate != 0 || m.TokensTotal != 0 || m.CostPerSuccessfulTask != 0 {
		t.Errorf("unbacked metrics must be zero: guardian=%v tokens=%v cost=%v",
			m.GuardianDisagreementRate, m.TokensTotal, m.CostPerSuccessfulTask)
	}
	for _, name := range []string{"guardian_disagreement_rate", "token_usage", "model_costs"} {
		note, ok := m.Coverage[name]
		if !ok || strings.TrimSpace(note) == "" {
			t.Errorf("coverage note missing for unbacked metric %q", name)
		}
		found := false
		for _, u := range m.Unmeasured {
			if u == name {
				found = true
			}
		}
		if !found {
			t.Errorf("unmeasured list omits %q (have %v)", name, m.Unmeasured)
		}
	}
}

func TestMetricsEmptyInputZeros(t *testing.T) {
	m, err := Summarize(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Summarize on empty root: %v", err)
	}
	assertNoNaN(t, m)
	if m.ReceiptsTotal != 0 || m.Tasks != 0 || m.ExperimentsTotal != 0 || m.ToolCalls != 0 {
		t.Errorf("empty input must yield zero counts: %+v", m)
	}
	for name, f := range map[string]float64{
		"task_completion_rate": m.TaskCompletionRate, "verification_pass_rate": m.VerificationPassRate,
		"verification_failure_rate": m.VerificationFailureRate, "evidence_completeness": m.EvidenceCompleteness,
		"evolution_acceptance_rate": m.EvolutionAcceptanceRate, "regression_rate": m.RegressionRate,
		"cost_per_successful_task": m.CostPerSuccessfulTask,
	} {
		if f != 0 {
			t.Errorf("empty input: %s = %v, want 0", name, f)
		}
	}
	if m.TotalDurationMS != 0 || m.MeanDurationMS != 0 || m.TotalCPUTimeMS != 0 || m.TokensTotal != 0 {
		t.Errorf("empty input must yield zero totals: %+v", m)
	}
	if len(m.Unmeasured) == 0 {
		t.Error("empty input must still name unmeasured metrics")
	}
}

func TestBudgetMeasuredVsEstimated(t *testing.T) {
	root := t.TempDir()
	seedMetricsWorld(t, root)

	u, err := Usage(root, "task-a", "sess-1", nil)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	byName := map[string]BudgetItem{}
	for _, it := range u.Items {
		byName[it.Name] = it
	}

	exec, ok := byName["execution_time"]
	if !ok || !exec.Measured || exec.Value != 3000 || exec.Unit != "ms" {
		t.Errorf("execution_time must be measured 3000ms, got %+v (present=%v)", exec, ok)
	}
	tool, ok := byName["tool_usage"]
	if !ok || !tool.Measured || tool.Value != 2 {
		t.Errorf("tool_usage must be measured 2 calls, got %+v (present=%v)", tool, ok)
	}
	xp, ok := byName["experiment_budget"]
	if !ok || !xp.Measured || xp.Value != 3 {
		t.Errorf("experiment_budget must be measured 3 attempts used, got %+v (present=%v)", xp, ok)
	}
	if tok, ok := byName["token_usage"]; !ok || tok.Measured || tok.Value != 0 || strings.TrimSpace(tok.Reason) == "" {
		t.Errorf("token_usage without figures must be unmeasured zero with reason, got %+v (present=%v)", tok, ok)
	}
	if cost, ok := byName["model_costs"]; !ok || cost.Measured || cost.Value != 0 || strings.TrimSpace(cost.Reason) == "" {
		t.Errorf("model_costs without figures must be unmeasured zero with reason, got %+v (present=%v)", cost, ok)
	}

	// Caller-supplied reliable figures flip tokens/costs to measured.
	figs := &Figures{Tokens: int64Ptr(1000), ModelCost: floatPtr(0.02), Currency: "USD"}
	u2, err := Usage(root, "task-a", "sess-1", figs)
	if err != nil {
		t.Fatalf("Usage with figures: %v", err)
	}
	byName2 := map[string]BudgetItem{}
	for _, it := range u2.Items {
		byName2[it.Name] = it
	}
	if tok := byName2["token_usage"]; !tok.Measured || tok.Value != 1000 {
		t.Errorf("token_usage with figures must be measured 1000, got %+v", tok)
	}
	if cost := byName2["model_costs"]; !cost.Measured || cost.Value != 0.02 {
		t.Errorf("model_costs with figures must be measured 0.02, got %+v", cost)
	}
	// Measured execution figures are untouched by supplied estimates.
	if exec2 := byName2["execution_time"]; !exec2.Measured || exec2.Value != 3000 {
		t.Errorf("execution_time must stay measured 3000ms, got %+v", exec2)
	}
}

type failExporter struct{ err error }

func (f failExporter) Export(Payload) error { return f.err }

func TestExporterFailureSurfacing(t *testing.T) {
	want := errors.New("sink unavailable")
	var e Exporter = failExporter{err: want}
	if err := e.Export(Payload{Name: "metrics"}); !errors.Is(err, want) {
		t.Fatalf("failing exporter returned %v, want %v", err, want)
	}
}

func TestFileExporterRoundTripRedacted(t *testing.T) {
	root := t.TempDir()
	fx := FileExporter{Root: root, Scope: "metrics-test"}
	secret := "sk-live-EXPORTER-UNIQUE-999"
	p := Payload{
		Name:   "metrics snapshot",
		Labels: map[string]any{"api_key": secret, "note": "token=" + secret, "task": "task-a"},
		Data:   map[string]any{"pass_rate": 0.75},
	}
	if err := fx.Export(p); err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := os.ReadFile(fx.Path())
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("export file leaks secret")
	}
	if !strings.Contains(string(raw), Redacted) {
		t.Errorf("export file has no %q marker", Redacted)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d JSONL lines, want 1", len(lines))
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("export line is not JSON: %v", err)
	}
	if _, ok := decoded["name"]; !ok {
		t.Errorf("export line missing name: %s", lines[0])
	}

	// The in-memory exporter keeps the same redacted payload for tests.
	var mem MemoryExporter
	if err := mem.Export(p); err != nil {
		t.Fatalf("MemoryExporter.Export: %v", err)
	}
	if len(mem.Payloads) != 1 {
		t.Fatalf("memory exporter kept %d payloads, want 1", len(mem.Payloads))
	}
	kept, _ := json.Marshal(mem.Payloads[0])
	if strings.Contains(string(kept), secret) {
		t.Errorf("memory exporter kept unredacted secret")
	}
}
