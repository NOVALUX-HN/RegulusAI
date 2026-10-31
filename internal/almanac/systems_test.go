package almanac

import (
	"strings"
	"testing"
	"time"
)

func TestSystemOfTypes(t *testing.T) {
	cases := map[string]string{
		TypeSessionNote: "episodic",
		TypeDiscovery:   "semantic",
		TypeDecision:    "decision",
		TypePreference:  "procedural",
		TypeLesson:      "evolution",
	}
	for typ, want := range cases {
		if got := SystemOf(typ); got != want {
			t.Fatalf("SystemOf(%q) = %q, want %q", typ, got, want)
		}
	}
	if got := SystemOf("nope"); got != "unknown" {
		t.Fatalf("unknown types must map to unknown, got %q", got)
	}
}

func TestSaveRecordsSourceOriginExpiry(t *testing.T) {
	root := t.TempDir()
	exp := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	o := &Observation{
		Type: TypeDiscovery, Title: "t", Content: "c",
		Source: "human", Origin: "owner/repo", ExpiresAt: &exp,
	}
	saved, err := Save(root, o, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source != "human" || saved.Origin != "owner/repo" {
		t.Fatalf("source/origin must persist, got %+v", saved)
	}
	if saved.ExpiresAt == nil || !saved.ExpiresAt.Equal(exp) {
		t.Fatalf("expiry must persist, got %+v", saved.ExpiresAt)
	}
	back, err := All(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0].Source != "human" {
		t.Fatalf("round trip must keep provenance, got %+v", back)
	}
}

func TestSaveDefaultsSource(t *testing.T) {
	root := t.TempDir()
	saved, err := Save(root, &Observation{Type: TypeDiscovery, Title: "t", Content: "c"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if saved.Source != SourceAgent {
		t.Fatalf("missing source must default to agent, got %q", saved.Source)
	}
}

func TestSaveRejectsBadSourceAndExpiry(t *testing.T) {
	root := t.TempDir()
	if _, err := Save(root, &Observation{Type: TypeDiscovery, Title: "t", Content: "c", Source: "oracle"}, time.Now().UTC()); err == nil {
		t.Fatal("unknown source must be rejected")
	} else if !strings.Contains(err.Error(), "source") {
		t.Fatalf("error must name source, got %q", err.Error())
	}
}

func TestSaveCollisionAdvancesIdentity(t *testing.T) {
	root := t.TempDir()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first, err := Save(root, &Observation{Type: TypeDiscovery, Title: "a", Content: "c"}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Save(root, &Observation{Type: TypeDiscovery, Title: "b", Content: "c2"}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("same-instant saves must not share identity, got %q twice", first.ID)
	}
	if !second.SavedAt.After(first.SavedAt) {
		t.Fatal("collision advance must move the timestamp forward")
	}
}

func TestExpiredDetection(t *testing.T) {
	past := time.Now().UTC().Add(-time.Second)
	future := time.Now().UTC().Add(time.Hour)
	if !IsExpired(&Observation{ExpiresAt: &past}, time.Now().UTC()) {
		t.Fatal("past expiry must count as expired")
	}
	if IsExpired(&Observation{ExpiresAt: &future}, time.Now().UTC()) {
		t.Fatal("future expiry must count as valid")
	}
	if IsExpired(&Observation{}, time.Now().UTC()) {
		t.Fatal("no expiry must count as valid")
	}
}
