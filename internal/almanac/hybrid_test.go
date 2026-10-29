package almanac

import (
	"os"
	"testing"
	"time"
)

func hybridMustSave(t *testing.T, root string, o *Observation, at time.Time) Observation {
	t.Helper()
	saved, err := Save(root, o, at)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestHybridNoRegressionContainsLexical(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Domain: "go", Title: "chose postgres", Content: "connection pooling for the db"}, base)
	hybridMustSave(t, root, &Observation{Type: TypeDiscovery, Domain: "go", Title: "pool leak found", Content: "postgres pool under load"}, base.Add(time.Hour))
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Domain: "ts", Title: "chose bun runtime", Content: "fast startup"}, base.Add(2*time.Hour))
	hybridMustSave(t, root, &Observation{Type: TypeSessionNote, Title: "postgres note", Content: "pooling everywhere today"}, base.Add(3*time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	q := Query{Terms: []string{"postgres"}, Limit: 100}
	lex, err := Search(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(lex) == 0 {
		t.Fatal("fixture must yield lexical hits")
	}
	hyb, degraded, err := HybridSearch(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Fatal("fresh index must not degrade")
	}
	inHybrid := map[string]bool{}
	for _, o := range hyb {
		inHybrid[o.ID] = true
	}
	for _, o := range lex {
		if !inHybrid[o.ID] {
			t.Fatalf("hybrid must contain every lexical hit at high limit: missing %s (%s)", o.ID, o.Title)
		}
	}
}

func TestHybridDomainScopeFirst(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Domain: "go", Title: "cache tuning", Content: "retrieval cache sizing"}, base)
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Domain: "ts", Title: "cache tuning", Content: "retrieval cache sizing"}, base.Add(time.Hour))
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	q := Query{Terms: []string{"cache"}, Domain: "go", Limit: 10}
	hyb, degraded, err := HybridSearch(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Fatal("fresh index must not degrade")
	}
	if len(hyb) < 2 {
		t.Fatalf("domain query must rank scoped hits before global fallback, got %+v", hyb)
	}
	if hyb[0].Domain != "go" {
		t.Fatalf("scoped domain hit must rank first, got %+v", hyb[0])
	}
	seenNonGo := false
	seenGoAfterNonGo := false
	for _, o := range hyb {
		if o.Domain != "go" {
			seenNonGo = true
		} else if seenNonGo {
			seenGoAfterNonGo = true
		}
	}
	if !seenNonGo {
		t.Fatalf("global fallback must follow scoped hits, got %+v", hyb)
	}
	if seenGoAfterNonGo {
		t.Fatalf("all scoped hits must rank before global fallback, got %+v", hyb)
	}
}

func TestHybridLessonNoDecayBeatsOldNote(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	lessonAt := now.Add(-60 * 24 * time.Hour)
	noteAt := now.Add(-5 * 24 * time.Hour)
	hybridMustSave(t, root, &Observation{
		Type: TypeLesson, Title: "retrieval cache tuning", Content: "retrieval cache sizing",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r.json"}, Confidence: "high"},
	}, lessonAt)
	hybridMustSave(t, root, &Observation{Type: TypeSessionNote, Title: "retrieval cache tuning", Content: "retrieval cache sizing"}, noteAt)
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	q := Query{Terms: []string{"retrieval", "cache"}, Limit: 10}
	hyb, degraded, err := HybridSearch(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Fatal("fresh index must not degrade")
	}
	if len(hyb) != 2 {
		t.Fatalf("want both hits, got %+v", hyb)
	}
	// Lexical alone ranks the newer note first (recency); hybrid decay
	// must flip it: the admitted lesson never decays.
	lex, err := Search(root, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(lex) != 2 || lex[0].Type != TypeSessionNote {
		t.Fatalf("fixture must rank the newer note first lexically, got %+v", lex)
	}
	if hyb[0].Type != TypeLesson {
		t.Fatalf("lesson with zero decay must beat the newer note, got %+v", hyb)
	}
}

func TestHybridStaleDegradesToLexical(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Title: "chose postgres", Content: "pooling"}, base)
	// Missing index degrades.
	hyb, degraded, err := HybridSearch(root, Query{Terms: []string{"postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if !degraded {
		t.Fatal("missing index must degrade")
	}
	lex, err := Search(root, Query{Terms: []string{"postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hyb) != len(lex) {
		t.Fatalf("degraded hybrid must equal lexical: hybrid %d vs lexical %d", len(hyb), len(lex))
	}
	// Fresh index does not degrade.
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	if _, degraded, err := HybridSearch(root, Query{Terms: []string{"postgres"}}); err != nil || degraded {
		t.Fatalf("fresh index must not degrade (degraded=%v, err=%v)", degraded, err)
	}
	// Stale index (log appended after reindex) degrades.
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Title: "second", Content: "more"}, base.Add(time.Hour))
	hyb, degraded, err = HybridSearch(root, Query{Terms: []string{"postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if !degraded {
		t.Fatal("stale index must degrade to lexical")
	}
	lex, err = Search(root, Query{Terms: []string{"postgres"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(hyb) != len(lex) || (len(hyb) > 0 && hyb[0].ID != lex[0].ID) {
		t.Fatalf("stale hybrid must equal lexical: hybrid %+v vs lexical %+v", hyb, lex)
	}
	// Corrupt index degrades.
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(IndexPath(root))
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, len(raw)/2)
	copy(bad, raw)
	if err := os.WriteFile(IndexPath(root), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	hyb, degraded, err = HybridSearch(root, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if !degraded {
		t.Fatal("corrupt index must degrade to lexical")
	}
	lex, err = Search(root, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hyb) != len(lex) {
		t.Fatalf("corrupt hybrid must equal lexical: %d vs %d", len(hyb), len(lex))
	}
}

func TestHybridVectorRespectsBucketFilters(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	old, err := Save(root, &Observation{Type: TypeDecision, Title: "old choice retrieval cache", Content: "retrieval cache sizing"}, base)
	if err != nil {
		t.Fatal(err)
	}
	hybridMustSave(t, root, &Observation{Type: TypeDecision, Title: "new choice retrieval cache", Content: "retrieval cache sizing"}, base.Add(time.Hour))
	if err := Relate(root, old.ID, RelationSupersedes, old.ID, base.Add(2*time.Hour)); err == nil {
		t.Fatal("self-relation must be refused")
	}
	all, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	var newID string
	for _, o := range all {
		if o.ID != old.ID {
			newID = o.ID
		}
	}
	if err := Relate(root, newID, RelationSupersedes, old.ID, base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Reindex(root); err != nil {
		t.Fatal(err)
	}
	hyb, degraded, err := HybridSearch(root, Query{Terms: []string{"retrieval", "cache"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if degraded {
		t.Fatal("fresh index must not degrade")
	}
	for _, o := range hyb {
		if o.ID == old.ID {
			t.Fatalf("vector channel must respect supersede-hiding like lexical (one decider): leaked %s", o.ID)
		}
	}
}
