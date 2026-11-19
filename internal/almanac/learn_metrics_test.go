// Learn metrics CRAP paydown (ComputeLearnMetrics split): the pure
// precision/recall/noise rate helpers, the fold/noise helpers, and
// store round-trips through t.TempDir()+LearnSave — contradiction
// noise and expiry retirement. No network, no shell.
package almanac

import (
	"testing"
	"time"
)

func TestLearnRateHelpersPure(t *testing.T) {
	if got := learnPrecisionRate(0, 0, 0); got != 1 {
		t.Fatalf("empty decided reads clean, got %v", got)
	}
	if got := learnPrecisionRate(1, 1, 0); got != 0.5 {
		t.Fatalf("precision = %v, want 0.5", got)
	}
	if got := learnRecallRate(0, 0); got != 1 {
		t.Fatalf("empty verified reads clean, got %v", got)
	}
	if got := learnRecallRate(1, 2); got != 0.5 {
		t.Fatalf("recall = %v, want 0.5", got)
	}
	if got := learnObsoleteRate(0, 0); got != 0 {
		t.Fatalf("empty obsolete reads zero, got %v", got)
	}
	if got := learnObsoleteRate(1, 4); got != 0.25 {
		t.Fatalf("obsolete rate = %v, want 0.25", got)
	}
	if got := learnNoiseRate(0, 0, 0, 0); got != 0 {
		t.Fatalf("empty noise reads zero, got %v", got)
	}
	if got := learnNoiseRate(1, 1, 1, 4); got != 0.75 {
		t.Fatalf("noise rate = %v, want 0.75", got)
	}
	if !isVerifiedStatus(StatusVerified) || !isVerifiedStatus(StatusAccepted) {
		t.Fatal("VERIFIED and ACCEPTED read verified")
	}
	if isVerifiedStatus(StatusRejected) || isVerifiedStatus("") {
		t.Fatal("REJECTED and empty never read verified")
	}
	past := t0.Add(-time.Hour)
	if !learnExpired(Observation{ExpiresAt: &past}, t0) {
		t.Fatal("past expiry reads expired")
	}
	future := t0.Add(time.Hour)
	if learnExpired(Observation{ExpiresAt: &future}, t0) {
		t.Fatal("future expiry never reads expired")
	}
	if learnExpired(Observation{}, t0) {
		t.Fatal("no expiry never reads expired")
	}
}

func TestLearnFoldHelpersPure(t *testing.T) {
	all := []Observation{
		{ID: "a", Type: KindPattern},
		{ID: "b", Type: "memory"},
		{ID: "c", Type: KindHypothesis},
	}
	learn := learnKindObservations(all)
	if len(learn) != 2 {
		t.Fatalf("kind filter keeps learn only, got %d", len(learn))
	}
	// Only pairs touching learn knowledge count, and each distinct
	// learn-kind side counts once.
	count, sides := learnContradictionSides(
		[]Conflict{{A: "a", B: "ghost"}, {A: "ghost", B: "a"}, {A: "x", B: "y"}},
		map[string]bool{"a": true},
	)
	if count != 2 || !sides["a"] || len(sides) != 1 {
		t.Fatalf("noise counts learn-touching pairs once: %d %v", count, sides)
	}
	// Every fold statement runs: verified-latest and still-useful,
	// hypothesis counts (plus the invariant watch), the
	// rejected/invalidated/superseded split and expiry retirement.
	expiredAt := t0.Add(-time.Hour)
	hExpiredAt := t0.Add(-2 * time.Hour)
	foldLearn := []Observation{
		{ID: "a", Type: KindPattern},
		{ID: "c", Type: KindHypothesis},
		{ID: "d", Type: KindPattern},
		{ID: "e", Type: KindError},
		{ID: "g", Type: KindPattern, ExpiresAt: &expiredAt},
		{ID: "h", Type: KindHypothesis, ExpiresAt: &hExpiredAt},
	}
	ends := map[string]string{"a": StatusVerified, "c": StatusRejected}
	sup := map[string]bool{"d": true}
	inv := map[string]bool{"e": true}
	ever := map[string]bool{"a": true, "h": true}
	fold := foldLearnObservations(foldLearn, ends, sup, inv, ever, t1)
	if fold.verifiedLatest != 1 || fold.verifiedActive != 1 {
		t.Fatalf("verified fold wrong: %+v", fold)
	}
	if fold.hypotheses != 2 || fold.hypothesesVerified != 1 {
		t.Fatalf("hypothesis fold wrong: %+v", fold)
	}
	if fold.active != 1 || fold.obsolete != 5 {
		t.Fatalf("retirement fold wrong: %+v", fold)
	}
	if fold.rejected != 1 || fold.invalidated != 1 || fold.superseded != 1 {
		t.Fatalf("split fold wrong: %+v", fold)
	}
	if fold.expired != 2 {
		t.Fatalf("expired fold wrong: %+v", fold)
	}
	if got := learnPrecisionRate(fold.verifiedLatest, fold.rejected, fold.invalidated); got != 1.0/3.0 {
		t.Fatalf("precision = %v, want 1/3", got)
	}
}

