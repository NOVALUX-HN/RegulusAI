package almanac

import (
	"testing"
	"time"

	"regulus-ai/internal/library"
)

// TestConfidenceConstantsMirrorLibrary pins the local literals to the shared
// vocabulary. The Almanac stays dependency-free by design, so the values are
// duplicated on purpose; a rename must break here instead of drifting.
func TestConfidenceConstantsMirrorLibrary(t *testing.T) {
	if ConfidenceLow != library.ConfidenceLow {
		t.Fatalf("low = %q, want %q", ConfidenceLow, library.ConfidenceLow)
	}
	if ConfidenceMedium != library.ConfidenceMedium {
		t.Fatalf("medium = %q, want %q", ConfidenceMedium, library.ConfidenceMedium)
	}
	if ConfidenceHigh != library.ConfidenceHigh {
		t.Fatalf("high = %q, want %q", ConfidenceHigh, library.ConfidenceHigh)
	}
}

// TestCorroborateIsIdempotentPerSource: repeating the same source adds nothing.
// Repetition by the same actor is not independent evidence.
func TestCorroborateIsIdempotentPerSource(t *testing.T) {
	o := Observation{Author: "a1"}
	for i := 0; i < 3; i++ {
		if err := Corroborate(&o, "host-b"); err != nil {
			t.Fatalf("corroborate: %v", err)
		}
	}
	if len(o.Corroborations) != 1 {
		t.Fatalf("corroborations = %v, want one entry", o.Corroborations)
	}
	// Case and padding are the same source.
	if err := Corroborate(&o, "  HOST-B "); err != nil {
		t.Fatalf("corroborate: %v", err)
	}
	if len(o.Corroborations) != 1 {
		t.Fatalf("a differently-cased source is the same source, got %v", o.Corroborations)
	}
}

// TestCorroborateRefusesUnsignedAndSelf: a corroboration needs a source, and an
// author cannot corroborate their own claim.
func TestCorroborateRefusesUnsignedAndSelf(t *testing.T) {
	o := Observation{Author: "a1"}
	if err := Corroborate(&o, "   "); err == nil {
		t.Fatal("an empty source must be refused")
	}
	if err := Corroborate(&o, "A1"); err == nil {
		t.Fatal("self-corroboration must be refused")
	}
	if err := Corroborate(nil, "host-b"); err == nil {
		t.Fatal("a nil observation must be refused")
	}
}

// TestDerivedConfidenceNeverPromotesOnOneSource is the parent's rule made
// executable: a single observation is never a strong claim.
func TestDerivedConfidenceNeverPromotesOnOneSource(t *testing.T) {
	o := Observation{Author: "a1", Confidence: ConfidenceLow}
	if got := DerivedConfidence(o); got != ConfidenceLow {
		t.Fatalf("no corroboration = %q, want low", got)
	}
	if err := Corroborate(&o, "host-b"); err != nil {
		t.Fatalf("corroborate: %v", err)
	}
	if got := DerivedConfidence(o); got == ConfidenceHigh {
		t.Fatal("one corroborating source must never reach high")
	}
}

// TestDerivedConfidenceBars walks the two thresholds.
func TestDerivedConfidenceBars(t *testing.T) {
	cases := []struct {
		name   string
		source []string
		want   string
	}{
		{"two independent", []string{"b", "c"}, ConfidenceMedium},
		{"three independent", []string{"b", "c", "d"}, ConfidenceMedium},
		{"four independent", []string{"b", "c", "d", "e"}, ConfidenceHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := Observation{Author: "a1", Confidence: ConfidenceLow}
			for _, s := range tc.source {
				if err := Corroborate(&o, s); err != nil {
					t.Fatalf("corroborate %q: %v", s, err)
				}
			}
			if got := DerivedConfidence(o); got != tc.want {
				t.Fatalf("derived = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDerivedConfidenceNeverDowngrades: an author-assigned high survives a low
// corroboration count. The count may raise confidence; it may not erase what the
// author knew.
func TestDerivedConfidenceNeverDowngrades(t *testing.T) {
	o := Observation{Author: "a1", Confidence: ConfidenceHigh}
	if got := DerivedConfidence(o); got != ConfidenceHigh {
		t.Fatalf("derived = %q, want the assigned high to survive", got)
	}
	o.Corroborations = []string{"A1"} // self-corroboration is not independent
	if got := DerivedConfidence(o); got != ConfidenceHigh {
		t.Fatalf("self-corroboration must not change the verdict, got %q", got)
	}
}

// TestRevalidateKeepsAStillTrueClaimEligible is gap G2: an expired claim is not
// discarded, it is renewed with an actor and a TTL.
func TestRevalidateKeepsAStillTrueClaimEligible(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	past := now.Add(-48 * time.Hour)
	o := Observation{ExpiresAt: &past}
	if !IsExpired(&o, now) {
		t.Fatal("precondition: the claim must start expired")
	}
	if err := Revalidate(&o, "operator", now, 7*24*time.Hour); err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if IsExpired(&o, now) {
		t.Fatal("a revalidated claim must be eligible again")
	}
	if o.RevalidatedAt == nil || !o.RevalidatedAt.Equal(now) {
		t.Fatalf("revalidated_at = %v, want %v", o.RevalidatedAt, now)
	}
	if o.ExpiresAt == nil || !o.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("expires_at = %v, want now+7d", o.ExpiresAt)
	}
}

// TestRevalidateRefusesUnsignedAndIndefinite: an unsigned renewal or an
// indefinite one is not evidence.
func TestRevalidateRefusesUnsignedAndIndefinite(t *testing.T) {
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	o := Observation{}
	if err := Revalidate(&o, "  ", now, time.Hour); err == nil {
		t.Fatal("an empty actor must be refused")
	}
	if err := Revalidate(&o, "operator", now, 0); err == nil {
		t.Fatal("a non-positive ttl must be refused")
	}
	if err := Revalidate(&o, "operator", time.Time{}, time.Hour); err == nil {
		t.Fatal("a zero timestamp must be refused")
	}
	if o.RevalidatedAt != nil || o.ExpiresAt != nil {
		t.Fatal("a refused revalidation must not touch the record")
	}
}

// TestCompatibleContexts: two claims coexist when they were established in
// different hosts or environments. Same context means a real conflict, which
// still needs resolution.
func TestCompatibleContexts(t *testing.T) {
	cases := []struct {
		name string
		a, b Observation
		want bool
	}{
		{"different hosts", Observation{Host: "linux"}, Observation{Host: "windows"}, true},
		{"different environments", Observation{Environment: "wsl"}, Observation{Environment: "native"}, true},
		{"same host", Observation{Host: "linux"}, Observation{Host: "linux"}, false},
		{"no context at all", Observation{}, Observation{}, false},
		{"one side unscoped", Observation{Host: "linux"}, Observation{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompatibleContexts(tc.a, tc.b); got != tc.want {
				t.Fatalf("compatible = %v, want %v", got, tc.want)
			}
		})
	}
}
