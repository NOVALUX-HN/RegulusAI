package envfacts

import (
	"os"
	"path/filepath"
	"testing"

	"regulus-ai/internal/almanac"
)

func TestParseStatusLine(t *testing.T) {
	if _, _, ok := parseStatusLine("   "); ok {
		t.Fatal("blank line must be rejected")
	}
	if _, _, ok := parseStatusLine("not-json"); ok {
		t.Fatal("malformed JSON must be rejected")
	}
	id, to, ok := parseStatusLine(`{"id":"obs_1","to":"accepted"}`)
	if !ok || id != "obs_1" || to != "accepted" {
		t.Fatalf("valid line must parse, got %q %q %v", id, to, ok)
	}
}

func TestLiveCacheHelpers(t *testing.T) {
	c := newLiveCache()
	indexObservations(c, []almanac.Observation{
		{ID: "a", TopicKey: "envfact/go/x"},
		{ID: "b", TopicKey: "envfact/go/x"},
	})
	if len(c.byID) != 2 || len(c.byTopic["envfact/go/x"]) != 2 {
		t.Fatalf("index must hold both observations, got %+v", c)
	}
	applyRelationsToCache(c, []almanac.Relation{
		{From: "a", To: "b", Relation: almanac.RelationSupersedes},
		{From: "missing", To: "b", Relation: almanac.RelationInvalidatedBy},
		{From: "a", To: "missing", Relation: almanac.RelationInvalidatedBy},
		{From: "a", To: "b", Relation: almanac.RelationInvalidatedBy},
	})
	if !c.sup["b"] || !c.inv["b"] {
		t.Fatalf("relations must mark sup+inv, got sup=%v inv=%v", c.sup, c.inv)
	}
	root := t.TempDir()
	loadStatusIntoCache(c, root)
	if len(c.status) != 0 {
		t.Fatalf("missing status file must leave cache empty, got %+v", c.status)
	}
	statusDir := filepath.Dir(almanac.StatusFile(root))
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(almanac.StatusFile(root), []byte("\n{\"id\":\"a\",\"to\":\"rejected\"}\nbad-line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loadStatusIntoCache(c, root)
	if c.status["a"] != "rejected" {
		t.Fatalf("status must load, got %+v", c.status)
	}
}