func TestLearnMetricsContradictionNoise(t *testing.T) {
	dir := t.TempDir()
	a := learnSeed(t, dir, KindPattern, "pool always", "pool every connection", func(o *Observation) {
		o.Evidence = []string{"go test passes"}
	})
	b := learnSeed(t, dir, KindPattern, "never pool", "pooling hurts here", nil)
	if err := RecordStatus(dir, a.ID, StatusVerified, "reviewer", "bench passes", t1); err != nil {
		t.Fatalf("verify a: %v", err)
	}
	if err := Relate(dir, a.ID, RelationConflictsWith, b.ID, t1); err != nil {
		t.Fatalf("relate: %v", err)
	}
	m, err := ComputeLearnMetrics(dir, t1.Add(time.Hour))
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	if m.Total != 2 || m.Verified != 1 || m.VerifiedEver != 1 {
		t.Fatalf("counts: %+v", m)
	}
	if m.Contradictions != 1 {
		t.Fatalf("contradictions = %d, want 1", m.Contradictions)
	}
	// Both endpoints are learn knowledge: two contradicted sides
	// over two observations read full noise.
	if m.NoiseRate != 1 {
		t.Fatalf("noise rate = %v, want 1", m.NoiseRate)
	}
	if m.Precision != 1 || m.UsefulRecall != 1 || m.ObsoleteRate != 0 {
		t.Fatalf("clean rates: %+v", m)
	}
}

func TestLearnMetricsExpiryObsolete(t *testing.T) {
	dir := t.TempDir()
	exp := t0.Add(time.Hour)
	v := learnSeed(t, dir, KindPattern, "timed pool", "pool within the hour", func(o *Observation) {
		o.ExpiresAt = &exp
		o.Evidence = []string{"go test passes"}
	})
	if err := RecordStatus(dir, v.ID, StatusVerified, "reviewer", "bench passes", t0); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// While fresh the verified knowledge stays active; once the
	// expiry passes it retires to obsolete without new writes.
	fresh, err := ComputeLearnMetrics(dir, t0.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("fresh metrics: %v", err)
	}
	if fresh.Total != 1 || fresh.Active != 1 || fresh.Obsolete != 0 || fresh.Expired != 0 {
		t.Fatalf("fresh counts: %+v", fresh)
	}
	if fresh.UsefulRecall != 1 {
		t.Fatalf("fresh recall = %v, want 1", fresh.UsefulRecall)
	}
	stale, err := ComputeLearnMetrics(dir, exp.Add(time.Hour))
	if err != nil {
		t.Fatalf("stale metrics: %v", err)
	}
	if stale.Active != 0 || stale.Obsolete != 1 || stale.Expired != 1 {
		t.Fatalf("stale counts: %+v", stale)
	}
	if stale.UsefulRecall != 0 {
		t.Fatalf("stale recall = %v, want 0 (verified once, useful never again)", stale.UsefulRecall)
	}
	// Retirement never unverifies: the latest state stays VERIFIED,
	// so precision holds while recall falls.
	if stale.Verified != 1 || stale.Precision != 1 {
		t.Fatalf("retirement keeps the verdict: %+v", stale)
	}
}
