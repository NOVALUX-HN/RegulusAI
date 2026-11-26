package context

import "testing"

func frag(id string, tokens int64, critical bool, source string, expired bool) Fragment {
	return Fragment{ID: id, Tokens: tokens, Critical: critical, Source: source, Expired: expired}
}

// TestMeasureLossCountsWhatWasLeftOut is the number the phase was missing: a
// percentage of the visible corpus that was not delivered.
func TestMeasureLossCountsWhatWasLeftOut(t *testing.T) {
	sel := Selection{
		Essential: []Fragment{frag("a", 10, true, "src-a", false), frag("b", 10, false, "src-b", false)},
		OnDemand:  []Fragment{frag("c", 10, false, "src-c", false)},
		Optional:  []Fragment{frag("d", 25, false, "src-d", false), frag("e", 25, false, "src-e", false)},
	}
	l := MeasureLoss(sel)
	if l.Visible != 5 || l.Delivered != 3 || l.Omitted != 2 {
		t.Fatalf("counts = %+v", l)
	}
	if l.OmittedTokens != 50 {
		t.Fatalf("omitted tokens = %d, want 50", l.OmittedTokens)
	}
	if l.LossPct != 40 {
		t.Fatalf("loss = %d%%, want 40", l.LossPct)
	}
	if l.EssentialOmitted != 0 {
		t.Fatal("no critical fragment was omitted in this selection")
	}
}

// TestMeasureLossFlagsACriticalOmission: a critical fragment in the optional layer
// is a selector bug, and the measurement must say so rather than bury it in a
// percentage.
func TestMeasureLossFlagsACriticalOmission(t *testing.T) {
	sel := Selection{
		Essential: []Fragment{frag("a", 10, false, "src-a", false)},
		Optional:  []Fragment{frag("adr", 10, true, "docs/adr.md", false)},
	}
	l := MeasureLoss(sel)
	if l.EssentialOmitted != 1 {
		t.Fatalf("essential omitted = %d, want 1 (the report must not hide it)", l.EssentialOmitted)
	}
}

// TestMeasureLossEmptySelectionIsZero: nothing visible means nothing lost, and no
// percentage is claimed.
func TestMeasureLossEmptySelectionIsZero(t *testing.T) {
	l := MeasureLoss(Selection{})
	if l.Visible != 0 || l.LossPct != 0 {
		t.Fatalf("empty selection = %+v", l)
	}
}

// TestMeasureRecoveryCountsReachableOmissions: an omitted fragment with a source is
// still addressable on demand; an expired one is not, because recovering stale
// content recovers a lie.
func TestMeasureRecoveryCountsReachableOmissions(t *testing.T) {
	sel := Selection{Optional: []Fragment{
		frag("a", 10, false, "docs/guide.md", false),
		frag("b", 10, false, "", false),
		frag("c", 10, false, "docs/old.md", true),
	}}
	r := MeasureRecovery(sel)
	if r.Omitted != 3 || r.Recoverable != 1 || r.Unreachable != 2 {
		t.Fatalf("recovery = %+v", r)
	}
	if r.RatePct != 33 {
		t.Fatalf("rate = %d%%, want 33", r.RatePct)
	}
}

// TestMeasureRecoveryNothingOmitted: nothing cut, so no rate is claimed.
func TestMeasureRecoveryNothingOmitted(t *testing.T) {
	r := MeasureRecovery(Selection{Essential: []Fragment{frag("a", 10, true, "s", false)}})
	if r.Omitted != 0 || r.RatePct != 0 {
		t.Fatalf("recovery = %+v", r)
	}
}

// TestPercentOfGuardsZero keeps the shared rounding honest.
func TestPercentOfGuardsZero(t *testing.T) {
	for _, tc := range []struct{ n, d, want int }{
		{0, 0, 0}, {1, 3, 33}, {2, 3, 67}, {1, 2, 50}, {4, 4, 100},
	} {
		if got := percentOf(tc.n, tc.d); got != tc.want {
			t.Fatalf("percentOf(%d,%d) = %d, want %d", tc.n, tc.d, got, tc.want)
		}
	}
}
