package cmdx

import (
	"testing"

	"regulus-ai/internal/bench"
)

// Covers applyLabHostVersions + labCompareExit without running labs.
func TestLabCompareHelpers(t *testing.T) {
	mk := func(host, verdict string) bench.LabResult {
		return bench.LabResult{
			Verdict: verdict,
			Outcome: bench.AgentOutcome{Host: host},
		}
	}
	// Explicit hostVersion wins for every host.
	rs := []bench.LabResult{mk("opencode", bench.LabNotRun), mk("claude", bench.LabNotRun)}
	applyLabHostVersions(rs, "v9", func() string { t.Fatal("probe must not run"); return "" })
	if rs[0].Outcome.HostVersion != "v9" || rs[1].Outcome.HostVersion != "v9" {
		t.Fatalf("explicit version must win, got %+v", rs)
	}
	// Probe cached once for opencode, untouched for others.
	calls := 0
	rs = []bench.LabResult{mk("opencode", "pass"), mk("opencode", "pass"), mk("claude", "pass")}
	applyLabHostVersions(rs, "", func() string { calls++; return "pv" })
	if calls != 1 {
		t.Fatalf("probe calls = %d, want 1 (cached)", calls)
	}
	if rs[0].Outcome.HostVersion != "pv" || rs[2].Outcome.HostVersion != "" {
		t.Fatalf("probe wiring wrong: %+v", rs)
	}

	// Exit mapping preserves exact contract.
	if _, code := labCompareExit(nil, 0); code != 3 {
		t.Fatalf("empty must be NOT RUN=3, got %d", code)
	}
	notRun := []bench.LabResult{mk("a", bench.LabNotRun)}
	if msg, code := labCompareExit(notRun, 5); code != 3 || msg == "" {
		t.Fatalf("all NOT_RUN must be 3 with message, got %d %q", code, msg)
	}
	mixed := []bench.LabResult{mk("a", "pass")}
	if _, code := labCompareExit(mixed, 0); code != 1 {
		t.Fatalf("evaluable 0 must be 1, got %d", code)
	}
	if _, code := labCompareExit(mixed, 2); code != 0 {
		t.Fatalf("evaluable must be 0, got %d", code)
	}
}
