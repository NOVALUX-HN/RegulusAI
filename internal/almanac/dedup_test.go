package almanac

// T3 (S2+S5) save-time dedup: eval fixture + composition tests.
//
// S2 eval fixture: committed pairs shaped like real Almanac observations.
// Paraphrase and typo pairs MUST trip the 0.92 refusal; unrelated pairs
// MUST save cleanly. The threshold comes from this data, not fiat — see
// the margin comment on DedupThreshold in dedup.go.

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// textPair is one eval-fixture entry: two retrieval texts (title +
// content, the EmbedObservation input) plus the expected verdict.
type textPair struct {
	name             string
	aTitle, aContent string
	bTitle, bContent string
	dup              bool // true: refusal expected (sim > threshold)
}

// dedupFixture pins the S2 eval: paraphrase/typo pairs refuse, unrelated
// pairs save.
var dedupFixture = []textPair{
	{
		name:     "paraphrase/postgres-pool",
		aTitle:   "chose postgres connection pooling",
		aContent: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		bTitle:   "picked postgres connection pooling",
		bContent: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		dup:      true,
	},
	{
		name:     "paraphrase/pool-leak",
		aTitle:   "postgres pool leaks idle connections",
		aContent: "the harvester stalls under load and idle connections never return; restart recovers the pool",
		bTitle:   "postgres pool leaking idle connections",
		bContent: "the harvester stalls under load and idle connections never return; a restart recovers the pool",
		dup:      true,
	},
	{
		name:     "typo/pooling",
		aTitle:   "chose postgres connection pooling",
		aContent: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		bTitle:   "chose postgres conenction pooling",
		bContent: "pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count",
		dup:      true,
	},
	{
		name:     "typo/cache-sizing",
		aTitle:   "cache the retrieval index in memory",
		aContent: "sizing the cache to the working set keeps p95 under fifty milliseconds",
		bTitle:   "cache the retrival index in memory",
		bContent: "sizing the cache to the working set keeps p95 under fifty miliseconds",
		dup:      true,
	},
	{
		name:     "unrelated/pool-vs-runtime",
		aTitle:   "chose postgres connection pooling",
		aContent: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		bTitle:   "chose bun runtime for the plugin host",
		bContent: "cold start under fifty milliseconds keeps the picker responsive",
		dup:      false,
	},
	{
		name:     "unrelated/leak-vs-trace",
		aTitle:   "postgres pool leaks idle connections",
		aContent: "the harvester stalls under load and idle connections never return; restart recovers the pool",
		bTitle:   "trace sessions prune to the call-order tail",
		bContent: "keeping the last two calls per session keeps the debug trail short",
		dup:      false,
	},
}

// pairSim is the cosine the save-time decider would see for a pair.
func pairSim(p textPair) float64 {
	return cosineVec(
		Embed(p.aTitle+"\n"+p.aContent),
		Embed(p.bTitle+"\n"+p.bContent),
	)
}

// TestDedupThresholdMatchesFixture is the S2 gate: every dup pair sits
// above the bar, every clean pair below it.
func TestDedupThresholdMatchesFixture(t *testing.T) {
	for _, p := range dedupFixture {
		sim := pairSim(p)
		t.Logf("fixture %s: sim=%.4f dup=%v", p.name, sim, p.dup)
		if p.dup && sim <= DedupThreshold {
			t.Errorf("dup pair %q must trip refusal: sim=%.4f <= %.2f", p.name, sim, DedupThreshold)
		}
		if !p.dup && sim > DedupThreshold {
			t.Errorf("clean pair %q must save: sim=%.4f > %.2f", p.name, sim, DedupThreshold)
		}
	}
}

// TestDedupLeavesExistingDoubleSaves pins suite preservation: the closest
// pre-existing double-save pairs from the committed CLI/hybrid suites
// must stay clear of the refusal bar.
func TestDedupLeavesExistingDoubleSaves(t *testing.T) {
	pairs := []textPair{
		{name: "existing/v1-v2-decision", aTitle: "v1 decision", aContent: "first", bTitle: "v2 decision", bContent: "second"},
		{name: "existing/postgres-second", aTitle: "chose postgres", aContent: "pooling everywhere", bTitle: "second choice", bContent: "more content"},
		{name: "existing/db-chain", aTitle: "chose postgres", aContent: "pooling everywhere", bTitle: "moved to sqlite", bContent: "simpler ops"},
	}
	for _, p := range pairs {
		sim := pairSim(p)
		t.Logf("existing %s: sim=%.4f", p.name, sim)
		if sim > DedupThreshold {
			t.Errorf("committed pair %q must stay clear of refusal: sim=%.4f > %.2f", p.name, sim, DedupThreshold)
		}
	}
}

