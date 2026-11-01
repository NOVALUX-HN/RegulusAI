package observability

// F1 proxy cost in metrics and budgets: trace duration/bytes fold
// into Summarize and Usage, token counts measure only when traces
// actually carry host-exposed figures, model costs stay honest zero.

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

func seedCostTrace(t *testing.T, root string) {
	t.Helper()
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	calls := []trace.Entry{
		{Session: "s", Message: "m1", Call: "c1", Tool: "read", Status: trace.StatusCompleted,
			Task: "F1", DurationMS: 100, OutputBytes: 1000, InputTokens: 10, OutputTokens: 5},
		{Session: "s", Message: "m2", Call: "c2", Tool: "write", Status: trace.StatusError,
			Task: "F1", DurationMS: 200, OutputBytes: 2000},
	}
	for i, e := range calls {
		if _, err := trace.Record(root, e, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("trace.Record: %v", err)
		}
	}
}

func TestSummarizeFoldsProxyCosts(t *testing.T) {
	root := t.TempDir()
	seedCostTrace(t, root)

	m, err := Summarize(root, "")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if m.ToolCalls != 2 {
		t.Errorf("tool calls = %d, want 2", m.ToolCalls)
	}
	if m.TraceDurationMS != 300 {
		t.Errorf("trace duration = %dms, want 300ms", m.TraceDurationMS)
	}
	if m.TraceOutputBytes != 3000 {
		t.Errorf("trace output = %dB, want 3000B", m.TraceOutputBytes)
	}
	// Traces carry token counts: measured from the store, leaving unmeasured.
	if m.TokensTotal != 15 {
		t.Errorf("tokens total = %d, want 15 (10 in + 5 out)", m.TokensTotal)
	}
	for _, u := range m.Unmeasured {
		if u == "token_usage" {
			t.Errorf("measured trace tokens must leave unmeasured (have %v)", m.Unmeasured)
		}
	}
	if note := m.Coverage["token_usage"]; !strings.Contains(note, "trace") {
		t.Errorf("measured token coverage must name the trace store, got %q", note)
	}
	// Tokens are not money: cost stays honest zero.
	if m.CostPerSuccessfulTask != 0 {
		t.Errorf("cost per successful task = %v, want honest 0", m.CostPerSuccessfulTask)
	}
	found := false
	for _, u := range m.Unmeasured {
		if u == "model_costs" {
			found = true
		}
	}
	if !found {
		t.Errorf("model_costs must stay unmeasured (have %v)", m.Unmeasured)
	}
}

func TestSummarizeProxyHonestWithoutTokens(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if _, err := trace.Record(root, trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "bash",
		Status: trace.StatusCompleted, DurationMS: 50, OutputBytes: 500,
	}, base); err != nil {
		t.Fatalf("trace.Record: %v", err)
	}
	m, err := Summarize(root, "")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if m.TraceDurationMS != 50 || m.TraceOutputBytes != 500 {
		t.Errorf("proxy = %dms/%dB, want 50ms/500B", m.TraceDurationMS, m.TraceOutputBytes)
	}
	if m.TokensTotal != 0 {
		t.Errorf("tokens total = %d, want honest 0 (host exposes none)", m.TokensTotal)
	}
	found := false
	for _, u := range m.Unmeasured {
		if u == "token_usage" {
			found = true
		}
	}
	if !found {
		t.Errorf("tokenless traces must keep token_usage unmeasured (have %v)", m.Unmeasured)
	}
}

