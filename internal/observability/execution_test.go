package observability

// NOV-34 (S17): correlated observability. One gate run's runID travels as
// trace execution_id; ReceiptSpan and ToolCallSpan carry it; the project
// projections join by execution_id==RunID besides task; legacy entries
// without an id keep working in the "(unattributed)" bucket; and every
// instrumentation failure is a returned error, never a panic.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/evidence"
	"regulus-ai/internal/gates"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func seedRunReceipt(t *testing.T, root, task, runID, gate string, at time.Time) {
	t.Helper()
	stdout, stdoutSHA := receipts.Capture("stdout of " + gate)
	stderr, stderrSHA := receipts.Capture("")
	rec := receipts.Receipt{
		Task: task, Gate: gate, Layer: 1, Command: "go test ./...",
		ExitCode: 0, Verdict: receipts.VerdictPass,
		StartedAt: at, DurationMS: 100,
		Stdout: stdout, StdoutSHA: stdoutSHA, Stderr: stderr, StderrSHA: stderrSHA,
		RunID: runID,
	}
	if _, err := receipts.Write(evidence.TaskDir(root, task), rec); err != nil {
		t.Fatalf("receipts.Write: %v", err)
	}
}

func seedRunTrace(t *testing.T, root, session, task, execID, tool, call string, at time.Time) {
	t.Helper()
	if _, err := trace.Record(root, trace.Entry{
		Session: session, Message: "m" + call, Call: call, Tool: tool,
		Status: trace.StatusCompleted, Task: task, ExecutionID: execID,
	}, at); err != nil {
		t.Fatalf("trace.Record %s/%s: %v", session, call, err)
	}
}

func spanByKind(spans []Span, kind string) *Span {
	for i := range spans {
		if k, _ := spans[i].Attributes["kind"].(string); k == kind {
			return &spans[i]
		}
	}
	return nil
}

// Record -> List -> ReceiptSpan -> ProjectTimeline with the same run_x:
// the receipt and every tagged tool call correlate on one execution id,
// even across sessions and across tasks.
func TestCorrelatedExecutionJoinsTimeline(t *testing.T) {
	root := t.TempDir()
	task := "demo/T9"
	run := "run_x00000000000001"
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	seedRunReceipt(t, root, task, run, "lint", base)
	seedRunTrace(t, root, "sess-main", task, run, "bash", "c1", base.Add(10*time.Second))
	// Same execution, other session AND other task: the execution join
	// (not the task) is what pulls it into this timeline.
	seedRunTrace(t, root, "sess-other", "other/task", run, "read", "c2", base.Add(20*time.Second))

	listed, err := trace.List(root, "sess-main")
	if err != nil {
		t.Fatalf("trace.List: %v", err)
	}
	if len(listed) != 1 || listed[0].ExecutionID != run {
		t.Fatalf("listed execution ids = %v, want [%s]", listed, run)
	}

	spans, err := ProjectTimeline(root, task, "sess-main")
	if err != nil {
		t.Fatalf("ProjectTimeline: %v", err)
	}
	ver := spanByKind(spans, KindVerification)
	if ver == nil {
		t.Fatal("timeline lacks the verification span")
	}
	if got, _ := ver.Attributes["execution_id"].(string); got != run {
		t.Errorf("verification execution_id = %q, want %q", got, run)
	}
	var tools []string
	var execIDs []string
	for _, s := range spans {
		if k, _ := s.Attributes["kind"].(string); k != KindToolCall {
			continue
		}
		tool, _ := s.Attributes["tool"].(string)
		tools = append(tools, tool)
		exec, _ := s.Attributes["execution_id"].(string)
		execIDs = append(execIDs, exec)
	}
	if len(tools) != 2 {
		t.Fatalf("timeline tool calls = %v, want [bash read] joined by execution id", tools)
	}
	for _, id := range execIDs {
		if id != run {
			t.Errorf("tool execution_id = %q, want %q", id, run)
		}
	}
	for _, s := range spans {
		if s.TraceID != spans[0].TraceID || s.CorrelationID != spans[0].CorrelationID {
			t.Errorf("span %q leaves the timeline correlation (%q/%q)", s.Name, s.TraceID, s.CorrelationID)
		}
	}
}

