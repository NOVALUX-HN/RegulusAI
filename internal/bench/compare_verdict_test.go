package bench

import "testing"

// Covers Compare REJECT paths: regression naming and pass-count drop.
func TestCompareVerdictRejectPaths(t *testing.T) {
	base := Report{
		Suite:   "s",
		Surface: "base",
		GitHead: "abc",
		Tasks: []TaskResult{
			{ID: "T1", Title: "one", Pass: true, DurationMS: 10, ProxyTokens: 5},
			{ID: "T2", Title: "two", Pass: false, DurationMS: 10, ProxyTokens: 5},
		},
		Passed: 1,
		Failed: 1,
	}
	// Regression: T1 drops from PASS to FAIL.
	candReg := Report{
		Suite:   "s",
		Surface: "cand",
		GitHead: "def",
		Tasks: []TaskResult{
			{ID: "T1", Title: "one", Pass: false, DurationMS: 12, ProxyTokens: 5},
			{ID: "T2", Title: "two", Pass: false, DurationMS: 10, ProxyTokens: 5},
		},
		Passed: 0,
		Failed: 2,
	}
	cmp := Compare(base, candReg)
	if cmp.Verdict != "REJECT" {
		t.Fatalf("regression verdict = %q, want REJECT", cmp.Verdict)
	}
	if cmp.Regressions != 1 {
		t.Fatalf("regressions = %d, want 1", cmp.Regressions)
	}
	if len(cmp.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(cmp.Rows))
	}
	if cmp.Rows[0].DeltaMS != 2 {
		t.Fatalf("deltaMS = %d, want 2", cmp.Rows[0].DeltaMS)
	}

	// Pass-count drop without a direct regression (extra task missing).
	candDrop := Report{
		Suite:   "s",
		Surface: "cand",
		GitHead: "def",
		Tasks: []TaskResult{
			{ID: "T1", Title: "one", Pass: true, DurationMS: 10, ProxyTokens: 5},
		},
		Passed: 0,
		Failed: 1,
	}
	cmp2 := Compare(base, candDrop)
	if cmp2.Verdict != "REJECT" {
		t.Fatalf("drop verdict = %q, want REJECT", cmp2.Verdict)
	}

	// Improvement: T2 flips to PASS, no regressions -> ADOPT.
	candImp := Report{
		Suite:   "s",
		Surface: "cand",
		GitHead: "def",
		Tasks: []TaskResult{
			{ID: "T1", Title: "one", Pass: true, DurationMS: 10, ProxyTokens: 5},
			{ID: "T2", Title: "two", Pass: true, DurationMS: 11, ProxyTokens: 6},
		},
		Passed: 2,
		Failed: 0,
	}
	cmp3 := Compare(base, candImp)
	if cmp3.Verdict != "ADOPT" {
		t.Fatalf("improvement verdict = %q, want ADOPT", cmp3.Verdict)
	}
	if cmp3.Improvements != 1 {
		t.Fatalf("improvements = %d, want 1", cmp3.Improvements)
	}
}
