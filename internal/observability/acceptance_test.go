// Phase 8 T4 (specs S7+S8): the failure-handling gaps of the S7 matrix
// plus the S8 acceptance criteria, in one file.
//
// Already covered elsewhere (not repeated here): log structure
// (TestRecordRoundTripAllS1Fields), correlation
// (TestProjectedTimelineCorrelation), trace propagation
// (TestChildSpanNesting), metrics rate math + empty input
// (TestMetricsRateMathOnFixtures, TestMetricsEmptyInputZeros),
// redaction (TestSecretsFullyRedacted, TestSpanSecretsRedacted,
// TestFileExporterRoundTripRedacted) and in-memory exporter failure
// surfacing (TestExporterFailureSurfacing).
//
// Added here (S7 gaps): malformed JSONL in the event log and in a span
// backing store is a returned error naming the line — never a panic,
// never a partial read masquerading as clean data; missing stores read
// as empty; an unwritable exporter path returns an error.
//
// S8: TestAcceptanceCriteria rebuilds one shared fixture (pass+fail
// receipts, ordered tool calls, budget entries, one evolve experiment)
// and proves all seven acceptance criteria against it.
package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/evolve"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

// acceptanceWorld is the shared S8 fixture: one task with a passing
// and a failing (metric-gated) receipt, three ordered tool calls, two
// budget ledger entries and one adopted evolve experiment.
type acceptanceWorld struct {
	root       string
	task       string
	session    string
	tools      []string
	buildDurMS int64
	failDurMS  int64
}

