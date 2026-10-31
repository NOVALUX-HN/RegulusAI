package almanac

import (
	"testing"
	"time"
)

func retrievalFixture(t *testing.T) (string, time.Time) {
	t.Helper()
	root := t.TempDir()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tick := now
	mustSave := func(o *Observation) Observation {
		t.Helper()
		tick = tick.Add(time.Second)
		saved, err := Save(root, o, tick)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	mustSave(&Observation{Type: TypeDiscovery, Title: "postgres pooling", Content: "pooling everywhere"})
	mustSave(&Observation{Type: TypeSessionNote, Title: "standup", Content: "postgres pooling discussed"})
	mustSave(&Observation{Type: TypeLesson, Title: "l", Content: "c",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "high"}})
	past := now.Add(-time.Hour)
	mustSave(&Observation{Type: TypeDiscovery, Title: "old news", Content: "postgres retired", ExpiresAt: &past})
	return root, now
}

func TestSearchSystemFilter(t *testing.T) {
	root, now := retrievalFixture(t)
	q := Query{Terms: []string{"postgres"}, System: SystemSemantic, Now: now}
	hits, err := Search(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("semantic bucket must hit discoveries")
	}
	for _, h := range hits {
		if SystemOf(h.Type) != SystemSemantic {
			t.Fatalf("system filter leaked %q (%s)", h.Title, h.Type)
		}
	}
	q.System = SystemEpisodic
	hits, err = Search(root, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Type != TypeSessionNote {
			t.Fatalf("episodic must return notes only, got %q", h.Type)
		}
	}
}

func TestSearchExcludesExpiredByDefault(t *testing.T) {
	root, now := retrievalFixture(t)
	hits, err := Search(root, Query{Terms: []string{"retired"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("expired must hide by default, got %+v", hits)
	}
	hits, err = Search(root, Query{Terms: []string{"retired"}, Now: now, IncludeStale: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("include-stale must resurface expired, got %+v", hits)
	}
	hits, err = Search(root, Query{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Title == "old news" {
			t.Fatal("unscoped search must not leak expired content")
		}
	}
}

func TestSearchExcludesRejectedAndInvalidated(t *testing.T) {
	root, now := retrievalFixture(t)
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	var target, other string
	for _, o := range all {
		if o.Title == "postgres pooling" {
			target = o.ID
		}
		if o.Title == "standup" {
			other = o.ID
		}
	}
	if err := RecordStatus(root, target, StatusRejected, "ana", "wrong", now); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, other, RelationInvalidatedBy, target, now); err != nil {
		t.Fatal(err)
	}
	hits, err := Search(root, Query{Terms: []string{"postgres"}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.ID == target {
			t.Fatalf("rejected+invalidated must hide, got %+v", hits)
		}
	}
	hits, err = Search(root, Query{Terms: []string{"postgres"}, Now: now, IncludeStale: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == target {
			found = true
		}
	}
	if !found {
		t.Fatal("include-stale must resurface rejected and invalidated")
	}
}

func TestEffectiveNowDefaults(t *testing.T) {
	q := Query{}
	if q.effectiveNow().IsZero() {
		t.Fatal("zero Now must resolve to a real instant")
	}
	fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	q.Now = fixed
	if !q.effectiveNow().Equal(fixed) {
		t.Fatal("explicit Now must win")
	}
}
