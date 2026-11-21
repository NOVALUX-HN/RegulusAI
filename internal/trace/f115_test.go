// F11.5 (NOV-91): token/cost accounting with verifiable provenance.
// RED: estimated source, reasoning preservation, task/agent/reason
// correlation keys, secret redaction, host provenance table,
// overflow-safe sums and duplicate-tolerant attribution.
package trace

import (
	"strings"
	"testing"
	"time"
)

func TestEstimatedSourceAccepted(t *testing.T) {
	u, err := ParseHostUsage([]byte(`{"input_tokens":10,"output_tokens":5,"source":"estimated","reason":"chars/4 fallback"}`))
	if err != nil {
		t.Fatalf("estimated source rejected: %v", err)
	}
	if u.Source != SourceEstimated || u.Reason != "chars/4 fallback" {
		t.Fatalf("estimated usage lost: %+v", u)
	}
	if _, err := Record(t.TempDir(), Entry{Session: "s", Message: "m", Call: "c", Tool: "t", Status: StatusCompleted, UsageSource: SourceEstimated}, time.Now().UTC()); err != nil {
		t.Fatalf("record with estimated source rejected: %v", err)
	}
}

func TestReasoningPreservedNotFoldedAway(t *testing.T) {
	u, err := ParseHostUsage([]byte(`{"input_tokens":100,"output_tokens":33,"reasoning_tokens":22,"source":"measured-host"}`))
	if err != nil {
		t.Fatalf("reasoning usage rejected: %v", err)
	}
	if u.ReasoningTokens != 22 || u.OutputTokens != 33 {
		t.Fatalf("reasoning lost: %+v", u)
	}
	for _, doc := range []string{
		`{"reasoning_tokens":-1}`,
		`{"reasoning_tokens":1000000000000000}`,
	} {
		if _, err := ParseHostUsage([]byte(doc)); err == nil {
			t.Fatalf("doc %q must fail, passed", doc)
		}
	}
}

func TestCorrelationKeysRoundTrip(t *testing.T) {
	u, err := ParseHostUsage([]byte(`{"input_tokens":7,"source":"measured-host","run_id":"ses-1","execution_id":"run-9","task":"agent/T1","agent":"worker-a"}`))
	if err != nil {
		t.Fatalf("correlated usage rejected: %v", err)
	}
	if u.Task != "agent/T1" || u.Agent != "worker-a" || u.RunID != "ses-1" || u.ExecutionID != "run-9" {
		t.Fatalf("correlation keys lost: %+v", u)
	}
	for _, doc := range []string{
		`{"task":"a` + "\n" + `b"}`,
		`{"agent":"x` + "\r" + `y"}`,
		`{"reason":"r` + "\n" + `"}`,
	} {
		if _, err := ParseHostUsage([]byte(doc)); err == nil {
			t.Fatalf("multiline doc %q must fail, passed", doc)
		}
	}
}

func TestRedactSecretsKeepsCounts(t *testing.T) {
	for _, secret := range []string{
		"sk-ant-oat1-fakevalue1234567890",
		"xoxb-fake-token-12345",
		"ghp_faketoken0123456789abcdef",
		"AKIAIOSFODNN7FAKEKEY",
		"-----BEGIN FAKE PRIVATE KEY-----",
	} {
		got := RedactSecrets("prefix " + secret + " suffix")
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked through redaction: %q -> %q", secret, got)
		}
		if !strings.Contains(got, "prefix") || !strings.Contains(got, "suffix") {
			t.Fatalf("redaction ate surrounding text: %q", got)
		}
	}
	u := RedactHostUsage(HostUsage{
		InputTokens: 10, OutputTokens: 5, Source: SourceMeasuredHost,
		Model: "m sk-ant-oat1-fakevalue1234567890", Host: "opencode",
	})
	if u.InputTokens != 10 || u.Source != SourceMeasuredHost {
		t.Fatalf("redaction touched counts/source: %+v", u)
	}
	if strings.Contains(u.Model, "sk-ant-oat1") {
		t.Fatalf("model secret not redacted: %q", u.Model)
	}
}

func TestHostProvenanceDeclaresEveryHost(t *testing.T) {
	for host, wantSource := range map[string]string{
		"opencode": SourceMeasuredHost,
		"claude":   SourceUnknown,
		"codex":    SourceUnknown,
		"devin":    SourceUnknown,
		"mystery":  SourceUnknown,
	} {
		p := DeclareHostProvenance(host)
		if p.Host != host || p.Source != wantSource {
			t.Fatalf("provenance(%q) = %+v, want source %q", host, p, wantSource)
		}
		if strings.TrimSpace(p.Reason) == "" {
			t.Fatalf("provenance(%q) has no verified reason", host)
		}
		if p.Source == SourceUnknown && len(p.Reason) < 20 {
			t.Fatalf("provenance(%q) unknown must carry a verified reason, got %q", host, p.Reason)
		}
	}
	devin := DeclareHostProvenance("devin")
	if !strings.Contains(devin.Reason, "exposes no") {
		t.Fatalf("devin reason must say the host exposes none, got %q", devin.Reason)
	}
}

