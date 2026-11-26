package almanac

import "testing"

// TestSummarizeWithoutBaselineIsNotComparable: a batch where every run reused
// knowledge has no baseline group, so no comparison is claimed.
func TestSummarizeWithoutBaselineIsNotComparable(t *testing.T) {
	u := Summarize([]ReuseEvent{
		{Reused: []string{"k1"}, Success: true, Retries: 0},
		{Reused: []string{"k2"}, Success: true, Retries: 1},
	})
	if u.Runs != 2 || u.Reused != 2 || u.Baseline != 0 {
		t.Fatalf("counts = %+v", u)
	}
	if u.ReuseRatePct != 100 {
		t.Fatalf("reuse rate = %d, want 100", u.ReuseRatePct)
	}
	if u.Comparable {
		t.Fatal("no baseline group means no comparison")
	}
	if u.RetryDeltaMilli != 0 {
		t.Fatalf("delta must stay zero without a baseline, got %d", u.RetryDeltaMilli)
	}
}

// TestSummarizeEmptyIsZero keeps the zero case honest: nothing measured, nothing
// claimed.
func TestSummarizeEmptyIsZero(t *testing.T) {
	u := Summarize(nil)
	if u.Runs != 0 || u.Comparable || u.ReuseRatePct != 0 {
		t.Fatalf("empty batch = %+v", u)
	}
}

// TestSummarizeComparesAgainstTheNoReuseBaseline is the measurement F18.3 asks
// for: reuse group versus baseline group, on the same batch.
func TestSummarizeComparesAgainstTheNoReuseBaseline(t *testing.T) {
	events := []ReuseEvent{
		// Reused and succeeded, no retries.
		{Reused: []string{"k1"}, Success: true, Retries: 0},
		// Reused and failed, two retries.
		{Reused: []string{"k2"}, Success: false, Retries: 2},
		// Baseline: retrieved nothing usable, failed with three retries.
		{Success: false, Retries: 3},
		// Baseline: succeeded after one retry.
		{Success: true, Retries: 1},
	}
	u := Summarize(events)

	if u.Runs != 4 || u.Reused != 2 || u.Baseline != 2 {
		t.Fatalf("counts = %+v", u)
	}
	if u.ReuseRatePct != 50 {
		t.Fatalf("reuse rate = %d, want 50", u.ReuseRatePct)
	}
	if u.SuccessWithReuse != 1 || u.SuccessRateWithPct != 50 {
		t.Fatalf("reuse success = %d/%d", u.SuccessWithReuse, u.SuccessRateWithPct)
	}
	if u.SuccessWithoutReuse != 1 || u.SuccessRateWithoutPct != 50 {
		t.Fatalf("baseline success = %d/%d", u.SuccessWithoutReuse, u.SuccessRateWithoutPct)
	}
	// Reuse group: (0+2)/2 = 1 retry. Baseline: (3+1)/2 = 2 retries.
	if u.MeanRetriesWithMilli != 1000 || u.MeanRetriesWithoutMilli != 2000 {
		t.Fatalf("means = %d / %d", u.MeanRetriesWithMilli, u.MeanRetriesWithoutMilli)
	}
	if !u.Comparable {
		t.Fatal("both groups are present, so it is comparable")
	}
	if u.RetryDeltaMilli != -1000 {
		t.Fatalf("delta = %d, want -1000 (reuse came with one fewer retry)", u.RetryDeltaMilli)
	}
}

// TestUsedKnowledgeRequiresAnActualReuse: retrieving without using anything is
// the baseline, not a reuse.
func TestUsedKnowledgeRequiresAnActualReuse(t *testing.T) {
	if (ReuseEvent{Matched: []string{"k1", "k2"}}).UsedKnowledge() {
		t.Fatal("matched but not reused is not reuse")
	}
	if !(ReuseEvent{Matched: []string{"k1"}, Reused: []string{"k1"}}).UsedKnowledge() {
		t.Fatal("a used entry is reuse")
	}
}

// TestPercentRoundsAndGuardsZero pins the rounding rule and the zero-denominator
// guard, so a rate never reads as known when nothing was measured.
func TestPercentRoundsAndGuardsZero(t *testing.T) {
	cases := []struct{ n, d, want int }{
		{0, 0, 0},
		{1, 3, 33},
		{2, 3, 67},
		{1, 2, 50},
		{5, 5, 100},
	}
	for _, tc := range cases {
		if got := percent(tc.n, tc.d); got != tc.want {
			t.Fatalf("percent(%d,%d) = %d, want %d", tc.n, tc.d, got, tc.want)
		}
	}
}
