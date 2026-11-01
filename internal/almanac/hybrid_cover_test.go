package almanac

import "testing"

// COVER (gap-sweep/G3): applyLatestLimit carried CRAP 26.32 at 26.7%
// coverage (Latest branch never exercised). Pure unit coverage, no
// behavior change.
func TestApplyLatestLimitKeepsFirstPerTopic(t *testing.T) {
	ranked := []Observation{
		{ID: "a1", TopicKey: "k1", Title: "first k1"},
		{ID: "b1", Title: "no topic stays"},
		{ID: "a2", TopicKey: "k1", Title: "second k1"},
		{ID: "c1", TopicKey: "k2", Title: "first k2"},
		{ID: "b2", Title: "second no topic stays"},
		{ID: "c2", TopicKey: "k2", Title: "second k2"},
	}
	got := applyLatestLimit(ranked, Query{Latest: true})
	want := []string{"a1", "b1", "c1", "b2"}
	if len(got) != len(want) {
		t.Fatalf("latest kept %d, want %d: %+v", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("latest[%d] = %q, want %q (full %+v)", i, got[i].ID, id, got)
		}
	}
}

func TestApplyLatestLimitTruncatesToLimit(t *testing.T) {
	ranked := []Observation{
		{ID: "a1", TopicKey: "k1"},
		{ID: "a2", TopicKey: "k1"},
		{ID: "b1", TopicKey: "k2"},
		{ID: "c1"},
	}
	got := applyLatestLimit(ranked, Query{Latest: true, Limit: 2})
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "b1" {
		t.Fatalf("latest+limit = %+v, want [a1 b1]", got)
	}
	plain := []Observation{{ID: "x1"}, {ID: "x2"}, {ID: "x3"}}
	got = applyLatestLimit(plain, Query{Limit: 2})
	if len(got) != 2 || got[0].ID != "x1" || got[1].ID != "x2" {
		t.Fatalf("limit = %+v, want [x1 x2]", got)
	}
	got = applyLatestLimit(plain, Query{})
	if len(got) != 3 {
		t.Fatalf("no latest/limit must keep all, got %+v", got)
	}
	var empty []Observation
	if got := applyLatestLimit(empty, Query{Latest: true, Limit: 5}); len(got) != 0 {
		t.Fatalf("empty must stay empty, got %+v", got)
	}
}
