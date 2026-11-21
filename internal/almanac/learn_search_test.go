// LearnSearch axis tests (CRAP split): filters, VerifiedOnly,
// ACL+denied, stale/superseded, limit/order. Each axis pins the
// helpers the thin LearnSearch dispatches to. Portable: t.TempDir
// only, no network. Reuses the learnSeed/t0/t1 fixtures.
package almanac

import (
	"strings"
	"testing"
	"time"
)

func TestLearnSearchQueryValidation(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LearnSearch(dir, LearnQuery{Kinds: []string{"discovery"}}); err == nil {
		t.Fatal("unknown kind must fail")
	} else if !strings.Contains(err.Error(), "kind") {
		t.Fatalf("kind error must name the kind, got: %v", err)
	}
	if _, _, err := LearnSearch(dir, LearnQuery{Scopes: []string{"galaxy"}}); err == nil {
		t.Fatal("unknown scope must fail")
	} else if !strings.Contains(err.Error(), "scope") {
		t.Fatalf("scope error must name the scope, got: %v", err)
	}
	hits, denied, err := LearnSearch(dir, LearnQuery{})
	if err != nil || len(hits) != 0 || denied != 0 {
		t.Fatalf("empty store reads clean: %v %d %v", hits, denied, err)
	}
}

func TestLearnSearchFilterAxes(t *testing.T) {
	dir := t.TempDir()
	learnSeed(t, dir, KindPattern, "pool pattern", "pool every connection", nil)
	learnSeed(t, dir, KindError, "nil map panic", "never write to nil maps", nil)
	learnSeed(t, dir, KindPattern, "shared cache", "cache shared results", func(o *Observation) {
		o.Scope = ScopeShared
	})
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}

	hits, denied, err := LearnSearch(dir, LearnQuery{Kinds: []string{KindError}, Viewer: viewer})
	if err != nil || denied != 0 || len(hits) != 1 || hits[0].Observation.Title != "nil map panic" {
		t.Fatalf("kind filter: %v %d %v", hits, denied, err)
	}
	hits, denied, err = LearnSearch(dir, LearnQuery{Scopes: []string{ScopeShared}, Viewer: viewer})
	if err != nil || denied != 0 || len(hits) != 1 || hits[0].Observation.Title != "shared cache" {
		t.Fatalf("scope filter: %v %d %v", hits, denied, err)
	}
	hits, _, err = LearnSearch(dir, LearnQuery{Terms: []string{"pool", "connection"}, Viewer: viewer})
	if err != nil || len(hits) != 1 || hits[0].Observation.Title != "pool pattern" {
		t.Fatalf("terms AND: %v %v", hits, err)
	}
	hits, _, err = LearnSearch(dir, LearnQuery{Terms: []string{"pool", "missing"}, Viewer: viewer})
	if err != nil || len(hits) != 0 {
		t.Fatalf("terms AND with a miss: %v %v", hits, err)
	}
	hits, _, err = LearnSearch(dir, LearnQuery{Kinds: []string{KindPattern}, Scopes: []string{ScopeShared}, Viewer: viewer})
	if err != nil || len(hits) != 1 || hits[0].Observation.Title != "shared cache" {
		t.Fatalf("kind+scope: %v %v", hits, err)
	}
}

