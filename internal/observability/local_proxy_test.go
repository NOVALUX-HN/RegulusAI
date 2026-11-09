package observability

// T1 local token proxy in metrics and budgets: Summarize sums the
// measured-local proxy (chars/4) alongside honestly-unknown host
// counts, and Usage carries a measured token_usage_local line while
// the host token_usage line stays unmeasured without figures.

import (
	"strings"
	"testing"
	"time"

	"regulus-ai/internal/trace"
)

func seedLocalProxyTrace(t *testing.T, root string) {
	t.Helper()
	base := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	calls := []trace.Entry{
		{Session: "s", Message: "m1", Call: "c1", Tool: "read", Status: trace.StatusCompleted,
			Task: "T1", DurationMS: 100, OutputBytes: 1000},
		{Session: "s", Message: "m2", Call: "c2", Tool: "write", Status: trace.StatusError,
			Task: "T1", DurationMS: 200, OutputBytes: 2000},
	}
	for i, e := range calls {
		if _, err := trace.Record(root, e, base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("trace.Record: %v", err)
		}
	}
}

func wantLocalProxy(t *testing.T, entries []trace.Entry) int64 {
	t.Helper()
	var want int64
	for _, e := range entries {
		want += trace.EstimateLocalTokens(e)
	}
	if want <= 0 {
		t.Fatal("want a positive local proxy total for the fixture")
	}
	return want
}

func TestSummarizeMeasuresLocalProxyWhileHostStaysUnknown(t *testing.T) {
	root := t.TempDir()
	seedLocalProxyTrace(t, root)

	m, err := Summarize(root, "")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	// Host counts stay honestly unknown: zero with the unmeasured note.
	if m.TokensTotal != 0 {
		t.Errorf("host tokens total = %d, want honest 0", m.TokensTotal)
	}
	found := false
	for _, u := range m.Unmeasured {
		if u == "token_usage" {
			found = true
		}
	}
	if !found {
		t.Errorf("host token_usage must stay unmeasured (have %v)", m.Unmeasured)
	}
	// The local proxy measures: recompute by hand from List.
	sessions, err := trace.Sessions(root)
	if err != nil {
		t.Fatal(err)
	}
	var entries []trace.Entry
	for _, s := range sessions {
		got, err := trace.List(root, s)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, got...)
	}
	want := wantLocalProxy(t, entries)
	if m.TokensLocalTotal != want {
		t.Errorf("local proxy total = %d, want hand-folded %d", m.TokensLocalTotal, want)
	}
	if note := m.Coverage["token_usage_local"]; !strings.Contains(note, "measured-local") {
		t.Errorf("local coverage must name the measured-local proxy, got %q", note)
	}
}

func TestSummarizeLocalProxyEmptyRootHonestZero(t *testing.T) {
	m, err := Summarize(t.TempDir(), "")
	if err != nil {
		t.Fatalf("Summarize on empty root: %v", err)
	}
	if m.TokensLocalTotal != 0 {
		t.Errorf("empty local proxy = %d, want 0", m.TokensLocalTotal)
	}
	if note := m.Coverage["token_usage_local"]; strings.TrimSpace(note) == "" {
		t.Error("empty input must still carry a local coverage note")
	}
}

func TestUsageMeasuresLocalTokenLine(t *testing.T) {
	root := t.TempDir()
	seedLocalProxyTrace(t, root)

	u, err := Usage(root, "", "s", nil)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	// Host line stays unmeasured without figures or host counts.
	tok, ok := u.Item(BudgetTokens)
	if !ok || tok.Measured || tok.Value != 0 || strings.TrimSpace(tok.Reason) == "" {
		t.Errorf("host token_usage must stay unmeasured zero with reason, got %+v (present=%v)", tok, ok)
	}
	// Local line measures with an honest reason naming both sides.
	local, ok := u.Item(BudgetTokensLocal)
	if !ok || !local.Measured {
		t.Fatalf("token_usage_local must be measured, got %+v (present=%v)", local, ok)
	}
	sessions, err := trace.Sessions(root)
	if err != nil {
		t.Fatal(err)
	}
	var entries []trace.Entry
	for _, s := range sessions {
		got, err := trace.List(root, s)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, got...)
	}
	if want := wantLocalProxy(t, entries); local.Value != float64(want) {
		t.Errorf("token_usage_local = %v, want hand-folded %d", local.Value, want)
	}
	if local.Unit != "tokens" {
		t.Errorf("token_usage_local unit = %q, want tokens", local.Unit)
	}
	for _, want := range []string{"measured-local", "host-exposed", "unknown"} {
		if !strings.Contains(local.Reason, want) {
			t.Errorf("local reason must contain %q, got %q", want, local.Reason)
		}
	}
}

func TestUsageLocalTokenLineEmptyScopeHonest(t *testing.T) {
	u, err := Usage(t.TempDir(), "", "", nil)
	if err != nil {
		t.Fatalf("Usage empty: %v", err)
	}
	local, ok := u.Item(BudgetTokensLocal)
	if !ok {
		t.Fatal("token_usage_local item absent on empty scope")
	}
	if local.Measured || local.Value != 0 || strings.TrimSpace(local.Reason) == "" {
		t.Errorf("empty local line must be unmeasured zero with reason, got %+v", local)
	}
}