func seedAcceptanceReceipt(t *testing.T, root, task, gate, verdict string, exit int, durMS, cpuMS int64, metric *float64, at time.Time) {
	t.Helper()
	stdout, stdoutSHA := receipts.Capture("stdout of " + gate + " for " + task)
	stderr, stderrSHA := receipts.Capture("")
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

func seedAcceptanceWorld(t *testing.T) acceptanceWorld {
	t.Helper()
	w := acceptanceWorld{
		root:       t.TempDir(),
		task:       "task-accept",
		session:    "sess-accept",
		tools:      []string{"bash", "read", "write"},
		buildDurMS: 1200,
		failDurMS:  800,
	}
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

	seedAcceptanceReceipt(t, w.root, w.task, "build", receipts.VerdictPass, 0, w.buildDurMS, 120, nil, base)
	failMetric := 0.4
	seedAcceptanceReceipt(t, w.root, w.task, "test", receipts.VerdictFail, 1, w.failDurMS, 80, &failMetric, base.Add(time.Second))

	for i, tool := range w.tools {
		if _, err := trace.Record(w.root, trace.Entry{
			Session: w.session, Message: "m" + tool, Call: "c" + tool,
			Tool: tool, Status: trace.StatusCompleted,
		}, base.Add(time.Duration(10+i)*time.Second)); err != nil {
			t.Fatalf("trace.Record: %v", err)
		}
	}

	for _, e := range []budget.Entry{
		{Kind: budget.KindReflect, Timestamp: base.Add(20 * time.Second), Task: w.task, Domain: "DOM", Outcome: budget.OutcomeAdmitted},
		{Kind: budget.KindProbe, Timestamp: base.Add(21 * time.Second), Task: w.task, Domain: "DOM", Outcome: budget.OutcomeProbeRun},
	} {
		if err := budget.Record(w.root, e); err != nil {
			t.Fatalf("budget.Record: %v", err)
		}
	}

	h := evolve.Hypothesis{
		Problem: "p", RootCause: "r", Change: "c",
		Benefit: "b", PotentialRegressions: "pr", EvaluationCriteria: "ec",
	}
	exp, err := evolve.ProposeExperiment(w.root, evolve.ExperimentSpec{
		ID: "exp-accept", Hypothesis: h, Baseline: "b", Candidate: "c",
		Dataset: "d", Domain: evolve.DomainDOM, Budget: evolve.Budget{MaxRuns: 3},
	})
	if err != nil {
		t.Fatalf("ProposeExperiment: %v", err)
	}
	if err := exp.Advance(w.root, evolve.StateRunning); err != nil {
		t.Fatalf("Advance RUNNING: %v", err)
	}
	probe := func(evolve.Experiment, int) (evolve.ProbeOutcome, error) {
		return evolve.ProbeOutcome{Pass: true, Detail: "fixture"}, nil
	}
	if _, err := evolve.RunExperiments(w.root, evolve.StrategySequential, []evolve.Experiment{exp}, evolve.RunCap{}, probe); err != nil {
		t.Fatalf("RunExperiments: %v", err)
	}
	exp, err = evolve.LoadExperiment(w.root, "exp-accept")
	if err != nil {
		t.Fatalf("LoadExperiment: %v", err)
	}
	if err := exp.SetOutcome("results", "no regression", "fixture decision"); err != nil {
		t.Fatalf("SetOutcome: %v", err)
	}
	if err := exp.Advance(w.root, evolve.StateEvaluated); err != nil {
		t.Fatalf("Advance EVALUATED: %v", err)
	}
	if err := exp.Advance(w.root, evolve.StateAdopted); err != nil {
		t.Fatalf("Advance ADOPTED: %v", err)
	}
	return w
}

// S7 failure handling: a malformed JSONL line in the event log is a
// defined error naming the line. The corrupt prefix must not come back
// as a clean partial read, and nothing may panic.
func TestAcceptanceMalformedEventLogDefined(t *testing.T) {
	root := t.TempDir()
	e := validEvent()
	e.TaskID = "task-malformed"
	if _, err := Record(root, e, time.Now()); err != nil {
		t.Fatalf("Record: %v", err)
	}
	f, err := os.OpenFile(File(root, e.TaskID), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		f.Close()
		t.Fatalf("append garbage: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	entries, err := List(root, e.TaskID)
	if err == nil {
		t.Fatal("List silently accepted a malformed JSONL line; want a defined error")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("malformed-line error should name the line, got: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List returned %d partial entries alongside the error; a corrupt log must not masquerade as a clean read", len(entries))
	}
}

// S7 failure handling: a malformed JSONL line in a span backing store
// (session trace) surfaces as a defined error through both the raw
// reader and the span projection — skipped-or-error, never silent,
// never a panic.
func TestAcceptanceMalformedSpanStoreDefined(t *testing.T) {
	root := t.TempDir()
	if _, err := trace.Record(root, trace.Entry{
		Session: "sess-bad", Message: "m", Call: "c",
		Tool: "bash", Status: trace.StatusCompleted,
	}, time.Now()); err != nil {
		t.Fatalf("trace.Record: %v", err)
	}
	f, err := os.OpenFile(trace.File(root, "sess-bad"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open trace for append: %v", err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		f.Close()
		t.Fatalf("append garbage: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close trace: %v", err)
	}

	if _, err := trace.List(root, "sess-bad"); err == nil {
		t.Fatal("trace.List silently accepted a malformed JSONL line; want a defined error")
	} else if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("malformed-line error should name the line, got: %v", err)
	}
	if _, err := ProjectToolCalls(root, "sess-bad", "task-bad", "trace-1", "corr-1"); err == nil {
		t.Fatal("ProjectToolCalls hid a malformed span store; want the error surfaced")
	}
}

// S7 failure handling: missing stores read as empty and every reader
// degrades honestly — no error, no panic, no invented data.
func TestAcceptanceMissingStoresReadEmpty(t *testing.T) {
	root := t.TempDir()

	entries, err := List(root, "no-such-task")
	if err != nil {
		t.Fatalf("List on missing log: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("missing event log read as %d entries, want empty", len(entries))
	}

	spans, err := ProjectTimeline(root, "no-such-task", "no-such-session")
	if err != nil {
		t.Fatalf("ProjectTimeline on missing stores: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("missing stores projected %d spans, want empty", len(spans))
	}

	projected, err := ProjectReceipts(root, "no-such-task", "trace-1", "corr-1")
	if err != nil {
		t.Fatalf("ProjectReceipts on missing task: %v", err)
	}
	if len(projected) != 0 {
		t.Errorf("missing receipts projected %d spans, want empty", len(projected))
	}

	u, err := Usage(root, "no-such-task", "no-such-session", nil)
	if err != nil {
		t.Fatalf("Usage on missing stores: %v", err)
	}
	exec, ok := u.Item(BudgetExecutionTime)
	if !ok || exec.Value != 0 {
		t.Errorf("missing stores: execution_time = %+v (present=%v), want honest zero", exec, ok)
	}
	for _, name := range []string{BudgetTokens, BudgetCosts} {
		it, ok := u.Item(name)
		if !ok {
			t.Errorf("missing stores: budget item %q absent", name)
			continue
		}
		if it.Measured || strings.TrimSpace(it.Reason) == "" {
			t.Errorf("missing stores: %q must stay unmeasured with a reason, got %+v", name, it)
		}
	}
}

// S7 exporter failures: a FileExporter that cannot write (a regular
// file blocks its directory path) returns the error instead of
// dropping the payload or panicking.
func TestAcceptanceExporterUnwritablePath(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	fx := FileExporter{Root: filepath.Join(blocker, "child"), Scope: "telemetry-test"}
	if err := fx.Export(Payload{Name: "probe"}); err == nil {
		t.Fatal("FileExporter silently dropped a payload it could not write; want the error")
	}
}

// S8 acceptance criteria on one shared fixture.
func TestAcceptanceCriteria(t *testing.T) {
	w := seedAcceptanceWorld(t)

	t.Run("reconstruct_executions", func(t *testing.T) {
		spans, err := ProjectTimeline(w.root, w.task, w.session)
		if err != nil {
			t.Fatalf("ProjectTimeline: %v", err)
		}
		if len(spans) == 0 {
			t.Fatal("timeline is empty; want the task's execution rebuilt from stores")
		}
		var got []string
		var prev time.Time
		for _, s := range spans {
			if s.StartTime.Before(prev) {
				t.Errorf("timeline out of order at span %q", s.Name)
			}
			prev = s.StartTime
			if kind, _ := s.Attributes["kind"].(string); kind == KindToolCall {
				tool, _ := s.Attributes["tool"].(string)
				got = append(got, tool)
			}
		}
		if len(got) != len(w.tools) {
			t.Fatalf("timeline shows %d tool calls %v, want %d %v", len(got), got, len(w.tools), w.tools)
		}
		for i := range w.tools {
			if got[i] != w.tools[i] {
				t.Fatalf("tool call order = %v, want %v", got, w.tools)
			}
		}
	})

	t.Run("identify_failures", func(t *testing.T) {
		spans, err := ProjectReceipts(w.root, w.task, "trace-accept", "corr-accept")
		if err != nil {
			t.Fatalf("ProjectReceipts: %v", err)
		}
		var failed *Span
		for i := range spans {
			if spans[i].Status == StatusError {
				failed = &spans[i]
			}
		}
		if failed == nil {
			t.Fatal("no failed receipt span; the failing gate must surface")
		}
		if gate, _ := failed.Attributes["gate"].(string); gate != "test" {
			t.Errorf("failed span gate = %q, want %q", gate, "test")
		}
		if code, _ := failed.Attributes["exit_code"].(int); code != 1 {
			t.Errorf("failed span exit_code = %v, want 1", code)
		}
		if verdict, _ := failed.Attributes["verdict"].(string); verdict != receipts.VerdictFail {
			t.Errorf("failed span verdict = %q, want %q", verdict, receipts.VerdictFail)
		}
	})

	t.Run("measure_performance", func(t *testing.T) {
		m, err := Summarize(w.root, w.task)
		if err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		if want := w.buildDurMS + w.failDurMS; m.TotalDurationMS != want {
			t.Errorf("total duration = %dms, want %dms", m.TotalDurationMS, want)
		}
		if m.MeanDurationMS <= 0 {
			t.Errorf("mean duration = %v, want > 0", m.MeanDurationMS)
		}
	})

	t.Run("measure_costs", func(t *testing.T) {
		u, err := Usage(w.root, w.task, w.session, nil)
		if err != nil {
			t.Fatalf("Usage: %v", err)
		}
		exec, ok := u.Item(BudgetExecutionTime)
		if !ok || !exec.Measured || exec.Value != float64(w.buildDurMS+w.failDurMS) || exec.Unit != "ms" {
			t.Errorf("execution_time must be measured %dms, got %+v (present=%v)",
				w.buildDurMS+w.failDurMS, exec, ok)
		}
		cost, ok := u.Item(BudgetCosts)
		if !ok || cost.Measured || cost.Value != 0 || strings.TrimSpace(cost.Reason) == "" {
			t.Errorf("model_costs without figures must be unmeasured zero with a reason, got %+v (present=%v)", cost, ok)
		}
	})

	t.Run("correlate_events", func(t *testing.T) {
		spans, err := ProjectTimeline(w.root, w.task, w.session)
		if err != nil {
			t.Fatalf("ProjectTimeline: %v", err)
		}
		var receiptCorr, toolCorr, receiptTrace, toolTrace string
		for _, s := range spans {
			kind, _ := s.Attributes["kind"].(string)
			switch kind {
			case KindVerification:
				receiptCorr, receiptTrace = s.CorrelationID, s.TraceID
			case KindToolCall:
				if toolCorr == "" {
					toolCorr, toolTrace = s.CorrelationID, s.TraceID
				}
			}
		}
		if receiptCorr == "" || toolCorr == "" {
			t.Fatal("timeline lacks receipt and tool spans to correlate")
		}
		if receiptCorr != toolCorr {
			t.Errorf("receipt correlation_id = %q, tool correlation_id = %q; want shared", receiptCorr, toolCorr)
		}
		if receiptTrace != toolTrace {
			t.Errorf("receipt trace_id = %q, tool trace_id = %q; want shared", receiptTrace, toolTrace)
		}
	})

	t.Run("detect_operational_problems", func(t *testing.T) {
		m, err := Summarize(w.root, w.task)
		if err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		if m.VerificationFailureRate <= 0 {
			t.Errorf("verification failure rate = %v, want > 0 to flag the failing gate", m.VerificationFailureRate)
		}
		if m.RegressionRate <= 0 {
			t.Errorf("regression rate = %v, want > 0 (metric-gated failure)", m.RegressionRate)
		}
		if m.TaskCompletionRate >= 1 {
			t.Errorf("task completion rate = %v, want < 1 with a failing task", m.TaskCompletionRate)
		}
	})

	t.Run("export_vendor_free", func(t *testing.T) {
		m, err := Summarize(w.root, w.task)
		if err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		spans, err := ProjectTimeline(w.root, w.task, w.session)
		if err != nil {
			t.Fatalf("ProjectTimeline: %v", err)
		}
		if len(spans) == 0 {
			t.Fatal("no spans to export")
		}
		p := Payload{
			Name:   "acceptance snapshot",
			Labels: map[string]any{"task": w.task},
			Data:   map[string]any{"metrics": m, "span": spans[0]},
		}

		var mem MemoryExporter
		if err := mem.Export(p); err != nil {
			t.Fatalf("MemoryExporter.Export: %v", err)
		}
		fx := FileExporter{Root: w.root, Scope: "acceptance"}
		if err := fx.Export(p); err != nil {
			t.Fatalf("FileExporter.Export: %v", err)
		}

		memRaw, err := json.Marshal(mem.Payloads[0])
		if err != nil {
			t.Fatalf("marshal memory payload: %v", err)
		}
		fileRaw, err := os.ReadFile(fx.Path())
		if err != nil {
			t.Fatalf("read export file: %v", err)
		}
		for name, raw := range map[string]string{"memory": string(memRaw), "file": strings.TrimSpace(string(fileRaw))} {
			if strings.Contains(strings.ToLower(raw), "otel") {
				t.Errorf("%s export carries a vendor SDK marker (%q)", name, "otel")
			}
			var decoded map[string]any
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				t.Errorf("%s export is not plain JSON: %v", name, err)
			}
		}
		// OTel-compatible field names survive without any SDK.
		for _, key := range []string{`"trace_id"`, `"span_id"`, `"service.name"`} {
			if !strings.Contains(string(memRaw), key) {
				t.Errorf("memory export lost OTel-compatible field name %s: %s", key, memRaw)
			}
			if !strings.Contains(string(fileRaw), key) {
				t.Errorf("file export lost OTel-compatible field name %s: %s", key, fileRaw)
			}
		}
		if lines := strings.Split(strings.TrimSpace(string(fileRaw)), "\n"); len(lines) != 1 {
			t.Errorf("file export has %d JSONL lines, want 1", len(lines))
		}
	})
}
