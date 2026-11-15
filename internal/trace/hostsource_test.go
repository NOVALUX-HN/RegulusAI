// NOV-62: host token sources stay labeled and never mix with the
// local proxy or with unknown.
package trace

import (
	"testing"
	"time"
)

func recordEntry(t *testing.T, dir, session, task string, e Entry) {
	t.Helper()
	e.Session = session
	e.Task = task
	if e.Message == "" {
		e.Message = "m"
	}
	if e.Call == "" {
		e.Call = "c-" + session + "-" + task
	}
	if e.Tool == "" {
		e.Tool = "read"
	}
	if e.Status == "" {
		e.Status = StatusCompleted
	}
	if _, err := Record(dir, e, time.Now().UTC()); err != nil {
		t.Fatalf("record: %v", err)
	}
}

func TestCostTokenSourceMeasuredHost(t *testing.T) {
	dir := t.TempDir()
	recordEntry(t, dir, "s1", "T1", Entry{InputTokens: 100, OutputTokens: 50, CacheRead: 10, Host: "opencode", Model: "m", UsageSource: SourceMeasuredHost})
	costs, err := CostByTask(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(costs) != 1 {
		t.Fatalf("costs = %d, want 1", len(costs))
	}
	tc := costs[0]
	if !tc.TokensKnown || tc.TokenSource != SourceMeasuredHost {
		t.Fatalf("source = %q known=%v, want measured-host true", tc.TokenSource, tc.TokensKnown)
	}
	if tc.TotalCacheRead != 10 {
		t.Fatalf("cache read = %d, want 10", tc.TotalCacheRead)
	}
}

func TestCostTokenSourceUnknownStaysUnknown(t *testing.T) {
	dir := t.TempDir()
	recordEntry(t, dir, "s1", "T1", Entry{DurationMS: 5})
	costs, err := CostByTask(dir)
	if err != nil {
		t.Fatal(err)
	}
	tc := costs[0]
	if tc.TokensKnown || tc.TokenSource != "" {
		t.Fatalf("tokenless bucket must stay unknown, got known=%v source=%q", tc.TokensKnown, tc.TokenSource)
	}
	if !tc.LocalTokensKnown || tc.TotalLocalTokens <= 0 {
		t.Fatal("local proxy must still measure the entry")
	}
}

func TestRecordRejectsNegativeCacheAndBadSource(t *testing.T) {
	dir := t.TempDir()
	if _, err := Record(dir, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, CacheRead: -1}, time.Now().UTC()); err == nil {
		t.Fatal("negative cache must fail")
	}
	if _, err := Record(dir, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, UsageSource: "trust-me"}, time.Now().UTC()); err == nil {
		t.Fatal("bad usage_source must fail")
	}
	if _, err := Record(dir, Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, Host: "x\ny"}, time.Now().UTC()); err == nil {
		t.Fatal("multiline host must fail")
	}
}
