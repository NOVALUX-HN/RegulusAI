// Usefulness measurement (F18.3 / NOV-144).
//
// The F18.1 audit found that the system could report precision, useful-recall
// and retrieval misses, but never whether retrieved knowledge was actually used
// or whether using it helped. Without that link, "the knowledge base is useful"
// is an assertion, not a measurement.
//
// ReuseEvent is that link, and Summarize compares the runs that reused knowledge
// against the runs that did not. The baseline is explicit: it is the no-reuse
// group of the same batch, never an assumed percentage, and no comparison is
// reported when one of the two groups is empty.
package almanac

// ReuseEvent is one observed retrieval followed by an outcome.
type ReuseEvent struct {
	// Query is free text for the operator; it never takes part in the maths.
	Query string `json:"query,omitempty"`
	// Matched is what retrieval returned. It is recorded so a reader can audit
	// a suspicious reuse claim without re-running the query.
	Matched []string `json:"matched,omitempty"`
	// Reused is the subset the run actually used. Empty means the run retrieved
	// but did not use anything, which is the baseline group.
	Reused []string `json:"reused,omitempty"`
	// Success is the task outcome the run reported.
	Success bool `json:"success"`
	// Retries is how many retries that run needed.
	Retries int `json:"retries"`
}

// UsedKnowledge reports whether the run actually used retrieved knowledge.
func (e ReuseEvent) UsedKnowledge() bool { return len(e.Reused) > 0 }

// Usefulness is the measured comparison. Every field is a count or a rate over
// the same batch, so it can be recomputed and audited.
//
// Rates are percentages rounded to the nearest integer, and the mean retries are
// carried in thousandths so a comparison never hides behind float formatting.
type Usefulness struct {
	Runs     int
	Reused   int
	Baseline int

	ReuseRatePct int

	SuccessWithReuse    int
	SuccessWithoutReuse int

	SuccessRateWithPct    int
	SuccessRateWithoutPct int

	MeanRetriesWithMilli    int
	MeanRetriesWithoutMilli int
	// RetryDeltaMilli is with minus without: negative means reuse came with
	// fewer retries. It is only meaningful when Comparable is true.
	RetryDeltaMilli int

	// Comparable reports whether both groups have at least one run. A
	// comparison against an empty baseline is not a comparison.
	Comparable bool
}

// Summarize reduces a batch of reuse events into the comparison.
func Summarize(events []ReuseEvent) Usefulness {
	var u Usefulness
	u.Runs = len(events)
	var retriesWith, retriesWithout int
	for _, e := range events {
		if e.UsedKnowledge() {
			u.Reused++
			if e.Success {
				u.SuccessWithReuse++
			}
			retriesWith += e.Retries
		} else {
			u.Baseline++
			if e.Success {
				u.SuccessWithoutReuse++
			}
			retriesWithout += e.Retries
		}
	}
	if u.Runs > 0 {
		u.ReuseRatePct = percent(u.Reused, u.Runs)
	}
	if u.Reused > 0 {
		u.SuccessRateWithPct = percent(u.SuccessWithReuse, u.Reused)
		u.MeanRetriesWithMilli = (retriesWith * 1000) / u.Reused
	}
	if u.Baseline > 0 {
		u.SuccessRateWithoutPct = percent(u.SuccessWithoutReuse, u.Baseline)
		u.MeanRetriesWithoutMilli = (retriesWithout * 1000) / u.Baseline
	}
	u.Comparable = u.Reused > 0 && u.Baseline > 0
	if u.Comparable {
		u.RetryDeltaMilli = u.MeanRetriesWithMilli - u.MeanRetriesWithoutMilli
	}
	return u
}

// percent rounds n/d to the nearest integer percentage. A zero denominator
// returns zero rather than pretending the rate is known.
func percent(n, d int) int {
	if d <= 0 {
		return 0
	}
	return (n*200 + d) / (2 * d)
}