func TestLearnSearchVerifiedOnlyAxis(t *testing.T) {
	dir := t.TempDir()
	hyp := learnSeed(t, dir, KindHypothesis, "pooling hunch", "pooling may speed path", nil)
	draft := learnSeed(t, dir, KindPattern, "pooling draft", "pooling speeds path draft", nil)
	fix := learnSeed(t, dir, KindVerified, "pooling fix", "pooling speeds path fix", nil)
	if err := RecordStatus(dir, fix.ID, StatusVerified, "reviewer", "bench passes", t1); err != nil {
		t.Fatalf("verify: %v", err)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	all, denied, err := LearnSearch(dir, LearnQuery{Terms: []string{"pooling"}, Viewer: viewer})
	if err != nil || denied != 0 || len(all) != 3 {
		t.Fatalf("default shows all three: %v %d %v", all, denied, err)
	}
	for _, h := range all {
		switch h.Observation.ID {
		case hyp.ID:
			if !h.Hypothesis || h.Verified {
				t.Fatalf("hypothesis never reads verified: %+v", h)
			}
			if got := LearnLabel(h.Observation, h.Verified); !strings.Contains(got, "no es un hecho") {
				t.Fatalf("hypothesis label must warn it is not a fact, got %q", got)
			}
		case fix.ID:
			if !h.Verified || h.Hypothesis {
				t.Fatalf("verified hit mislabeled: %+v", h)
			}
			if got := LearnLabel(h.Observation, h.Verified); got != "verificado" {
				t.Fatalf("verified label = %q (want verificado)", got)
			}
		case draft.ID:
			if h.Verified || h.Hypothesis {
				t.Fatalf("draft mislabeled: %+v", h)
			}
			if got := LearnLabel(h.Observation, h.Verified); got != "no verificado" {
				t.Fatalf("draft label = %q (want no verificado)", got)
			}
		}
	}
	got, _, err := LearnSearch(dir, LearnQuery{Terms: []string{"pooling"}, VerifiedOnly: true, Viewer: viewer})
	if err != nil || len(got) != 1 || got[0].Observation.ID != fix.ID || !got[0].Verified {
		t.Fatalf("verified-only keeps exactly the fix: %v %v", got, err)
	}
}

func TestLearnSearchDeniedAxis(t *testing.T) {
	dir := t.TempDir()
	learnSeed(t, dir, KindPattern, "alpha secret", "classified pooling detail", func(o *Observation) {
		o.Scope = ScopePrivate
	})
	learnSeed(t, dir, KindPattern, "beta secret", "other matter", func(o *Observation) {
		o.Scope = ScopePrivate
	})
	beta := LearnViewer{AgentID: "agent-beta", Scopes: []string{MemoryReadScope}, Task: "other"}
	hits, denied, err := LearnSearch(dir, LearnQuery{Terms: []string{"pooling"}, Viewer: beta})
	if err != nil || len(hits) != 0 || denied != 1 {
		t.Fatalf("matching private counts denied: %v %d %v", hits, denied, err)
	}
	hits, denied, err = LearnSearch(dir, LearnQuery{Terms: []string{"missing"}, Viewer: beta})
	if err != nil || len(hits) != 0 || denied != 0 {
		t.Fatalf("filtered-out never reaches the ACL: %v %d %v", hits, denied, err)
	}
}

func TestLearnSearchRetirementAxes(t *testing.T) {
	dir := t.TempDir()
	winner := learnSeed(t, dir, KindPattern, "pool always", "pool every connection", nil)
	loser := learnSeed(t, dir, KindPattern, "never pool", "pooling hurts here", nil)
	if err := Relate(dir, winner.ID, RelationSupersedes, loser.ID, t1); err != nil {
		t.Fatalf("relate: %v", err)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, _, err := LearnSearch(dir, LearnQuery{Viewer: viewer})
	if err != nil {
		t.Fatalf("LearnSearch: %v", err)
	}
	ids := map[string]bool{}
	for _, h := range hits {
		ids[h.Observation.ID] = true
	}
	if !ids[winner.ID] || ids[loser.ID] {
		t.Fatalf("loser hides, winner stays: %v", ids)
	}
	kept, _, err := LearnSearch(dir, LearnQuery{Viewer: viewer, IncludeSuperseded: true})
	if err != nil || len(kept) != 2 {
		t.Fatalf("include-superseded resurfaces: %v %v", kept, err)
	}
	past := t0.Add(-time.Hour)
	learnSeed(t, dir, KindPattern, "old trick", "expired pooling trick", func(o *Observation) {
		o.ExpiresAt = &past
	})
	hits, _, err = LearnSearch(dir, LearnQuery{Terms: []string{"expired"}, Viewer: viewer})
	if err != nil || len(hits) != 0 {
		t.Fatalf("expired hides by default: %v %v", hits, err)
	}
	stale, _, err := LearnSearch(dir, LearnQuery{Terms: []string{"expired"}, IncludeStale: true, Viewer: viewer})
	if err != nil || len(stale) != 1 {
		t.Fatalf("include-stale resurfaces: %v %v", stale, err)
	}
}

func TestLearnSearchLimitOrderAxis(t *testing.T) {
	dir := t.TempDir()
	first := learnSeed(t, dir, KindPattern, "first", "shared topic alpha", nil)
	learnSeed(t, dir, KindPattern, "second", "shared topic alpha", nil)
	learnSeed(t, dir, KindHypothesis, "third hunch", "shared topic alpha", nil)
	if err := Pin(dir, first.ID, true, t1); err != nil {
		t.Fatalf("pin: %v", err)
	}
	viewer := LearnViewer{AgentID: "agent-alpha", Scopes: []string{MemoryReadScope}, Task: "t1"}
	hits, denied, err := LearnSearch(dir, LearnQuery{Viewer: viewer})
	if err != nil || denied != 0 || len(hits) != 3 {
		t.Fatalf("LearnSearch: %v %d %v", hits, denied, err)
	}
	if hits[0].Observation.ID != first.ID {
		t.Fatalf("pinned leads: %v", hits)
	}
	limited, _, err := LearnSearch(dir, LearnQuery{Viewer: viewer, Limit: 2})
	if err != nil || len(limited) != 2 {
		t.Fatalf("limit keeps the first N: %v %v", limited, err)
	}
	if limited[0].Observation.ID != hits[0].Observation.ID || limited[1].Observation.ID != hits[1].Observation.ID {
		t.Fatalf("limit preserves order: %v vs %v", limited, hits)
	}
	for _, h := range hits {
		if h.Observation.Type == KindHypothesis && (!h.Hypothesis || h.Verified) {
			t.Fatalf("hypothesis flag survives ordering: %+v", h)
		}
	}
}