func TestUsageProxyDetailAndTraceTokens(t *testing.T) {
	root := t.TempDir()
	seedCostTrace(t, root)

	u, err := Usage(root, "", "s", nil)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	tool, ok := u.Item(BudgetToolUsage)
	if !ok || !tool.Measured || tool.Value != 2 {
		t.Fatalf("tool_usage must be measured 2 calls, got %+v (present=%v)", tool, ok)
	}
	if !strings.Contains(tool.Detail, "300ms") || !strings.Contains(tool.Detail, "3000B") {
		t.Errorf("tool detail must carry the proxy sums, got %q", tool.Detail)
	}
	if !strings.Contains(tool.Detail, "zero = unknown") {
		t.Errorf("tool detail must state the unknown rule, got %q", tool.Detail)
	}
	tok, ok := u.Item(BudgetTokens)
	if !ok || !tok.Measured || tok.Value != 15 {
		t.Errorf("trace token counts must measure 15, got %+v (present=%v)", tok, ok)
	}
	if !strings.Contains(tok.Reason, "never estimated") {
		t.Errorf("trace token reason must disclaim estimation, got %q", tok.Reason)
	}

	// Caller-supplied figures take precedence over trace sums.
	figs := &Figures{Tokens: int64Ptr(1000), ModelCost: floatPtr(0.02), Currency: "USD"}
	u2, err := Usage(root, "", "s", figs)
	if err != nil {
		t.Fatalf("Usage with figures: %v", err)
	}
	tok2, _ := u2.Item(BudgetTokens)
	if !tok2.Measured || tok2.Value != 1000 {
		t.Errorf("figures must take precedence, got %+v", tok2)
	}

	// Without figures or trace tokens: honest zero with a reason.
	empty := t.TempDir()
	u3, err := Usage(empty, "", "", nil)
	if err != nil {
		t.Fatalf("Usage empty: %v", err)
	}
	tok3, _ := u3.Item(BudgetTokens)
	if tok3.Measured || tok3.Value != 0 || strings.TrimSpace(tok3.Reason) == "" {
		t.Errorf("tokenless usage must be unmeasured zero with reason, got %+v", tok3)
	}
	cost3, _ := u3.Item(BudgetCosts)
	if cost3.Measured || cost3.Value != 0 || strings.TrimSpace(cost3.Reason) == "" {
		t.Errorf("model costs must stay unmeasured zero with reason, got %+v", cost3)
	}
}

func TestToolCallSpanCarriesProxyCost(t *testing.T) {
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s, err := ToolCallSpan(trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "read",
		Status: trace.StatusCompleted, Task: "F1",
		DurationMS: 100, OutputBytes: 1000, InputTokens: 999, OutputTokens: 999,
		At: at,
	}, "task-1", "trace-1", "corr-1")
	if err != nil {
		t.Fatalf("ToolCallSpan: %v", err)
	}
	if s.Attributes["task"] != "F1" {
		t.Errorf("span task = %v, want F1", s.Attributes["task"])
	}
	if s.Attributes["duration_ms"] != int64(100) {
		t.Errorf("span duration_ms = %v, want 100", s.Attributes["duration_ms"])
	}
	if s.Attributes["output_bytes"] != int64(1000) {
		t.Errorf("span output_bytes = %v, want 1000", s.Attributes["output_bytes"])
	}
	// Token counts stay out of spans: the redaction deny-list would
	// scrub every key containing "token" — silent data loss dressed
	// as telemetry. They live on in traces, costs, metrics, budgets.
	for _, key := range []string{"input_tokens", "output_tokens"} {
		if v, ok := s.Attributes[key]; ok {
			t.Errorf("span must not carry %q (redaction would scrub it), got %v", key, v)
		}
	}

	// Unmeasured costs stay out of the span entirely.
	bare, err := ToolCallSpan(trace.Entry{
		Session: "s", Message: "m", Call: "c", Tool: "read",
		Status: trace.StatusCompleted, At: at,
	}, "task-1", "trace-1", "corr-1")
	if err != nil {
		t.Fatalf("ToolCallSpan bare: %v", err)
	}
	for _, key := range []string{"task", "duration_ms", "output_bytes"} {
		if _, ok := bare.Attributes[key]; ok {
			t.Errorf("unmeasured cost must stay out of the span, found %q", key)
		}
	}
}
