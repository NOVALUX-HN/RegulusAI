package almanac

import (
	"strings"
	"testing"
)

// TestConfidenceRankUsesDerivedConfidence wires F18.2's derived confidence into
// ranking: a lesson corroborated by independent sources must outrank one that is
// merely asserted. Before this, ranking read the raw field and corroboration had
// no effect anywhere.
func TestConfidenceRankUsesDerivedConfidence(t *testing.T) {
	corroborated := Observation{Lesson: &Lesson{Confidence: ConfidenceLow}}
	for _, src := range []string{"b", "c", "d", "e"} {
		if err := Corroborate(&corroborated, src); err != nil {
			t.Fatalf("corroborate %q: %v", src, err)
		}
	}
	if got := confidenceRank(corroborated); got != 0 {
		t.Fatalf("four independent sources must rank high (0), got %d", got)
	}

	twoSources := Observation{Lesson: &Lesson{Confidence: ConfidenceLow}}
	for _, src := range []string{"b", "c"} {
		if err := Corroborate(&twoSources, src); err != nil {
			t.Fatalf("corroborate %q: %v", src, err)
		}
	}
	if got := confidenceRank(twoSources); got != 1 {
		t.Fatalf("two independent sources must rank medium (1), got %d", got)
	}

	asserted := Observation{Lesson: &Lesson{Confidence: ConfidenceLow}}
	if got := confidenceRank(asserted); got != 2 {
		t.Fatalf("an un-corroborated low lesson must rank last (2), got %d", got)
	}
}

// TestSearchFiltersByHostAndEnvironment wires the new dimensions into the query
// path, not only into the context block.
func TestSearchFiltersByHostAndEnvironment(t *testing.T) {
	root := t.TempDir()
	for _, o := range []*Observation{
		{Type: "discovery", Title: "on linux", Content: "linux lesson", Author: "a1", Host: "linux"},
		{Type: "discovery", Title: "on windows", Content: "windows lesson", Author: "a1", Host: "windows"},
		{Type: "discovery", Title: "unscoped", Content: "no host", Author: "a1"},
	} {
		if _, err := Save(root, o, now()); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	hits, err := Search(root, Query{Host: "linux"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var titles []string
	for _, h := range hits {
		titles = append(titles, h.Title)
	}
	joined := strings.Join(titles, "|")
	if !strings.Contains(joined, "on linux") || !strings.Contains(joined, "unscoped") {
		t.Fatalf("a host query must return that host plus unscoped records, got %q", joined)
	}
	if strings.Contains(joined, "on windows") {
		t.Fatalf("a host query must exclude another host, got %q", joined)
	}

	// Same rule for environment, and the case-insensitive match.
	if _, err := Save(root, &Observation{Type: "discovery", Title: "in wsl", Content: "wsl lesson", Author: "a1", Environment: "wsl"}, now()); err != nil {
		t.Fatalf("save: %v", err)
	}
	envHits, err := Search(root, Query{Environment: "WSL"})
	if err != nil {
		t.Fatalf("search env: %v", err)
	}
	found := false
	for _, h := range envHits {
		if h.Title == "in wsl" {
			found = true
		}
	}
	if !found {
		t.Fatal("environment filtering must match case-insensitively")
	}
}

// TestResolveContradictionRefusesAcrossContexts is the wiring for coexistence:
// when the two claims were established in different hosts, the resolver must
// refuse instead of destroying one of them.
func TestResolveContradictionRefusesAcrossContexts(t *testing.T) {
	root := t.TempDir()
	a, err := Save(root, &Observation{Type: "discovery", Title: "linux truth", Content: "A", Author: "a1", Host: "linux"}, now())
	if err != nil {
		t.Fatalf("save a: %v", err)
	}
	b, err := Save(root, &Observation{Type: "discovery", Title: "windows truth", Content: "B", Author: "a1", Host: "windows"}, now())
	if err != nil {
		t.Fatalf("save b: %v", err)
	}
	if err := RelateAttributed(root, a.ID, RelationConflictsWith, b.ID, "operator", now()); err != nil {
		t.Fatalf("relate: %v", err)
	}

	if err := ResolveContradiction(root, a.ID, b.ID, "operator", now()); err == nil {
		t.Fatal("resolving across different hosts must be refused so both claims survive")
	}
	// Neither may be superseded by the refusal.
	claims, err := observationsByID(root, a.ID, b.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("both claims must survive, got %d", len(claims))
	}

	// In the same context the resolver still settles the conflict.
	c, err := Save(root, &Observation{Type: "discovery", Title: "same host one", Content: "C", Author: "a1", Host: "linux"}, now())
	if err != nil {
		t.Fatalf("save c: %v", err)
	}
	d, err := Save(root, &Observation{Type: "discovery", Title: "same host two", Content: "D", Author: "a1", Host: "linux"}, now())
	if err != nil {
		t.Fatalf("save d: %v", err)
	}
	if err := RelateAttributed(root, c.ID, RelationConflictsWith, d.ID, "operator", now()); err != nil {
		t.Fatalf("relate: %v", err)
	}
	if err := ResolveContradiction(root, c.ID, d.ID, "operator", now()); err != nil {
		t.Fatalf("a same-context contradiction must still resolve: %v", err)
	}
}