func TestDevinUnknownNeverZero(t *testing.T) {
	u, err := DevinUnknown()
	if err != nil {
		t.Fatalf("DevinUnknown: %v", err)
	}
	if u.Source != SourceUnknown || u.Host != "devin" {
		t.Fatalf("devin unknown mislabeled: %+v", u)
	}
	if u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Fatalf("devin unknown must carry no counts: %+v", u)
	}
	if strings.TrimSpace(u.Reason) == "" {
		t.Fatal("devin unknown must carry the verified reason")
	}
}

func TestAddCountsOverflowFailsClosed(t *testing.T) {
	max := int64(^uint64(0) >> 1)
	if _, err := addCounts(max, 1); err == nil {
		t.Fatal("maxInt64+1 must fail closed, passed")
	}
	if _, err := addCounts(maxUsageCount, 1); err != nil {
		t.Fatalf("small sums must pass: %v", err)
	}
	if got, err := addCounts(100, 200); err != nil || got != 300 {
		t.Fatalf("addCounts(100,200) = %d,%v, want 300,nil", got, err)
	}
}

func TestDedupEntriesKeepsFirst(t *testing.T) {
	mk := func(msg, call string) Entry {
		return Entry{Session: "s", Message: msg, Call: call, Tool: "read", Status: StatusCompleted, InputTokens: 10}
	}
	in := []Entry{mk("m1", "c1"), mk("m1", "c1"), mk("m2", "c2")}
	out := DedupEntries(in)
	if len(out) != 2 {
		t.Fatalf("dedup = %d entries, want 2", len(out))
	}
	if len(in) != 3 {
		t.Fatal("dedup must not mutate its input")
	}
}

func TestReconcileTotalsMatchesPrimary(t *testing.T) {
	primary := HostUsage{InputTokens: 110, OutputTokens: 33, ReasoningTokens: 22, CacheRead: 5, Source: SourceMeasuredHost, Host: "opencode", RunID: "ses-1"}
	entries := []Entry{
		{Session: "ses-1", Message: "m1", Call: "c1", Tool: "read", Status: StatusCompleted, InputTokens: 100, OutputTokens: 30, ReasoningTokens: 20, CacheRead: 5, UsageSource: SourceMeasuredHost},
		{Session: "ses-1", Message: "m2", Call: "c2", Tool: "read", Status: StatusCompleted, InputTokens: 10, OutputTokens: 3, ReasoningTokens: 2, UsageSource: SourceMeasuredHost},
	}
	if err := ReconcileTotals(primary, entries); err != nil {
		t.Fatalf("matching totals must reconcile: %v", err)
	}
	short := entries[:1]
	if err := ReconcileTotals(primary, short); err == nil {
		t.Fatal("short totals must not reconcile, passed")
	} else if !strings.Contains(err.Error(), "input_tokens") {
		t.Fatalf("gap error must name the field, got %v", err)
	}
	if err := ReconcileTotals(HostUsage{Source: SourceUnknown}, nil); err != nil {
		t.Fatalf("unknown primary with no attribution reconciles vacuously: %v", err)
	}
}

func TestCorrelateUsageMapsKeys(t *testing.T) {
	u := HostUsage{InputTokens: 100, OutputTokens: 33, ReasoningTokens: 22, CacheRead: 5,
		Model: "beam-free", Host: "opencode", HostVersion: "2.2.5",
		Source: SourceMeasuredHost, RunID: "ses-1"}
	e, err := CorrelateUsage(u, CorrelateKeys{Session: "ses-1", Message: "m", Call: "c", Tool: "read", Task: "agent/T1", Agent: "worker-a", ExecutionID: "run-9"})
	if err != nil {
		t.Fatalf("CorrelateUsage: %v", err)
	}
	if e.Session != "ses-1" || e.Task != "agent/T1" || e.Agent != "worker-a" || e.ExecutionID != "run-9" {
		t.Fatalf("keys not mapped: %+v", e)
	}
	if e.InputTokens != 100 || e.ReasoningTokens != 22 || e.UsageSource != SourceMeasuredHost {
		t.Fatalf("counts/source not carried: %+v", e)
	}
	if _, err := CorrelateUsage(u, CorrelateKeys{Session: "other", Message: "m", Call: "c", Tool: "read"}); err == nil {
		t.Fatal("session/run mismatch must fail closed, passed")
	}
	bad := HostUsage{InputTokens: 5, Source: SourceUnknown, Host: "devin"}
	if _, err := CorrelateUsage(bad, CorrelateKeys{Session: "s", Message: "m", Call: "c", Tool: "t"}); err == nil {
		t.Fatal("unknown provenance with counts must fail closed, passed")
	}
}
