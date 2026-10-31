package almanac

import (
	"strings"
	"testing"
	"time"
)

func saveObs(t *testing.T, root string, o *Observation) Observation {
	t.Helper()
	saved, err := Save(root, o, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestEffectiveStatusDefaults(t *testing.T) {
	root := t.TempDir()
	plain := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "t", Content: "c"})
	if st, err := EffectiveStatus(root, plain.ID); err != nil || st != StatusObserved {
		t.Fatalf("saved observations are OBSERVED, got %q/%v", st, err)
	}
	lesson := saveObs(t, root, &Observation{Type: TypeLesson, Title: "t", Content: "c",
		Lesson: &Lesson{Cue: "c", Lesson: "l", Evidence: []string{"r"}, Confidence: "high"}})
	if st, err := EffectiveStatus(root, lesson.ID); err != nil || st != StatusAccepted {
		t.Fatalf("reflect-admitted lessons are ACCEPTED, got %q/%v", st, err)
	}
	imp := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "t", Content: "c", Source: SourceImport})
	if st, err := EffectiveStatus(root, imp.ID); err != nil || st != StatusProposed {
		t.Fatalf("imports stay PROPOSED until reviewed, got %q/%v", st, err)
	}
	if _, err := EffectiveStatus(root, "obs_nope"); err == nil {
		t.Fatal("unknown ids must fail")
	}
}

func TestRecordStatusTransitions(t *testing.T) {
	root := t.TempDir()
	o := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "t", Content: "c"})
	now := time.Now().UTC()
	if err := RecordStatus(root, o.ID, StatusVerified, "ana", "checked vs rec_1", now); err != nil {
		t.Fatal(err)
	}
	if st, _ := EffectiveStatus(root, o.ID); st != StatusVerified {
		t.Fatalf("want VERIFIED, got %q", st)
	}
	hist, err := StatusHistory(root, o.ID)
	if err != nil || len(hist) != 1 || hist[0].By != "ana" {
		t.Fatalf("history must record the transition, got %+v/%v", hist, err)
	}
	if err := RecordStatus(root, o.ID, StatusAccepted, "ana", "promote", now); err != nil {
		t.Fatal(err)
	}
	if err := RecordStatus(root, o.ID, StatusObserved, "ana", "back", now); err == nil {
		t.Fatal("backward transitions must be refused")
	}
	if err := RecordStatus(root, o.ID, StatusVerified, "ana", "x", now); err == nil {
		t.Fatal("terminal ACCEPTED must not transition")
	}
	if err := RecordStatus(root, "obs_nope", StatusVerified, "ana", "x", now); err == nil {
		t.Fatal("unknown ids must be refused")
	}
	if err := RecordStatus(root, o.ID, StatusVerified, "", "x", now); err == nil {
		t.Fatal("transitions need an actor")
	}
}

func TestSupersededComputedFromRelations(t *testing.T) {
	root := t.TempDir()
	old := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "old", Content: "c", TopicKey: "k"})
	newer := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "new", Content: "c2", TopicKey: "k"})
	if err := Relate(root, newer.ID, RelationSupersedes, old.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if st, _ := EffectiveStatus(root, old.ID); st != StatusSuperseded {
		t.Fatalf("superseded target must read SUPERSEDED, got %q", st)
	}
	if st, _ := EffectiveStatus(root, newer.ID); st != StatusObserved {
		t.Fatalf("superseding observation stays active, got %q", st)
	}
}

func TestNewRelationKinds(t *testing.T) {
	root := t.TempDir()
	a := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "a", Content: "c"})
	b := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "b", Content: "c"})
	for _, kind := range []string{RelationInvalidatedBy, RelationDerivedFrom, RelationVerifiedBy, RelationRelatedTo} {
		if err := Relate(root, a.ID, kind, b.ID, time.Now().UTC()); err != nil {
			t.Fatalf("kind %q must be accepted: %v", kind, err)
		}
	}
	if err := Relate(root, a.ID, "likes", b.ID, time.Now().UTC()); err == nil {
		t.Fatal("unknown kinds must still be refused")
	} else if !strings.Contains(err.Error(), "likes") {
		t.Fatalf("error must name the kind, got %q", err.Error())
	}
}

func TestInvalidatedIDs(t *testing.T) {
	root := t.TempDir()
	a := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "a", Content: "c"})
	b := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "b", Content: "c"})
	if err := Relate(root, a.ID, RelationInvalidatedBy, b.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stale, err := InvalidatedIDs(root)
	if err != nil {
		t.Fatal(err)
	}
	if !stale[b.ID] || stale[a.ID] {
		t.Fatalf("invalidated target must be stale, got %v", stale)
	}
}

func TestContradictions(t *testing.T) {
	root := t.TempDir()
	a := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "a", Content: "c"})
	b := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "b", Content: "c"})
	c := saveObs(t, root, &Observation{Type: TypeDiscovery, Title: "c", Content: "c"})
	if err := Relate(root, a.ID, RelationConflictsWith, b.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := Relate(root, a.ID, RelationRelated, c.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	conflicts, err := Contradictions(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].A != a.ID || conflicts[0].B != b.ID {
		t.Fatalf("only the live conflict must surface, got %+v", conflicts)
	}
	// Rejecting one side resolves the contradiction from the active set.
	if err := RecordStatus(root, b.ID, StatusRejected, "ana", "wrong", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	conflicts, err = Contradictions(root, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("rejected side must leave the active set, got %+v", conflicts)
	}
}
