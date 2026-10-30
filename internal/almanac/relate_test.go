package almanac

// T4 (S9) relate vector candidates: `memory relate --suggest <id>` prints
// top-k similar observations from the vector channel (cosine, same bucket
// decider as hybrid: superseded excluded, self excluded) above a floor.
// No relation is written — suggestion only.

import (
	"os"
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
	sugs, rebuilt, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt {
		t.Fatal("fresh index must not rebuild")
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
	sugs, _, err := SuggestRelated(root, src.ID)
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
	before, _, err := SuggestRelated(root, src.ID)
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
	after, _, err := SuggestRelated(root, src.ID)
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
	sugs, _, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sugs) != 0 {
		t.Fatalf("unrelated observations below the floor must be filtered: %+v", sugs)
	}
}

// R2 (S2): a save with no reindex yet must not refuse — suggest rebuilds
// the missing index once and answers from the fresh vector channel.
func TestSuggestMissingIndexAutoRebuilds(t *testing.T) {
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
	sugs, rebuilt, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatalf("missing index must rebuild, not refuse: %v", err)
	}
	if !rebuilt {
		t.Fatal("missing index must report rebuilt=true")
	}
	if len(sugs) == 0 || sugs[0].ID != para.ID {
		t.Fatalf("rebuilt index must rank the paraphrase first: %+v", sugs)
	}
	if state, err := IndexState(root); err != nil || state != "fresh" {
		t.Fatalf("rebuild must leave a fresh index: state=%q err=%v", state, err)
	}
}

// R2 (S2): a save past the last reindex must not refuse — suggest rebuilds
// the stale index once, and the rebuilt index sees the post-reindex save
// (a stale read would miss it).
func TestSuggestStaleIndexAutoRebuilds(t *testing.T) {
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
	para := suggestSeed(t, root, &Observation{
		Type:    TypeDecision,
		Title:   "picked postgres connection pooling",
		Content: "pooling keeps p95 latency flat under load; sized the pool to twice the worker count",
	}, base.Add(2*time.Hour))
	sugs, rebuilt, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatalf("stale index must rebuild, not refuse: %v", err)
	}
	if !rebuilt {
		t.Fatal("stale index must report rebuilt=true")
	}
	seen := false
	for _, s := range sugs {
		if s.ID == para.ID {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("rebuilt index must see the post-reindex save: %+v", sugs)
	}
}

// R2 (S2): a corrupt index reads as stale and must rebuild too — same
// repair, same reindex pointer on failure.
func TestSuggestCorruptIndexAutoRebuilds(t *testing.T) {
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
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, len(raw))
	copy(bad, raw)
	bad[0] ^= 0xff // magic
	if err := os.WriteFile(IndexPath(root), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	sugs, rebuilt, err := SuggestRelated(root, src.ID)
	if err != nil {
		t.Fatalf("corrupt index must rebuild, not refuse: %v", err)
	}
	if !rebuilt {
		t.Fatal("corrupt index must report rebuilt=true")
	}
	if len(sugs) == 0 || sugs[0].ID != para.ID {
		t.Fatalf("rebuilt index must rank the paraphrase first: %+v", sugs)
	}
}

func TestSuggestUnknownIDRefuses(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	suggestSeed(t, root, &Observation{Type: TypeDecision, Title: "v1", Content: "x"}, base)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SuggestRelated(root, "obs_nope"); err == nil {
		t.Fatal("unknown id must refuse, got nil")
	} else if !strings.Contains(err.Error(), "no observation") {
		t.Fatalf("unknown id must report no observation, got %q", err)
	}
}

// R2 (S2): an unknown id refuses before any rebuild — bad input never
// triggers a write.
func TestSuggestUnknownIDWritesNothing(t *testing.T) {
	root := t.TempDir()
	suggestSeed(t, root, &Observation{Type: TypeDecision, Title: "v1", Content: "x"}, time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC))
	if _, _, err := SuggestRelated(root, "obs_nope"); err == nil {
		t.Fatal("unknown id must refuse, got nil")
	}
	if _, err := os.Stat(IndexPath(root)); !os.IsNotExist(err) {
		t.Fatal("unknown id must not build an index")
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
	sugs, _, err := SuggestRelated(root, src.ID)
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
	if _, _, err := SuggestRelated(root, src.ID); err != nil {
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