// dedupSeed saves one observation, failing the test on error.
func dedupSeed(t *testing.T, root string, o *Observation, at time.Time) Observation {
	t.Helper()
	saved, err := Save(root, o, at)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

// dupErrOf runs the decider and unwraps the refusal.
func dupErrOf(t *testing.T, root string, o *Observation) *DuplicateError {
	t.Helper()
	err := CheckDuplicate(root, o)
	if err == nil {
		t.Fatal("expected a near-duplicate refusal, got nil")
	}
	var dup *DuplicateError
	if !errors.As(err, &dup) {
		t.Fatalf("refusal must be a *DuplicateError, got %T (%v)", err, err)
	}
	return dup
}

func TestCheckDuplicateRefusesParaphrase(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	seed := dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	dup := dupErrOf(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	})
	if dup.ID != seed.ID {
		t.Fatalf("refusal must name the near-duplicate id: want %s, got %s", seed.ID, dup.ID)
	}
	if all, err := All(root); err != nil || len(all) != 1 {
		t.Fatalf("refused check must leave the log untouched: %+v (%v)", all, err)
	}
}

func TestCheckDuplicateRefusesTypo(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	seed := dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "cache the retrieval index in memory",
		Content: "sizing the cache to the working set keeps p95 under fifty milliseconds",
	}, base)
	dup := dupErrOf(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "cache the retrival index in memory",
		Content: "sizing the cache to the working set keeps p95 under fifty miliseconds",
	})
	if dup.ID != seed.ID {
		t.Fatalf("refusal must name the near-duplicate id: want %s, got %s", seed.ID, dup.ID)
	}
}

func TestCheckDuplicateAllowsUnrelated(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	if err := CheckDuplicate(root, &Observation{
		Type:    TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}); err != nil {
		t.Fatalf("unrelated observation must save cleanly, got: %v", err)
	}
}

func TestCheckDuplicateTopicKeyBypass(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	dedupSeed(t, root, &Observation{
		Type:     TypeDecision,
		Title:    "chose postgres connection pooling",
		Content:  "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		TopicKey: "pooling",
	}, base)
	// Declared grouping wins: a paraphrase carrying the same topic key
	// appends as a revision instead of refusing.
	if err := CheckDuplicate(root, &Observation{
		Type:     TypeDecision,
		Title:    "picked postgres connection pooling",
		Content:  "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		TopicKey: "pooling",
	}); err != nil {
		t.Fatalf("matching topic key must bypass dedup, got: %v", err)
	}
	// A fresh topic key does not bypass: the same paraphrase with an
	// unclaimed key still refuses.
	if err := CheckDuplicate(root, &Observation{
		Type:     TypeDecision,
		Title:    "picked postgres connection pooling",
		Content:  "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
		TopicKey: "brand-new-chain",
	}); err == nil {
		t.Fatal("unclaimed topic key must not bypass dedup")
	}
}

func TestCheckDuplicateSkipsSuperseded(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	old := dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	live := dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}, base.Add(time.Hour))
	if err := Relate(root, live.ID, RelationSupersedes, old.ID, base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// A paraphrase of the hidden observation saves cleanly.
	if err := CheckDuplicate(root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}); err != nil {
		t.Fatalf("superseded observations must be excluded from comparison, got: %v", err)
	}
	// The live observation still guards: its own paraphrase refuses and
	// names it, not the hidden one.
	dup := dupErrOf(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	})
	if dup.ID != live.ID {
		t.Fatalf("refusal must name the live near-duplicate: want %s, got %s", live.ID, dup.ID)
	}
}

func TestCheckDuplicateUsesFreshIndex(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	dedupSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// With a fresh on-disk index the decider must still refuse the
	// paraphrase (index path agrees with the on-the-fly path).
	dupErrOf(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	})
}

func TestDuplicateErrorNamesIDAndHintsTopicKey(t *testing.T) {
	err := CheckDuplicate(t.TempDir(), &Observation{Type: TypeDecision, Title: "x", Content: "y"})
	if err != nil {
		t.Fatalf("empty log must save cleanly, got: %v", err)
	}
	dup := &DuplicateError{ID: "obs_abc", Similarity: 0.97}
	msg := dup.Error()
	if !strings.Contains(msg, "obs_abc") {
		t.Fatalf("refusal message must name the id, got %q", msg)
	}
	if !strings.Contains(msg, "topic key") {
		t.Fatalf("refusal message must hint at setting a topic key, got %q", msg)
	}
}
