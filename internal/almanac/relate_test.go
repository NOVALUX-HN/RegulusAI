package almanac

// T4 (S9) relate vector candidates: `memory relate --suggest <id>` prints
// top-k similar observations from the vector channel (cosine, same bucket
// decider as hybrid: superseded excluded, self excluded) above a floor.
// No relation is written — suggestion only.

import (
	"strings"
	"testing"
	"time"
)

func suggestSeed(t *testing.T, root string, o *Observation, at time.Time) Observation {
	t.Helper()
	saved, err := Save(root, o, at)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestSuggestRanksParaphraseFirst(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	para := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}, base.Add(2*time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	sugs, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) == 0 {
		t.Fatal("paraphrase must be suggested")
	}
	if sugs[0].ID != para.ID {
		t.Fatalf("paraphrase must rank first: got %+v", sugs)
	}
	if sugs[0].Cosine < SuggestFloor {
		t.Fatalf("top suggestion must clear the floor %.2f: got %.4f", SuggestFloor, sugs[0].Cosine)
	}
	if sugs[0].Title != para.Title {
		t.Fatalf("suggestion must carry the title: got %+v", sugs[0])
	}
}

func TestSuggestExcludesSelf(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	sugs, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sugs {
		if s.ID == src.ID {
			t.Fatalf("suggestion must exclude the source itself: %+v", sugs)
		}
	}
}

func TestSuggestExcludesSuperseded(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	live := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	hidden := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres conenction pooling",
		Content: "pooling keeps p95 latnecy flat under load; sized the pool to twice the worker count",
	}, base.Add(2*time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	// Sanity: the hidden typo variant clears the floor on its own, so the
	// test proves hiding rather than filtering.
	before, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	seenHidden := false
	for _, s := range before {
		if s.ID == hidden.ID {
			seenHidden = true
		}
	}
	if !seenHidden {
		t.Fatalf("fixture must rank the to-be-hidden variant above the floor first: %+v", before)
	}
	if err := Relate(root, live.ID, RelationSupersedes, hidden.ID, base.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range after {
		if s.ID == hidden.ID {
			t.Fatalf("superseded targets must be excluded like the hybrid channel: %+v", after)
		}
	}
	seenLive := false
	for _, s := range after {
		if s.ID == live.ID {
			seenLive = true
		}
	}
	if !seenLive {
		t.Fatalf("live paraphrase must survive supersede-hiding: %+v", after)
	}
}

func TestSuggestFloorFiltersUnrelated(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose bun runtime for the plugin host",
		Content: "cold start under fifty milliseconds keeps the picker responsive",
	}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	sugs, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) != 0 {
		t.Fatalf("unrelated observations below the floor must be filtered: %+v", sugs)
	}
}

func TestSuggestMissingIndexRefuses(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres",
		Content: "pooling everywhere",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres",
		Content: "pooling everywhere",
	}, base.Add(time.Hour))
	if _, err := SuggestRelated(root, src.ID); err == nil {
		t.Fatal("missing index must refuse, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "reindex") {
		t.Fatalf("missing-index refusal must point at reindex, got %q", err)
	}
}

func TestSuggestStaleIndexRefuses(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres",
		Content: "pooling everywhere",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres",
		Content: "pooling everywhere",
	}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "second choice",
		Content: "more content",
	}, base.Add(2*time.Hour))
	if _, err := SuggestRelated(root, src.ID); err == nil {
		t.Fatal("stale index must refuse, got nil")
	} else if !strings.Contains(strings.ToLower(err.Error()), "reindex") {
		t.Fatalf("stale-index refusal must point at reindex, got %q", err)
	}
}

func TestSuggestUnknownIDRefuses(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	suggestSeed(t, root, &Observation{Type: TypeDecision, Title: "v1", Content: "x"}, base)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, err := SuggestRelated(root, "obs_nope"); err == nil {
		t.Fatal("unknown id must refuse, got nil")
	} else if !strings.Contains(err.Error(), "no observation") {
		t.Fatalf("unknown id must report no observation, got %q", err)
	}
}

func TestSuggestRespectsTopK(t *testing.T) {
	if SuggestTopK != 5 {
		t.Fatalf("top-k must be 5, got %d", SuggestTopK)
	}
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	sugs, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) > SuggestTopK {
		t.Fatalf("suggestions must cap at top-%d: got %d", SuggestTopK, len(sugs))
	}
}

func TestSuggestWritesNoRelation(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	src := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "chose postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base)
	suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, err := SuggestRelated(root, src.ID); err != nil {
		t.Fatal(err)
	}
	rels, err := Relations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 0 {
		t.Fatalf("suggestion must write no relation: %+v", rels)
	}
}

func TestRelatePlainPathUntouched(t *testing.T) {
	root := t.TempDir()
	a := suggestSeed(t, root, &Observation{Type: TypeDecision, Title: "v1", Content: "x"}, time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	b := suggestSeed(t, root, &Observation{Type: TypeDecision, Title: "v2", Content: "y"}, time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC))
	if err := Relate(root, a.ID, RelationRelated, b.ID, time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	rels, err := Relations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].From != a.ID || rels[0].Relation != RelationRelated || rels[0].To != b.ID {
		t.Fatalf("plain relate must still record: %+v", rels)
	}
}