// The task selector also accepts a run id: ProjectReceipts joins by
// execution_id==RunID besides task.
func TestProjectReceiptsFindsByRunID(t *testing.T) {
	root := t.TempDir()
	run := "run_x00000000000002"
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	seedRunReceipt(t, root, "demo/T9", run, "lint", base)
	spans, err := ProjectReceipts(root, run, "trace-1", "corr-1")
	if err != nil {
		t.Fatalf("ProjectReceipts by run id: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("ProjectReceipts(run) = %d spans, want 1", len(spans))
	}
	if got, _ := spans[0].Attributes["execution_id"].(string); got != run {
		t.Errorf("execution_id = %q, want %q", got, run)
	}
}

// Legacy entries without ids keep working: no execution_id attribute,
// and the "(unattributed)" bucket still distinguishes unknown.
func TestLegacyTraceStaysUnattributed(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if _, err := trace.Record(root, trace.Entry{
		Session: "sess-legacy", Message: "m", Call: "c",
		Tool: "bash", Status: trace.StatusCompleted,
	}, base); err != nil {
		t.Fatalf("trace.Record legacy: %v", err)
	}
	entries, err := trace.List(root, "sess-legacy")
	if err != nil {
		t.Fatalf("trace.List: %v", err)
	}
	if key := trace.ExecutionKey(entries[0]); key != trace.UnattributedExecution {
		t.Errorf("legacy execution bucket = %q, want %q", key, trace.UnattributedExecution)
	}
	span, err := ToolCallSpan(entries[0], "task-legacy", "trace-1", "corr-1")
	if err != nil {
		t.Fatalf("ToolCallSpan legacy: %v", err)
	}
	if _, ok := span.Attributes["execution_id"]; ok {
		t.Errorf("legacy tool span carries execution_id = %v, want it absent", span.Attributes["execution_id"])
	}
	if _, err := ProjectTimeline(root, "task-legacy", "sess-legacy"); err != nil {
		t.Fatalf("ProjectTimeline legacy: %v", err)
	}
}

// Instrumentation failures return errors, never panics and never silent
// partial data.
func TestExecutionInstrumentationFailureIsError(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if _, err := trace.Record(root, trace.Entry{
		Session: "sess-bad", Message: "m", Call: "c",
		Tool: "bash", Status: trace.StatusCompleted,
	}, base); err != nil {
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
	if _, err := ProjectToolCalls(root, "sess-bad", "task-bad", "trace-1", "corr-1"); err == nil {
		t.Error("ProjectToolCalls hid a malformed trace; want the error surfaced")
	} else if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("malformed error = %q, want it to name line 2", err)
	}
	if _, err := ProjectTimeline(root, "task-bad", "sess-bad"); err == nil {
		t.Error("ProjectTimeline hid a malformed trace; want the error surfaced")
	}
	if _, err := ReceiptSpan(receipts.Receipt{
		Task: "bad\ntask", Gate: "g", StartedAt: base,
	}, "trace-1", "corr-1"); err == nil {
		t.Error("ReceiptSpan accepted a newline task; want a single-line error")
	}
	if _, err := ToolCallSpan(trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "t", Status: trace.StatusCompleted,
	}, "bad\ntask", "trace-1", "corr-1"); err == nil {
		t.Error("ToolCallSpan accepted a newline task; want a single-line error")
	}
}

// Propagation contract: the gate run's runID is the execution id the
// trace hook records. Explicit context wins, else the run's runID, else
// "" (probe path: no correlation, legacy calls unchanged).
func TestGateRunIDPropagationContract(t *testing.T) {
	if got := gates.ExecutionIDFromContext(context.Background()); got != "" {
		t.Errorf("empty ctx execution id = %q, want empty", got)
	}
	ctx := gates.ContextWithExecutionID(context.Background(), "run_ctx")
	if got := gates.ExecutionIDFromContext(ctx); got != "run_ctx" {
		t.Errorf("ctx execution id = %q, want run_ctx", got)
	}
	if got := gates.ResolveExecutionID(ctx, "run_stamp"); got != "run_ctx" {
		t.Errorf("ResolveExecutionID(ctx, stamp) = %q, want run_ctx (context wins)", got)
	}
	if got := gates.ResolveExecutionID(context.Background(), "run_stamp"); got != "run_stamp" {
		t.Errorf("ResolveExecutionID(bg, stamp) = %q, want run_stamp (run fallback)", got)
	}
	if got := gates.ResolveExecutionID(context.Background(), ""); got != "" {
		t.Errorf("ResolveExecutionID(bg, empty) = %q, want empty (probe path)", got)
	}
}
