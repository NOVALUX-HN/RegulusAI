package observability

// Phase 8 T2 (specs S2+S4): OTel-compatible spans projected read-only
// from existing stores. RED test: pins nesting, correlation, OTel field
// names and projection honesty before the implementation exists.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/budget"
	"regulus-ai/internal/evidence"
	"regulus-ai/internal/receipts"
	"regulus-ai/internal/trace"
)

func TestChildSpanNesting(t *testing.T) {
	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	parent, err := StartSpan("gauntlet", "gate.run", "task-1", "corr-1", start)
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	child, err := parent.ChildSpan("tool.exec", start.Add(time.Second))
	if err != nil {
		t.Fatalf("ChildSpan: %v", err)
	}
	if child.TraceID != parent.TraceID {
		t.Errorf("child trace_id = %q, want parent %q", child.TraceID, parent.TraceID)
	}
	if child.CorrelationID != parent.CorrelationID {
		t.Errorf("child correlation_id = %q, want parent %q", child.CorrelationID, parent.CorrelationID)
	}
	if child.ParentSpanID != parent.SpanID {
		t.Errorf("child parent_span_id = %q, want parent span %q", child.ParentSpanID, parent.SpanID)
	}
	if child.SpanID == parent.SpanID {
		t.Error("child reused parent span_id")
	}
	if child.TaskID != parent.TaskID {
		t.Errorf("child task_id = %q, want parent %q", child.TaskID, parent.TaskID)
	}
}

func TestOTelFieldNamePin(t *testing.T) {
	start := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s, err := StartSpan("core", "task.lifecycle", "task-1", "corr-1", start)
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	s.EndSpan(start.Add(time.Minute), StatusOK)
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal span: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal span: %v", err)
	}
	// Snake_case OTel-compatible names, pinned: renaming any of these
	// breaks every backend mapping built on this export.
	for _, key := range []string{
		"trace_id", "span_id", "parent_span_id", "service.name",
		"name", "task_id", "correlation_id",
		"start_time", "end_time", "status", "attributes",
	} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("span JSON missing OTel field %q: %s", key, raw)
		}
	}
	for _, banned := range []string{"traceId", "spanId", "parentSpanId", "serviceName", "correlationId", "taskId", "startTime", "endTime"} {
		if _, ok := decoded[banned]; ok {
			t.Errorf("span JSON uses camelCase field %q (want snake_case): %s", banned, raw)
		}
	}
}

func seedTimeline(t *testing.T, root string) {
	t.Helper()
	task := "feat/T1"
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if _, err := receipts.Write(evidence.TaskDir(root, task), receipts.Receipt{
		Task: task, Gate: "unit-tests", Layer: 1,
		Command: "go test ./...", ExitCode: 0, Verdict: receipts.VerdictPass,
		StartedAt: at, DurationMS: 1500,
	}); err != nil {
		t.Fatalf("receipts.Write: %v", err)
	}
	if _, err := trace.Record(root, trace.Entry{
		Session: "sess-1", Message: "m1", Call: "c1",
		Tool: "bash", Status: trace.StatusCompleted,
	}, at.Add(time.Second)); err != nil {
		t.Fatalf("trace.Record: %v", err)
	}
	if err := budget.Record(root, budget.Entry{
		Kind: budget.KindReflect, Timestamp: at.Add(2 * time.Second),
		Task: task, Domain: "DOM", Outcome: budget.OutcomeAdmitted,
	}); err != nil {
		t.Fatalf("budget.Record: %v", err)
	}
}

func TestProjectedTimelineCorrelation(t *testing.T) {
	root := t.TempDir()
	seedTimeline(t, root)
	spans, err := ProjectTimeline(root, "feat/T1", "sess-1")
	if err != nil {
		t.Fatalf("ProjectTimeline: %v", err)
	}
	if len(spans) < 3 {
		t.Fatalf("got %d spans, want at least 3 (receipt+tool+budget)", len(spans))
	}
	corr := spans[0].CorrelationID
	if corr == "" {
		t.Fatal("timeline spans have empty correlation_id")
	}
	kinds := map[string]bool{}
	for _, s := range spans {
		if s.CorrelationID != corr {
			t.Errorf("span %q correlation_id = %q, want timeline %q", s.Name, s.CorrelationID, corr)
		}
		if s.TraceID != spans[0].TraceID {
			t.Errorf("span %q trace_id = %q, want timeline %q", s.Name, s.TraceID, spans[0].TraceID)
		}
		if k, ok := s.Attributes["kind"]; ok {
			kinds[k.(string)] = true
		}
	}
	for _, want := range []string{KindVerification, KindToolCall, KindTaskLifecycle} {
		if !kinds[want] {
			t.Errorf("timeline missing kind %q (have %v)", want, kinds)
		}
	}
}

func TestProjectionHonesty(t *testing.T) {
	// Kinds with no backing store today must report absence: empty,
	// no error, never invented data.
	for name, fn := range map[string]func() ([]Span, error){
		"guardian":         ProjectGuardianExecutions,
		"memory-retrieval": ProjectMemoryRetrievals,
		"external":         ProjectExternalIntegrations,
	} {
		spans, err := fn()
		if err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
		if len(spans) != 0 {
			t.Errorf("%s: got %d spans, want empty (no backing store)", name, len(spans))
		}
	}
}

func TestSpanSecretsRedacted(t *testing.T) {
	start := time.Now().UTC()
	secret := "sk-live-REDACTME-UNIQUE-12345"
	s, err := StartSpan("core", "task with api_key="+secret, "task-1", "corr-1", start)
	if err != nil {
		t.Fatalf("StartSpan: %v", err)
	}
	s.Attributes["output"] = "token=" + secret
	s = RedactSpan(s)
	raw, _ := json.Marshal(s)
	if strings.Contains(string(raw), secret) {
		t.Errorf("span leaks secret: %s", raw)
	}
	if !strings.Contains(string(raw), Redacted) {
		t.Errorf("span has no %q marker: %s", Redacted, raw)
	}
}
